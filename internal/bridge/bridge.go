package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fcc/internal/bot"
	"fcc/internal/config"
	"fcc/internal/dialog"
	"fcc/internal/log"
	"fcc/internal/terminal"
)

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

	// 调优参数（全部可选，传零值则使用内部默认值）
	CaptureIntervalMin           time.Duration
	CaptureIntervalMax           time.Duration
	SendTimeoutMin               time.Duration
	SendTimeoutMax               time.Duration
	InterruptDebounce            time.Duration
	AdaptiveCaptureMin           time.Duration
	AdaptiveCaptureMax           time.Duration
	AdaptiveCaptureIdleThreshold int
	PendingTableIdleWait         time.Duration
	PendingCodeIdleWait          time.Duration
	MaxMarkdownLen               int
	WelcomeDelay                 time.Duration
	WelcomeTimeout               time.Duration
	ImageCleanupMaxAge           time.Duration
	ImageCleanupInterval         time.Duration
	CodexInputDelay              time.Duration

	// Bot 重试退避
	BotRetryBackoff    time.Duration
	BotRetryMaxBackoff time.Duration
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
	isCodex           bool   // 仅 Codex 命令启用 Codex TUI 工具日志过滤
	targetName        string // 首次启动欢迎消息的目标用户

	lastUserMessage string     // 最近从飞书收到的用户消息，用于过滤 tmux 回显
	lastUserMsgMu   sync.Mutex // 保护 lastUserMessage

	dialogLogger *dialog.Logger // 飞书-Claude 对话日志记录器

	// 调优参数（从 BridgeConfig 传入）
	captureIntervalMin           time.Duration
	captureIntervalMax           time.Duration
	sendTimeoutMin               time.Duration
	sendTimeoutMax               time.Duration
	adaptiveCaptureMin           time.Duration
	adaptiveCaptureMax           time.Duration
	adaptiveCaptureIdleThreshold int
	pendingTableIdleWait         time.Duration
	pendingCodeIdleWait          time.Duration
	maxMarkdownLen               int
	welcomeDelay                 time.Duration
	welcomeTimeout               time.Duration
	imageCleanupMaxAge           time.Duration
	imageCleanupInterval         time.Duration
	codexInputDelay              time.Duration
}

