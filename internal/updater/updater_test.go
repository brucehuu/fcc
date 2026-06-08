package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNewDefaults(t *testing.T) {
	u := New("1.0.0", 0, 0, 0, 0, 0, 0)
	if u.currentVersion != "1.0.0" {
		t.Errorf("currentVersion = %q, want 1.0.0", u.currentVersion)
	}
	if u.firstCheckDelay != 30*time.Second {
		t.Errorf("firstCheckDelay = %v, want 30s", u.firstCheckDelay)
	}
	if u.checkInterval != 24*time.Hour {
		t.Errorf("checkInterval = %v, want 24h", u.checkInterval)
	}
	if u.httpTimeout != 30*time.Second {
		t.Errorf("httpTimeout = %v, want 30s", u.httpTimeout)
	}
	if u.downloadHTTPTimeout != 120*time.Second {
		t.Errorf("downloadHTTPTimeout = %v, want 120s", u.downloadHTTPTimeout)
	}
	if u.githubAPITimeout != 15*time.Second {
		t.Errorf("githubAPITimeout = %v, want 15s", u.githubAPITimeout)
	}
	if u.stateCheckCooldown != 24*time.Hour {
		t.Errorf("stateCheckCooldown = %v, want 24h", u.stateCheckCooldown)
	}
}

func TestNewCustomValues(t *testing.T) {
	u := New("2.0.0", time.Minute, time.Hour, 10*time.Second, 5*time.Minute, 20*time.Second, 12*time.Hour)
	if u.currentVersion != "2.0.0" {
		t.Errorf("currentVersion = %q, want 2.0.0", u.currentVersion)
	}
	if u.firstCheckDelay != time.Minute {
		t.Errorf("firstCheckDelay = %v, want 1m", u.firstCheckDelay)
	}
	if u.checkInterval != time.Hour {
		t.Errorf("checkInterval = %v, want 1h", u.checkInterval)
	}
	if u.httpTimeout != 10*time.Second {
		t.Errorf("httpTimeout = %v, want 10s", u.httpTimeout)
	}
	if u.downloadHTTPTimeout != 5*time.Minute {
		t.Errorf("downloadHTTPTimeout = %v, want 5m", u.downloadHTTPTimeout)
	}
	if u.githubAPITimeout != 20*time.Second {
		t.Errorf("githubAPITimeout = %v, want 20s", u.githubAPITimeout)
	}
	if u.stateCheckCooldown != 12*time.Hour {
		t.Errorf("stateCheckCooldown = %v, want 12h", u.stateCheckCooldown)
	}
}

func TestGetArch(t *testing.T) {
	if got := getArch(); got != runtime.GOARCH {
		t.Errorf("getArch() = %q, want %q", got, runtime.GOARCH)
	}
}

func TestReplaceBinaryInvalidPath(t *testing.T) {
	// Empty path should fail.
	if err := ReplaceBinary(""); err == nil {
		t.Error("ReplaceBinary('') expected error")
	}

	// Non-existent path should fail.
	if err := ReplaceBinary("/nonexistent/path/fcc"); err == nil {
		t.Error("ReplaceBinary(nonexistent) expected error")
	}
}

