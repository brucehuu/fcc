//go:build darwin

package tray

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCommandFromToolEmptyBoth(t *testing.T) {
	if got := commandFromTool("", ""); got != "" {
		t.Errorf("commandFromTool(\"\", \"\") = %q, want empty", got)
	}
}

func TestConfigHTML(t *testing.T) {
	html := configHTML()
	if len(html) == 0 {
		t.Fatal("configHTML() returned empty string")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("configHTML() missing DOCTYPE")
	}
	if !strings.Contains(html, "fcc Configuration") {
		t.Error("configHTML() missing title")
	}
}

func TestGetTenantAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/auth/v3/tenant_access_token/internal" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["app_id"] == "" || body["app_secret"] == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 10003, "msg": "invalid params"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":                0,
			"msg":                 "ok",
			"tenant_access_token": "test-token-123",
		})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	token, err := getTenantAccessToken("app1", "secret1")
	if err != nil {
		t.Fatalf("getTenantAccessToken() error = %v", err)
	}
	if token != "test-token-123" {
		t.Errorf("token = %q, want test-token-123", token)
	}
}

func TestGetTenantAccessTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 10003, "msg": "invalid app_id"})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	_, err := getTenantAccessToken("bad", "bad")
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
	if !strings.Contains(err.Error(), "invalid app_id") {
		t.Errorf("error = %q, want to contain 'invalid app_id'", err.Error())
	}
}

func TestGetAllChats(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items":       []map[string]string{{"chat_id": "chat-1"}, {"chat_id": "chat-2"}},
					"has_more":    true,
					"page_token":  "page2",
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items":       []map[string]string{{"chat_id": "chat-3"}},
					"has_more":    false,
					"page_token":  "",
				},
			})
		}
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	chats, err := getAllChats("token")
	if err != nil {
		t.Fatalf("getAllChats() error = %v", err)
	}
	if len(chats) != 3 {
		t.Fatalf("got %d chats, want 3", len(chats))
	}
	if chats[0] != "chat-1" || chats[1] != "chat-2" || chats[2] != "chat-3" {
		t.Errorf("chats = %v, want [chat-1 chat-2 chat-3]", chats)
	}
}

func TestGetChatMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/members") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]string{
					{"member_id": "user-1", "name": "Alice"},
					{"member_id": "user-2", "name": "Bob"},
				},
				"has_more": false,
			},
		})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	members, err := getChatMembers("token", "chat-1")
	if err != nil {
		t.Fatalf("getChatMembers() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2", len(members))
	}
	if members[0].OpenID != "user-1" || members[0].Name != "Alice" {
		t.Errorf("member[0] = %+v, want user-1/Alice", members[0])
	}
}

func TestSendTestMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["receive_id"] != "user-1" {
			t.Errorf("receive_id = %q, want user-1", body["receive_id"])
		}
		if body["msg_type"] != "text" {
			t.Errorf("msg_type = %q, want text", body["msg_type"])
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "msg": "ok"})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	err := sendTestMessage("token", "user-1")
	if err != nil {
		t.Fatalf("sendTestMessage() error = %v", err)
	}
}

func TestSendTestMessageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 230001, "msg": "permission denied"})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	err := sendTestMessage("token", "user-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want to contain 'permission denied'", err.Error())
	}
}

func TestSignalFCC(t *testing.T) {
	// Write a PID file pointing to a non-existent process.
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "fcc.pid")

	// Test with no PID file (should not panic).
	signalFCC(10) // SIGUSR1

	// Test with invalid PID content.
	os.WriteFile(pidFile, []byte("not-a-number"), 0644)
	// signalFCC reads /tmp/fcc.pid, not our temp file, so this tests the
	// function doesn't panic on missing file.
	signalFCC(10)

	// Test with a PID that doesn't exist.
	os.WriteFile(pidFile, []byte("999999999"), 0644)
	signalFCC(10)
}

func TestBoolString(t *testing.T) {
	if got := boolString(true); got != "true" {
		t.Errorf("boolString(true) = %q", got)
	}
	if got := boolString(false); got != "false" {
		t.Errorf("boolString(false) = %q", got)
	}
}

func TestDetectToolEmptyCommand(t *testing.T) {
	if got := detectTool(""); got != "custom" {
		t.Errorf("detectTool(\"\") = %q, want custom", got)
	}
}

func TestDetectToolPathPrefix(t *testing.T) {
	if got := detectTool("/usr/local/bin/claude --flag"); got != "claude" {
		t.Errorf("detectTool(path/claude) = %q, want claude", got)
	}
}

func TestGetAllChatsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 99999, "msg": "auth failed"})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	_, err := getAllChats("bad-token")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetChatMembersPagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items":      []map[string]string{{"member_id": "u1", "name": "A"}},
					"has_more":   true,
					"page_token": "p2",
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items":      []map[string]string{{"member_id": "u2", "name": "B"}},
					"has_more":   false,
					"page_token": "",
				},
			})
		}
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	members, err := getChatMembers("token", "chat-1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2", len(members))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestGetTenantAccessTokenNetworkError(t *testing.T) {
	oldURL := feishuBaseURL
	feishuBaseURL = "http://127.0.0.1:1" // unreachable
	defer func() { feishuBaseURL = oldURL }()

	_, err := getTenantAccessToken("app", "secret")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestCommandFromToolWhitespaceDefault(t *testing.T) {
	// Unknown tool with only whitespace command.
	if got := commandFromTool("unknown", "   "); got != "unknown" {
		t.Errorf("commandFromTool(unknown, spaces) = %q, want unknown", got)
	}
}

func TestAiToolConfigUpdatesClaude(t *testing.T) {
	cmd, bypass, updates, err := aiToolConfigUpdates("claude", "", true)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if cmd != "claude" {
		t.Errorf("cmd = %q, want claude", cmd)
	}
	if !bypass {
		t.Error("bypass should remain true for claude")
	}
	if updates["COMMAND"] != "claude" {
		t.Errorf("COMMAND = %q", updates["COMMAND"])
	}
	if updates["BYPASS_PERMISSIONS"] != "true" {
		t.Errorf("BYPASS_PERMISSIONS = %q", updates["BYPASS_PERMISSIONS"])
	}
}

func TestKillConfigWindowNoProcess(t *testing.T) {
	// Should not panic when no config window is running.
	KillConfigWindow()
}

func TestGetChatMembersError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 99999})
	}))
	defer srv.Close()

	oldURL := feishuBaseURL
	feishuBaseURL = srv.URL
	defer func() { feishuBaseURL = oldURL }()

	_, err := getChatMembers("token", "chat-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDetectToolFlagsSkipped(t *testing.T) {
	// Commands with only flags should return "custom" since no base name matches.
	if got := detectTool("--verbose --debug"); got != "custom" {
		t.Errorf("detectTool(flags only) = %q, want custom", got)
	}
}

func TestDetectToolCodexPath(t *testing.T) {
	if got := detectTool("/usr/local/bin/codex"); got != "codex" {
		t.Errorf("detectTool(/usr/local/bin/codex) = %q, want codex", got)
	}
}

func TestDetectToolBackslashPath(t *testing.T) {
	if got := detectTool(`C:\tools\opencode`); got != "opencode" {
		t.Errorf("detectTool(backslash path) = %q, want opencode", got)
	}
}
