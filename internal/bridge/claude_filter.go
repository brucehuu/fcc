package bridge

import (
	"regexp"
	"strings"
)

// spinnerRunes 是 TUI loading 动画常用的盲文点阵字符和状态符号。
// Claude Code 在 Thinking/Analyzing/Seasoning 等状态时用这些字符做动态刷新。
var spinnerRunes = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏✱✢✣✤✥✦✧✻✽"

var ansiEscapeRe = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)

// statusPromptRe 匹配各种已知的状态提示开头。
// 例如：Waiting...、Running...
var statusPromptRe = regexp.MustCompile(`(?i)^(Waiting|Running)(?:\.\.\.|…)`)

// decorativeStatusRe 匹配形如 "✽ Bunning..." 的 TUI 装饰状态行。
// 特征：行首一个特殊符号 + 空格 + ing 结尾的单词 + 省略号（... 或 …）。
var decorativeStatusRe = regexp.MustCompile(`(?i)^[^\pL\pN\s_]\s+[\pL]+ing(?:\.\.\.|…)`)

// genericStatusRe 匹配带时间/token 后缀的 ing... 状态行。
// 例如：Seasoning... (1.2s)、Jitterbugging... 128 tokens、Drizzling... thought for 3s
var genericStatusRe = regexp.MustCompile(`(?i)^[\pL]+ing(?:\.\.\.|…).*(?:\(\d+(?:\.\d+)?[smh]\)|\d+\s*tokens?|thought for\s+\d+(?:\.\d+)?[smh])`)

// symbolWordForRe 匹配形如 "✻ Crunched for 26s" 的 TUI 状态行。
// 特征：行首一个特殊符号 + 空格 + 单词 + for + 时间。
var symbolWordForRe = regexp.MustCompile(`(?i)^[^\pL\pN\s_]\s+[\pL]+\s+for\s+\d+(?:\.\d+)?[smh]`)

// tipLineRe 匹配形如 "⎿  Tip: ..." 的 Claude TUI 提示行。
var tipLineRe = regexp.MustCompile(`^⎿\s+Tip:`)

// liveProgressRe 匹配 Claude TUI 的实时任务行。
// 例如："○ Explore Find filter logic 44s"。这些行只是在刷新耗时，不应发到飞书。
var liveProgressRe = regexp.MustCompile(`^[○◦◯]\s+`)

var liveProgressContinuationRe = regexp.MustCompile(`\b\d+(?:\.\d+)?[smh]\s*$`)

// toolProgressRe 匹配 Claude TUI 的工具调用进度头。
// 例如："⏺ Bash(...)"、"▣ ⎿ Bash(...)"。工具执行期间会随进度面板反复出现。
var toolProgressRe = regexp.MustCompile(`^(?:[^\pL\pN\s_]\s+)?(?:[⎿└╰]\s*)?(?:Bash|Read|Edit|Write|MultiEdit|Grep|Glob|Task|TodoWrite|WebFetch|WebSearch|LS|NotebookEdit)\(`)

// userEchoPromptRe 匹配 Claude TUI 输入回显前缀。
// 例如："❯ 你好"、"› 你好"、"> 你好"。
var userEchoPromptRe = regexp.MustCompile(`^[❯›>]\s*`)

var claudeTUIUserPromptRe = regexp.MustCompile(`^[❯›]\s*`)

var claudeAssistantTextRe = regexp.MustCompile(`^⏺\s+`)

// isClaudeDecorativeLine 判断一行是否是 Claude TUI 的纯装饰性状态行。
// 这些行在终端中会被动态刷新覆盖，通过 tmux capture 抓到后不应发送到飞书，
// 否则每次时间/token 计数变化都会产生一条无意义的飞书消息。
func isClaudeDecorativeLine(line string) bool {
	line = normalizeClaudeLine(line)
	if line == "" {
		return false
	}

	// 规则1：包含盲文 spinner 或状态符号（✱ 等）→ 几乎肯定是 TUI 状态行
	if containsSpinnerRune(line) {
		return true
	}

	// 规则2：以 Waiting... / Running... 开头（已知短状态词）
	if statusPromptRe.MatchString(line) {
		return true
	}

	// 规则3：形如 "✽ Bunning..." 的 TUI 装饰状态行
	if decorativeStatusRe.MatchString(line) {
		return true
	}

	// 规则4：带时间/token 后缀的 ing... 状态行
	if genericStatusRe.MatchString(line) {
		return true
	}

	// 规则5：形如 "✻ Crunched for 26s" 的 TUI 状态行
	if symbolWordForRe.MatchString(line) {
		return true
	}

	// 规则6：形如 "⎿  Tip: ..." 的 Claude TUI 提示行
	if tipLineRe.MatchString(line) {
		return true
	}

	// 规则7：Claude 实时任务/工具进度行
	if IsClaudeLiveProgressLine(line) || isClaudeToolProgressLine(line) {
		return true
	}

	return false
}

