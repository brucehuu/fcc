package bridge

import "strings"

// formatBlockToMarkdown 把单个 block 转为 markdown 格式。
// 表格块保持 markdown 表格；其他内容直接以富文本 Markdown 发送。
func formatBlockToMarkdown(block string) string {
	block = strings.TrimSpace(block)
	if block == "" || strings.Contains(block, "```") {
		return block
	}
	if prefix, rest, ok := splitBeforeCodexAssistantBullet(block); ok {
		if isOnlyCodexStaleCodePrefix(prefix) {
			return formatBlockToMarkdown(rest)
		}
		formattedRest := formatBlockToMarkdown(rest)
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			return formattedRest
		}
		return prefix + "\n\n" + formattedRest
	}
	if fenced := fenceJSONLikeBlock(block); fenced != block {
		return fenced
	}
	if fenced := fenceGoLikeBlock(block); fenced != block {
		return fenced
	}
	return fenceJavaScriptLikeBlock(block)
}

func splitBeforeCodexAssistantBullet(block string) (string, string, bool) {
	lines := strings.Split(block, "\n")
	for i := 1; i < len(lines); i++ {
		if isCodexReflowBulletAnchor(lines[i]) {
			return strings.Join(lines[:i], "\n"), strings.Join(lines[i:], "\n"), true
		}
	}
	return "", "", false
}

func isOnlyCodexStaleCodePrefix(prefix string) bool {
	lines := strings.Split(strings.TrimSpace(prefix), "\n")
	if len(lines) == 0 || len(lines) > 3 {
		return false
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !isCodexReflowCodeLikeLine(line) {
			return false
		}
	}
	return true
}

func fenceJSONLikeBlock(block string) string {
	lines := strings.Split(block, "\n")
	start := -1
	for i, line := range lines {
		if isJSONBlockStart(line) {
			start = i
			break
		}
	}
	if start < 0 || len(lines)-start < 4 {
		return block
	}

	codeLines := append([]string(nil), lines[start:]...)
	codeLines[0] = normalizeCodeStartLine(codeLines[0])
	jsonLike := 0
	for _, line := range codeLines {
		if isJSONLikeLine(line) {
			jsonLike++
		}
	}
	if jsonLike*2 < len(codeLines) {
		return block
	}

	code := strings.TrimSpace(strings.Join(codeLines, "\n"))
	if code == "" {
		return block
	}
	fenced := "```json\n" + code + "\n```"
	prefix := strings.TrimSpace(strings.Join(lines[:start], "\n"))
	if prefix == "" {
		return fenced
	}
	return prefix + "\n\n" + fenced
}

func splitStreamingJSONStart(block string) (string, []string, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	for i, line := range lines {
		if !isJSONBlockStart(line) {
			continue
		}
		codeLines := append([]string(nil), lines[i:]...)
		codeLines[0] = normalizeCodeStartLine(codeLines[0])
		return strings.TrimSpace(strings.Join(lines[:i], "\n")), codeLines, true
	}
	return "", nil, false
}

func isJSONCodeComplete(lines []string) bool {
	code := strings.TrimSpace(strings.Join(lines, "\n"))
	if code == "" {
		return false
	}
	if jsonBracketBalance(code) > 0 {
		return false
	}
	return strings.HasSuffix(code, "}") || strings.HasSuffix(code, "]")
}

func isGoCodeComplete(lines []string) bool {
	if len(lines) < 4 {
		return false
	}
	codeLike := 0
	for _, line := range lines {
		if isGoCodeLine(line) {
			codeLike++
		}
	}
	if codeLike < 4 {
		return false
	}
	return goBraceBalance(lines) <= 0
}

func fencedCodeBlock(lang string, lines []string) string {
	code := strings.TrimSpace(strings.Join(lines, "\n"))
	if code == "" {
		return ""
	}
	if lang == "" {
		return "```\n" + code + "\n```"
	}
	return "```" + lang + "\n" + code + "\n```"
}

