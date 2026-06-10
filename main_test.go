package main

import (
	"os"
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

func TestIsGlobalBinDir(t *testing.T) {
	home := "/Users/tester"
	tests := []struct {
		dir  string
		want bool
	}{
		{"/usr/local/bin", true},
		{"/usr/local/bin/", true},
		{"/opt/homebrew/bin", true},
		{"/bin", true},
		{"/sbin", true},
		{"/usr/bin", true},
		{"/usr/sbin", true},
		{"/Users/tester/.local/bin", true},
		{"/Users/tester/.local/bin/", true},
		{"/Users/tester/code/fcc", false},
		{"/tmp", false},
		{".", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			if got := isGlobalBinDir(tt.dir, home); got != tt.want {
				t.Errorf("isGlobalBinDir(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestIsGlobalBinDirWithoutHome(t *testing.T) {
	// Without home set, .local/bin should not match.
	if isGlobalBinDir("/Users/tester/.local/bin", "") {
		t.Error("isGlobalBinDir(.local/bin, empty home) = true, want false")
	}
}

func TestProcessIcon(t *testing.T) {
	// Read the real icon file.
	data, err := os.ReadFile("assets/fcc-logo.png")
	if err != nil {
		t.Skipf("cannot read icon file: %v", err)
	}

	result := processIcon(data, 0)
	if len(result) == 0 {
		t.Fatal("processIcon returned empty data")
	}

	// With padding should produce different size.
	padded := processIcon(data, 20)
	if len(padded) == 0 {
		t.Fatal("processIcon with padding returned empty data")
	}
}
