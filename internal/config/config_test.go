package config

import (
	"os"
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
