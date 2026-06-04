package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppID             string
	AppSecret         string
	Command           string
	BypassPermissions bool
	CodexQueueMode    string        // "guide" or "queue"
	CaptureInterval   time.Duration // tmux 捕获间隔
	SendTimeout       time.Duration // 单条消息发送超时
	SendRetries       int           // 失败重试次数
	TMUXHistoryLines  int           // tmux 历史缓冲区行数
	LogLevel          string        // "debug" | "info" | "warn" | "error"
	NoisePatterns     []string      // 噪音过滤关键词列表
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = ".env"
	}
	if _, err := os.Stat(path); err == nil {
		if err := godotenv.Load(path); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	}
	return parseEnv()
}

// Reload 强制重新读取 .env 文件并覆盖当前进程的环境变量。
// 用于热重载场景（配置页面修改后，主进程需要读到最新值）。
func Reload(path string) (*Config, error) {
	if path == "" {
		path = ".env"
	}
	if _, err := os.Stat(path); err == nil {
		if err := godotenv.Overload(path); err != nil {
			return nil, fmt.Errorf("failed to overload env file: %w", err)
		}
	}
	return parseEnv()
}

// parseEnv 从当前进程环境变量解析配置。
func parseEnv() (*Config, error) {
	appID := os.Getenv("LARK_APP_ID")
	appSecret := os.Getenv("LARK_APP_SECRET")
	command := os.Getenv("COMMAND")
	bypass := strings.TrimSpace(os.Getenv("BYPASS_PERMISSIONS"))
	codexMode := strings.ToLower(strings.TrimSpace(os.Getenv("CODEX_QUEUE_MODE")))
	captureInterval := strings.TrimSpace(os.Getenv("CAPTURE_INTERVAL"))
	sendTimeout := strings.TrimSpace(os.Getenv("SEND_TIMEOUT"))
	sendRetries := strings.TrimSpace(os.Getenv("SEND_RETRIES"))
	tmuxHistory := strings.TrimSpace(os.Getenv("TMUX_HISTORY_LINES"))
	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))

	if appID == "" {
		return nil, fmt.Errorf("LARK_APP_ID is required")
	}
	if appSecret == "" {
		return nil, fmt.Errorf("LARK_APP_SECRET is required")
	}
	if command == "" {
		command = "bash"
	}
	if codexMode == "" {
		codexMode = "guide"
	}
	if codexMode != "guide" && codexMode != "queue" {
		return nil, fmt.Errorf("CODEX_QUEUE_MODE must be 'guide' or 'queue', got %q", codexMode)
	}
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "debug", "info", "warn", "warning", "error":
	default:
		return nil, fmt.Errorf("LOG_LEVEL must be one of debug/info/warn/warning/error, got %q", logLevel)
	}

	noisePatterns := os.Getenv("NOISE_PATTERNS")
	if noisePatterns == "" {
		noisePatterns = "fluttering,nesting,thinking"
	}
	var noisePatternsList []string
	for _, p := range strings.Split(noisePatterns, ",") {
		if p = strings.TrimSpace(p); p != "" {
			noisePatternsList = append(noisePatternsList, p)
		}
	}

	cfg := &Config{
		AppID:             appID,
		AppSecret:         appSecret,
		Command:           command,
		BypassPermissions: strings.ToLower(bypass) == "true" || bypass == "1",
		CodexQueueMode:    codexMode,
		CaptureInterval:   parseDuration(captureInterval, 3*time.Second),
		SendTimeout:       parseDuration(sendTimeout, 10*time.Second),
		SendRetries:       max(parseInt(sendRetries, 3), 1),
		TMUXHistoryLines:  parseInt(tmuxHistory, 2000),
		LogLevel:          logLevel,
		NoisePatterns:     noisePatternsList,
	}
	return cfg, nil
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return def
	}
	return d
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// UpdateAppID 更新 .env 文件中的 LARK_APP_ID 值。
func UpdateAppID(path, appID string) error {
	if path == "" {
		path = ".env"
	}
	return updateEnvVar(path, "LARK_APP_ID", appID)
}

// UpdateAppSecret 更新 .env 文件中的 LARK_APP_SECRET 值。
func UpdateAppSecret(path, appSecret string) error {
	if path == "" {
		path = ".env"
	}
	return updateEnvVar(path, "LARK_APP_SECRET", appSecret)
}

// UpdateCommand 更新 .env 文件中的 COMMAND 值。
func UpdateCommand(path, command string) error {
	if path == "" {
		path = ".env"
	}
	return updateEnvVar(path, "COMMAND", command)
}

// UpdateBypassPermissions 更新 .env 文件中的 BYPASS_PERMISSIONS 值。
func UpdateBypassPermissions(path string, bypass bool) error {
	if path == "" {
		path = ".env"
	}
	val := "false"
	if bypass {
		val = "true"
	}
	return updateEnvVar(path, "BYPASS_PERMISSIONS", val)
}

// updateEnvVar 逐行扫描 env 文件，替换或追加指定 key。
func updateEnvVar(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read env file failed: %w", err)
	}

	prefix := key + "="
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			// 保留行首缩进/注释标记（如果有）
			idx := strings.Index(line, prefix)
			lines[i] = line[:idx] + prefix + value
			found = true
			break
		}
	}
	if !found {
		if len(lines) == 1 && lines[0] == "" {
			lines[0] = prefix + value
		} else {
			lines = append(lines, prefix+value)
		}
	}

	out := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		return fmt.Errorf("write env file failed: %w", err)
	}
	return nil
}
