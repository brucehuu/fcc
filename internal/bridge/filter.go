package bridge

import (
	"strings"
	"unicode/utf8"
)

// filterPane 过滤噪音，并将 Unicode 表格转换为 Markdown 表格
func (b *Bridge) filterPane(pane string) string {
	lines := strings.Split(pane, "\n")
	var result []string
	var tableBuf []string
	var codexPlainTableBuf []string
	inTable := false

	flushCodexPlainTable := func() {
		if len(codexPlainTableBuf) == 0 {
			return
		}
		result = append(result, flushCodexPlainTable(codexPlainTableBuf)...)
		codexPlainTableBuf = nil
	}

	b.lastUserMsgMu.Lock()
	userMsg := b.lastUserMessage
	b.lastUserMsgMu.Unlock()

	skipClaudeProgressContinuation := false
	skipClaudeToolOutput := false
	skipCodexToolOutput := false
	for i := 0; i < len(lines); i++ {
		rawLine := strings.TrimRight(lines[i], " \t\r")
		line := strings.TrimSpace(rawLine)
		if line == "" {
			if b.isCodex {
				flushCodexPlainTable()
				if len(result) > 0 && result[len(result)-1] != "" {
					result = append(result, "")
				}
			}
			skipClaudeToolOutput = false
			continue
		}

		if b.isCodex && skipCodexToolOutput {
			if isCodexAssistantTextLine(line) {
				skipCodexToolOutput = false
			} else {
				continue
			}
		}

		if b.isCodex && isCodexNoiseLine(line, userMsg) {
			continue
		}

		if b.isCodex && isCodexToolActivityLine(line) {
			skipCodexToolOutput = true
			continue
		}

		if b.isCodex && isCodexPlainTableLine(rawLine) {
			if inTable {
				result = append(result, flushTable(tableBuf)...)
				tableBuf = nil
				inTable = false
			}
			codexPlainTableBuf = append(codexPlainTableBuf, rawLine)
			continue
		}

		if b.isCodex && len(codexPlainTableBuf) > 0 && isCodexPlainTableContinuationLine(rawLine) {
			codexPlainTableBuf = append(codexPlainTableBuf, rawLine)
			continue
		}

		if b.isCodex {
			if tableLine, ok := codexBulletMarkdownTableLine(rawLine); ok {
				if inTable {
					result = append(result, flushTable(tableBuf)...)
					tableBuf = nil
					inTable = false
				}
				flushCodexPlainTable()
				result = append(result, tableLine)
				continue
			}
		}

		if b.isClaude && skipClaudeToolOutput {
			if isClaudeToolProgressLine(line) {
				continue
			}
			if IsClaudeAssistantTextLine(line) || IsClaudeUserPromptLine(line) {
				skipClaudeToolOutput = false
			} else {
				continue
			}
		}

		if b.isClaude && isClaudeToolProgressLine(line) {
			skipClaudeToolOutput = true
			continue
		}

		if b.isClaude && IsClaudeTUIUserPromptLine(line) {
			continue
		}

		if b.isClaude && skipClaudeProgressContinuation && IsClaudeLiveProgressContinuation(line) {
			continue
		}
		skipClaudeProgressContinuation = false

		if b.isClaude && IsClaudeLiveProgressLine(line) {
			skipClaudeProgressContinuation = true
			continue
		}

		if b.isClaude && IsClaudeUserEchoLine(line, userMsg) {
			continue
		}

		if b.isNoiseLine(line) {
			continue
		}

		flushCodexPlainTable()

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
		result = append(result, rawLine)
	}

	flushCodexPlainTable()
	if inTable && len(tableBuf) > 0 {
		result = append(result, flushTable(tableBuf)...)
	}
	if b.isCodex {
		result = reflowCodexWrappedLines(result)
		result = normalizeCodexShallowMarkdownListIndent(result)
		result = emphasizeCodexHeadingLines(result)
	}
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return strings.Join(result, "\n")
}

// isNoiseLine 判断是否是噪音行（边框、进度提示、shortcut 提示等）

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
	if strings.Contains(lower, "esc to interrupt") ||
		strings.Contains(lower, "for shortcuts") ||
		strings.Contains(lower, "bypass permissions on") ||
		strings.Contains(lower, "shift+tab to cycle") ||
		strings.Contains(lower, "press up to edit queued messages") {
		return true
	}
	// Claude TUI 输入提示符 "> " 或 ">" 单独成行
	if isTUIPrompt(line) {
		return true
	}
	// 无条件过滤 Claude Tip 行（⎿  Tip: ...）
	if IsTipLine(line) {
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

// isTUIPrompt 判断是否是 TUI 输入框提示符
// Claude Code 提示符类似 "> " 开头后跟用户输入
func isTUIPrompt(line string) bool {
	return strings.HasPrefix(line, ">") && len(strings.TrimSpace(line)) <= 2
}

// isTableContentLine 判断是否是表格内容行（包含 │ 或全角 ｜，但不是树形）

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

func reflowCodexWrappedLines(lines []string) []string {
	var result []string
	inCodexAnswer := false

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			result = append(result, line)
			continue
		}

		if isCodexReflowBulletAnchor(trimmed) {
			inCodexAnswer = true
			result = append(result, line)
			continue
		}

		if inCodexAnswer && len(result) > 0 && shouldMergeCodexWrappedLine(result[len(result)-1], line) {
			result[len(result)-1] = joinCodexWrappedLine(result[len(result)-1], line)
			continue
		}

		if shouldInsertBlankAfterMarkdownList(result, line) {
			result = append(result, "")
		}
		result = append(result, line)
	}
	return result
}

func normalizeCodexShallowMarkdownListIndent(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = normalizeCodexShallowMarkdownListLine(line)
	}
	return out
}

