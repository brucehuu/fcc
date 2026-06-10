package terminal

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTrimLeadingBlankLines(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\nworld", "hello\nworld"},
		{"\n\nhello", "hello"},
		{"\r\n\r\nhello", "hello"},
		{"\n\n", ""},
		{"", ""},
		{"hello", "hello"},
		{" \nhello", " \nhello"}, // space-only line is NOT blank
	}
	for _, tt := range tests {
		got := trimLeadingBlankLines(tt.input)
		if got != tt.want {
			t.Errorf("trimLeadingBlankLines(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewTmuxSession(t *testing.T) {
	s := NewTmuxSession("test-session")
	if s == nil {
		t.Fatal("NewTmuxSession returned nil")
	}
	if s.session != "test-session" {
		t.Errorf("session = %q, want test-session", s.session)
	}
}

func TestSendKeysEmpty(t *testing.T) {
	s := NewTmuxSession("test-session")
	if err := s.SendKeys(""); err != nil {
		t.Errorf("SendKeys('') error = %v", err)
	}
	if err := s.SendKeys("\n\n"); err != nil {
		t.Errorf("SendKeys(empty lines) error = %v", err)
	}
}

func TestSendLiteralEmpty(t *testing.T) {
	s := NewTmuxSession("test-session")
	if err := s.SendLiteral(""); err != nil {
		t.Errorf("SendLiteral('') error = %v", err)
	}
}

func TestSendSpecialKey(t *testing.T) {
	s := NewTmuxSession("nonexistent-session")
	// Will fail since session doesn't exist, but covers the code path.
	err := s.SendSpecialKey("Escape")
	if err == nil {
		t.Error("SendSpecialKey expected error for nonexistent session")
	}
}

func TestSendLiteralNonEmpty(t *testing.T) {
	s := NewTmuxSession("nonexistent-session")
	// Will fail since session doesn't exist, but covers the code path.
	err := s.SendLiteral("hello world")
	if err == nil {
		t.Error("SendLiteral expected error for nonexistent session")
	}
}

func TestSendKeysNonEmpty(t *testing.T) {
	s := NewTmuxSession("nonexistent-session")
	// Will fail since session doesn't exist, but covers the code path.
	err := s.SendKeys("hello world")
	if err == nil {
		t.Error("SendKeys expected error for nonexistent session")
	}
}

func TestIsAvailable(t *testing.T) {
	s := NewTmuxSession("test")
	// tmux may or may not be installed; just verify it doesn't panic.
	_ = s.IsAvailable()
}

func TestHasSession(t *testing.T) {
	s := NewTmuxSession("nonexistent-session-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	if s.HasSession() {
		t.Error("HasSession() = true for nonexistent session")
	}
}

func TestKillNoSession(t *testing.T) {
	s := NewTmuxSession("nonexistent-session-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := s.Kill(); err != nil {
		t.Errorf("Kill() error = %v", err)
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{"simple", "claude", []string{"claude"}},
		{"with args", "claude --skip-permissions", []string{"claude", "--skip-permissions"}},
		{"double quoted", "claude \"foo bar\"", []string{"claude", "foo bar"}},
		{"single quoted", "claude 'foo bar'", []string{"claude", "foo bar"}},
		{"quote concatenation", "foo\"bar\"", []string{"foobar"}},
		{"mixed quotes", "foo'bar'baz", []string{"foobarbaz"}},
		{"escaped space", "claude foo\\ bar", []string{"claude", "foo bar"}},
		{"npx style", "npx -y codex", []string{"npx", "-y", "codex"}},
		{"path", "/usr/local/bin/claude", []string{"/usr/local/bin/claude"}},
		{"empty", "", nil},
		{"only spaces", "   ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitCommand(tt.command)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

// tmuxAvailable checks if tmux is installed and the server is reachable.
func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestTmuxIntegration(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := fmt.Sprintf("fcc-test-%d", time.Now().UnixNano())
	s := NewTmuxSession(sessionName)

	// Cleanup: ensure session is killed even if test fails.
	defer func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}()

	// Start a session running bash.
	err := s.Start("bash", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify session exists.
	if !s.HasSession() {
		t.Fatal("HasSession() = false after Start()")
	}

	// WaitReady should succeed for an active session.
	if err := s.WaitReady(); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}

	// CaptureVisible should return non-empty content.
	content, err := s.CaptureVisible(0)
	if err != nil {
		t.Fatalf("CaptureVisible(0) error = %v", err)
	}
	if len(strings.TrimSpace(content)) == 0 {
		t.Error("CaptureVisible(0) returned empty content")
	}

	// CaptureVisible with history should also work.
	contentHist, err := s.CaptureVisible(100)
	if err != nil {
		t.Fatalf("CaptureVisible(100) error = %v", err)
	}
	if len(contentHist) == 0 {
		t.Error("CaptureVisible(100) returned empty content")
	}

	// SendKeys should work on a live session.
	if err := s.SendKeys("echo hello-fcc-test"); err != nil {
		t.Fatalf("SendKeys() error = %v", err)
	}

	// Give the command time to execute.
	time.Sleep(500 * time.Millisecond)

	// Capture should now contain our echo output.
	content, err = s.CaptureVisible(0)
	if err != nil {
		t.Fatalf("CaptureVisible() after SendKeys error = %v", err)
	}
	if !strings.Contains(content, "hello-fcc-test") {
		t.Errorf("CaptureVisible() = %q, want to contain 'hello-fcc-test'", content)
	}

	// SendLiteral without Enter - should type but not execute.
	if err := s.SendLiteral("# literal-text"); err != nil {
		t.Fatalf("SendLiteral() error = %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// SendSpecialKey: send Ctrl-C to clear the line.
	if err := s.SendSpecialKey("C-c"); err != nil {
		t.Fatalf("SendSpecialKey(C-c) error = %v", err)
	}

	// Kill the session.
	if err := s.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	// Session should no longer exist.
	if s.HasSession() {
		t.Error("HasSession() = true after Kill()")
	}
}

func TestTmuxStartDuplicate(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := fmt.Sprintf("fcc-test-dup-%d", time.Now().UnixNano())
	s := NewTmuxSession(sessionName)

	defer func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}()

	if err := s.Start("bash", ""); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	// Second Start should fail since session already exists.
	err := s.Start("bash", "")
	if err == nil {
		t.Fatal("second Start() should fail for existing session")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

func TestTmuxStartWithWorkDir(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := fmt.Sprintf("fcc-test-wd-%d", time.Now().UnixNano())
	s := NewTmuxSession(sessionName)

	defer func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}()

	workDir := "/tmp"
	if err := s.Start("bash", workDir); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for session to be ready before sending commands.
	if err := s.WaitReady(); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}

	// Send pwd command and check output.
	if err := s.SendKeys("pwd"); err != nil {
		t.Fatalf("SendKeys(pwd) error = %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	content, err := s.CaptureVisible(0)
	if err != nil {
		t.Fatalf("CaptureVisible() error = %v", err)
	}
	// /tmp on macOS may resolve to /private/tmp
	if !strings.Contains(content, "tmp") {
		t.Errorf("CaptureVisible() = %q, want to contain 'tmp'", content)
	}
}

func TestKillExistingSession(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := fmt.Sprintf("fcc-test-kill-%d", time.Now().UnixNano())
	s := NewTmuxSession(sessionName)

	defer func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}()

	if err := s.Start("bash", ""); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := s.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	// Kill again should be a no-op (no error).
	if err := s.Kill(); err != nil {
		t.Errorf("Kill() on already killed session error = %v", err)
	}
}
