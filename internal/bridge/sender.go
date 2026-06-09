package bridge

import (
	"context"
	"strings"
	"time"

	"fcc/internal/log"
)

// sendBlocks 把 diff 内容格式化为 markdown 并追加到 receiver 的累积 buffer 中，
// 然后更新已有消息或发送新消息。所有内容最终只体现在一条不断追加的消息里。
func (b *Bridge) sendBlocks(ctx context.Context, key receiverKey, diff string) {
	blocks := splitDiffIntoBlocks(diff)
	log.Infof("[bridge] sendBlocks: receiver=%s blocks=%d diffPreview=%q", key.id, len(blocks), log.Truncate(diff, 200))

	// sendMu 已锁定，串行访问 contentBuf 和 messageID
	val, _ := b.receivers.Load(key)
	state := val.(*receiverState)

	if len(state.pendingTable) > 0 || hasMarkdownTableLineBlock(blocks) {
		b.sendBlocksWithTables(ctx, key, state, blocks)
		return
	}

	b.sendMarkdownBlocks(ctx, key, state, blocks)
}

func hasMarkdownTableLineBlock(blocks []string) bool {
	for _, block := range blocks {
		if isMarkdownTableLineBlock(block) {
			return true
		}
	}
	return false
}

func (b *Bridge) sendBlocksWithTables(ctx context.Context, key receiverKey, state *receiverState, blocks []string) {
	var textBlocks []string

	flushText := func() {
		if len(textBlocks) == 0 {
			return
		}
		b.sendMarkdownBlocks(ctx, key, state, textBlocks)
		textBlocks = nil
	}
	flushTable := func() {
		if len(state.pendingTable) == 0 {
			return
		}
		if !isMarkdownTable(strings.Join(state.pendingTable, "\n")) {
			return
		}
		table := strings.Join(state.pendingTable, "\n")
		state.pendingTable = nil
		state.pendingTableSince = time.Time{}
		b.sendInteractiveTableOrMarkdown(ctx, key, state, table)
	}
	appendPendingTable := func(block string) {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		state.pendingTable = append(state.pendingTable, lines...)
		state.pendingTableSince = time.Now()
	}

	for _, block := range blocks {
		if block == "" {
			continue
		}
		if !isMarkdownTableLineBlock(block) {
			flushTable()
			textBlocks = append(textBlocks, block)
			continue
		}

		flushText()
		if isMarkdownTable(block) {
			if len(state.pendingTable) > 0 {
				flushTable()
			}
			appendPendingTable(block)
			continue
		}

		appendPendingTable(block)
	}

	flushText()
}

func (b *Bridge) flushPendingTableIfReady(ctx context.Context, key receiverKey, state *receiverState, wait time.Duration) bool {
	state.sendMu.Lock()
	defer state.sendMu.Unlock()
	if len(state.pendingTable) == 0 || time.Since(state.pendingTableSince) < wait {
		return false
	}

	table := strings.Join(state.pendingTable, "\n")
	if !isMarkdownTable(table) {
		return false
	}
	state.pendingTable = nil
	state.pendingTableSince = time.Time{}
	b.sendInteractiveTableOrMarkdown(ctx, key, state, table)
	return true
}

func (b *Bridge) flushPendingCodeIfReady(ctx context.Context, key receiverKey, state *receiverState, wait time.Duration) bool {
	state.sendMu.Lock()
	defer state.sendMu.Unlock()
	if len(state.pendingCode) == 0 || time.Since(state.pendingCodeSince) < wait {
		return false
	}

	code := fencedCodeBlock(state.pendingCodeLang, state.pendingCode)
	state.pendingCode = nil
	state.pendingCodeLang = ""
	state.pendingCodeSince = time.Time{}
	b.sendMarkdownContent(ctx, key, state, code)
	return true
}

func (b *Bridge) sendInteractiveTableOrMarkdown(ctx context.Context, key receiverKey, state *receiverState, table string) {
	timeout := b.sendTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	err := b.messenger.SendInteractiveTable(sendCtx, key.kind, key.id, table)
	cancel()
	if err != nil {
		log.Warnf("[bridge] send interactive table failed: %v, falling back to markdown", err)
		b.sendMarkdownBlocks(ctx, key, state, []string{table})
		return
	}

	b.metrics.messagesSent.Add(1)
	// 表格独立成卡片；后续文本新开卡片，保持表格前后消息顺序。
	state.messageID = ""
	state.contentBuf.Reset()
}

func (b *Bridge) sendMarkdownBlocks(ctx context.Context, key receiverKey, state *receiverState, blocks []string) {
	var newContent strings.Builder
	appendMarkdown := func(md string) {
		md = strings.TrimSpace(md)
		if md == "" {
			return
		}
		if newContent.Len() > 0 {
			newContent.WriteString("\n")
		}
		newContent.WriteString(md)
	}
	flushMarkdown := func() {
		incoming := compactMarkdownSpacing(newContent.String())
		if incoming == "" {
			newContent.Reset()
			return
		}
		b.sendMarkdownContent(ctx, key, state, incoming)
		newContent.Reset()
	}

	for _, block := range blocks {
		if block == "" {
			continue
		}
		if block == markdownCardBreak {
			flushMarkdown()
			state.messageID = ""
			state.contentBuf.Reset()
			continue
		}
		b.appendMarkdownBlock(ctx, key, state, block, appendMarkdown)
	}
	flushMarkdown()
}

