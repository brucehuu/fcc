package terminal

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type TmuxSession struct {
	session string
}

func NewTmuxSession(session string) *TmuxSession {
	return &TmuxSession{session: session}
}

func (t *TmuxSession) IsAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (t *TmuxSession) Start(command, workDir string) error {
	if t.HasSession() {
		return fmt.Errorf("tmux session %q already exists: kill it manually with `tmux kill-session -t %s` or re-attach with `tmux attach -t %s`", t.session, t.session, t.session)
	}
	// 启动 tmux server
	_ = exec.Command("tmux", "start-server").Run()

	args := []string{"new-session", "-d", "-s", t.session}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	// 把命令字符串拆分为多个参数，支持双引号/单引号包裹的带空格参数
	cmdParts := SplitCommand(command)
	args = append(args, cmdParts...)
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session failed: %w, output: %s", err, string(out))
	}

	// session 创建后再设置终端类型（限定到当前 session）
	_ = exec.Command("tmux", "set-option", "-t", t.session, "default-terminal", "screen-256color").Run()
	// 确保 session 内 pane 的 TERM 环境变量正确
	_ = exec.Command("tmux", "set-environment", "-t", t.session, "TERM", "screen-256color").Run()
	return nil
}

// WaitReady 阻塞等待 tmux session 中的命令完全就绪
// 至少等待 300ms 给命令进程 spawn 时间，或检测到 pane 内容发生变化
func (t *TmuxSession) WaitReady() error {
	const maxAttempts = 50
	const minWaitRounds = 3 // 至少等 300ms
	var prev string
	for i := 0; i < maxAttempts; i++ {
		out, err := exec.Command("tmux", "capture-pane", "-p", "-t", t.session).Output()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		s := string(out)
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			if i >= minWaitRounds || (i > 0 && strings.TrimSpace(prev) != trimmed) {
				return nil
			}
		}
		prev = s
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("tmux session %q not ready after 5s", t.session)
}

func (t *TmuxSession) HasSession() bool {
	cmd := exec.Command("tmux", "has-session", "-t", t.session)
	return cmd.Run() == nil
}

func (t *TmuxSession) SendKeys(text string) error {
	// 统一换行符：Windows 粘贴的 \r\n 先转成 \n
	text = trimLeadingBlankLines(text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")

	// 把多行文本用 \r\n 连接，一次 send-keys -l 发送
	// \r\n 在终端中等价于逐行按 Enter
	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\r\n")
		}
		sb.WriteString(line)
	}
	// 只在 builder 末尾没有 \r\n 时才追加，避免输入末尾已有换行时双重 Enter
	if !strings.HasSuffix(sb.String(), "\r\n") {
		sb.WriteString("\r\n")
	}

	cmd := exec.Command("tmux", "send-keys", "-t", t.session, "-l", sb.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys failed: %w, output: %s", err, string(out))
	}
	return nil
}

// SendLiteral 只发送文本本身，不追加回车。需要提交时调用方再发送 Enter 特殊键。
func (t *TmuxSession) SendLiteral(text string) error {
	text = trimLeadingBlankLines(text)
	if text == "" {
		return nil
	}
	cmd := exec.Command("tmux", "send-keys", "-t", t.session, "-l", text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys literal failed: %w, output: %s", err, string(out))
	}
	return nil
}

// SendSpecialKey 发送 tmux 特殊按键（不加 -l，解析键名）
// 支持: Escape, C-c, C-d, Enter, Space, Tab, BSpace 等
func (t *TmuxSession) SendSpecialKey(key string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.session, key)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("send-keys %s failed: %w", key, err)
	}
	return nil
}

func trimLeadingBlankLines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	start := 0
	for start < len(lines) && lines[start] == "" {
		start++
	}
	if start == len(lines) {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

// CaptureVisible 捕获 pane 内容，包含最近 historyLines 行历史
// historyLines <= 0 时只捕获当前可见区域
func (t *TmuxSession) CaptureVisible(historyLines int) (string, error) {
	args := []string{"capture-pane", "-p", "-t", t.session}
	if historyLines > 0 {
		// -S -N 表示从当前行往上 N 行开始
		args = append(args, "-S", fmt.Sprintf("-%d", historyLines))
	}
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *TmuxSession) Kill() error {
	if !t.HasSession() {
		return nil
	}
	cmd := exec.Command("tmux", "kill-session", "-t", t.session)
	return cmd.Run()
}

// SplitCommand 把命令字符串按空格拆分为参数，支持双引号/单引号包裹的带空格参数和反斜杠转义。
func SplitCommand(command string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false

	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch {
		case r == '\\':
			escaped = true
		case !inQuote && (r == '"' || r == '\''):
			inQuote = true
			quoteChar = r
			// 不 flush current，让引号内外内容拼接（如 foo"bar" -> foobar）
		case inQuote && r == quoteChar:
			inQuote = false
			// 不 flush，让引号内外内容保持连续（如 foo'bar'baz -> foobarbaz）
		case !inQuote && (r == ' ' || r == '\t'):
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		current.WriteRune('\\')
	}
	if current.Len() > 0 || inQuote {
		args = append(args, current.String())
	}

	return args
}
