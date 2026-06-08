package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_Normal(t *testing.T) {
	// 保存并恢复原始环境
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "test_app_id")
	os.Setenv("LARK_APP_SECRET", "test_secret")
	os.Setenv("COMMAND", "claude")
	os.Setenv("BYPASS_PERMISSIONS", "true")
	os.Setenv("CODEX_QUEUE_MODE", "queue")
	os.Setenv("CAPTURE_INTERVAL", "5s")
	os.Setenv("SEND_TIMEOUT", "15s")
	os.Setenv("SEND_RETRIES", "5")
	os.Setenv("TMUX_HISTORY_LINES", "3000")
	os.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}

	if cfg.AppID != "test_app_id" {
		t.Errorf("AppID = %q, want test_app_id", cfg.AppID)
	}
	if cfg.AppSecret != "test_secret" {
		t.Errorf("AppSecret = %q, want test_secret", cfg.AppSecret)
	}
	if cfg.Command != "claude" {
		t.Errorf("Command = %q, want claude", cfg.Command)
	}
	if !cfg.BypassPermissions {
		t.Error("BypassPermissions = false, want true")
	}
	if cfg.CodexQueueMode != "queue" {
		t.Errorf("CodexQueueMode = %q, want queue", cfg.CodexQueueMode)
	}
	if cfg.CaptureInterval != 5*time.Second {
		t.Errorf("CaptureInterval = %v, want 5s", cfg.CaptureInterval)
	}
	if cfg.SendTimeout != 15*time.Second {
		t.Errorf("SendTimeout = %v, want 15s", cfg.SendTimeout)
	}
	if cfg.SendRetries != 5 {
		t.Errorf("SendRetries = %d, want 5", cfg.SendRetries)
	}
	if cfg.TMUXHistoryLines != 3000 {
		t.Errorf("TMUXHistoryLines = %d, want 3000", cfg.TMUXHistoryLines)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoad_Defaults(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "test_id")
	os.Setenv("LARK_APP_SECRET", "test_secret")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}

	if cfg.Command != "bash" {
		t.Errorf("Command default = %q, want bash", cfg.Command)
	}
	if cfg.CodexQueueMode != "guide" {
		t.Errorf("CodexQueueMode default = %q, want guide", cfg.CodexQueueMode)
	}
	if cfg.CaptureInterval != 3*time.Second {
		t.Errorf("CaptureInterval default = %v, want 3s", cfg.CaptureInterval)
	}
	if cfg.SendTimeout != 10*time.Second {
		t.Errorf("SendTimeout default = %v, want 10s", cfg.SendTimeout)
	}
	if cfg.SendRetries != 3 {
		t.Errorf("SendRetries default = %d, want 3", cfg.SendRetries)
	}
	if cfg.TMUXHistoryLines != 2000 {
		t.Errorf("TMUXHistoryLines default = %d, want 2000", cfg.TMUXHistoryLines)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want info", cfg.LogLevel)
	}
}

func TestLoad_RequiredFields(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	_, err := Load("")
	if err == nil {
		t.Fatal("Load(\"\") expected error for missing required fields")
	}

	os.Setenv("LARK_APP_ID", "id")
	_, err = Load("")
	if err == nil {
		t.Fatal("Load(\"\") expected error for missing LARK_APP_SECRET")
	}
}

func TestLoad_InvalidCodexQueueMode(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")
	os.Setenv("CODEX_QUEUE_MODE", "invalid")

	_, err := Load("")
	if err == nil {
		t.Fatal("Load(\"\") expected error for invalid CODEX_QUEUE_MODE")
	}
}

