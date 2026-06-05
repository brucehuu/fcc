package bridge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"feishu-connect/internal/bot"
	"feishu-connect/internal/config"
	"feishu-connect/internal/log"
	"feishu-connect/internal/terminal"
)

// receiverKey 唯一标识一个飞书接收方（私聊用户或群）
type receiverKey struct {
	id   string
	kind string // "open_id" or "chat_id"
}

// receiverState 维护单个接收方的状态
type receiverState struct {
	lastPane string
	ready    bool // baseline 是否已初始化，防止竞态发送完整 pane
	mu       sync.Mutex
	sendMu   sync.Mutex

	// 累积消息模式：所有内容追加到一条 markdown 消息中
	messageID  string        // 当前累积消息的 open_message_id
	contentBuf strings.Builder // 累积的 markdown 内容
}

// BridgeConfig 创建 Bridge 所需的全部配置
type BridgeConfig struct {
	AppID             string
	AppSecret         string
	Command           string
	WorkDir           string
	BypassPermissions bool
	CodexQueueMode    string
	CaptureInterval   time.Duration
	SendTimeout       time.Duration
	TMUXHistoryLines  int
	SendRetries       int
	NoisePatterns     []string
	TargetName        string // 首次启动时发送欢迎消息的目标用户名称
}

type Bridge struct {
	messenger         Messenger
	term              Terminal
	termMu            sync.RWMutex // 保护 term 的热替换
	receivers         sync.Map     // map[receiverKey]*receiverState
	captureInterval   time.Duration
	sendTimeout       time.Duration
	historyLines      int
	interruptDebounce time.Duration
	interruptMu       sync.Mutex
	lastInterrupt     time.Time
	wg                sync.WaitGroup
	sendWg            sync.WaitGroup
	closeOnce         sync.Once
	noisePatterns     []string
	metrics           bridgeMetrics
	isClaude          bool   // 仅 Claude 命令启用装饰性状态行过滤
	targetName        string // 首次启动欢迎消息的目标用户

	lastUserMessage   string      // 最近从飞书收到的用户消息，用于过滤 tmux 回显
	lastUserMsgMu     sync.Mutex  // 保护 lastUserMessage
}

// bridgeMetrics 轻量级运行时指标
type bridgeMetrics struct {
	messagesReceived atomic.Uint64
	messagesSent     atomic.Uint64
	captures         atomic.Uint64
	diffHits         atomic.Uint64
	diffMisses       atomic.Uint64
}

// buildCommand 根据配置构建完整的启动命令（追加 bypass 参数、codex queue mode 等）。
func buildCommand(cfg *BridgeConfig) string {
	command := cfg.Command
	// 如果启用了 bypass permissions，追加对应工具的免确认参数
	if cfg.BypassPermissions {
		if isClaudeCommand(command) && !strings.Contains(command, "--dangerously-skip-permissions") {
			command += " --dangerously-skip-permissions"
		} else if isCodexCommand(command) && !strings.Contains(command, "--dangerously-bypass-approvals-and-sandbox") {
			command += " --dangerously-bypass-approvals-and-sandbox"
		}
		// opencode 目前不支持类似的免确认参数
	}

	// codex 使用配置的队列模式（默认 guide）
	if isCodexCommand(command) && !strings.Contains(command, "followUpQueueMode") {
		command += fmt.Sprintf(` -c desktop.followUpQueueMode=%q`, cfg.CodexQueueMode)
	}

	return command
}

