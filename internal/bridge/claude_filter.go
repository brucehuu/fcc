package bridge

import (
	"regexp"
	"strings"
)

// spinnerRunes 是 TUI loading 动画常用的盲文点阵字符和状态符号。
// Claude Code 在 Thinking/Analyzing/Seasoning 等状态时用这些字符做动态刷新。
var spinnerRunes = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏✱"

// statusPromptRe 匹配各种已知的状态提示开头。
// 例如：Waiting...、Running...
var statusPromptRe = regexp.MustCompile(`^(Waiting|Running)\.\.\.`)

// decorativeStatusRe 匹配形如 "✽ Bunning..." 的 TUI 装饰状态行。
// 特征：行首一个特殊符号 + 空格 + ing 结尾的单词 + 省略号（... 或 …）。
var decorativeStatusRe = regexp.MustCompile(`^[^a-zA-Z0-9\s_]\s+\w+ing(?:\.\.\.|…)`)

// genericStatusRe 匹配带时间/token 后缀的 ing... 状态行。
// 例如：Seasoning... (1.2s)、Jitterbugging... 128 tokens、Drizzling... thought for 3s
var genericStatusRe = regexp.MustCompile(`^\w+ing(?:\.\.\.|…).*(?:\(\d+[smh]\)|tokens|thought for)`)

// symbolWordForRe 匹配形如 "✻ Crunched for 26s" 的 TUI 状态行。
// 特征：行首一个特殊符号 + 空格 + 单词 + for + 时间。
var symbolWordForRe = regexp.MustCompile(`^[^a-zA-Z0-9\s_]\s+\w+\s+for\s+\d+[smh]`)

// isClaudeDecorativeLine 判断一行是否是 Claude TUI 的纯装饰性状态行。
// 这些行在终端中会被动态刷新覆盖，通过 tmux capture 抓到后不应发送到飞书，
// 否则每次时间/token 计数变化都会产生一条无意义的飞书消息。
func isClaudeDecorativeLine(line string) bool {
	line = strings.TrimSpace(line)
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

	return false
}

func containsSpinnerRune(s string) bool {
	for _, r := range s {
		if strings.ContainsRune(spinnerRunes, r) {
			return true
		}
	}
	return false
}
