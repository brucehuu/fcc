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

func TestAIToolConfigUpdates(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		command    string
		bypass     bool
		wantCmd    string
		wantBypass string
		wantErr    bool
	}{
		{
			name:       "codex with bypass",
			tool:       "codex",
			bypass:     true,
			wantCmd:    "codex",
			wantBypass: "true",
		},
		{
			name:       "opencode disables bypass",
			tool:       "opencode",
			bypass:     true,
			wantCmd:    "opencode",
			wantBypass: "false",
		},
		{
			name:       "custom command trims and disables bypass",
			tool:       "custom",
			command:    " bash -il ",
			bypass:     true,
			wantCmd:    "bash -il",
			wantBypass: "false",
		},
		{
			name:    "empty custom command",
			tool:    "custom",
			command: "  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, bypass, updates, err := aiToolConfigUpdates(tt.tool, tt.command, tt.bypass)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("aiToolConfigUpdates() error = %v", err)
			}
			if cmd != tt.wantCmd {
				t.Fatalf("command = %q, want %q", cmd, tt.wantCmd)
			}
			if boolString(bypass) != tt.wantBypass {
				t.Fatalf("bypass = %v, want %s", bypass, tt.wantBypass)
			}
			if updates["COMMAND"] != tt.wantCmd {
				t.Fatalf("updates COMMAND = %q, want %q", updates["COMMAND"], tt.wantCmd)
			}
			if updates["BYPASS_PERMISSIONS"] != tt.wantBypass {
				t.Fatalf("updates BYPASS_PERMISSIONS = %q, want %q", updates["BYPASS_PERMISSIONS"], tt.wantBypass)
			}
			if _, ok := updates["LARK_APP_ID"]; ok {
				t.Fatal("AI tool updates should not include LARK_APP_ID")
			}
			if _, ok := updates["LARK_APP_SECRET"]; ok {
				t.Fatal("AI tool updates should not include LARK_APP_SECRET")
			}
		})
	}
}

func TestDetectToolWithExtension(t *testing.T) {
	// Windows-style path with .exe extension.
	if got := detectTool(`C:\\Program Files\\claude.exe`); got != "claude" {
		t.Errorf("detectTool(claude.exe) = %q, want claude", got)
	}
}

func TestDetectToolPathWithDot(t *testing.T) {
	// Path containing dots but not as extension.
	if got := detectTool("/opt/node.v20/bin/claude"); got != "claude" {
		t.Errorf("detectTool(...) = %q, want claude", got)
	}
}

func TestCommandFromToolDefaultWithCommand(t *testing.T) {
	// Unknown tool with non-empty command falls back to command.
	if got := commandFromTool("aider", "aider --model sonnet"); got != "aider --model sonnet" {
		t.Errorf("commandFromTool() = %q, want aider --model sonnet", got)
	}
}
