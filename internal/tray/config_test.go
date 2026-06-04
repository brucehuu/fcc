//go:build darwin

package tray

import "testing"

func TestDetectTool(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"claude", "claude"},
		{"codex -c desktop.foo=bar", "codex"},
		{"npx -y opencode", "opencode"},
		{"bash -il", "custom"},
		{"aider --model sonnet", "custom"},
	}

	for _, tt := range tests {
		if got := detectTool(tt.command); got != tt.want {
			t.Errorf("detectTool(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func TestCommandFromTool(t *testing.T) {
	tests := []struct {
		tool    string
		command string
		want    string
	}{
		{"claude", "bash -il", "claude"},
		{"codex", "", "codex"},
		{"opencode", "", "opencode"},
		{"custom", " bash -il ", "bash -il"},
		{"", "aider", "aider"},
	}

	for _, tt := range tests {
		if got := commandFromTool(tt.tool, tt.command); got != tt.want {
			t.Errorf("commandFromTool(%q, %q) = %q, want %q", tt.tool, tt.command, got, tt.want)
		}
	}
}
