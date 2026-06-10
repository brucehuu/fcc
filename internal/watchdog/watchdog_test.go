package watchdog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestStripEnv(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		key  string
		want []string
	}{
		{
			name: "remove existing",
			env:  []string{"FOO=bar", "WATCHDOG=1", "BAZ=qux"},
			key:  "WATCHDOG",
			want: []string{"FOO=bar", "BAZ=qux"},
		},
		{
			name: "remove non-existing",
			env:  []string{"FOO=bar", "BAZ=qux"},
			key:  "WATCHDOG",
			want: []string{"FOO=bar", "BAZ=qux"},
		},
		{
			name: "remove from empty",
			env:  []string{},
			key:  "WATCHDOG",
			want: []string{},
		},
		{
			name: "remove last",
			env:  []string{"WATCHDOG=1"},
			key:  "WATCHDOG",
			want: []string{},
		},
		{
			name: "partial match not removed",
			env:  []string{"WATCHDOG_EXTRA=1", "WATCHDOG=1"},
			key:  "WATCHDOG",
			want: []string{"WATCHDOG_EXTRA=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripEnv(tt.env, tt.key)
			if len(got) != len(tt.want) {
				t.Fatalf("stripEnv() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("stripEnv()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSetCheckInterval(t *testing.T) {
	// Save original.
	orig := checkInterval
	defer func() { checkInterval = orig }()

	SetCheckInterval(10 * time.Second)
	if checkInterval != 10*time.Second {
		t.Errorf("checkInterval = %v, want 10s", checkInterval)
	}

	// Zero should not change.
	SetCheckInterval(0)
	if checkInterval != 10*time.Second {
		t.Errorf("checkInterval = %v, want 10s (unchanged)", checkInterval)
	}
}

func TestWriteAndRemoveFCCPID(t *testing.T) {
	// Use a temp PID file to avoid interfering with real one.
	orig := fccPidFile
	fccPidFile = t.TempDir() + "/fcc.pid"
	defer func() { fccPidFile = orig }()

	// Write.
	if err := WriteFCCPID(); err != nil {
		t.Fatalf("WriteFCCPID() error = %v", err)
	}

	data, err := os.ReadFile(fccPidFile)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(data) == "" {
		t.Fatal("PID file is empty")
	}

	// Remove.
	RemoveFCCPID()
	if _, err := os.Stat(fccPidFile); !os.IsNotExist(err) {
		t.Fatal("PID file should be removed")
	}

	// Remove again should be safe (no panic).
	RemoveFCCPID()
}

func TestProcessExists(t *testing.T) {
	// Current process should exist.
	if !processExists(os.Getpid()) {
		t.Error("processExists(current pid) = false, want true")
	}

	// A very high PID is unlikely to exist.
	if processExists(999999) {
		t.Error("processExists(999999) = true, want false")
	}
}

func TestIsFCCRunningNoPIDFile(t *testing.T) {
	// Use a temp PID file.
	orig := fccPidFile
	fccPidFile = t.TempDir() + "/nonexistent.pid"
	defer func() { fccPidFile = orig }()

	if isFCCRunning() {
		t.Error("isFCCRunning() = true, want false (no PID file)")
	}
}

func TestIsFCCRunningWithValidPID(t *testing.T) {
	// Use a temp PID file with current process PID.
	orig := fccPidFile
	fccPidFile = t.TempDir() + "/fcc.pid"
	defer func() { fccPidFile = orig }()

	if err := os.WriteFile(fccPidFile, []byte("1\n"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// PID 1 (init) should always exist on Unix.
	if !isFCCRunning() {
		t.Error("isFCCRunning() = false, want true (PID 1 exists)")
	}
}

func TestKillFromPIDFile(t *testing.T) {
	// Use a temp PID file with non-existent PID.
	orig := fccPidFile
	fccPidFile = t.TempDir() + "/fcc.pid"
	defer func() { fccPidFile = orig }()

	// Non-existent file should not panic.
	killFromPIDFile(fccPidFile)

	// Invalid PID content should not panic.
	if err := os.WriteFile(fccPidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	killFromPIDFile(fccPidFile)

	// Non-existent PID should not panic.
	if err := os.WriteFile(fccPidFile, []byte("999999\n"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	killFromPIDFile(fccPidFile)
}

func TestIsWatchdogRunningNoFile(t *testing.T) {
	orig := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/nonexistent.pid"
	defer func() { watchdogPidFile = orig }()

	if isWatchdogRunning() {
		t.Error("isWatchdogRunning() = true, want false (no file)")
	}
}

func TestIsWatchdogRunningInvalidPID(t *testing.T) {
	orig := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = orig }()

	if err := os.WriteFile(watchdogPidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if isWatchdogRunning() {
		t.Error("isWatchdogRunning() = true, want false (invalid PID)")
	}
}

func TestIsFCCRunningNoFile(t *testing.T) {
	orig := fccPidFile
	fccPidFile = t.TempDir() + "/nonexistent.pid"
	defer func() { fccPidFile = orig }()

	if isFCCRunning() {
		t.Error("isFCCRunning() = true, want false (no file)")
	}
	if IsFCCRunning() {
		t.Error("IsFCCRunning() = true, want false (no file)")
	}
}

func TestStopNoPIDFile(t *testing.T) {
	orig := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/nonexistent.pid"
	defer func() { watchdogPidFile = orig }()

	// Should not panic.
	Stop()
}

func TestStopInvalidPID(t *testing.T) {
	orig := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = orig }()

	if err := os.WriteFile(watchdogPidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Should not panic.
	Stop()
}

func TestForkIfNeededAlreadyRunning(t *testing.T) {
	// Use temp PID file.
	origWatchdog := watchdogPidFile
	origLock := "/tmp/fcc-watchdog.lock"
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = origWatchdog }()

	// Write current process PID so isWatchdogRunning returns true.
	if err := os.WriteFile(watchdogPidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Should return nil immediately since watchdog appears to be running.
	if err := ForkIfNeeded(); err != nil {
		t.Errorf("ForkIfNeeded() error = %v, want nil", err)
	}

	// Clean up lock file if created.
	os.Remove(origLock)
}

func TestIsTmuxSessionRunning(t *testing.T) {
	// Just verify it doesn't panic. Result depends on whether tmux and an
	// "fcc" session are actually present on the test machine.
	_ = isTmuxSessionRunning()
}

func TestKillFromPIDFileValidPID(t *testing.T) {
	// Start a child process that sleeps.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("Cannot start child process: %v", err)
	}
	defer cmd.Process.Kill()

	// Write child PID to temp file.
	tmpFile := t.TempDir() + "/test.pid"
	if err := os.WriteFile(tmpFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Call killFromPIDFile — should not panic even if signalling fails.
	killFromPIDFile(tmpFile)

	// Process may or may not be killed depending on permissions,
	// but the function itself should have executed the full path.
}

func TestStopWithValidPID(t *testing.T) {
	orig := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = orig }()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("Cannot start child process: %v", err)
	}
	defer cmd.Process.Kill()

	if err := os.WriteFile(watchdogPidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Should execute the full Stop path without panic.
	Stop()

	// PID file should be removed.
	if _, err := os.Stat(watchdogPidFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed after Stop()")
	}
}

// --- RunWithContext tests ---

func TestRunWithContextFCCNotRunning(t *testing.T) {
	origWatchdogPid := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = origWatchdogPid }()

	origCheckInterval := checkInterval
	checkInterval = 10 * time.Millisecond
	defer func() { checkInterval = origCheckInterval }()

	origIsFCC := isFCCRunningFunc
	origIsTmux := isTmuxSessionRunningFunc
	origStartFCC := startFCCFunc
	defer func() {
		isFCCRunningFunc = origIsFCC
		isTmuxSessionRunningFunc = origIsTmux
		startFCCFunc = origStartFCC
	}()

	startCalled := false
	isFCCRunningFunc = func() bool { return false }
	isTmuxSessionRunningFunc = func() bool { return true }
	startFCCFunc = func() error {
		startCalled = true
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := RunWithContext(ctx); err != nil {
		t.Fatalf("RunWithContext() error = %v", err)
	}

	if !startCalled {
		t.Error("startFCCFunc should have been called")
	}
}

func TestRunWithContextTmuxGone(t *testing.T) {
	origWatchdogPid := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = origWatchdogPid }()

	origFCCPid := fccPidFile
	fccPidFile = t.TempDir() + "/fcc.pid"
	defer func() { fccPidFile = origFCCPid }()

	origCheckInterval := checkInterval
	checkInterval = 10 * time.Millisecond
	defer func() { checkInterval = origCheckInterval }()

	origIsFCC := isFCCRunningFunc
	origIsTmux := isTmuxSessionRunningFunc
	defer func() {
		isFCCRunningFunc = origIsFCC
		isTmuxSessionRunningFunc = origIsTmux
	}()

	isFCCRunningFunc = func() bool { return true }
	isTmuxSessionRunningFunc = func() bool { return false }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := RunWithContext(ctx); err != nil {
		t.Fatalf("RunWithContext() error = %v", err)
	}
}

func TestRunWithContextStartFCCError(t *testing.T) {
	origWatchdogPid := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = origWatchdogPid }()

	origCheckInterval := checkInterval
	checkInterval = 10 * time.Millisecond
	defer func() { checkInterval = origCheckInterval }()

	origIsFCC := isFCCRunningFunc
	origStartFCC := startFCCFunc
	defer func() {
		isFCCRunningFunc = origIsFCC
		startFCCFunc = origStartFCC
	}()

	isFCCRunningFunc = func() bool { return false }
	startFCCFunc = func() error { return fmt.Errorf("start failed") }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not return error even if startFCC fails.
	if err := RunWithContext(ctx); err != nil {
		t.Fatalf("RunWithContext() error = %v", err)
	}
}

func TestRunWithContextWritePIDError(t *testing.T) {
	// Use a non-existent directory to force write failure.
	origWatchdogPid := watchdogPidFile
	watchdogPidFile = "/nonexistent/dir/watchdog.pid"
	defer func() { watchdogPidFile = origWatchdogPid }()

	if err := RunWithContext(context.Background()); err == nil {
		t.Fatal("RunWithContext() expected error")
	}
}

// --- forkWatchdog tests ---

func TestForkWatchdogSuccess(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	if err := forkWatchdog(); err != nil {
		t.Fatalf("forkWatchdog() error = %v", err)
	}
}

func TestForkWatchdogStartError(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("/nonexistent/path/to/binary")
	}

	if err := forkWatchdog(); err == nil {
		t.Fatal("forkWatchdog() expected error")
	}
}

// --- startFCC tests ---

func TestStartFCCSuccess(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	if err := startFCC(); err != nil {
		t.Fatalf("startFCC() error = %v", err)
	}
}

func TestStartFCCTmuxKillError(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		if len(arg) > 0 && arg[0] == "kill-session" {
			return exec.Command("false")
		}
		return exec.Command("true")
	}

	if err := startFCC(); err != nil {
		t.Fatalf("startFCC() error = %v, tmux kill error should be ignored", err)
	}
}

func TestStartFCCStartError(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		if len(arg) == 0 {
			// The main fcc command
			return exec.Command("/nonexistent/path")
		}
		return exec.Command("true")
	}

	if err := startFCC(); err == nil {
		t.Fatal("startFCC() expected error")
	}
}

// --- Reset tests ---

func TestReset(t *testing.T) {
	origWatchdogPid := watchdogPidFile
	origFCCPid := fccPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	fccPidFile = t.TempDir() + "/fcc.pid"
	defer func() {
		watchdogPidFile = origWatchdogPid
		fccPidFile = origFCCPid
	}()

	// Write PID files so killFromPIDFile has something to read.
	os.WriteFile(watchdogPidFile, []byte("999999\n"), 0644)
	os.WriteFile(fccPidFile, []byte("999998\n"), 0644)

	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	// Should not panic.
	Reset()

	// PID files should be removed.
	if _, err := os.Stat(watchdogPidFile); !os.IsNotExist(err) {
		t.Error("watchdog PID file should be removed after Reset()")
	}
	if _, err := os.Stat(fccPidFile); !os.IsNotExist(err) {
		t.Error("fcc PID file should be removed after Reset()")
	}
}

// --- ForkIfNeeded tests ---

func TestForkIfNeededAcquireLock(t *testing.T) {
	// Ensure isWatchdogRunning returns false so we go into the lock path.
	origWatchdogPid := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/nonexistent.pid"
	defer func() { watchdogPidFile = origWatchdogPid }()

	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	lockFile := "/tmp/fcc-watchdog.lock"
	os.Remove(lockFile)
	defer os.Remove(lockFile)

	if err := ForkIfNeeded(); err != nil {
		t.Fatalf("ForkIfNeeded() error = %v", err)
	}
}

// --- isTmuxSessionRunning tests ---

func TestIsTmuxSessionRunningTrue(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	if !isTmuxSessionRunning() {
		t.Error("isTmuxSessionRunning() = false, want true")
	}
}

func TestIsTmuxSessionRunningFalse(t *testing.T) {
	origExec := execCommandFunc
	defer func() { execCommandFunc = origExec }()

	execCommandFunc = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("false")
	}

	if isTmuxSessionRunning() {
		t.Error("isTmuxSessionRunning() = true, want false")
	}
}

func TestStopNonExistentPID(t *testing.T) {
	orig := watchdogPidFile
	watchdogPidFile = t.TempDir() + "/watchdog.pid"
	defer func() { watchdogPidFile = orig }()

	if err := os.WriteFile(watchdogPidFile, []byte("999999\n"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Should remove PID file and return without panic.
	Stop()

	if _, err := os.Stat(watchdogPidFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed after Stop()")
	}
}
