package terminal

import (
	"reflect"
	"testing"
)

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