func New(cfg *BridgeConfig) (*Bridge, error) {
	command := buildCommand(cfg)

	captureInterval := cfg.CaptureInterval
	if captureInterval < 500*time.Millisecond {
		captureInterval = 500 * time.Millisecond
	}
	if captureInterval > 60*time.Second {
		captureInterval = 60 * time.Second
	}

	sendTimeout := cfg.SendTimeout
	if sendTimeout < 1*time.Second {
		sendTimeout = 1 * time.Second
	}
	if sendTimeout > 120*time.Second {
		sendTimeout = 120 * time.Second
	}

	b := &Bridge{
		captureInterval:   captureInterval,
		sendTimeout:       sendTimeout,
		historyLines:      cfg.TMUXHistoryLines,
		interruptDebounce: 500 * time.Millisecond,
		noisePatterns:     cfg.NoisePatterns,
		isClaude:          isClaudeCommand(command),
		targetName:        cfg.TargetName,
	}
	b.messenger = bot.New(cfg.AppID, cfg.AppSecret, b.handleMessage, cfg.SendRetries)

	tm := terminal.NewTmuxSession("fcc")
	if !tm.IsAvailable() {
		b.messenger.Close()
		return nil, fmt.Errorf("tmux is not installed. Please install it with: brew install tmux")
	}
	if err := tm.Start(command, cfg.WorkDir); err != nil {
		b.messenger.Close()
		return nil, fmt.Errorf("failed to start tmux session: %w", err)
	}
	b.term = tm

	return b, nil
}

func isClaudeCommand(command string) bool {
	return matchCommand(command, "claude")
}

func isCodexCommand(command string) bool {
	return matchCommand(command, "codex")
}

func isOpenCodeCommand(command string) bool {
	return matchCommand(command, "opencode")
}

// matchCommand 在命令字符串中查找指定的可执行文件名。
// 支持：
//   - 直接命令: claude, codex
//   - 绝对/相对路径: /usr/local/bin/claude, ./bin/codex
//   - npx 启动: npx claude, npx -y codex
//   - 跳过选项: claude --foo, codex -c foo=bar
func matchCommand(command, name string) bool {
	fields := terminal.SplitCommand(command)
	if len(fields) == 0 {
		return false
	}
	target := strings.ToLower(name)
	for _, f := range fields {
		// 跳过选项（以 - 开头）
		if strings.HasPrefix(f, "-") {
			continue
		}
		// 取路径中的 basename
		base := f
		if idx := strings.LastIndexAny(f, "/\\"); idx >= 0 {
			base = f[idx+1:]
		}
		// 跳过 .exe 等后缀
		if dot := strings.LastIndex(base, "."); dot > 0 {
			base = base[:dot]
		}
		if strings.EqualFold(base, target) {
			return true
		}
	}
	return false
}

// interruptKeywords 飞书端发送这些关键词会触发中断（发送 ESC 给 tmux）
var interruptKeywords = []string{"stop", "esc", "中断", "取消", "cancel", "quit", "q"}

func isInterruptCommand(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, kw := range interruptKeywords {
		if lower == kw {
			return true
		}
	}
	return false
}

func (b *Bridge) handleMessage(chatType, openID, chatID, text string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[bridge] handleMessage panic: %v", r)
		}
	}()

	var key receiverKey
	if chatType == "group" {
		key = receiverKey{id: chatID, kind: "chat_id"}
	} else {
		key = receiverKey{id: openID, kind: "open_id"}
	}

	// 懒加载 receiver 状态
	state, loaded := b.receivers.LoadOrStore(key, &receiverState{})

	// 首次创建的 receiver，立即初始化 baseline，避免首屏完整输出被当作 diff 发送
	if !loaded {
		if pane, err := b.term.CaptureVisible(b.historyLines); err == nil {
			s := state.(*receiverState)
			s.mu.Lock()
			s.lastPane = b.filterPane(pane)
			s.ready = true
			s.mu.Unlock()
		}
	}

	// 检测中断命令，发送 Escape 键给 tmux
	if isInterruptCommand(text) {
		b.interruptMu.Lock()
		debounced := time.Since(b.lastInterrupt) < b.interruptDebounce
		if !debounced {
			b.lastInterrupt = time.Now()
		}
		b.interruptMu.Unlock()
		if debounced {
			log.Debug("[bridge] interrupt debounced")
			return
		}
		log.Info("[bridge] interrupt signal received, sending Escape to tmux")
		b.termMu.RLock()
		term := b.term
		b.termMu.RUnlock()
		if err := term.SendSpecialKey("Escape"); err != nil {
			log.Warnf("[bridge] failed to send Escape: %v", err)
		}
		return
	}

	// 每次收到飞书新消息，重置累积状态，下次 diff 会新开一个 interactive
	s := state.(*receiverState)
	s.sendMu.Lock()
	s.messageID = ""
	s.contentBuf.Reset()
	s.sendMu.Unlock()

	// 记录用户消息（trim 后），用于后续过滤 tmux 中的回显 + Tip 组合
	b.lastUserMsgMu.Lock()
	b.lastUserMessage = strings.TrimSpace(text)
	b.lastUserMsgMu.Unlock()

	b.metrics.messagesReceived.Add(1)
	log.Debugf("[bridge] sending to tmux: %q", log.Truncate(text, 80))
	b.termMu.RLock()
	term := b.term
	b.termMu.RUnlock()
	if err := term.SendKeys(text); err != nil {
		log.Warnf("[bridge] failed to send keys: %v", err)
	}
}

