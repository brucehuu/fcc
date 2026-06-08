package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	TargetName        string
	CodexQueueMode    string        // "guide" or "queue"
	CaptureInterval   time.Duration // tmux 捕获间隔
	SendTimeout       time.Duration // 单条消息发送超时
	SendRetries       int           // 失败重试次数
	TMUXHistoryLines  int           // tmux 历史缓冲区行数
	LogLevel          string        // "debug" | "info" | "warn" | "error"
	NoisePatterns     []string      // 噪音过滤关键词列表

	// Bridge 行为调优（全部可选，带默认值）
	CaptureIntervalMin           time.Duration // 默认 500ms
	CaptureIntervalMax           time.Duration // 默认 60s
	SendTimeoutMin               time.Duration // 默认 1s
	SendTimeoutMax               time.Duration // 默认 120s
	InterruptDebounce            time.Duration // 默认 500ms
	AdaptiveCaptureMin           time.Duration // 默认 1s
	AdaptiveCaptureMax           time.Duration // 默认 5s
	AdaptiveCaptureIdleThreshold int           // 默认 3
	PendingTableIdleWait         time.Duration // 默认 12s
	PendingCodeIdleWait          time.Duration // 默认 5s
	MaxMarkdownLen               int           // 默认 3000
	WelcomeDelay                 time.Duration // 默认 3s
	WelcomeTimeout               time.Duration // 默认 30s
	ImageCleanupMaxAge           time.Duration // 默认 7*24h
	ImageCleanupInterval         time.Duration // 默认 24h
	CodexInputDelay              time.Duration // 默认 150ms

	// Bot 重试调优
	BotRetryBackoff    time.Duration // 默认 500ms
	BotRetryMaxBackoff time.Duration // 默认 30s

	// Watchdog
	WatchdogCheckInterval time.Duration // 默认 6s

	// Updater
	UpdaterFirstCheckDelay time.Duration // 默认 30s
	UpdaterCheckInterval   time.Duration // 默认 24h
	UpdaterHTTPTimeout     time.Duration // 默认 30s
	DownloadHTTPTimeout    time.Duration // 默认 120s
	GithubAPITimeout       time.Duration // 默认 15s

	// Main
	ShutdownTimeout time.Duration // 默认 5s
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
	targetName := strings.TrimSpace(os.Getenv("TARGET_NAME"))
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
		TargetName:        targetName,
		CodexQueueMode:    codexMode,
		CaptureInterval:   parseDuration(captureInterval, 3*time.Second),
		SendTimeout:       parseDuration(sendTimeout, 10*time.Second),
		SendRetries:       max(parseInt(sendRetries, 3), 1),
		TMUXHistoryLines:  parseInt(tmuxHistory, 2000),
		LogLevel:          logLevel,
		NoisePatterns:     noisePatternsList,

		// Bridge 调优（全部从环境变量读取，空则使用默认值）
		CaptureIntervalMin:           parseDuration(os.Getenv("CAPTURE_INTERVAL_MIN"), 500*time.Millisecond),
		CaptureIntervalMax:           parseDuration(os.Getenv("CAPTURE_INTERVAL_MAX"), 60*time.Second),
		SendTimeoutMin:               parseDuration(os.Getenv("SEND_TIMEOUT_MIN"), 1*time.Second),
		SendTimeoutMax:               parseDuration(os.Getenv("SEND_TIMEOUT_MAX"), 120*time.Second),
		InterruptDebounce:            parseDuration(os.Getenv("INTERRUPT_DEBOUNCE"), 500*time.Millisecond),
		AdaptiveCaptureMin:           parseDuration(os.Getenv("ADAPTIVE_CAPTURE_MIN"), 1*time.Second),
		AdaptiveCaptureMax:           parseDuration(os.Getenv("ADAPTIVE_CAPTURE_MAX"), 5*time.Second),
		AdaptiveCaptureIdleThreshold: parseInt(os.Getenv("ADAPTIVE_CAPTURE_IDLE_THRESHOLD"), 3),
		PendingTableIdleWait:         parseDuration(os.Getenv("PENDING_TABLE_IDLE_WAIT"), 12*time.Second),
		PendingCodeIdleWait:          parseDuration(os.Getenv("PENDING_CODE_IDLE_WAIT"), 5*time.Second),
		MaxMarkdownLen:               parseInt(os.Getenv("MAX_MARKDOWN_LEN"), 3000),
		WelcomeDelay:                 parseDuration(os.Getenv("WELCOME_DELAY"), 3*time.Second),
		WelcomeTimeout:               parseDuration(os.Getenv("WELCOME_TIMEOUT"), 30*time.Second),
		ImageCleanupMaxAge:           parseDuration(os.Getenv("IMAGE_CLEANUP_MAX_AGE"), 7*24*time.Hour),
		ImageCleanupInterval:         parseDuration(os.Getenv("IMAGE_CLEANUP_INTERVAL"), 24*time.Hour),
		CodexInputDelay:              parseDuration(os.Getenv("CODEX_INPUT_DELAY"), 150*time.Millisecond),

		// Bot 重试
		BotRetryBackoff:    parseDuration(os.Getenv("BOT_RETRY_BACKOFF"), 500*time.Millisecond),
		BotRetryMaxBackoff: parseDuration(os.Getenv("BOT_RETRY_MAX_BACKOFF"), 30*time.Second),

		// Watchdog
		WatchdogCheckInterval: parseDuration(os.Getenv("WATCHDOG_CHECK_INTERVAL"), 6*time.Second),

		// Updater
		UpdaterFirstCheckDelay: parseDuration(os.Getenv("UPDATER_FIRST_CHECK_DELAY"), 30*time.Second),
		UpdaterCheckInterval:   parseDuration(os.Getenv("UPDATER_CHECK_INTERVAL"), 24*time.Hour),
		UpdaterHTTPTimeout:     parseDuration(os.Getenv("UPDATER_HTTP_TIMEOUT"), 30*time.Second),
		DownloadHTTPTimeout:    parseDuration(os.Getenv("DOWNLOAD_HTTP_TIMEOUT"), 120*time.Second),
		GithubAPITimeout:       parseDuration(os.Getenv("GITHUB_API_TIMEOUT"), 15*time.Second),

		// Main
		ShutdownTimeout: parseDuration(os.Getenv("SHUTDOWN_TIMEOUT"), 5*time.Second),
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

// UpdateEnvVars atomically updates several .env keys in one write.
func UpdateEnvVars(path string, updates map[string]string) error {
	if path == "" {
		path = ".env"
	}
	return updateEnvVars(path, updates)
}

// updateEnvVar scans the env file line by line, replacing or appending one key.
func updateEnvVar(path, key, value string) error {
	return updateEnvVars(path, map[string]string{key: value})
}

func updateEnvVars(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read env file failed: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	remaining := make(map[string]string, len(updates))
	for k, v := range updates {
		remaining[k] = v
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 跳过注释行，避免误改
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		for key, value := range remaining {
			prefix := key + "="
			if strings.HasPrefix(trimmed, prefix) {
				idx := strings.Index(line, prefix)
				lines[i] = line[:idx] + prefix + value
				delete(remaining, key)
				break
			}
		}
	}

	for key, value := range remaining {
		line := key + "=" + value
		if len(lines) == 1 && lines[0] == "" {
			lines[0] = line
			continue
		}
		lines = append(lines, line)
	}

	out := strings.Join(lines, "\n")
	if err := writeFileAtomic(path, []byte(out), 0600); err != nil {
		return fmt.Errorf("write env file failed: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
