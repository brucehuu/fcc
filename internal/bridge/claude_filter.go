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

// genericStatusRe 匹配通用的 ing... 状态提示。
// Claude TUI 会用各种随机的 ing 词做动态状态（Seasoning、Jitterbugging、Drizzling 等），
// 逐个维护不现实，所以用通用规则：
// - 前面有特殊符号（非字母数字）后跟 ing... 词
// - 或者包含时间标记 (Xs)、token 计数或 thought for
var genericStatusRe = regexp.MustCompile(`[^a-zA-Z0-9\s_]\s*\w+ing\.\.\.|\w+ing\.\.\..*(\(\d+[smh]\)|tokens|thought for)`)

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

	// 规则3：通用 ing... 状态提示（覆盖 Seasoning、Jitterbugging、Drizzling 等未知词）
	if genericStatusRe.MatchString(line) {
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