// buildCommand 根据配置构建完整的启动命令（追加 bypass 参数、codex queue mode 等）。
func buildCommand(cfg *BridgeConfig) string {
	command := cfg.Command
	// 如果启用了 bypass permissions，追加对应工具的免确认参数
	if cfg.BypassPermissions {
		if isClaudeCommand(command) {
			if !strings.Contains(command, "--dangerously-skip-permissions") {
				command += " --dangerously-skip-permissions"
			}
			if !strings.Contains(command, "--permission-mode") {
				command += " --permission-mode bypassPermissions"
			}
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

	captureIntervalMin := dval(cfg.CaptureIntervalMin, 500*time.Millisecond)
	captureIntervalMax := dval(cfg.CaptureIntervalMax, 60*time.Second)
	captureInterval := cfg.CaptureInterval
	if captureInterval < captureIntervalMin {
		captureInterval = captureIntervalMin
	}
	if captureInterval > captureIntervalMax {
		captureInterval = captureIntervalMax
	}

	sendTimeoutMin := dval(cfg.SendTimeoutMin, 1*time.Second)
	sendTimeoutMax := dval(cfg.SendTimeoutMax, 120*time.Second)
	sendTimeout := cfg.SendTimeout
	if sendTimeout < sendTimeoutMin {
		sendTimeout = sendTimeoutMin
	}
	if sendTimeout > sendTimeoutMax {
		sendTimeout = sendTimeoutMax
	}

	b := &Bridge{
		captureInterval:              captureInterval,
		sendTimeout:                  sendTimeout,
		historyLines:                 cfg.TMUXHistoryLines,
		interruptDebounce:            dval(cfg.InterruptDebounce, 500*time.Millisecond),
		noisePatterns:                cfg.NoisePatterns,
		isClaude:                     isClaudeCommand(command),
		isCodex:                      isCodexCommand(command),
		targetName:                   cfg.TargetName,
		captureIntervalMin:           captureIntervalMin,
		captureIntervalMax:           captureIntervalMax,
		sendTimeoutMin:               sendTimeoutMin,
		sendTimeoutMax:               sendTimeoutMax,
		adaptiveCaptureMin:           dval(cfg.AdaptiveCaptureMin, 1*time.Second),
		adaptiveCaptureMax:           dval(cfg.AdaptiveCaptureMax, 5*time.Second),
		adaptiveCaptureIdleThreshold: ival(cfg.AdaptiveCaptureIdleThreshold, 3),
		pendingTableIdleWait:         dval(cfg.PendingTableIdleWait, 12*time.Second),
		pendingCodeIdleWait:          dval(cfg.PendingCodeIdleWait, 5*time.Second),
		maxMarkdownLen:               ival(cfg.MaxMarkdownLen, 3000),
		welcomeDelay:                 dval(cfg.WelcomeDelay, 3*time.Second),
		welcomeTimeout:               dval(cfg.WelcomeTimeout, 30*time.Second),
		imageCleanupMaxAge:           dval(cfg.ImageCleanupMaxAge, 7*24*time.Hour),
		imageCleanupInterval:         dval(cfg.ImageCleanupInterval, 24*time.Hour),
		codexInputDelay:              dval(cfg.CodexInputDelay, 150*time.Millisecond),
	}
	b.messenger = bot.New(cfg.AppID, cfg.AppSecret, b.handleMessage, cfg.SendRetries, cfg.BotRetryBackoff, cfg.BotRetryMaxBackoff)

	imageDir := ".fcc/images"
	if cfg.WorkDir != "" {
		imageDir = filepath.Join(cfg.WorkDir, ".fcc", "images")
	}
	b.messenger.SetImageDir(imageDir)

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

	logDir := ".fcc/logs"
	if cfg.WorkDir != "" {
		logDir = filepath.Join(cfg.WorkDir, ".fcc", "logs")
	}
	b.dialogLogger = dialog.NewLogger(logDir)

	return b, nil
}

// dval returns v if positive, otherwise def.
func dval(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

// ival returns v if positive, otherwise def.
func ival(v, def int) int {
	if v > 0 {
		return v
	}
	return def
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

func looksLikeImagePath(text string) bool {
	ext := strings.ToLower(filepath.Ext(text))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" {
		return false
	}
	info, err := os.Stat(text)
	return err == nil && !info.IsDir()
}

// toRelativePath 把绝对路径转为相对当前工作目录的路径，避免终端折行导致 @path 失效。
func toRelativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	sep := string(filepath.Separator)
	if !strings.HasPrefix(absPath, cwd+sep) {
		return absPath
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

// extractImagePaths 从 text 中提取所有本地图片路径（支持空格或换行分隔）。
func extractImagePaths(text string) []string {
	fields := strings.Fields(text)
	var paths []string
	for _, f := range fields {
		if looksLikeImagePath(f) {
			paths = append(paths, f)
		}
	}
	return paths
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

	// 如果收到的是本地图片路径，包装成自然语言提示让 Claude Code 用 Read 工具读取。
	// Claude Code 的 Read 工具支持图片文件（自动转 base64），无需依赖 @path 交互式选择器。
	if paths := extractImagePaths(text); len(paths) > 0 {
		var imgParts []string
		for _, p := range paths {
			rel := toRelativePath(p)
			log.Debugf("[bridge] detected image path, isClaude=%v: abs=%s rel=%s", b.isClaude, p, rel)
			imgParts = append(imgParts, rel)
		}
		// 提取非图片路径的文字内容
		otherText := text
		for _, p := range paths {
			otherText = strings.Replace(otherText, p, "", 1)
		}
		otherText = strings.TrimSpace(otherText)

		if b.isClaude {
			if otherText == "" {
				text = fmt.Sprintf("请查看并分析以下图片：%s", strings.Join(imgParts, "，"))
			} else {
				text = fmt.Sprintf("%s（图片路径：%s，请查看分析）", otherText, strings.Join(imgParts, "，"))
			}
		} else {
			if otherText == "" {
				text = fmt.Sprintf("（用户发送了图片：%s）", strings.Join(imgParts, "，"))
			} else {
				text = fmt.Sprintf("%s（用户发送了图片：%s）", otherText, strings.Join(imgParts, "，"))
			}
		}
	} else if strings.HasPrefix(text, "[") && strings.Contains(text, "图片") {
		log.Warnf("[bridge] received image-related fallback text: %q", text)
	}

	// 记录用户消息（trim 后），用于后续过滤 tmux 中的回显 + Tip 组合
	b.lastUserMsgMu.Lock()
	b.lastUserMessage = strings.TrimSpace(text)
	b.lastUserMsgMu.Unlock()

	b.metrics.messagesReceived.Add(1)
	if b.dialogLogger != nil {
		b.dialogLogger.LogQuestion(text)
	}
	log.Debugf("[bridge] sending to tmux: %q", log.Truncate(text, 80))
	b.termMu.RLock()
	term := b.term
	b.termMu.RUnlock()
	if err := b.sendUserInputToTerminal(term, text); err != nil {
		log.Warnf("[bridge] failed to send keys: %v", err)
	}
}

func (b *Bridge) sendUserInputToTerminal(term Terminal, text string) error {
	log.Debugf("[bridge] sendUserInputToTerminal: isCodex=%v text=%q", b.isCodex, log.Truncate(text, 200))
	if !b.isCodex {
		if err := term.SendKeys(text); err != nil {
			return err
		}
		return term.SendSpecialKey("Enter")
	}
	if err := term.SendLiteral(text); err != nil {
		return err
	}
	time.Sleep(b.codexInputDelay)
	return term.SendSpecialKey("C-m")
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
		case <-time.After(b.welcomeDelay):
		}
		if b.targetName != "" {
			welcomeCtx, cancel := context.WithTimeout(ctx, b.welcomeTimeout)
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
	if err := b.CleanupOldImages(b.imageCleanupMaxAge); err != nil {
		log.Warnf("[bridge] image cleanup failed: %v", err)
	}
	ticker := time.NewTicker(b.imageCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.CleanupOldImages(b.imageCleanupMaxAge); err != nil {
				log.Warnf("[bridge] image cleanup: %v", err)
			}
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
	minInterval := b.adaptiveCaptureMin
	maxInterval := b.adaptiveCaptureMax
	maxConsecutiveIdle := b.adaptiveCaptureIdleThreshold
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
	receiverCount := 0

	// 遍历所有 receiver，并发计算 diff 并发送
	b.receivers.Range(func(k, v interface{}) bool {
		receiverCount++
		key := k.(receiverKey)
		state := v.(*receiverState)

		state.mu.Lock()
		if !state.ready || filtered == state.lastPane {
			state.mu.Unlock()
			if b.flushPendingTableIfReady(ctx, key, state, b.pendingTableIdleWait) || b.flushPendingCodeIfReady(ctx, key, state, b.pendingCodeIdleWait) {
				hasDiff = true
			}
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
		log.Infof("[bridge] captureAndSend diff len=%d for receiver=%s", len(diff), key.id)
		if b.dialogLogger != nil {
			b.dialogLogger.LogAnswer(diff)
		}

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

	log.Debugf("[bridge] captureAndSend: receivers=%d paneLines=%d filteredLines=%d hasDiff=%v",
		receiverCount, len(strings.Split(pane, "\n")), len(strings.Split(filtered, "\n")), hasDiff)

	if hasDiff {
		b.metrics.diffHits.Add(1)
	} else {
		b.metrics.diffMisses.Add(1)
	}
	return hasDiff
}

const (
	markdownCardBreak = "\f"
)

func (b *Bridge) Close() {
	b.closeOnce.Do(func() {
		// 只关闭 bot，保留 tmux session 让用户可以重新 attach
		b.messenger.Close()
		if b.dialogLogger != nil {
			b.dialogLogger.Close()
		}
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
	if err := oldTerm.Kill(); err != nil {
		log.Debugf("[bridge] kill old tmux session: %v", err)
	}
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
	b.isClaude = isClaudeCommand(command)
	b.isCodex = isCodexCommand(command)
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
