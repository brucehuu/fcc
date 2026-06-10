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

func TestResolveWorkDirExplicitPath(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveWorkDir([]string{dir})
	if err != nil {
		t.Fatalf("resolveWorkDir() error = %v", err)
	}
	if got != dir {
		t.Errorf("resolveWorkDir() = %q, want %q", got, dir)
	}
}

func TestResolveWorkDirExplicitRelativePath(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory to use as relative path.
	if err := os.Mkdir(filepath.Join(dir, "project"), 0755); err != nil {
		t.Fatal(err)
	}
	// Change to parent dir so relative path works.
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	got, err := resolveWorkDir([]string{"project"})
	if err != nil {
		t.Fatalf("resolveWorkDir() error = %v", err)
	}
	// Should resolve to an absolute path ending with "project".
	if !filepath.IsAbs(got) {
		t.Errorf("resolveWorkDir() = %q, want absolute path", got)
	}
	if filepath.Base(got) != "project" {
		t.Errorf("resolveWorkDir() = %q, want base name 'project'", got)
	}
	// Should be a valid directory.
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Errorf("resolved path %q is not a valid directory", got)
	}
}

func TestResolveWorkDirInvalidPath(t *testing.T) {
	_, err := resolveWorkDir([]string{"/nonexistent/path/that/does/not/exist"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestResolveWorkDirFileNotDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("hello"), 0644)

	_, err := resolveWorkDir([]string{file})
	if err == nil {
		t.Fatal("expected error for file path (not directory)")
	}
}

func TestResolveWorkDirNoArgs(t *testing.T) {
	// Without args, should fall back to cwd/exe-based logic.
	got, err := resolveWorkDir(nil)
	if err != nil {
		t.Fatalf("resolveWorkDir(nil) error = %v", err)
	}
	if got == "" {
		t.Fatal("resolveWorkDir(nil) returned empty string")
	}
}

func TestResolveWorkDirEmptyArgs(t *testing.T) {
	got, err := resolveWorkDir([]string{})
	if err != nil {
		t.Fatalf("resolveWorkDir([]) error = %v", err)
	}
	if got == "" {
		t.Fatal("resolveWorkDir([]) returned empty string")
	}
}

func TestIsTempDir(t *testing.T) {
	tmpDir := os.TempDir()
	// A subdirectory of temp dir should be detected as temp.
	sub := filepath.Join(tmpDir, "go-build123")
	if !isTempDir(sub) {
		t.Errorf("isTempDir(%q) = false, want true", sub)
	}
	// A regular directory should not be temp.
	if isTempDir("/usr/local/bin") {
		t.Error("isTempDir(/usr/local/bin) = true, want false")
	}
}