func TestLoad_CaseInsensitive(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")
	os.Setenv("CODEX_QUEUE_MODE", "QUEUE")
	os.Setenv("LOG_LEVEL", "DEBUG")
	os.Setenv("BYPASS_PERMISSIONS", " TRUE ")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	if cfg.CodexQueueMode != "queue" {
		t.Errorf("CodexQueueMode = %q, want queue", cfg.CodexQueueMode)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if !cfg.BypassPermissions {
		t.Error("BypassPermissions = false, want true")
	}
}

func TestLoad_LogLevelWarning(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")
	os.Setenv("LOG_LEVEL", "warning")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	if cfg.LogLevel != "warning" {
		t.Errorf("LogLevel = %q, want warning", cfg.LogLevel)
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")
	os.Setenv("LOG_LEVEL", "verbose")

	_, err := Load("")
	if err == nil {
		t.Fatal("Load(\"\") expected error for invalid LOG_LEVEL")
	}
}

func TestLoad_SendRetriesZero(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")
	os.Setenv("SEND_RETRIES", "0")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	if cfg.SendRetries != 1 {
		t.Errorf("SendRetries = %d, want 1 (minimum)", cfg.SendRetries)
	}
}

func TestParseDuration(t *testing.T) {
	if parseDuration("", 3*time.Second) != 3*time.Second {
		t.Error("parseDuration empty should return default")
	}
	if parseDuration("5s", 3*time.Second) != 5*time.Second {
		t.Error("parseDuration valid should return parsed")
	}
	if parseDuration("invalid", 3*time.Second) != 3*time.Second {
		t.Error("parseDuration invalid should return default")
	}
	if parseDuration("0s", 3*time.Second) != 0 {
		t.Error("parseDuration zero should return 0")
	}
	if parseDuration("-1s", 3*time.Second) != 3*time.Second {
		t.Error("parseDuration negative should return default")
	}
}

func TestParseInt(t *testing.T) {
	if parseInt("", 3) != 3 {
		t.Error("parseInt empty should return default")
	}
	if parseInt("5", 3) != 5 {
		t.Error("parseInt valid should return parsed")
	}
	if parseInt("invalid", 3) != 3 {
		t.Error("parseInt invalid should return default")
	}
	if parseInt("0", 3) != 0 {
		t.Error("parseInt zero should return 0")
	}
	if parseInt("-1", 3) != 3 {
		t.Error("parseInt negative should return default")
	}
}

func TestLoad_NoisePatterns(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")
	os.Setenv("NOISE_PATTERNS", "  loading ,  ,thinking  ,  ")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	want := []string{"loading", "thinking"}
	if len(cfg.NoisePatterns) != len(want) {
		t.Fatalf("NoisePatterns = %v, want %v", cfg.NoisePatterns, want)
	}
	for i := range want {
		if cfg.NoisePatterns[i] != want[i] {
			t.Errorf("NoisePatterns[%d] = %q, want %q", i, cfg.NoisePatterns[i], want[i])
		}
	}
}

func TestUpdateEnvVars_UpdatesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	initial := strings.Join([]string{
		"LARK_APP_ID=old_id",
		"LARK_APP_SECRET=old_secret",
		"COMMAND=claude",
		"BYPASS_PERMISSIONS=false",
	}, "\n")
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	err := UpdateEnvVars(path, map[string]string{
		"COMMAND":            "bash -il",
		"BYPASS_PERMISSIONS": "true",
		"LARK_APP_SECRET":    "new_secret",
	})
	if err != nil {
		t.Fatalf("UpdateEnvVars() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"LARK_APP_ID=old_id",
		"LARK_APP_SECRET=new_secret",
		"COMMAND=bash -il",
		"BYPASS_PERMISSIONS=true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("updated env missing %q:\n%s", want, got)
		}
	}
}

func TestLoad_NoisePatternsEmptyDefaults(t *testing.T) {
	orig := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range orig {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	os.Clearenv()
	os.Setenv("LARK_APP_ID", "id")
	os.Setenv("LARK_APP_SECRET", "secret")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	want := []string{"fluttering", "nesting", "thinking"}
	if len(cfg.NoisePatterns) != len(want) {
		t.Fatalf("NoisePatterns default = %v, want %v", cfg.NoisePatterns, want)
	}
}

// splitEnv splits "KEY=VALUE" into ["KEY", "VALUE"]
func splitEnv(s string) [2]string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}

func TestUpdateEnvVar(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")

	// Create initial file.
	if err := os.WriteFile(tmpFile, []byte("KEY1=old1\nKEY2=old2\n"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Update existing key.
	if err := updateEnvVar(tmpFile, "KEY1", "new1"); err != nil {
		t.Fatalf("updateEnvVar error = %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	content := string(data)
	if !strings.Contains(content, "KEY1=new1") {
		t.Errorf("expected KEY1=new1 in file, got %q", content)
	}
	if !strings.Contains(content, "KEY2=old2") {
		t.Errorf("expected KEY2=old2 in file, got %q", content)
	}

	// Append new key.
	if err := updateEnvVar(tmpFile, "KEY3", "val3"); err != nil {
		t.Fatalf("updateEnvVar error = %v", err)
	}

	data, _ = os.ReadFile(tmpFile)
	content = string(data)
	if !strings.Contains(content, "KEY3=val3") {
		t.Errorf("expected KEY3=val3 in file, got %q", content)
	}
}

func TestUpdateEnvVars(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")

	if err := os.WriteFile(tmpFile, []byte("A=1\n"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if err := updateEnvVars(tmpFile, map[string]string{
		"A": "updated",
		"B": "new",
	}); err != nil {
		t.Fatalf("updateEnvVars error = %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	content := string(data)
	if !strings.Contains(content, "A=updated") {
		t.Errorf("expected A=updated, got %q", content)
	}
	if !strings.Contains(content, "B=new") {
		t.Errorf("expected B=new, got %q", content)
	}
}

func TestUpdateAppID(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")
	if err := UpdateAppID(tmpFile, "test-id"); err != nil {
		t.Fatalf("UpdateAppID error = %v", err)
	}
	data, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(data), "LARK_APP_ID=test-id") {
		t.Errorf("expected LARK_APP_ID=test-id, got %q", string(data))
	}
}

func TestUpdateAppSecret(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")
	if err := UpdateAppSecret(tmpFile, "test-secret"); err != nil {
		t.Fatalf("UpdateAppSecret error = %v", err)
	}
	data, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(data), "LARK_APP_SECRET=test-secret") {
		t.Errorf("expected LARK_APP_SECRET=test-secret, got %q", string(data))
	}
}

func TestUpdateCommand(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")
	if err := UpdateCommand(tmpFile, "claude"); err != nil {
		t.Fatalf("UpdateCommand error = %v", err)
	}
	data, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(data), "COMMAND=claude") {
		t.Errorf("expected COMMAND=claude, got %q", string(data))
	}
}

func TestUpdateBypassPermissions(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")
	if err := UpdateBypassPermissions(tmpFile, true); err != nil {
		t.Fatalf("UpdateBypassPermissions error = %v", err)
	}
	data, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(data), "BYPASS_PERMISSIONS=true") {
		t.Errorf("expected BYPASS_PERMISSIONS=true, got %q", string(data))
	}

	if err := UpdateBypassPermissions(tmpFile, false); err != nil {
		t.Fatalf("UpdateBypassPermissions error = %v", err)
	}
	data, _ = os.ReadFile(tmpFile)
	if !strings.Contains(string(data), "BYPASS_PERMISSIONS=false") {
		t.Errorf("expected BYPASS_PERMISSIONS=false, got %q", string(data))
	}
}

func TestReload(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), ".env")
	content := "LARK_APP_ID=test-id\nLARK_APP_SECRET=test-secret\nCOMMAND=claude\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := Reload(tmpFile)
	if err != nil {
		t.Fatalf("Reload error = %v", err)
	}
	if cfg.Command != "claude" {
		t.Errorf("Command = %q, want claude", cfg.Command)
	}
	if cfg.AppID != "test-id" {
		t.Errorf("AppID = %q, want test-id", cfg.AppID)
	}
}
