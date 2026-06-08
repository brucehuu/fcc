package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"fcc/internal/log"
)

// Updater manages checking, downloading, and applying updates.
type Updater struct {
	currentVersion      string
	httpClient          *http.Client
	state               *State
	mu                  sync.RWMutex
	firstCheckDelay     time.Duration
	checkInterval       time.Duration
	httpTimeout         time.Duration
	downloadHTTPTimeout time.Duration
	githubAPITimeout    time.Duration
	stateCheckCooldown  time.Duration
}

// New creates an updater for the given current version.
// All durations are optional; zero values fall back to sensible defaults.
func New(currentVersion string, firstCheckDelay, checkInterval, httpTimeout, downloadHTTPTimeout, githubAPITimeout, stateCheckCooldown time.Duration) *Updater {
	if httpTimeout <= 0 {
		httpTimeout = 30 * time.Second
	}
	if firstCheckDelay <= 0 {
		firstCheckDelay = 30 * time.Second
	}
	if checkInterval <= 0 {
		checkInterval = 24 * time.Hour
	}
	if downloadHTTPTimeout <= 0 {
		downloadHTTPTimeout = 120 * time.Second
	}
	if githubAPITimeout <= 0 {
		githubAPITimeout = 15 * time.Second
	}
	if stateCheckCooldown <= 0 {
		stateCheckCooldown = 24 * time.Hour
	}
	return &Updater{
		currentVersion:      currentVersion,
		httpClient:          &http.Client{Timeout: httpTimeout},
		firstCheckDelay:     firstCheckDelay,
		checkInterval:       checkInterval,
		httpTimeout:         httpTimeout,
		downloadHTTPTimeout: downloadHTTPTimeout,
		githubAPITimeout:    githubAPITimeout,
		stateCheckCooldown:  stateCheckCooldown,
	}
}

// Start runs the background check loop.
// It checks once at startup (delayed) and then every configured interval.
func (u *Updater) Start(ctx context.Context) {
	// Delay first check to avoid interfering with startup.
	time.Sleep(u.firstCheckDelay)

	for {
		u.checkAndDownload(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(u.checkInterval):
		}
	}
}

// checkAndDownload performs a single check-and-download cycle.
func (u *Updater) checkAndDownload(ctx context.Context) {
	state, _ := LoadState()
	if state == nil {
		state = &State{CurrentVersion: u.currentVersion, Status: StatusIdle}
	}
	if !ShouldCheck(state, u.stateCheckCooldown) {
		log.Info("[updater] check skipped, last check within cooldown")
		return
	}
	result := u.checkNow()
	u.mu.Lock()
	u.state = result
	u.mu.Unlock()
}

// CheckNow performs an immediate version check and download.
// It can be called from the config window helper or background loop.
// Uses default timeouts (no custom configuration).
func CheckNow(currentVersion string) *State {
	u := New(currentVersion, 0, 0, 0, 0, 0, 0)
	return u.checkNow()
}

// checkNow performs an immediate version check and download.
func (u *Updater) checkNow() *State {
	state, err := LoadState()
	if err != nil {
		state = &State{CurrentVersion: u.currentVersion, Status: StatusIdle}
	}
	state.CurrentVersion = u.currentVersion

	state.Status = StatusChecking
	state.CheckedAt = time.Now()
	if err := SaveState(state); err != nil {
		log.Warnf("[updater] save state: %v", err)
	}

	log.Info("[updater] checking for updates...")
	githubClient := &http.Client{Timeout: u.githubAPITimeout}
	rel, err := FetchLatest(githubClient)
	if err != nil {
		log.Warnf("[updater] fetch latest: %v", err)
		state.Status = StatusFailed
		state.Error = err.Error()
		if err := SaveState(state); err != nil {
			log.Warnf("[updater] save state: %v", err)
		}
		return state
	}

	latest := ParseVersion(rel.TagName)
	state.LatestVersion = latest

	if !CompareVersions(u.currentVersion, latest) {
		log.Infof("[updater] up to date (current=%s, latest=%s)", u.currentVersion, latest)
		state.Status = StatusUpToDate
		if err := SaveState(state); err != nil {
			log.Warnf("[updater] save state: %v", err)
		}
		return state
	}

	log.Infof("[updater] new version available: %s (current=%s)", latest, u.currentVersion)

	// Check if we already have this version downloaded.
	if state.Status == StatusDownloaded && state.LatestVersion == latest {
		log.Info("[updater] already downloaded, waiting for user to apply")
		return state
	}

	// Find the asset for current architecture.
	binaryURL, _ := AssetForArch(rel)
	if binaryURL == "" {
		log.Warnf("[updater] no asset found for arch %s", "darwin-"+getArch())
		state.Status = StatusFailed
		state.Error = "no asset for current architecture"
		if err := SaveState(state); err != nil {
			log.Warnf("[updater] save state: %v", err)
		}
		return state
	}

	shaURL := AssetSHA256URL(rel)

	state.Status = StatusDownloading
	if err := SaveState(state); err != nil {
		log.Warnf("[updater] save state: %v", err)
	}

	downloadClient := &http.Client{Timeout: u.downloadHTTPTimeout}
	binaryPath, checksum, err := Download(downloadClient, binaryURL, shaURL, latest)
	if err != nil {
		log.Warnf("[updater] download failed: %v", err)
		state.Status = StatusFailed
		state.Error = err.Error()
		if err := SaveState(state); err != nil {
			log.Warnf("[updater] save state: %v", err)
		}
		return state
	}

	state.Status = StatusDownloaded
	state.Path = binaryPath
	state.SHA256 = checksum
	state.DownloadedAt = time.Now()
	state.Error = ""
	if err := SaveState(state); err != nil {
		log.Warnf("[updater] save state: %v", err)
	}

	log.Infof("[updater] downloaded %s to %s", latest, binaryPath)
	return state
}

