package dialog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger 按小时记录飞书-Claude 对话日志。
type Logger struct {
	mu      sync.Mutex
	writer  *bufio.Writer
	file    *os.File
	curHour string
	logDir  string
}

// NewLogger 创建对话日志记录器，logDir 为日志目录（如 ./.fcc/logs）。
func NewLogger(logDir string) *Logger {
	return &Logger{logDir: logDir}
}

func (d *Logger) ensureWriter() error {
	hour := time.Now().Format("2006010215")
	if d.writer != nil && d.curHour == hour {
		return nil
	}
	if d.file != nil {
		d.writer.Flush()
		d.file.Close()
	}
	if err := os.MkdirAll(d.logDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(d.logDir, hour+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	d.file = f
	d.writer = bufio.NewWriter(f)
	d.curHour = hour
	return nil
}

// LogQuestion 记录用户从飞书发来的问题。
func (d *Logger) LogQuestion(text string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.ensureWriter(); err != nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(d.writer, "[%s] [飞书->问]\n%s\n\n", ts, text)
	d.writer.Flush()
}

// LogAnswer 记录 Claude 的回复（diff 增量内容）。
func (d *Logger) LogAnswer(text string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.ensureWriter(); err != nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(d.writer, "[%s] [Claude->答]\n%s\n\n", ts, text)
	d.writer.Flush()
}

// Close 刷新并关闭当前日志文件。
func (d *Logger) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.writer != nil {
		d.writer.Flush()
	}
	if d.file != nil {
		d.file.Close()
	}
}