// Start 启动后台 goroutine（飞书连接 + output capture），不 attach tmux
// 返回的 err 始终是 ctx.Err()
func (b *Bridge) Start(ctx context.Context) error {
	b.wg.Add(4)
	go func() {
		defer b.wg.Done()
		if err := b.messenger.Start(ctx); err != nil {
			log.Warnf("[bridge] bot stopped: %v", err)
		}
	}()
	go func() {
		defer b.wg.Done()
		b.forwardOutput(ctx)
	}()
	go func() {
		defer b.wg.Done()
		b.runImageCleanup(ctx)
	}()
	go func() {
		defer b.wg.Done()
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		if b.targetName != "" {
			welcomeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := b.messenger.SendWelcome(welcomeCtx, b.targetName, "FCC is coming..."); err != nil {
				log.Warnf("[bridge] send welcome: %v", err)
			}
			cancel()
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}

func (b *Bridge) runImageCleanup(ctx context.Context) {
	if err := b.CleanupOldImages(7 * 24 * time.Hour); err != nil {
		log.Warnf("[bridge] image cleanup failed: %v", err)
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = b.CleanupOldImages(7 * 24 * time.Hour)
		}
	}
}

// Shutdown 优雅关闭：等待所有 goroutine 退出，最长等待 ctx 超时
func (b *Bridge) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		b.sendWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Info("[bridge] all goroutines exited")
		return nil
	case <-ctx.Done():
		log.Warnf("[bridge] shutdown timeout: %v", ctx.Err())
		return ctx.Err()
	}
}

// WaitReady 阻塞等待底层 tmux session 就绪，供 main 在 attach 前调用
func (b *Bridge) WaitReady() error {
	b.termMu.RLock()
	term := b.term
	b.termMu.RUnlock()
	return term.WaitReady()
}

func (b *Bridge) forwardOutput(ctx context.Context) {
	// 首次 capture 作为 baseline（每个 receiver 独立初始化）
	// 失败时不初始化，避免下次 diff 发送完整 pane
	b.termMu.RLock()
	term := b.term
	b.termMu.RUnlock()
	pane, err := term.CaptureVisible(b.historyLines)
	if err != nil {
		log.Warnf("[bridge] initial capture failed: %v", err)
	} else {
		filtered := b.filterPane(pane)
		b.receivers.Range(func(k, v interface{}) bool {
			state := v.(*receiverState)
			state.mu.Lock()
			state.lastPane = filtered
			state.ready = true
			state.mu.Unlock()
			return true
		})
	}

	// 自适应捕获间隔：有 diff 时缩短，无 diff 时延长
	const (
		minInterval        = 1 * time.Second
		maxInterval        = 5 * time.Second
		maxConsecutiveIdle = 3
	)
	interval := b.captureInterval
	if interval < minInterval {
		interval = minInterval
	}
	if interval > maxInterval {
		interval = maxInterval
	}
	consecutiveIdle := 0

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			hasDiff := b.captureAndSend(ctx)
			if hasDiff {
				interval = max(interval*2/3, minInterval)
				consecutiveIdle = 0
			} else {
				consecutiveIdle++
				if consecutiveIdle >= maxConsecutiveIdle {
					interval = min(interval*3/2, maxInterval)
					consecutiveIdle = 0
				}
			}
			timer.Reset(interval)
		}
	}
}