// State returns the current update state (safe for concurrent read).
func (u *Updater) State() *State {
	u.mu.RLock()
	s := u.state
	u.mu.RUnlock()
	if s != nil {
		cp := *s
		return &cp
	}
	s, _ = LoadState()
	if s == nil {
		s = &State{CurrentVersion: u.currentVersion, Status: StatusIdle}
	}
	u.mu.Lock()
	u.state = s
	u.mu.Unlock()
	cp := *s
	return &cp
}

// HasUpdate returns true if a downloaded update is waiting to be applied.
func (u *Updater) HasUpdate() bool {
	s := u.State()
	return s.Status == StatusDownloaded && s.Path != ""
}

// PendingVersion returns the version waiting to be applied.
func (u *Updater) PendingVersion() string {
	s := u.State()
	if s.Status == StatusDownloaded {
		return s.LatestVersion
	}
	return ""
}

// Apply performs the self-update: replaces the running binary and exits.
// Callers should ensure the process can be restarted (e.g. by watchdog).
func (u *Updater) Apply() error {
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if state.Status != StatusDownloaded || state.Path == "" {
		return fmt.Errorf("no update available")
	}
	if err := ReplaceBinary(state.Path); err != nil {
		return err
	}

	// Clean up state file so the new version starts fresh.
	_ = os.Remove(statePath())

	log.Infof("[updater] applied update %s, exiting for restart", state.LatestVersion)

	// Exit cleanly; watchdog will restart us.
	os.Exit(0)
	return nil // unreachable
}

// ReplaceBinary replaces the current executable with the one at newPath.
// It is safe to call from a helper process (e.g. --config-window).
func ReplaceBinary(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	// Resolve symlinks to get the real binary path.
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}

	oldPath := exe + ".old"

	// Remove any existing .old file.
	_ = os.Remove(oldPath)

	// Move current binary to .old
	if err := os.Rename(exe, oldPath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: fcc is installed in a system directory. Please run fcc with sudo or install to a user directory (e.g. ~/.local/bin) to enable auto-updates")
		}
		return fmt.Errorf("rename old binary: %w", err)
	}

	// Move new binary into place.
	if err := os.Rename(newPath, exe); err != nil {
		// Rollback: restore old binary.
		_ = os.Rename(oldPath, exe)
		return fmt.Errorf("rename new binary: %w", err)
	}

	if err := os.Chmod(exe, 0755); err != nil {
		// Rollback: copy old binary back to exe (more reliable than rename across filesystems).
		if data, readErr := os.ReadFile(oldPath); readErr == nil {
			_ = os.WriteFile(exe, data, 0755)
		}
		_ = os.Remove(oldPath)
		_ = os.Remove(newPath)
		return fmt.Errorf("chmod new binary: %w", err)
	}

	// 替换成功，清理旧版本文件
	_ = os.Remove(oldPath)
	return nil
}

// TriggerRestart sends SIGTERM to the fcc main process to trigger a restart.
// This is used by the config window helper to apply an update.
func TriggerRestart() error {
	data, err := os.ReadFile("/tmp/fcc.pid")
	if err != nil {
		return fmt.Errorf("read pid file: %w", err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("parse pid: %w", err)
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func getArch() string {
	return runtime.GOARCH
}
