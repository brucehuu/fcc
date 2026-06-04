package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	githubOwner = "brucehuu"
	githubRepo  = "fcc"
)

// Release represents the latest GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

// Asset is a release binary attachment.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// FetchLatest queries the GitHub API for the latest release.
func FetchLatest(client *http.Client) (*Release, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", githubOwner, githubRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "fcc-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api %d: %s", resp.StatusCode, string(body))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// ParseVersion extracts the semver from a GitHub tag like "v0.0.2".
func ParseVersion(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

// AssetForArch returns the download URL for the current architecture.
func AssetForArch(rel *Release) (downloadURL string, size int64) {
	name := fmt.Sprintf("fcc-darwin-%s", runtime.GOARCH)
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL, a.Size
		}
	}
	return "", 0
}

// AssetSHA256URL returns the URL for the SHA256 checksum file.
func AssetSHA256URL(rel *Release) string {
	name := fmt.Sprintf("fcc-darwin-%s.sha256", runtime.GOARCH)
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// CompareVersions returns true if remote > local.
// Supports simple semver: 0.0.1, 0.0.13, 0.1.0.
func CompareVersions(local, remote string) bool {
	lp := splitVersion(local)
	rp := splitVersion(remote)
	for i := 0; i < 3; i++ {
		if i >= len(lp) || i >= len(rp) {
			break
		}
		if rp[i] > lp[i] {
			return true
		}
		if rp[i] < lp[i] {
			return false
		}
	}
	return len(rp) > len(lp)
}

func splitVersion(v string) []int {
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		var n int
		fmt.Sscanf(p, "%d", &n)
		out = append(out, n)
	}
	return out
}