func (b *Bridge) captureAndSend(ctx context.Context) bool {
	b.metrics.captures.Add(1)
	b.termMu.RLock()
	term := b.term
	b.termMu.RUnlock()
	pane, err := term.CaptureVisible(b.historyLines)
	if err != nil {
		log.Warnf("[bridge] capture pane failed: %v", err)
		return false
	}

	filtered := b.filterPane(pane)
	hasDiff := false

	// 遍历所有 receiver，并发计算 diff 并发送
	b.receivers.Range(func(k, v interface{}) bool {
		key := k.(receiverKey)
		state := v.(*receiverState)

		state.mu.Lock()
		if !state.ready || filtered == state.lastPane {
			state.mu.Unlock()
			return true
		}
		last := state.lastPane
		state.lastPane = filtered
		state.mu.Unlock()

		diff := diffPane(last, filtered)
		if diff == "" {
			return true
		}
		hasDiff = true

		// Send to the same receiver serially so newer diffs cannot overtake older ones.
		state.sendMu.Lock()
		b.sendWg.Add(1)
		go func() {
			defer b.sendWg.Done()
			defer state.sendMu.Unlock()
			b.sendBlocks(ctx, key, diff)
		}()
		return true
	})

	if hasDiff {
		b.metrics.diffHits.Add(1)
	} else {
		b.metrics.diffMisses.Add(1)
	}
	return hasDiff
}

const maxMarkdownLen = 3000 // interactive 卡片 JSON 有长度限制，内容留余量

// sendBlocks 把 diff 内容格式化为 markdown 并追加到 receiver 的累积 buffer 中，
// 然后更新已有消息或发送新消息。所有内容最终只体现在一条不断追加的消息里。
func (b *Bridge) sendBlocks(ctx context.Context, key receiverKey, diff string) {
	blocks := splitDiffIntoBlocks(diff)

	// sendMu 已锁定，串行访问 contentBuf 和 messageID
	val, _ := b.receivers.Load(key)
	state := val.(*receiverState)

	var newContent strings.Builder
	for _, block := range blocks {
		if block == "" {
			continue
		}
		md := formatBlockToMarkdown(block)
		if md == "" {
			continue
		}
		if newContent.Len() > 0 {
			newContent.WriteString("\n\n")
		}
		newContent.WriteString(md)
	}
	if newContent.Len() == 0 {
		return
	}

	// 追加到累积 buffer
	if state.contentBuf.Len() > 0 {
		state.contentBuf.WriteString("\n\n")
	}
	state.contentBuf.WriteString(newContent.String())

	// 截断过长的内容（从头部截断，保留尾部最新内容）
	content := state.contentBuf.String()
	if len(content) > maxMarkdownLen {
		content = truncateMarkdownContent(content, maxMarkdownLen)
		state.contentBuf.Reset()
		state.contentBuf.WriteString(content)
	}

	// 发送或更新
	timeout := b.sendTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if state.messageID != "" {
		if err := b.messenger.UpdateMessage(sendCtx, state.messageID, content); err != nil {
			log.Warnf("[bridge] update message failed: %v, opening new card", err)

			// 旧卡片已无法更新（可能内容超限），清空累积，只发本次 diff
			state.messageID = ""
			state.contentBuf.Reset()

			// 本次 diff 单独发送，超长则截断
			fresh := newContent.String()
			if len(fresh) > maxMarkdownLen {
				fresh = fresh[:maxMarkdownLen-20] + "\n...（内容已截断）"
			}
			msgID, err := b.messenger.SendMarkdown(sendCtx, key.kind, key.id, fresh)
			if err != nil {
				log.Warnf("[bridge] send new card failed: %v", err)
			} else {
				state.messageID = msgID
				state.contentBuf.WriteString(fresh)
				b.metrics.messagesSent.Add(1)
			}
		} else {
			b.metrics.messagesSent.Add(1)
		}
	} else {
		msgID, err := b.messenger.SendMarkdown(sendCtx, key.kind, key.id, content)
		if err != nil {
			log.Warnf("[bridge] send markdown failed: %v", err)
		} else {
			state.messageID = msgID
			b.metrics.messagesSent.Add(1)
		}
	}
}

