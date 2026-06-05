package log

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureOutput(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestSetLevel(t *testing.T) {
	// Test debug level shows debug messages
	SetLevel("debug")
	output := captureOutput(func() {
		Debug("debug msg")
	})
	if !strings.Contains(output, "debug msg") {
		t.Errorf("debug level should show debug messages, got: %q", output)
	}

	// Test info level filters debug messages
	SetLevel("info")
	output = captureOutput(func() {
		Debug("debug msg")
	})
	if strings.Contains(output, "debug msg") {
		t.Errorf("info level should filter debug messages, got: %q", output)
	}

	// Test warn level filters info messages
	SetLevel("warn")
	output = captureOutput(func() {
		Info("info msg")
	})
	if strings.Contains(output, "info msg") {
		t.Errorf("warn level should filter info messages, got: %q", output)
	}

	// Test error level filters warn messages
	SetLevel("error")
	output = captureOutput(func() {
		Warn("warn msg")
	})
	if strings.Contains(output, "warn msg") {
		t.Errorf("error level should filter warn messages, got: %q", output)
	}

	// Test error level shows error messages
	output = captureOutput(func() {
		Errorf("error msg")
	})
	if !strings.Contains(output, "error msg") {
		t.Errorf("error level should show error messages, got: %q", output)
	}
}

func TestTruncate(t *testing.T) {
	if Truncate("hello", 10) != "hello" {
		t.Errorf("Truncate short string should return as-is")
	}
	truncated := Truncate("hello world", 5)
	if truncated != "he..." {
		t.Errorf("Truncate long string = %q, want he...", truncated)
	}
	if Truncate("hello world", 3) != "hel" {
		t.Errorf("Truncate with max=3 = %q, want hel", Truncate("hello world", 3))
	}
}
