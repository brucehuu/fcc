package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Download fetches the update binary and its SHA256 checksum.
func Download(client *http.Client, binaryURL, shaURL, version string) (binaryPath string, checksum string, err error) {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	dir := DownloadDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", fmt.Errorf("mkdir download dir: %w", err)
	}

	// Fetch SHA256 checksum first.
	checksum, err = fetchChecksum(client, shaURL)
	if err != nil {
		return "", "", fmt.Errorf("fetch checksum: %w", err)
	}

	// Download binary.
	binaryPath = filepath.Join(dir, fmt.Sprintf("fcc-%s", version))
	if err := fetchFile(client, binaryURL, binaryPath); err != nil {
		return "", "", fmt.Errorf("fetch binary: %w", err)
	}

	// Verify SHA256.
	if err := verifySHA256(binaryPath, checksum); err != nil {
		os.Remove(binaryPath)
		return "", "", fmt.Errorf("verify sha256: %w", err)
	}

	// Make executable.
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return "", "", fmt.Errorf("chmod binary: %w", err)
	}

	return binaryPath, checksum, nil
}

func fetchChecksum(client *http.Client, url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("no sha256 url provided")
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// Format: "<hash>  <filename>" or just "<hash>"
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum file")
	}
	return strings.ToLower(fields[0]), nil
}

func fetchFile(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(dest)
		return err
	}
	return nil
}

func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}