// splitDiffIntoBlocks 将 diff 内容按消息类型拆分为多个块
// 连续的 Markdown 表格行作为一个块，其他行作为普通文本块
func splitDiffIntoBlocks(diff string) []string {
	lines := strings.Split(diff, "\n")
	var blocks []string
	var textBuf []string
	var tableBuf []string

	flushText := func() {
		if len(textBuf) > 0 {
			joined := strings.Join(textBuf, "\n")
			if joined != "" {
				blocks = append(blocks, joined)
			}
			textBuf = nil
		}
	}
	flushTable := func() {
		if len(tableBuf) > 0 {
			joined := strings.Join(tableBuf, "\n")
			if joined != "" {
				blocks = append(blocks, joined)
			}
			tableBuf = nil
		}
	}

	for _, line := range lines {
		if isMarkdownTableLine(line) {
			flushText()
			tableBuf = append(tableBuf, line)
		} else {
			flushTable()
			textBuf = append(textBuf, line)
		}
	}
	flushText()
	flushTable()
	return blocks
}

func isMarkdownTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Markdown 表格行以 | 开头或结尾
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

func isMarkdownTable(block string) bool {
	lines := strings.Split(block, "\n")
	if len(lines) < 2 {
		return false
	}
	// 至少有两行且都是表格行
	for _, line := range lines {
		if !isMarkdownTableLine(line) {
			return false
		}
	}
	return true
}

// filterPane 过滤噪音，并将 Unicode 表格转换为 Markdown 表格
func (b *Bridge) filterPane(pane string) string {
	lines := strings.Split(pane, "\n")
	var result []string
	var tableBuf []string
	inTable := false

	b.lastUserMsgMu.Lock()
	userMsg := b.lastUserMessage
	b.lastUserMsgMu.Unlock()

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		if b.isNoiseLine(line) {
			continue
		}

		// 过滤"用户消息 + Tip"模式：当前行是用户消息，下一行是 Tip
		if line == userMsg && i+1 < len(lines) {
			nextLine := strings.TrimSpace(lines[i+1])
			if IsTipLine(nextLine) {
				i++ // 跳过下一行（Tip）
				continue
			}
		}

		// 表格内容行：包含 │ 但不是树形连接符
		if isTableContentLine(line) {
			if !inTable {
				inTable = true
			}
			if md := convertTableLine(line); md != "" {
				tableBuf = append(tableBuf, md)
			}
			continue
		}

		// 非表格行：如果之前在表格中，先 flush 表格
		if inTable {
			result = append(result, flushTable(tableBuf)...)
			tableBuf = nil
			inTable = false
		}
		result = append(result, line)
	}

	if inTable && len(tableBuf) > 0 {
		result = append(result, flushTable(tableBuf)...)
	}
	return strings.Join(result, "\n")
}

// isNoiseLine 判断是否是噪音行（边框、进度提示、shortcut 提示等）
func (b *Bridge) isNoiseLine(line string) bool {
	if isTableBorderOnly(line) || isHorizontalBorder(line) {
		return true
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "ctrl+") {
		return true
	}
	for _, pattern := range b.noisePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	if strings.Contains(lower, "esc to interrupt") || strings.Contains(lower, "for shortcuts") {
		return true
	}
	// Claude TUI 输入提示符 "> " 或 ">" 单独成行
	if isTUIPrompt(line) {
		return true
	}
	// 仅 Claude 命令：过滤 TUI 装饰性状态行（spinner、Thinking... 等）
	if b.isClaude && isClaudeDecorativeLine(line) {
		return true
	}
	return false
}

