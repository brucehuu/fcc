package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadSuccess(t *testing.T) {
	// Create a temp binary content and its SHA256 checksum.
	binaryContent := []byte("fake binary content")
	h := sha256.Sum256(binaryContent)
	checksum := hex.EncodeToString(h[:])

	// Mock server that serves both binary and checksum.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(binaryContent)
		case "/checksum":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "%s  fcc-test\n", checksum)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Override HOME to use a temp directory for downloads.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	client := server.Client()
	binaryPath, gotChecksum, err := Download(client, server.URL+"/binary", server.URL+"/checksum", "1.0.0")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if gotChecksum != checksum {
		t.Errorf("Download() checksum = %q, want %q", gotChecksum, checksum)
	}

	// Verify the binary was written and is executable.
	if _, err := os.Stat(binaryPath); err != nil {
		t.Errorf("Download() binary not found at %s: %v", binaryPath, err)
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("Download() binary not executable: mode = %o", info.Mode())
	}

	// Verify file content.
	gotContent, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(gotContent) != string(binaryContent) {
		t.Errorf("Download() binary content mismatch")
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	binaryContent := []byte("fake binary content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.WriteHeader(http.StatusOK)
			w.Write(binaryContent)
		case "/checksum":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  fcc-test\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	client := server.Client()
	_, _, err := Download(client, server.URL+"/binary", server.URL+"/checksum", "1.0.0")
	if err == nil {
		t.Fatal("Download() expected error for checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("Download() error = %q, want to contain 'sha256 mismatch'", err.Error())
	}

	// Verify the binary file was cleaned up.
	downloadDir := filepath.Join(tmpDir, ".fcc", "download")
	entries, _ := os.ReadDir(downloadDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "fcc-1.0.0") {
			t.Errorf("Download() did not clean up binary file after checksum mismatch: %s", e.Name())
		}
	}
}

func TestDownloadOversizedFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.WriteHeader(http.StatusOK)
			// Write more than 100MB + 1 byte.
			const maxSize = 100 * 1024 * 1024
			largeData := make([]byte, maxSize+1)
			w.Write(largeData)
		case "/checksum":
			// Calculate checksum of the oversized data.
			const maxSize = 100 * 1024 * 1024
			largeData := make([]byte, maxSize+1)
			h := sha256.Sum256(largeData)
			checksum := hex.EncodeToString(h[:])
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "%s  fcc-test\n", checksum)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	client := server.Client()
	_, _, err := Download(client, server.URL+"/binary", server.URL+"/checksum", "1.0.0")
	if err == nil {
		t.Fatal("Download() expected error for oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum download size") {
		t.Errorf("Download() error = %q, want to contain 'exceeds maximum download size'", err.Error())
	}
}

func TestDownloadNilClient(t *testing.T) {
	binaryContent := []byte("fake binary content")
	h := sha256.Sum256(binaryContent)
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.WriteHeader(http.StatusOK)
			w.Write(binaryContent)
		case "/checksum":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "%s  fcc-test\n", checksum)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Pass nil client — function should use default client and not panic.
	binaryPath, gotChecksum, err := Download(nil, server.URL+"/binary", server.URL+"/checksum", "1.0.0")
	if err != nil {
		t.Fatalf("Download(nil client) error = %v", err)
	}
	if gotChecksum != checksum {
		t.Errorf("Download(nil client) checksum = %q, want %q", gotChecksum, checksum)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Errorf("Download(nil client) binary not found at %s: %v", binaryPath, err)
	}
}

func TestFetchChecksumHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := server.Client()
	_, err := fetchChecksum(client, server.URL)
	if err == nil {
		t.Fatal("fetchChecksum() expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("fetchChecksum() error = %q, want to contain 'status 500'", err.Error())
	}
}

func TestFetchFileHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "test.bin")

	client := server.Client()
	err := fetchFile(client, server.URL, dest)
	if err == nil {
		t.Fatal("fetchFile() expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("fetchFile() error = %q, want to contain 'status 404'", err.Error())
	}
}