func jsonBracketBalance(s string) int {
	balance := 0
	inString := false
	escaped := false
	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{', '[':
			balance++
		case '}', ']':
			balance--
		}
	}
	return balance
}

func goBraceBalance(lines []string) int {
	balance := 0
	inString := false
	inRawString := false
	escaped := false
	for _, line := range lines {
		for _, r := range line {
			if escaped {
				escaped = false
				continue
			}
			if inRawString {
				if r == '`' {
					inRawString = false
				}
				continue
			}
			if inString {
				if r == '\\' {
					escaped = true
				} else if r == '"' {
					inString = false
				}
				continue
			}
			switch r {
			case '`':
				inRawString = true
			case '"':
				inString = true
			case '{':
				balance++
			case '}':
				balance--
			}
		}
	}
	return balance
}

func isJSONBlockStart(line string) bool {
	trimmed := normalizeCodeStartLine(line)
	return trimmed == "{" || trimmed == "["
}

func normalizeCodeStartLine(line string) string {
	trimmed := strings.TrimSpace(line)
	for {
		next := strings.TrimSpace(strings.TrimLeft(trimmed, "🔘○●◉⦿◦•"))
		if next == trimmed {
			return trimmed
		}
		trimmed = next
	}
}

func isJSONLikeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	if trimmed == "{" || trimmed == "}" || trimmed == "[" || trimmed == "]" || trimmed == "}," || trimmed == "]," {
		return true
	}
	if strings.HasPrefix(trimmed, `"`) || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "}") ||
		strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "]") {
		return true
	}
	return strings.HasSuffix(trimmed, ",") && (strings.Contains(trimmed, `":`) || strings.Contains(trimmed, `": `))
}

func fenceGoLikeBlock(block string) string {
	lines := strings.Split(block, "\n")
	start := -1
	for i, line := range lines {
		if isGoCodeStartLine(line) {
			start = i
			break
		}
	}
	if start < 0 {
		return block
	}

	end := start
	codeLike := 0
	braceBalance := 0
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if i > start && codeLike >= 4 && braceBalance <= 0 && isLikelyProseLine(line) {
			break
		}
		if isGoCodeLine(line) {
			codeLike++
		}
		braceBalance += strings.Count(line, "{") - strings.Count(line, "}")
		end = i + 1
	}
	if end-start < 4 || codeLike < 4 {
		return block
	}

	code := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if code == "" {
		return block
	}
	fenced := "```go\n" + code + "\n```"
	prefix := strings.TrimSpace(strings.Join(lines[:start], "\n"))
	suffix := strings.TrimSpace(strings.Join(lines[end:], "\n"))
	if prefix != "" {
		fenced = prefix + "\n\n" + fenced
	}
	if suffix != "" {
		fenced += "\n\n" + suffix
	}
	return fenced
}

func fenceJavaScriptLikeBlock(block string) string {
	lines := strings.Split(block, "\n")
	start := -1
	for i, line := range lines {
		if isJavaScriptCodeStartLine(line) {
			start = i
			break
		}
	}
	if start < 0 {
		return block
	}

	end := start
	codeLike := 0
	braceBalance := 0
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if i > start && codeLike >= 4 && braceBalance <= 0 && isLikelyProseLine(trimmed) {
			break
		}
		if isJavaScriptCodeLine(line) {
			codeLike++
		}
		braceBalance += codeBraceDelta(line)
		end = i + 1
	}
	if end-start < 3 || codeLike < 3 {
		return block
	}

	codeLines := append([]string(nil), lines[start:end]...)
	codeLines[0] = normalizeCodeStartLine(codeLines[0])
	if !isJavaScriptCodeComplete(codeLines) {
		return block
	}
	code := strings.TrimSpace(strings.Join(codeLines, "\n"))
	if code == "" {
		return block
	}
	fenced := "```javascript\n" + code + "\n```"
	prefix := strings.TrimSpace(strings.Join(lines[:start], "\n"))
	suffix := strings.TrimSpace(strings.Join(lines[end:], "\n"))
	if prefix != "" {
		fenced = prefix + "\n\n" + fenced
	}
	if suffix != "" {
		fenced += "\n\n" + suffix
	}
	return fenced
}

