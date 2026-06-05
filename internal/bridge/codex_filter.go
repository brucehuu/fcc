package bridge

import (
	"regexp"
	"strings"
)

var (
	codexPromptRe       = regexp.MustCompile(`^[❯›]\s*$`)
	codexUserEchoRe     = regexp.MustCompile(`^[❯›]\s*`)
	codexContextLeftRe  = regexp.MustCompile(`(?i)\b\d+%\s+context\s+left\b`)
	codexModelFooterRe  = regexp.MustCompile(`(?i)^(?:gpt|o\d|codex)[\w.\- ]*\s+[·•]\s+~?/`)
	codexWorkedFooterRe = regexp.MustCompile(`(?i)^[─-]+\s*Worked for\s+(?:\d+(?:\.\d+)?[smh]\s*)+[─-]*$`)
)

var codexToolActivityPrefixes = []string{
	"• Ran ",
	"• Explored",
	"• Exploring",
	"• Read ",
	"• Search ",
	"• Searched ",
	"• Listed ",
	"• Opened ",
	"• Edited ",
	"• Applied ",
	"• Created ",
	"• Deleted ",
	"• Moved ",
	"• Updated Plan",
}

type codexPlainTableCell struct {
	text  string
	start int
}

func isCodexNoiseLine(line, userMessage string) bool {
	line = normalizeCodexLine(line)
	if line == "" {
		return true
	}
	if codexPromptRe.MatchString(line) || isCodexPromptEchoLine(line) || isCodexUserEchoLine(line, userMessage) {
		return true
	}

	lower := strings.ToLower(line)
	if strings.Contains(lower, "tab to queue message") ||
		strings.Contains(lower, "to view transcript") ||
		strings.Contains(lower, "ctrl + t") ||
		strings.HasPrefix(lower, "openai codex ") ||
		strings.HasPrefix(lower, "model:") ||
		strings.HasPrefix(lower, "directory:") ||
		strings.HasPrefix(lower, "permissions:") ||
		strings.HasPrefix(lower, "tip: try the codex app.") ||
		strings.Contains(lower, "codex app") && strings.Contains(lower, "app-landing-page") ||
		strings.HasPrefix(lower, "tip: use the openai docs mcp ") ||
		strings.Contains(lower, "codex mcp add") && strings.Contains(lower, "developers.openai.com/mcp") ||
		strings.HasPrefix(lower, "tip: type / to open the command popup") ||
		strings.HasPrefix(lower, "tip: new use /fast ") ||
		strings.Contains(lower, "starting mcp servers") ||
		strings.Contains(lower, "mcp server is not logged in") ||
		strings.Contains(lower, "mcp startup incomplete") ||
		strings.Contains(lower, "heads up") && strings.Contains(lower, "limit left") ||
		strings.Contains(lower, "you have less than") && strings.Contains(lower, "limit left") ||
		lower == "page=true" {
		return true
	}
	if codexContextLeftRe.MatchString(line) ||
		codexModelFooterRe.MatchString(line) ||
		codexWorkedFooterRe.MatchString(line) {
		return true
	}
	return false
}

func isCodexToolActivityLine(line string) bool {
	line = normalizeCodexLine(line)
	for _, prefix := range codexToolActivityPrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func isCodexAssistantTextLine(line string) bool {
	line = normalizeCodexLine(line)
	return strings.HasPrefix(line, "• ") && !isCodexToolActivityLine(line)
}

func isCodexPlainTableLine(line string) bool {
	cells := splitCodexPlainTableLine(line)
	if len(cells) < 2 {
		return false
	}
	trimmed := normalizeCodexLine(line)
	return !strings.HasPrefix(trimmed, "└") &&
		!strings.HasPrefix(trimmed, "├") &&
		!strings.HasPrefix(trimmed, "|") &&
		!strings.HasSuffix(trimmed, "|")
}

func isCodexPlainTableContinuationLine(line string) bool {
	if !hasCodexTableLeadingIndent(line) {
		return false
	}
	cells := splitCodexPlainTableCells(line)
	if len(cells) != 1 {
		return false
	}
	trimmed := normalizeCodexLine(line)
	if trimmed == "" || isCodexPromptEchoLine(trimmed) || isCodexToolActivityLine(trimmed) {
		return false
	}
	return !strings.HasPrefix(trimmed, "• ") &&
		!strings.HasPrefix(trimmed, "|") &&
		!strings.HasSuffix(trimmed, "|")
}

func hasCodexTableLeadingIndent(line string) bool {
	line = normalizeCodexTableLine(line)
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func flushCodexPlainTable(lines []string) []string {
	if len(lines) < 2 {
		return nil
	}

	rows := make([][]string, 0, len(lines))
	var colStarts []int
	for _, line := range lines {
		cells := splitCodexPlainTableCells(line)
		if isCodexPlainTableSeparatorCellSet(cells) {
			continue
		}
		if len(cells) < 2 {
			if len(cells) == 1 && colStarts != nil && len(rows) >= 2 {
				appendCodexPlainTableContinuation(rows[len(rows)-1], cells, colStarts)
				continue
			}
			return nil
		}

		if colStarts == nil {
			colStarts = make([]int, len(cells))
			for i, cell := range cells {
				colStarts[i] = cell.start
			}
			rows = append(rows, codexPlainTableCellTexts(cells, len(colStarts)))
			continue
		}

		if len(cells) == len(colStarts) {
			rows = append(rows, codexPlainTableCellTexts(cells, len(colStarts)))
			continue
		}

		if len(rows) < 2 {
			return nil
		}
		appendCodexPlainTableContinuation(rows[len(rows)-1], cells, colStarts)
	}
	if len(rows) < 2 {
		return nil
	}

	markdown := make([]string, 0, len(rows))
	for _, cells := range rows {
		markdown = append(markdown, "| "+strings.Join(cells, " | ")+" |")
	}
	return flushTable(markdown)
}

func splitCodexPlainTableLine(line string) []string {
	cells := splitCodexPlainTableCells(line)
	texts := make([]string, 0, len(cells))
	for _, cell := range cells {
		texts = append(texts, cell.text)
	}
	return texts
}

func splitCodexPlainTableCells(line string) []codexPlainTableCell {
	line = normalizeCodexTableLine(line)
	line = strings.TrimRight(line, " \t")
	if strings.HasPrefix(strings.TrimLeft(line, " \t"), "•") {
		line = strings.TrimLeft(line, " \t")
		line = strings.TrimLeft(strings.TrimPrefix(line, "•"), " \t")
	}
	if line == "" {
		return nil
	}

	var cells []codexPlainTableCell
	var cell strings.Builder
	cellStart := -1
	col := 0

	runes := []rune(line)
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == ' ' || r == '\t' {
			start := i
			width := 0
			hasTab := false
			for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
				if runes[i] == '\t' {
					hasTab = true
					width += tabAdvance(col + width)
				} else {
					width++
				}
				i++
			}
			spaceCount := i - start
			isDelimiter := hasTab || spaceCount >= 2
			if isDelimiter && cellStart >= 0 {
				text := strings.TrimSpace(cell.String())
				if text != "" {
					cells = append(cells, codexPlainTableCell{text: text, start: cellStart})
				}
				cell.Reset()
				cellStart = -1
			} else if cellStart >= 0 {
				cell.WriteString(strings.Repeat(" ", spaceCount))
			}
			col += width
			continue
		}

		if cellStart < 0 {
			cellStart = col
		}
		cell.WriteRune(r)
		col += runeDisplayWidth(r)
		i++
	}

	if cellStart >= 0 {
		text := strings.TrimSpace(cell.String())
		if text != "" {
			cells = append(cells, codexPlainTableCell{text: text, start: cellStart})
		}
	}
	return cells
}

