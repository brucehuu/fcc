package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const stateFileName = "update-state.json"

// State tracks the update lifecycle.
type State struct {
	CurrentVersion string    `json:"current"`
	LatestVersion  string    `json:"latest"`
	Status         string    `json:"status"`
	Path           string    `json:"path,omitempty"`
	SHA256         string    `json:"sha256,omitempty"`
	CheckedAt      time.Time `json:"checkedAt,omitempty"`
	DownloadedAt   time.Time `json:"downloadedAt,omitempty"`
	Error          string    `json:"error,omitempty"`
}

const (
	StatusIdle        = "idle"
	StatusChecking    = "checking"
	StatusUpToDate    = "uptodate"
	StatusDownloading = "downloading"
	StatusDownloaded  = "downloaded"
	StatusFailed      = "failed"
)

func fccDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".fcc")
}

func statePath() string {
	return filepath.Join(fccDir(), stateFileName)
}

// LoadState reads the persisted update state.
func LoadState() (*State, error) {
	data, err := os.ReadFile(statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Status: StatusIdle}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveState persists the update state.
func SaveState(s *State) error {
	dir := fccDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), data, 0644)
}

// ShouldCheck returns true if the last check was more than 24 hours ago.
func ShouldCheck(s *State) bool {
	if s.CheckedAt.IsZero() {
		return true
	}
	return time.Since(s.CheckedAt) > 24*time.Hour
}

// DownloadDir returns the directory for downloaded update binaries.
func DownloadDir() string {
	return filepath.Join(fccDir(), "download")
}
