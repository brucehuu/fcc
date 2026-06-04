package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"feishu-connect/internal/log"
)

// Updater manages checking, downloading, and applying updates.
type Updater struct {
	currentVersion string
	httpClient     *http.Client
	state          *State
}

// New creates an updater for the given current version.
func New(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Start runs the background check loop.
// It checks once at startup (delayed) and then every 24 hours.
func (u *Updater) Start(ctx context.Context) {
	// Delay first check to avoid interfering with startup.
	time.Sleep(30 * time.Second)

	for {
		u.checkAndDownload(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}

// checkAndDownload performs a single check-and-download cycle.
func (u *Updater) checkAndDownload(ctx context.Context) {
	state, _ := LoadState()
	if state == nil {
		state = &State{CurrentVersion: u.currentVersion, Status: StatusIdle}
	}
	if !ShouldCheck(state) {
		log.Info("[updater] check skipped, last check within 24h")
		return
	}
	result := CheckNow(u.currentVersion)
	u.state = result
}

// CheckNow performs an immediate version check and download.
// It can be called from the config window helper or background loop.
func CheckNow(currentVersion string) *State {
	state, err := LoadState()
	if err != nil {
		state = &State{CurrentVersion: currentVersion, Status: StatusIdle}
	}
	state.CurrentVersion = currentVersion

	state.Status = StatusChecking
	state.CheckedAt = time.Now()
	_ = SaveState(state)

	log.Info("[updater] checking for updates...")
	rel, err := FetchLatest(nil)
	if err != nil {
		log.Warnf("[updater] fetch latest: %v", err)
		state.Status = StatusFailed
		state.Error = err.Error()
		_ = SaveState(state)
		return state
	}

	latest := ParseVersion(rel.TagName)
	state.LatestVersion = latest

	if !CompareVersions(currentVersion, latest) {
		log.Infof("[updater] up to date (current=%s, latest=%s)", currentVersion, latest)
		state.Status = StatusUpToDate
		_ = SaveState(state)
		return state
	}

	log.Infof("[updater] new version available: %s (current=%s)", latest, currentVersion)

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
		_ = SaveState(state)
		return state
	}

	shaURL := AssetSHA256URL(rel)

	state.Status = StatusDownloading
	_ = SaveState(state)

	binaryPath, checksum, err := Download(nil, binaryURL, shaURL, latest)
	if err != nil {
		log.Warnf("[updater] download failed: %v", err)
		state.Status = StatusFailed
		state.Error = err.Error()
		_ = SaveState(state)
		return state
	}

	state.Status = StatusDownloaded
	state.Path = binaryPath
	state.SHA256 = checksum
	state.DownloadedAt = time.Now()
	state.Error = ""
	_ = SaveState(state)

	log.Infof("[updater] downloaded %s to %s", latest, binaryPath)
	return state
}

// State returns the current update state (safe for concurrent read).
func (u *Updater) State() *State {
	if u.state == nil {
		s, _ := LoadState()
		if s == nil {
			s = &State{CurrentVersion: u.currentVersion, Status: StatusIdle}
		}
		u.state = s
	}
	return u.state
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
		// Rollback.
		_ = os.Rename(exe, newPath)
		_ = os.Rename(oldPath, exe)
		return fmt.Errorf("chmod new binary: %w", err)
	}

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
