package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifySHA256(t *testing.T) {
	// Create a temp file with known content.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Calculate expected hash.
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	// Test correct hash.
	if err := verifySHA256(path, expected); err != nil {
		t.Errorf("verifySHA256() error = %v", err)
	}

	// Test wrong hash.
	if err := verifySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Error("verifySHA256() expected error for wrong hash")
	}
}

func TestVerifySHA256FileNotFound(t *testing.T) {
	err := verifySHA256("/nonexistent/path", "abc")
	if err == nil {
		t.Error("verifySHA256() expected error for missing file")
	}
}

func TestFetchChecksum(t *testing.T) {
	// This test would need a mock HTTP server. For now, test the error path.
	// The fetchChecksum function uses a real HTTP client, so we test invalid URL.
	_, err := fetchChecksum(nil, "")
	if err == nil {
		t.Error("fetchChecksum() expected error for empty URL")
	}
}

func TestFetchChecksumWithServer(t *testing.T) {
	expected := "abc123"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  fcc-darwin-arm64\n", expected)
	}))
	defer ts.Close()

	got, err := fetchChecksum(http.DefaultClient, ts.URL)
	if err != nil {
		t.Fatalf("fetchChecksum() error = %v", err)
	}
	if got != expected {
		t.Errorf("fetchChecksum() = %q, want %q", got, expected)
	}
}

func TestFetchChecksumServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := fetchChecksum(http.DefaultClient, ts.URL)
	if err == nil {
		t.Error("fetchChecksum() expected error for 404")
	}
}

func TestFetchFile(t *testing.T) {
	content := []byte("binary data")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "downloaded.bin")

	if err := fetchFile(http.DefaultClient, ts.URL, dest); err != nil {
		t.Fatalf("fetchFile() error = %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestFetchFileServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "fail.bin")

	err := fetchFile(http.DefaultClient, ts.URL, dest)
	if err == nil {
		t.Error("fetchFile() expected error for 500")
	}
}

func TestDownload(t *testing.T) {
	// Create a mock binary and its SHA256 checksum.
	binaryContent := []byte("fake binary")
	h := sha256.Sum256(binaryContent)
	checksum := hex.EncodeToString(h[:])

	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer binaryServer.Close()

	shaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  fcc-darwin-arm64\n", checksum)
	}))
	defer shaServer.Close()

	// Set temp download dir.
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	path, gotChecksum, err := Download(http.DefaultClient, binaryServer.URL, shaServer.URL, "1.0.0")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if gotChecksum != checksum {
		t.Errorf("checksum = %q, want %q", gotChecksum, checksum)
	}

	// Verify file exists and is executable.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("downloaded file should be executable")
	}
}

func TestDownloadEmptySHAURL(t *testing.T) {
	_, _, err := Download(http.DefaultClient, "http://example.com/bin", "", "1.0.0")
	if err == nil {
		t.Error("Download() expected error for empty SHA URL")
	}
}

func TestReplaceBinary(t *testing.T) {
	// Create a temp dir with a fake "current" binary and a "new" binary.
	tmpDir := t.TempDir()
	current := filepath.Join(tmpDir, "fcc")
	newBin := filepath.Join(tmpDir, "fcc-new")

	if err := os.WriteFile(current, []byte("old"), 0755); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if err := os.WriteFile(newBin, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Override os.Executable by creating a wrapper... but os.Executable is not mockable.
	// Instead, test ReplaceBinary with invalid paths.
	// Test that ReplaceBinary with non-existent new path fails.
	err := ReplaceBinary("/nonexistent/path")
	if err == nil {
		t.Error("ReplaceBinary() expected error for non-existent path")
	}
}