func (b *Bridge) appendMarkdownBlock(ctx context.Context, key receiverKey, state *receiverState, block string, appendMarkdown func(string)) {
	_ = ctx
	_ = key

	for {
		block = strings.TrimSpace(block)
		if block == "" {
			return
		}

		if state.pendingCodeLang != "" {
			done, remainder := appendPendingCodeLines(state, strings.Split(block, "\n"))
			if done != "" {
				appendMarkdown(done)
			}
			if strings.TrimSpace(remainder) == "" {
				return
			}
			block = remainder
			continue
		}

		prefix, codeLines, ok := splitStreamingJSONStart(block)
		if ok && !isJSONCodeComplete(codeLines) {
			appendMarkdown(formatBlockToMarkdown(prefix))
			state.pendingCodeLang = "json"
			state.pendingCode = append([]string(nil), codeLines...)
			state.pendingCodeSince = time.Now()
			return
		}

		prefix, codeLines, ok = splitStreamingGoStart(block)
		if ok && !isGoCodeComplete(codeLines) {
			appendMarkdown(formatBlockToMarkdown(prefix))
			state.pendingCodeLang = "go"
			state.pendingCode = append([]string(nil), codeLines...)
			state.pendingCodeSince = time.Now()
			return
		}

		prefix, codeLines, ok = splitStreamingJavaScriptStart(block)
		if ok && !isJavaScriptCodeComplete(codeLines) {
			appendMarkdown(formatBlockToMarkdown(prefix))
			state.pendingCodeLang = "javascript"
			state.pendingCode = append([]string(nil), codeLines...)
			state.pendingCodeSince = time.Now()
			return
		}

		appendMarkdown(formatBlockToMarkdown(block))
		return
	}
}

func (b *Bridge) sendMarkdownContent(ctx context.Context, key receiverKey, state *receiverState, incoming string) {
	for _, chunk := range splitMarkdownContent(incoming, b.maxMarkdownLen) {
		b.sendMarkdownChunk(ctx, key, state, chunk)
	}
}

func (b *Bridge) sendMarkdownChunk(ctx context.Context, key receiverKey, state *receiverState, incoming string) {
	log.Infof("[bridge] sendMarkdownChunk: receiver=%s len=%d preview=%q", key.id, len(incoming), log.Truncate(incoming, 200))
	// 追加到累积 buffer
	current := compactMarkdownSpacing(state.contentBuf.String())
	separator := ""
	if current != "" {
		separator = "\n"
	}
	content := compactMarkdownSpacing(current + separator + incoming)
	if current != "" && b.maxMarkdownLen > 0 && len(content) > b.maxMarkdownLen {
		state.messageID = ""
		state.contentBuf.Reset()
		content = incoming
	}

	if state.contentBuf.Len() > 0 {
		state.contentBuf.Reset()
	}
	state.contentBuf.WriteString(content)

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

			fresh := content
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

func appendPendingCodeLines(state *receiverState, lines []string) (string, string) {
	for i, line := range lines {
		if isCodexReflowBulletAnchor(line) && isOnlyCodexStaleCodePrefix(strings.Join(state.pendingCode, "\n")) {
			state.pendingCode = nil
			state.pendingCodeLang = ""
			state.pendingCodeSince = time.Time{}
			remainder := strings.TrimSpace(strings.Join(lines[i:], "\n"))
			return "", remainder
		}

		if state.pendingCodeLang == "javascript" &&
			isJavaScriptCodeComplete(state.pendingCode) &&
			!isJavaScriptCodeLine(line) &&
			isLikelyProseLine(line) {
			done := fencedCodeBlock(state.pendingCodeLang, state.pendingCode)
			state.pendingCode = nil
			state.pendingCodeLang = ""
			state.pendingCodeSince = time.Time{}
			return done, strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}

		state.pendingCode = append(state.pendingCode, line)
		if isPendingCodeComplete(state.pendingCodeLang, state.pendingCode) {
			if state.pendingCodeLang == "javascript" && nextJavaScriptLineContinues(lines[i+1:]) {
				continue
			}
			done := fencedCodeBlock(state.pendingCodeLang, state.pendingCode)
			state.pendingCode = nil
			state.pendingCodeLang = ""
			state.pendingCodeSince = time.Time{}
			remainder := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			return done, remainder
		}
	}
	return "", ""
}

func nextJavaScriptLineContinues(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return isJavaScriptCodeLine(line)
	}
	return false
}

func isPendingCodeComplete(lang string, lines []string) bool {
	switch lang {
	case "json":
		return isJSONCodeComplete(lines)
	case "go":
		return isGoCodeComplete(lines)
	case "javascript":
		return isJavaScriptCodeComplete(lines)
	default:
		return len(lines) > 0
	}
}
