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
	codexTableSepRe     = regexp.MustCompile(`(?:\t+| {2,})`)
)

var codexToolActivityPrefixes = []string{
	"• Ran ",
	"• Explored",
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
		strings.HasPrefix(lower, "tip: try the codex app.") ||
		strings.HasPrefix(lower, "tip: new use /fast ") ||
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

func flushCodexPlainTable(lines []string) []string {
	if len(lines) < 2 {
		return append([]string(nil), lines...)
	}

	rows := make([][]string, 0, len(lines))
	colCount := 0
	for _, line := range lines {
		cells := splitCodexPlainTableLine(line)
		if isCodexPlainTableSeparatorCells(cells) {
			continue
		}
		if len(cells) < 2 {
			return append([]string(nil), lines...)
		}
		if colCount == 0 {
			colCount = len(cells)
		} else if len(cells) != colCount {
			return append([]string(nil), lines...)
		}
		rows = append(rows, cells)
	}
	if len(rows) < 2 {
		return append([]string(nil), lines...)
	}

	markdown := make([]string, 0, len(rows))
	for _, cells := range rows {
		markdown = append(markdown, "| "+strings.Join(cells, " | ")+" |")
	}
	return flushTable(markdown)
}

func splitCodexPlainTableLine(line string) []string {
	line = normalizeCodexLine(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, "•"))
	if line == "" || !codexTableSepRe.MatchString(line) {
		return nil
	}

	rawCells := codexTableSepRe.Split(line, -1)
	cells := make([]string, 0, len(rawCells))
	for _, cell := range rawCells {
		cell = strings.TrimSpace(cell)
		if cell != "" {
			cells = append(cells, cell)
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
