package updater

import (
	"crypto/sha256"
	"encoding/hex"
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