func splitStreamingJavaScriptStart(block string) (string, []string, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	for i, line := range lines {
		if !isJavaScriptCodeStartLine(line) {
			continue
		}
		codeLines := append([]string(nil), lines[i:]...)
		codeLines[0] = normalizeCodeStartLine(codeLines[0])
		return strings.TrimSpace(strings.Join(lines[:i], "\n")), codeLines, true
	}
	return "", nil, false
}

func splitStreamingGoStart(block string) (string, []string, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	for i, line := range lines {
		if !isGoCodeStartLine(line) {
			continue
		}
		return strings.TrimSpace(strings.Join(lines[:i], "\n")), append([]string(nil), lines[i:]...), true
	}
	return "", nil, false
}

func isGoCodeStartLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "package ") ||
		strings.HasPrefix(trimmed, "import ") ||
		strings.HasPrefix(trimmed, "func ") ||
		(strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, " struct"))
}

func isGoCodeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	if isGoCodeStartLine(trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, `"`) || strings.HasPrefix(trimmed, "//") {
		return true
	}
	if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "{") || trimmed == ")" || trimmed == "(" {
		return true
	}
	if strings.Contains(trimmed, ":=") || strings.Contains(trimmed, " := ") || strings.Contains(trimmed, " = ") ||
		strings.Contains(trimmed, ".") || strings.Contains(trimmed, "(") || strings.Contains(trimmed, ")") ||
		strings.Contains(trimmed, "`") {
		return true
	}
	return false
}

func isJavaScriptCodeComplete(lines []string) bool {
	if len(lines) < 3 {
		return false
	}
	codeLike := 0
	balance := 0
	for _, line := range lines {
		if isJavaScriptCodeLine(line) {
			codeLike++
		}
		balance += codeBraceDelta(line)
	}
	return codeLike >= 3 && balance <= 0
}

func isJavaScriptCodeStartLine(line string) bool {
	trimmed := normalizeCodeStartLine(line)
	return strings.HasPrefix(trimmed, "function ") ||
		strings.HasPrefix(trimmed, "async function ") ||
		strings.HasPrefix(trimmed, "const ") ||
		strings.HasPrefix(trimmed, "let ") ||
		strings.HasPrefix(trimmed, "var ") ||
		strings.HasPrefix(trimmed, "class ") ||
		strings.HasPrefix(trimmed, "export ") ||
		strings.HasPrefix(trimmed, "import ")
}

func isJavaScriptCodeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	trimmed = normalizeCodeStartLine(trimmed)
	if isJavaScriptCodeStartLine(trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
		return true
	}
	if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, ");") || strings.HasPrefix(trimmed, "];") || strings.HasPrefix(trimmed, "};") {
		return true
	}
	if strings.Contains(trimmed, "=>") ||
		strings.Contains(trimmed, " = ") ||
		strings.Contains(trimmed, ": ") ||
		strings.Contains(trimmed, ".") ||
		strings.Contains(trimmed, "(") ||
		strings.Contains(trimmed, ")") ||
		strings.Contains(trimmed, "`") ||
		strings.HasSuffix(trimmed, ",") ||
		strings.HasSuffix(trimmed, ";") {
		return true
	}
	return false
}

func codeBraceDelta(line string) int {
	balance := 0
	inSingle := false
	inDouble := false
	inBacktick := false
	escaped := false
	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && (inSingle || inDouble || inBacktick) {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !inDouble && !inBacktick {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && !inBacktick {
				inDouble = !inDouble
			}
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
		case '{':
			if !inSingle && !inDouble && !inBacktick {
				balance++
			}
		case '}':
			if !inSingle && !inDouble && !inBacktick {
				balance--
			}
		}
	}
	return balance
}

func isLikelyProseLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if !containsCJK(trimmed) {
		return false
	}
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	return !strings.ContainsAny(trimmed, "{}();:=`\"")
}

func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf') {
			return true
		}
	}
	return false
}