// isTUIPrompt 判断是否是 TUI 输入框提示符
// Claude Code 提示符类似 "> " 开头后跟用户输入
func isTUIPrompt(line string) bool {
	return strings.HasPrefix(line, ">") && len(strings.TrimSpace(line)) <= 2
}

// isTableContentLine 判断是否是表格内容行（包含 │ 或全角 ｜，但不是树形）
func isTableContentLine(line string) bool {
	return (strings.Contains(line, "│") || strings.Contains(line, "｜")) && !strings.Contains(line, "├──") && !strings.Contains(line, "└──")
}

func isTableBorderOnly(line string) bool {
	hasBorderChar := false
	for _, c := range line {
		if c == ' ' {
			continue
		}
		if strings.ContainsRune("│｜┌┬┐├┼┤└┴┘─━", c) {
			hasBorderChar = true
		} else {
			return false
		}
	}
	return hasBorderChar
}

func isHorizontalBorder(line string) bool {
	trimmed := strings.TrimSpace(line)
	if utf8.RuneCountInString(trimmed) < 5 {
		return false
	}
	for _, c := range trimmed {
		if c != '─' && c != '━' && c != '-' && c != '=' {
			return false
		}
	}
	return true
}

func convertTableLine(line string) string {
	line = strings.TrimSpace(line)
	var parts []string

	// 标准终端表格中边框前后有空格（如 "│ A │ B │"）
	// 用精确算法：只有前后是空格的 │/｜ 才是边框，避免单元格内容含 │ 被误判
	if strings.Contains(line, " │ ") || strings.Contains(line, " ｜ ") {
		var sb strings.Builder
		runes := []rune(line)
		for i := 0; i < len(runes); i++ {
			if (runes[i] == '│' || runes[i] == '｜') && (i == 0 || runes[i-1] == ' ') && (i == len(runes)-1 || runes[i+1] == ' ') {
				parts = append(parts, sb.String())
				sb.Reset()
			} else {
				sb.WriteRune(runes[i])
			}
		}
		parts = append(parts, sb.String())
	} else {
		// 紧凑表格无空格（如 "│A│B│"），回退到原始方案
		if strings.Contains(line, "｜") {
			parts = strings.Split(line, "｜")
		} else {
			parts = strings.Split(line, "│")
		}
	}

	var cols []string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		// 跳过分割产生的首尾空字符串，但保留中间的空列（空单元格）
		if part == "" && (i == 0 || i == len(parts)-1) {
			continue
		}
		cols = append(cols, part)
	}
	if len(cols) == 0 {
		return ""
	}
	return "| " + strings.Join(cols, " | ") + " |"
}

func parseTableCells(line string) []string {
	parts := strings.Split(line, "|")
	var cells []string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" && (i == 0 || i == len(parts)-1) {
			continue
		}
		cells = append(cells, part)
	}
	return cells
}

// formatBlockToMarkdown 把单个 block 转为 markdown 格式。
// 表格块保持 markdown 表格；其他内容直接以纯文本发送（lark_md 会渲染基础 markdown）。
func formatBlockToMarkdown(block string) string {
	return strings.TrimSpace(block)
}

// truncateMarkdownContent 从头部截断内容，保留尾部。
// 截断位置尽量选在换行处，避免破坏代码块。
func truncateMarkdownContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	cutoff := len(content) - maxLen + 50 // 留 50 字符给提示
	if idx := strings.Index(content[cutoff:], "\n"); idx >= 0 {
		cutoff += idx + 1
	}
	return "...（前面内容已省略）\n\n" + content[cutoff:]
}

func flushTable(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	colCount := len(parseTableCells(lines[0]))
	if colCount <= 0 {
		return lines
	}
	sep := "|"
	for i := 0; i < colCount; i++ {
		sep += " --- |"
	}
	return append([]string{lines[0], sep}, lines[1:]...)
}