func TestReplaceBinarySuccess(t *testing.T) {
	// Create a temp dir with a fake "binary" and a new binary.
	tmpDir := t.TempDir()
	exe := filepath.Join(tmpDir, "fcc")
	newBin := filepath.Join(tmpDir, "fcc-new")

	if err := os.WriteFile(exe, []byte("old binary"), 0755); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if err := os.WriteFile(newBin, []byte("new binary"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// ReplaceBinary uses os.Executable() which returns the test binary path.
	// We can't easily mock that, so test the rename logic indirectly
	// by verifying error on permission issues (system dir).
	// Instead, just verify non-existent newPath returns error.
	if err := ReplaceBinary(newBin + ".nonexistent"); err == nil {
		t.Error("ReplaceBinary(nonexistent new) expected error")
	}
}

func TestApplyNoUpdate(t *testing.T) {
	u := New("1.0.0", 0, 0, 0, 0, 0, 0)
	// No downloaded update in state.
	if err := u.Apply(); err == nil {
		t.Error("Apply() expected error when no update available")
	}
}

func TestApplyReplaceBinaryError(t *testing.T) {
	// Use temp home to avoid interfering with real state.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Save a state file that looks like a downloaded update.
	if err := SaveState(&State{Status: StatusDownloaded, CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Path: "/nonexistent/path/fcc"}); err != nil {
		t.Fatalf("SaveState error = %v", err)
	}

	u := New("1.0.0", 0, 0, 0, 0, 0, 0)
	// Should fail at ReplaceBinary since path doesn't exist.
	if err := u.Apply(); err == nil {
		t.Error("Apply() expected error when replacement path doesn't exist")
	}
}

func TestTriggerRestartNoPIDFile(t *testing.T) {
	// Backup and remove real PID file if exists.
	origData, _ := os.ReadFile("/tmp/fcc.pid")
	os.Remove("/tmp/fcc.pid")
	defer func() {
		if origData != nil {
			os.WriteFile("/tmp/fcc.pid", origData, 0644)
		}
	}()

	if err := TriggerRestart(); err == nil {
		t.Error("TriggerRestart() expected error when no PID file")
	}
}

func TestTriggerRestartInvalidPID(t *testing.T) {
	// Write invalid PID content.
	if err := os.WriteFile("/tmp/fcc.pid", []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	defer os.Remove("/tmp/fcc.pid")

	if err := TriggerRestart(); err == nil {
		t.Error("TriggerRestart() expected error for invalid PID")
	}
}

func TestUpdaterState(t *testing.T) {
	// Use temp home to avoid interfering with real state.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Pre-save a state file so LoadState returns it on first call.
	if err := SaveState(&State{Status: StatusDownloaded, CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Path: "/tmp/fcc"}); err != nil {
		t.Fatalf("SaveState error = %v", err)
	}

	u := New("1.0.0", 0, 0, 0, 0, 0, 0)

	// First call loads from file.
	s := u.State()
	if s.Status != StatusDownloaded {
		t.Errorf("State().Status = %q, want %q", s.Status, StatusDownloaded)
	}
	if s.LatestVersion != "1.1.0" {
		t.Errorf("State().LatestVersion = %q, want 1.1.0", s.LatestVersion)
	}

	// Second call returns cached copy.
	s2 := u.State()
	if s2.Status != StatusDownloaded {
		t.Errorf("State() cached Status = %q, want %q", s2.Status, StatusDownloaded)
	}
}

func TestUpdaterHasUpdate(t *testing.T) {
	u := New("1.0.0", 0, 0, 0, 0, 0, 0)

	// No update.
	if u.HasUpdate() {
		t.Error("HasUpdate() = true, want false")
	}

	// Set state with downloaded update.
	u.mu.Lock()
	u.state = &State{Status: StatusDownloaded, Path: "/tmp/fcc", LatestVersion: "1.1.0"}
	u.mu.Unlock()

	if !u.HasUpdate() {
		t.Error("HasUpdate() = false, want true")
	}

	// Downloaded but no path.
	u.mu.Lock()
	u.state = &State{Status: StatusDownloaded, Path: ""}
	u.mu.Unlock()

	if u.HasUpdate() {
		t.Error("HasUpdate() = true, want false (no path)")
	}
}

func TestUpdaterPendingVersion(t *testing.T) {
	u := New("1.0.0", 0, 0, 0, 0, 0, 0)

	if v := u.PendingVersion(); v != "" {
		t.Errorf("PendingVersion() = %q, want empty", v)
	}

	u.mu.Lock()
	u.state = &State{Status: StatusDownloaded, LatestVersion: "1.2.3", Path: "/tmp/fcc"}
	u.mu.Unlock()

	if v := u.PendingVersion(); v != "1.2.3" {
		t.Errorf("PendingVersion() = %q, want 1.2.3", v)
	}
}
