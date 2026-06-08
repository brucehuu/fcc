package terminal

import (
	"fmt"
	"reflect"
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