func normalizeCodexShallowMarkdownListLine(line string) string {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent == 0 || indent > 2 || indent+1 >= len(line) {
		return line
	}
	marker := line[indent]
	if (marker == '-' || marker == '*' || marker == '+') && line[indent+1] == ' ' {
		return line[indent:]
	}
	return line
}

func emphasizeCodexHeadingLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		if isCodexHeadingLine(line, nextNonBlankLine(lines, i+1)) {
			out[i] = "**" + strings.TrimSpace(line) + "**"
			continue
		}
		out[i] = line
	}
	return out
}

func nextNonBlankLine(lines []string, start int) string {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func isCodexHeadingLine(line, next string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "**") || strings.HasSuffix(trimmed, "**") {
		return false
	}
	if next == "" {
		return false
	}
	if isMarkdownTableLine(trimmed) || isMarkdownListItemStart(trimmed) || isMarkdownListItemStart(strings.TrimSpace(next)) {
		return false
	}
	if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(strings.TrimSpace(next), "```") {
		return false
	}
	if strings.ContainsAny(trimmed, "/\\`|{}[]()") || strings.Contains(trimmed, ".go") {
		return false
	}
	if strings.ContainsAny(trimmed, "，。；：,.!！?？") {
		return false
	}
	runeCount := utf8.RuneCountInString(trimmed)
	if runeCount < 2 || runeCount > 16 {
		return false
	}
	return containsCJK(trimmed)
}

func shouldInsertBlankAfterMarkdownList(result []string, next string) bool {
	if len(result) == 0 {
		return false
	}
	nextTrimmed := strings.TrimSpace(next)
	if nextTrimmed == "" || isMarkdownListItemStart(nextTrimmed) || isMarkdownTableLine(nextTrimmed) {
		return false
	}
	if strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t") {
		return false
	}
	for i := len(result) - 1; i >= 0; i-- {
		prev := strings.TrimSpace(result[i])
		if prev == "" {
			return false
		}
		return isMarkdownListItemStart(prev)
	}
	return false
}

func shouldMergeCodexWrappedLine(prev, next string) bool {
	prev = strings.TrimRight(prev, " \t")
	nextTrimmed := strings.TrimSpace(next)
	prevTrimmed := strings.TrimSpace(prev)
	if prevTrimmed == "" || nextTrimmed == "" {
		return false
	}
	if isMarkdownTableLine(prevTrimmed) || isMarkdownTableLine(nextTrimmed) {
		return false
	}
	if isCodexReflowBulletAnchor(nextTrimmed) || isMarkdownListItemStart(nextTrimmed) {
		return false
	}
	if strings.HasPrefix(nextTrimmed, "```") || strings.HasPrefix(prevTrimmed, "```") {
		return false
	}
	if isCodexReflowCodeLikeLine(prevTrimmed) || isCodexReflowCodeLikeLine(nextTrimmed) {
		return false
	}
	if endsCodexReflowParagraph(prevTrimmed) {
		return false
	}
	return containsCJK(prevTrimmed) || containsCJK(nextTrimmed)
}

func joinCodexWrappedLine(prev, next string) string {
	prev = strings.TrimRight(prev, " \t")
	next = strings.TrimSpace(next)
	return prev + codexWrappedLineSeparator(prev, next) + next
}

func codexWrappedLineSeparator(prev, next string) string {
	last, _ := utf8.DecodeLastRuneInString(prev)
	first, _ := utf8.DecodeRuneInString(next)
	if last == utf8.RuneError || first == utf8.RuneError {
		return " "
	}
	if isNoSpaceBeforeRune(first) || isNoSpaceAfterRune(last) {
		return ""
	}
	if isCJKRune(last) && isCJKRune(first) {
		return ""
	}
	return " "
}

func isCodexReflowBulletAnchor(line string) bool {
	return strings.HasPrefix(normalizeCodexLine(line), "• ")
}

func isMarkdownListItemStart(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && (line[i] == '.' || line[i] == ')') && line[i+1] == ' '
}

func endsCodexReflowParagraph(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(line)
	return strings.ContainsRune("。！？；：.!?;:", r)
}

func isCodexReflowCodeLikeLine(line string) bool {
	line = strings.TrimSpace(normalizeCodeStartLine(line))
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") ||
		strings.HasPrefix(line, "[") || strings.HasPrefix(line, "]") ||
		strings.HasPrefix(line, `"`) ||
		strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "package ") ||
		strings.HasPrefix(line, "import ") ||
		strings.HasPrefix(line, "func ") ||
		strings.HasPrefix(line, "function ") ||
		strings.HasPrefix(line, "async function ") ||
		strings.HasPrefix(line, "const ") ||
		strings.HasPrefix(line, "let ") ||
		strings.HasPrefix(line, "var ") ||
		strings.HasPrefix(line, "class ") {
		return true
	}
	return strings.Contains(line, ":=") || strings.HasSuffix(line, "{") || strings.HasSuffix(line, ";")
}

func isCJKRune(r rune) bool {
	return (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf')
}

func isNoSpaceBeforeRune(r rune) bool {
	return strings.ContainsRune("，。！？；：、,.!?;:)]}）】》”’", r)
}

func isNoSpaceAfterRune(r rune) bool {
	return strings.ContainsRune("([{（【《“‘/，、", r)
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

func codexBulletMarkdownTableLine(line string) (string, bool) {
	trimmed := normalizeCodexLine(line)
	if !strings.HasPrefix(trimmed, "• ") {
		return "", false
	}
	candidate := strings.TrimSpace(strings.TrimPrefix(trimmed, "• "))
	if !isMarkdownTableLine(candidate) {
		return "", false
	}
	return candidate, true
}
