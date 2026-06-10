package main

import (
	"path/filepath"
	"testing"
)

func TestChooseDefaultWorkDir(t *testing.T) {
	home := "/Users/tester"
	tests := []struct {
		name    string
		cwd     string
		exePath string
		want    string
	}{
		{
			name:    "copied project binary uses executable directory",
			cwd:     home,
			exePath: "/Users/tester/code/admin_claude_bailu/fcc",
			want:    "/Users/tester/code/admin_claude_bailu",
		},
		{
			name:    "global install uses shell working directory",
			cwd:     "/Users/tester/code/admin_claude_bailu",
			exePath: "/usr/local/bin/fcc",
			want:    "/Users/tester/code/admin_claude_bailu",
		},
		{
			name:    "home local bin uses shell working directory",
			cwd:     "/Users/tester/code/fcc",
			exePath: "/Users/tester/.local/bin/fcc",
			want:    "/Users/tester/code/fcc",
		},
		{
			name:    "homebrew bin uses shell working directory",
			cwd:     "/Users/tester/code/fcc",
			exePath: "/opt/homebrew/bin/fcc",
			want:    "/Users/tester/code/fcc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseDefaultWorkDir(tt.cwd, tt.exePath, home); got != tt.want {
				t.Fatalf("chooseDefaultWorkDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChooseDefaultWorkDirIgnoresTempGoRunBinary(t *testing.T) {
	cwd := "/Users/tester/code/fcc"
	exePath := filepath.Join(t.TempDir(), "go-build123", "fcc")
	if got := chooseDefaultWorkDir(cwd, exePath, "/Users/tester"); got != cwd {
		t.Fatalf("chooseDefaultWorkDir() = %q, want %q", got, cwd)
	}
}