func diffPane(old, new string) string {
	if old == "" {
		return new
	}

	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// 使用 LCS（最长公共子序列）计算行级 diff
	// 保留新增行的顺序和位置信息，能正确检测 TUI 重排/重写场景
	lcsLines := lcs(oldLines, newLines)

	// 用 map 记录 LCS 中每行的出现次数（处理重复行）
	lcsCount := make(map[string]int)
	for _, line := range lcsLines {
		lcsCount[line]++
	}

	var added []string
	for _, line := range newLines {
		if lcsCount[line] > 0 {
			lcsCount[line]--
			continue
		}
		added = append(added, line)
	}
	return strings.Join(added, "\n")
}

// lcs 计算两个字符串切片的最长公共子序列
func lcs(a, b []string) []string {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return nil
	}

	// dp[i][j] 表示 a[:i] 和 b[:j] 的 LCS 长度
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// 回溯找 LCS
	var result []string
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, a[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	// 反转
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func (b *Bridge) Close() {
	b.closeOnce.Do(func() {
		// 只关闭 bot，保留 tmux session 让用户可以重新 attach
		b.messenger.Close()
	})
}

// RestartTmux 热重启 tmux session：读取最新配置、kill 旧 session、启动新 session、重置 receiver baseline。
func (b *Bridge) RestartTmux(workDir string) error {
	cfg, err := config.Reload(".env")
	if err != nil {
		return fmt.Errorf("reload config failed: %w", err)
	}

	command := buildCommand(&BridgeConfig{
		Command:           cfg.Command,
		BypassPermissions: cfg.BypassPermissions,
		CodexQueueMode:    cfg.CodexQueueMode,
	})

	// 先同步 kill 旧 session，确保同名 session 可被重新创建
	b.termMu.RLock()
	oldTerm := b.term
	b.termMu.RUnlock()
	_ = oldTerm.Kill()
	// 等待旧 session 完全消失，避免创建同名 session 失败
	for i := 0; i < 20; i++ {
		if !oldTerm.HasSession() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 创建新 session
	tm := terminal.NewTmuxSession("fcc")
	if !tm.IsAvailable() {
		return fmt.Errorf("tmux is not available")
	}
	if err := tm.Start(command, workDir); err != nil {
		return fmt.Errorf("start new tmux session failed: %w", err)
	}

	b.termMu.Lock()
	b.term = tm
	b.termMu.Unlock()

	if err := tm.WaitReady(); err != nil {
		return fmt.Errorf("new tmux session not ready: %w", err)
	}

	// 重置所有 receiver 的 baseline，防止新旧 tmux 内容 diff 错误。
	// lastPane 设为空字符串，下一次 diff 会发送完整 pane 内容。
	// 同时清空累积消息，避免旧内容和新 session 混在一起。
	b.receivers.Range(func(k, v interface{}) bool {
		state := v.(*receiverState)
		state.sendMu.Lock()
		state.messageID = ""
		state.contentBuf.Reset()
		state.sendMu.Unlock()
		state.mu.Lock()
		state.lastPane = ""
		state.ready = true
		state.mu.Unlock()
		return true
	})

	log.Infof("[bridge] tmux restarted with command: %s", command)
	return nil
}

// CleanupOldImages 删除超过 maxAge 的本地图片，释放磁盘
func (b *Bridge) CleanupOldImages(maxAge time.Duration) error {
	return b.messenger.CleanupOldImages(maxAge)
}

// LogMetrics 输出运行时指标到日志
func (b *Bridge) LogMetrics() {
	log.Infof("[metrics] received=%d sent=%d captures=%d diffHits=%d diffMisses=%d",
		b.metrics.messagesReceived.Load(),
		b.metrics.messagesSent.Load(),
		b.metrics.captures.Load(),
		b.metrics.diffHits.Load(),
		b.metrics.diffMisses.Load(),
	)
}
