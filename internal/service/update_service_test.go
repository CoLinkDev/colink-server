package service

import (
	"testing"
	"time"

	"colink-server/internal/model"
)

func TestNormalizeVersion(t *testing.T) {
	tests := map[string]string{
		"1.2.3":        "1.2.3",
		"v1.2.3":       "1.2.3",
		"V1.2.3":       "1.2.3",
		"  v1.2.3  ":   "1.2.3",
		"v1.2.3-beta":  "1.2.3-beta",
		"v1.2.3+build": "1.2.3+build",
	}

	for raw, want := range tests {
		got, err := normalizeVersion(raw)
		if err != nil {
			t.Fatalf("normalizeVersion(%q): %v", raw, err)
		}
		if got != want {
			t.Fatalf("normalizeVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeVersionRejectsInvalidSemver(t *testing.T) {
	for _, raw := range []string{"", "1.2", "1.2.x", "version-1.2.3"} {
		if _, err := normalizeVersion(raw); err == nil {
			t.Fatalf("expected normalizeVersion(%q) to fail", raw)
		}
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "1.2.4", right: "1.2.3", want: 1},
		{left: "1.3.0", right: "1.2.9", want: 1},
		{left: "2.0.0", right: "1.9.9", want: 1},
		{left: "1.2.3", right: "1.2.3", want: 0},
		{left: "1.2.3", right: "1.2.4", want: -1},
		{left: "v1.2.3-beta", right: "1.2.3", want: 0},
	}

	for _, tt := range tests {
		left, err := parseSemver(tt.left)
		if err != nil {
			t.Fatalf("parseSemver(%q): %v", tt.left, err)
		}
		right, err := parseSemver(tt.right)
		if err != nil {
			t.Fatalf("parseSemver(%q): %v", tt.right, err)
		}
		got := sign(left.compare(right))
		if got != tt.want {
			t.Fatalf("compare %q vs %q = %d, want %d", tt.left, tt.right, got, tt.want)
		}
	}
}

func TestFilterReleaseAssets(t *testing.T) {
	assets := []githubAsset{
		{Name: "CoLink_1.2.7_x64-setup.exe"},
		{Name: "CoLink_1.2.7_x64_en-US.msi"},
		{Name: "CoLink_1.2.7_amd64.deb"},
		{Name: "CoLink_1.2.7_amd64.AppImage"},
		{Name: "app-release.apk"},
	}

	tests := []struct {
		platform string
		count    int
	}{
		{platform: "android", count: 1},
		{platform: "windows", count: 2},
		{platform: "linux", count: 2},
	}

	for _, test := range tests {
		t.Run(test.platform, func(t *testing.T) {
			if actual := len(filterReleaseAssets(test.platform, assets)); actual != test.count {
				t.Fatalf("filtered %d assets, want %d", actual, test.count)
			}
		})
	}
}

func TestAssetNeedsRefresh(t *testing.T) {
	previous := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	asset := githubAsset{UpdatedAt: previous.Add(time.Hour)}
	if !assetNeedsRefresh(asset, model.ReleaseAsset{}, false) {
		t.Fatal("expected new asset to be downloaded")
	}
	if !assetNeedsRefresh(asset, model.ReleaseAsset{}, true) {
		t.Fatal("expected legacy asset to be downloaded")
	}
	if !assetNeedsRefresh(asset, model.ReleaseAsset{SourceUpdatedAt: &previous}, true) {
		t.Fatal("expected changed source asset to be downloaded")
	}
	if assetNeedsRefresh(asset, model.ReleaseAsset{SourceUpdatedAt: &asset.UpdatedAt}, true) {
		t.Fatal("expected unchanged source asset to be reused")
	}
}

func sign(value int) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
