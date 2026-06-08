package updater

import (
	"runtime"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.0.0", "1.0.0"},
		{"v0.0.2", "0.0.2"},
		{"1.0.0", "1.0.0"},
		{"v12.34.56", "12.34.56"},
		{"", ""},
		{"v", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseVersion(tt.input)
			if got != tt.want {
				t.Errorf("ParseVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAssetForArch(t *testing.T) {
	rel := &Release{
		Assets: []Asset{
			{Name: "fcc-darwin-amd64", BrowserDownloadURL: "https://example.com/amd64", Size: 1000},
			{Name: "fcc-darwin-arm64", BrowserDownloadURL: "https://example.com/arm64", Size: 2000},
			{Name: "fcc-linux-amd64", BrowserDownloadURL: "https://example.com/linux", Size: 3000},
		},
	}

	url, size := AssetForArch(rel)
	wantName := "fcc-darwin-" + runtime.GOARCH
	var found bool
	for _, a := range rel.Assets {
		if a.Name == wantName {
			found = true
			if url != a.BrowserDownloadURL {
				t.Errorf("AssetForArch() url = %q, want %q", url, a.BrowserDownloadURL)
			}
			if size != a.Size {
				t.Errorf("AssetForArch() size = %d, want %d", size, a.Size)
			}
			break
		}
	}
	if !found {
		// Current arch not in test data, expect empty.
		if url != "" || size != 0 {
			t.Errorf("AssetForArch() = (%q, %d), want empty for arch %s", url, size, runtime.GOARCH)
		}
	}
}

func TestAssetForArchNotFound(t *testing.T) {
	rel := &Release{
		Assets: []Asset{
			{Name: "fcc-linux-amd64", BrowserDownloadURL: "https://example.com/linux", Size: 1000},
		},
	}
	url, size := AssetForArch(rel)
	if url != "" || size != 0 {
		t.Errorf("AssetForArch() = (%q, %d), want empty", url, size)
	}
}

func TestAssetSHA256URL(t *testing.T) {
	rel := &Release{
		Assets: []Asset{
			{Name: "fcc-darwin-" + runtime.GOARCH + ".sha256", BrowserDownloadURL: "https://example.com/sha256"},
			{Name: "fcc-darwin-" + runtime.GOARCH, BrowserDownloadURL: "https://example.com/binary"},
		},
	}

	url := AssetSHA256URL(rel)
	if url != "https://example.com/sha256" {
		t.Errorf("AssetSHA256URL() = %q, want %q", url, "https://example.com/sha256")
	}
}

func TestAssetSHA256URLNotFound(t *testing.T) {
	rel := &Release{Assets: []Asset{{Name: "other"}}}
	if url := AssetSHA256URL(rel); url != "" {
		t.Errorf("AssetSHA256URL() = %q, want empty", url)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		local  string
		remote string
		want   bool
	}{
		{"0.0.1", "0.0.2", true},
		{"0.0.2", "0.0.1", false},
		{"0.0.1", "0.0.1", false},
		{"0.0.1", "0.1.0", true},
		{"0.1.0", "1.0.0", true},
		{"1.0.0", "0.1.0", false},
		{"0.0.1", "0.0.1.1", true},
		{"0.0.1.1", "0.0.1", false},
		{"", "0.0.1", true},
		{"0.0.1", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.local+"_vs_"+tt.remote, func(t *testing.T) {
			got := CompareVersions(tt.local, tt.remote)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %v, want %v", tt.local, tt.remote, got, tt.want)
			}
		})
	}
}

func TestSplitVersion(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"1.0.0", []int{1, 0, 0}},
		{"0.0.2", []int{0, 0, 2}},
		{"12.34.56", []int{12, 34, 56}},
		{"1", []int{1}},
		{"1.a.3", []int{1, 0, 3}},
		{"v1.0.0", []int{0, 0, 0}}, // 'v' parsed as 0
		{"", []int{0}},             // empty string splits to [""] which parses to 0
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitVersion(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