// IsTipLine 判断一行是否是 Claude TUI 的 Tip 提示行。
func IsTipLine(line string) bool {
	line = normalizeClaudeLine(line)
	return tipLineRe.MatchString(line)
}

func IsClaudeLiveProgressLine(line string) bool {
	line = normalizeClaudeLine(line)
	return liveProgressRe.MatchString(line)
}

func IsClaudeLiveProgressContinuation(line string) bool {
	line = normalizeClaudeLine(line)
	return liveProgressContinuationRe.MatchString(line)
}

func isClaudeToolProgressLine(line string) bool {
	line = normalizeClaudeLine(line)
	return toolProgressRe.MatchString(line)
}

func IsClaudeAssistantTextLine(line string) bool {
	line = normalizeClaudeLine(line)
	return claudeAssistantTextRe.MatchString(line) && !isClaudeToolProgressLine(line)
}

func IsClaudeUserPromptLine(line string) bool {
	line = normalizeClaudeLine(line)
	return userEchoPromptRe.MatchString(line)
}

func IsClaudeTUIUserPromptLine(line string) bool {
	line = normalizeClaudeLine(line)
	return claudeTUIUserPromptRe.MatchString(line)
}

func IsClaudeUserEchoLine(line, userMessage string) bool {
	line = normalizeClaudeLine(line)
	userMessage = normalizeClaudeLine(userMessage)
	if line == "" || userMessage == "" || !userEchoPromptRe.MatchString(line) {
		return false
	}

	echo := userEchoPromptRe.ReplaceAllString(line, "")
	if echo == userMessage {
		return true
	}

	// 多行消息只比较第一行；tmux capture 里的输入框通常只带一个提示符前缀。
	firstLine, _, _ := strings.Cut(userMessage, "\n")
	return echo == normalizeClaudeLine(firstLine)
}

// MatchClaudeUserEchoWrapped 检测被终端折行的用户输入回显。
// 当用户消息太长导致 tmux 折行时，回显会分成多行（第一行有 ❯/›/> 前缀，后续行只有前导空格）。
// 返回匹配的行数（0 表示不是用户回显）。
func MatchClaudeUserEchoWrapped(lines []string, start int, userMessage string) int {
	if start >= len(lines) {
		return 0
	}
	line := normalizeClaudeLine(lines[start])
	if !userEchoPromptRe.MatchString(line) {
		return 0
	}

	echo := userEchoPromptRe.ReplaceAllString(line, "")
	normUserMsg := normalizeWhitespace(userMessage)

	if normalizeWhitespace(echo) == normUserMsg {
		return 1
	}

	firstLine, _, _ := strings.Cut(userMessage, "\n")
	if normalizeWhitespace(echo) == normalizeWhitespace(firstLine) {
		return 1
	}

	// 折行检测：echo 是 userMessage 的前缀，尝试拼接后续行
	var builder strings.Builder
	builder.WriteString(echo)
	matched := 1
	for j := start + 1; j < len(lines) && matched < 20; j++ {
		nextRaw := lines[j]
		// 如果下一行也带有提示符，说明是新的输入，停止
		if userEchoPromptRe.MatchString(normalizeClaudeLine(nextRaw)) {
			break
		}
		next := strings.TrimSpace(nextRaw)
		if next == "" {
			break
		}
		builder.WriteString(next)
		matched++
		if normalizeWhitespace(builder.String()) == normUserMsg {
			return matched
		}
	}
	return 0
}

func normalizeWhitespace(s string) string {
	var sb strings.Builder
	inSpace := false
	for _, r := range strings.TrimSpace(s) {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			inSpace = true
			continue
		}
		if inSpace && sb.Len() > 0 {
			sb.WriteRune(' ')
		}
		sb.WriteRune(r)
		inSpace = false
	}
	return sb.String()
}

func normalizeClaudeLine(line string) string {
	line = ansiEscapeRe.ReplaceAllString(line, "")
	line = strings.NewReplacer(
		"\u00a0", " ",
		"\u202f", " ",
		"\u2007", " ",
	).Replace(line)
	return strings.TrimSpace(line)
}

func containsSpinnerRune(s string) bool {
	for _, r := range s {
		if strings.ContainsRune(spinnerRunes, r) {
			return true
		}
	}
	return false
}
