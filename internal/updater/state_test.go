package updater

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFccDir(t *testing.T) {
	dir := fccDir()
	if dir == "" {
		t.Fatal("fccDir() returned empty string")
	}
	if !filepath.IsAbs(dir) && dir != "." {
		t.Fatalf("fccDir() = %q, want absolute path or '.'", dir)
	}
}

func TestStatePath(t *testing.T) {
	path := statePath()
	if path == "" {
		t.Fatal("statePath() returned empty string")
	}
	if filepath.Base(path) != stateFileName {
		t.Fatalf("statePath() basename = %q, want %q", filepath.Base(path), stateFileName)
	}
}

func TestDownloadDir(t *testing.T) {
	dir := DownloadDir()
	if dir == "" {
		t.Fatal("DownloadDir() returned empty string")
	}
	if filepath.Base(dir) != "download" {
		t.Fatalf("DownloadDir() basename = %q, want 'download'", filepath.Base(dir))
	}
}

func TestLoadStateNotFound(t *testing.T) {
	// Use a temp dir to avoid interfering with real state.
	origDir := fccDir()
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origDir)

	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.Status != StatusIdle {
		t.Fatalf("LoadState() status = %q, want %q", state.Status, StatusIdle)
	}
	if state.CurrentVersion != "" {
		t.Fatalf("LoadState() current version = %q, want empty", state.CurrentVersion)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	// Use a temp dir.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	state := &State{
		CurrentVersion: "1.0.0",
		LatestVersion:  "1.1.0",
		Status:         StatusDownloaded,
		Path:           "/tmp/fcc-1.1.0",
		SHA256:         "abc123",
		CheckedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		DownloadedAt:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Error:          "",
	}

	if err := SaveState(state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	loaded, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if loaded.CurrentVersion != state.CurrentVersion {
		t.Errorf("CurrentVersion = %q, want %q", loaded.CurrentVersion, state.CurrentVersion)
	}
	if loaded.LatestVersion != state.LatestVersion {
		t.Errorf("LatestVersion = %q, want %q", loaded.LatestVersion, state.LatestVersion)
	}
	if loaded.Status != state.Status {
		t.Errorf("Status = %q, want %q", loaded.Status, state.Status)
	}
	if loaded.Path != state.Path {
		t.Errorf("Path = %q, want %q", loaded.Path, state.Path)
	}
	if loaded.SHA256 != state.SHA256 {
		t.Errorf("SHA256 = %q, want %q", loaded.SHA256, state.SHA256)
	}
	if loaded.Error != state.Error {
		t.Errorf("Error = %q, want %q", loaded.Error, state.Error)
	}
}

func TestShouldCheck(t *testing.T) {
	tests := []struct {
		name      string
		checkedAt time.Time
		cooldown  time.Duration
		want      bool
	}{
		{"never checked", time.Time{}, time.Hour, true},
		{"just checked", time.Now(), time.Hour, false},
		{"checked long ago", time.Now().Add(-2 * time.Hour), time.Hour, true},
		{"zero cooldown", time.Now(), 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{CheckedAt: tt.checkedAt}
			got := ShouldCheck(s, tt.cooldown)
			if got != tt.want {
				t.Errorf("ShouldCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveStateInvalidDir(t *testing.T) {
	// Set HOME to a path that cannot be created as a directory.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/dev/null/invalid")
	defer os.Setenv("HOME", origHome)

	state := &State{Status: StatusIdle}
	if err := SaveState(state); err == nil {
		t.Fatal("SaveState() expected error for invalid directory")
	}
}
