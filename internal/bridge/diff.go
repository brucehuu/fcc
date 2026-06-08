package bridge

import "strings"

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

// splitDiffIntoBlocks 将 diff 内容按消息类型拆分为多个块
// 连续的 Markdown 表格行作为一个块，其他行作为普通文本块
func splitDiffIntoBlocks(diff string) []string {
	lines := reflowWrappedMarkdownTableLines(strings.Split(diff, "\n"))
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
	appendCardBreak := func() {
		if len(blocks) == 0 || blocks[len(blocks)-1] == markdownCardBreak {
			return
		}
		blocks = append(blocks, markdownCardBreak)
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flushText()
			flushTable()
			appendCardBreak()
			continue
		}
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
	for len(blocks) > 0 && blocks[len(blocks)-1] == markdownCardBreak {
		blocks = blocks[:len(blocks)-1]
	}
	return blocks
}

func reflowWrappedMarkdownTableLines(lines []string) []string {
	var result []string
	var current string

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if current != "" {
			if line == "" {
				result = append(result, current, raw)
				current = ""
				continue
			}
			current += " " + line
			if strings.HasSuffix(line, "|") {
				result = append(result, current)
				current = ""
			}
			continue
		}

		if strings.HasPrefix(line, "|") && !strings.HasSuffix(line, "|") {
			current = line
			continue
		}
		result = append(result, raw)
	}

	if current != "" {
		result = append(result, current)
	}
	return result
}

func isMarkdownTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Markdown 表格行以 | 开头或结尾
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

func isMarkdownTable(block string) bool {
	lines := strings.Split(block, "\n")
	if len(lines) < 2 || !isMarkdownSeparatorLine(lines[1]) {
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

func isMarkdownTableLineBlock(block string) bool {
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !isMarkdownTableLine(line) {
			return false
		}
	}
	return true
}

func isMarkdownSeparatorLine(line string) bool {
	cells := parseTableCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		cell = strings.TrimPrefix(strings.TrimSuffix(cell, ":"), ":")
		if len(cell) < 3 {
			return false
		}
		for _, r := range cell {
			if r != '-' {
				return false
			}
		}
	}
	return true
}

// filterPane 过滤噪音，并将 Unicode 表格转换为 Markdown 表格

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
// 表格块保持 markdown 表格；其他内容直接以富文本 Markdown 发送。

func compactMarkdownSpacing(content string) string {
	lines := strings.Split(content, "\n")
	var compact []string
	blankSeen := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blankSeen || len(compact) == 0 {
				continue
			}
			blankSeen = true
			compact = append(compact, "")
			continue
		}
		blankSeen = false
		compact = append(compact, line)
	}
	for len(compact) > 0 && compact[len(compact)-1] == "" {
		compact = compact[:len(compact)-1]
	}
	return strings.Join(compact, "\n")
}

func splitMarkdownContent(content string, maxLen int) []string {
	content = compactMarkdownSpacing(content)
	if content == "" {
		return nil
	}
	if maxLen <= 0 || len(content) <= maxLen {
		return []string{content}
	}

	var chunks []string
	remaining := content
	for len(remaining) > maxLen {
		cut := strings.LastIndex(remaining[:maxLen], "\n")
		if cut <= 0 {
			cut = strings.LastIndex(remaining[:maxLen], " ")
		}
		if cut <= 0 {
			cut = maxLen
		}
		chunks = append(chunks, strings.TrimSpace(remaining[:cut]))
		remaining = strings.TrimSpace(remaining[cut:])
	}
	if remaining != "" {
		chunks = append(chunks, remaining)
	}
	return chunks
}
