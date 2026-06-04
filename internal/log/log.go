package log

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const (
	levelDebug int32 = iota
	levelInfo
	levelWarn
	levelError
)

var currentLevel atomic.Int32

func init() {
	currentLevel.Store(levelInfo)
}

func SetLevel(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		currentLevel.Store(levelDebug)
	case "warn", "warning":
		currentLevel.Store(levelWarn)
	case "error":
		currentLevel.Store(levelError)
	default:
		currentLevel.Store(levelInfo)
	}
}

func Debug(msg string) {
	write(levelDebug, "DEBUG", msg)
}

func Debugf(format string, args ...interface{}) {
	write(levelDebug, "DEBUG", fmt.Sprintf(format, args...))
}

func Info(msg string) {
	write(levelInfo, "INFO", msg)
}

func Infof(format string, args ...interface{}) {
	write(levelInfo, "INFO", fmt.Sprintf(format, args...))
}

func Warn(msg string) {
	write(levelWarn, "WARN", msg)
}

func Warnf(format string, args ...interface{}) {
	write(levelWarn, "WARN", fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...interface{}) {
	write(levelError, "ERROR", fmt.Sprintf(format, args...))
}

func Truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func write(level int32, label, msg string) {
	if level < currentLevel.Load() {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "%s [%s] %s\n", ts, label, msg)
}