func isCodexPlainTableSeparatorCells(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !isCodexPlainTableSeparatorCell(cell) {
			return false
		}
	}
	return true
}

func isCodexPlainTableSeparatorCellSet(cells []codexPlainTableCell) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !isCodexPlainTableSeparatorCell(cell.text) {
			return false
		}
	}
	return true
}

func isCodexPlainTableSeparatorCell(cell string) bool {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return false
	}
	for _, r := range cell {
		switch r {
		case '-', '_', '─', '━', '=':
			continue
		default:
			return false
		}
	}
	return true
}

func codexPlainTableCellTexts(cells []codexPlainTableCell, colCount int) []string {
	row := make([]string, colCount)
	for i := 0; i < len(cells) && i < colCount; i++ {
		row[i] = cells[i].text
	}
	return row
}

func appendCodexPlainTableContinuation(row []string, cells []codexPlainTableCell, colStarts []int) {
	for _, cell := range cells {
		idx := nearestCodexPlainTableColumn(cell.start, colStarts)
		row[idx] = appendCodexPlainTableCellText(row[idx], cell.text)
	}
}

func appendCodexPlainTableCellText(prev, next string) string {
	prev = strings.TrimSpace(prev)
	next = strings.TrimSpace(next)
	if prev == "" {
		return next
	}
	if next == "" {
		return prev
	}
	return prev + codexWrappedLineSeparator(prev, next) + next
}

func nearestCodexPlainTableColumn(start int, colStarts []int) int {
	best := 0
	bestDelta := absInt(start - colStarts[0])
	for i := 1; i < len(colStarts); i++ {
		delta := absInt(start - colStarts[i])
		if delta < bestDelta {
			best = i
			bestDelta = delta
		}
	}
	return best
}

func normalizeCodexTableLine(line string) string {
	line = ansiEscapeRe.ReplaceAllString(line, "")
	return strings.NewReplacer(
		"\r", "",
		"\u00a0", " ",
		"\u202f", " ",
		"\u2007", " ",
	).Replace(line)
}

func tabAdvance(col int) int {
	const tabStop = 4
	advance := tabStop - col%tabStop
	if advance == 0 {
		return tabStop
	}
	return advance
}

func runeDisplayWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x1100 &&
		(r <= 0x115f ||
			r == 0x2329 ||
			r == 0x232a ||
			(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
			(r >= 0xac00 && r <= 0xd7a3) ||
			(r >= 0xf900 && r <= 0xfaff) ||
			(r >= 0xfe10 && r <= 0xfe19) ||
			(r >= 0xfe30 && r <= 0xfe6f) ||
			(r >= 0xff00 && r <= 0xff60) ||
			(r >= 0xffe0 && r <= 0xffe6)):
		return 2
	default:
		return 1
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func isCodexUserEchoLine(line, userMessage string) bool {
	line = normalizeCodexLine(line)
	userMessage = normalizeCodexLine(userMessage)
	if line == "" || userMessage == "" || !codexUserEchoRe.MatchString(line) {
		return false
	}

	echo := codexUserEchoRe.ReplaceAllString(line, "")
	if echo == userMessage {
		return true
	}

	firstLine, _, _ := strings.Cut(userMessage, "\n")
	return echo == normalizeCodexLine(firstLine)
}

func isCodexPromptEchoLine(line string) bool {
	line = normalizeCodexLine(line)
	if !codexUserEchoRe.MatchString(line) {
		return false
	}
	echo := strings.TrimSpace(codexUserEchoRe.ReplaceAllString(line, ""))
	return echo != ""
}

func normalizeCodexLine(line string) string {
	return normalizeClaudeLine(line)
}
