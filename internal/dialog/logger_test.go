package dialog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerBasicWrite(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)

	l.LogQuestion("hello world")
	l.Close()

	hour := time.Now().Format("2006010215")
	logFile := filepath.Join(dir, hour+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[飞书->问]") {
		t.Errorf("log missing question marker, got: %s", content)
	}
	if !strings.Contains(content, "hello world") {
		t.Errorf("log missing question text, got: %s", content)
	}
}

func TestLoggerAnswerWrite(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)

	l.LogAnswer("some response")
	l.Close()

	hour := time.Now().Format("2006010215")
	logFile := filepath.Join(dir, hour+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[Claude->答]") {
		t.Errorf("log missing answer marker, got: %s", content)
	}
	if !strings.Contains(content, "some response") {
		t.Errorf("log missing answer text, got: %s", content)
	}
}

func TestLoggerMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)

	l.LogQuestion("q1")
	l.LogAnswer("a1")
	l.LogQuestion("q2")
	l.Close()

	hour := time.Now().Format("2006010215")
	logFile := filepath.Join(dir, hour+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	count := strings.Count(content, "[飞书->问]")
	if count != 2 {
		t.Errorf("expected 2 questions, got %d", count)
	}
	count = strings.Count(content, "[Claude->答]")
	if count != 1 {
		t.Errorf("expected 1 answer, got %d", count)
	}
}

func TestLoggerHourRollover(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)

	// Set current hour to a fake past hour to force rollover.
	l.curHour = "1999010100"

	l.LogQuestion("after rollover")
	l.Close()

	// Should create a file for the current hour, not 1999010100.
	hour := time.Now().Format("2006010215")
	logFile := filepath.Join(dir, hour+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "after rollover") {
		t.Errorf("log missing text after rollover")
	}
}

func TestLoggerCloseWithoutWrite(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)
	l.Close()

	// No file should be created.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files, got %d", len(entries))
	}
}

func TestLoggerCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)
	l.LogQuestion("test")
	l.Close()
	// Second close should not panic.
	l.Close()
}
