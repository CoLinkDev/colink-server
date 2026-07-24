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
		{Name: "CoLink_1.2.7_x64-setup.nsis.zip"},
		{Name: "CoLink_1.2.7_x64-setup.nsis.zip.sig"},
		{Name: "CoLink_1.2.7_amd64.deb"},
		{Name: "CoLink_1.2.7_amd64.AppImage"},
		{Name: "app-release.apk"},
	}

	tests := []struct {
		platform string
		count    int
	}{
		{platform: "android", count: 1},
		{platform: "windows", count: 4},
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

func TestSelectTauriAssets(t *testing.T) {
	tests := []struct {
		name          string
		assets        []model.ReleaseAsset
		wantArchive   bool
		wantSignature bool
	}{
		{
			name: "complete pair",
			assets: []model.ReleaseAsset{
				{FileName: "CoLink_1.2.7_x64-setup.nsis.zip"},
				{FileName: "CoLink_1.2.7_x64-setup.nsis.zip.sig"},
			},
			wantArchive: true, wantSignature: true,
		},
		{
			name: "archive only",
			assets: []model.ReleaseAsset{
				{FileName: "CoLink_1.2.7_x64-setup.nsis.zip"},
			},
			wantArchive: true,
		},
		{
			name: "signature only",
			assets: []model.ReleaseAsset{
				{FileName: "CoLink_1.2.7_x64-setup.nsis.zip.sig"},
			},
			wantSignature: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive, signature := selectTauriAssets(test.assets)
			if (archive != nil) != test.wantArchive {
				t.Fatalf("archive present = %t, want %t", archive != nil, test.wantArchive)
			}
			if (signature != nil) != test.wantSignature {
				t.Fatalf("signature present = %t, want %t", signature != nil, test.wantSignature)
			}
		})
	}
}

func TestNormalizeUpdateArchitecture(t *testing.T) {
	tests := []struct {
		platform string
		input    string
		want     string
	}{
		{platform: "windows", input: "", want: updateArchitectureX64},
		{platform: "windows", input: "amd64", want: updateArchitectureX64},
		{platform: "windows", input: "aarch64", want: updateArchitectureARM64},
		{platform: "linux", input: "", want: updateArchitectureX64},
		{platform: "linux", input: "x86_64", want: updateArchitectureX64},
		{platform: "linux", input: "arm64", want: updateArchitectureARM64},
		{platform: "android", input: "arm64-v8a", want: updateArchitectureARM64V8A},
		{platform: "android", input: "x86", want: updateArchitectureAndroidX86},
	}

	for _, test := range tests {
		got, err := normalizeUpdateArchitecture(test.platform, test.input)
		if err != nil {
			t.Fatalf("normalizeUpdateArchitecture(%q, %q): %v", test.platform, test.input, err)
		}
		if got != test.want {
			t.Fatalf("normalizeUpdateArchitecture(%q, %q) = %q, want %q", test.platform, test.input, got, test.want)
		}
	}

	for _, test := range []struct {
		platform string
		input    string
	}{
		{platform: "android", input: "arm64"},
		{platform: "windows", input: "arm64-v8a"},
	} {
		if _, err := normalizeUpdateArchitecture(test.platform, test.input); err == nil {
			t.Fatalf("expected normalizeUpdateArchitecture(%q, %q) to fail", test.platform, test.input)
		}
	}
}

func TestSelectUpdateAsset(t *testing.T) {
	assets := []model.ReleaseAsset{
		{FileName: "CoLink_1.2.7_x64-setup.exe"},
		{FileName: "CoLink_1.2.7_x64-setup.nsis.zip"},
		{FileName: "CoLink_1.2.7_x64-setup.nsis.zip.sig"},
		{FileName: "CoLink_1.2.7_arm64-setup.exe"},
		{FileName: "CoLink_1.2.7_amd64.deb"},
		{FileName: "CoLink_1.2.7_arm64.AppImage"},
		{FileName: "colink-arm64-v8a-release.apk"},
		{FileName: "colink-armeabi-v7a-release.apk"},
		{FileName: "colink-universal-release.apk"},
	}

	tests := []struct {
		name          string
		platform      string
		architecture  string
		legacyAndroid bool
		want          string
	}{
		{name: "windows x64 installer", platform: "windows", architecture: updateArchitectureX64, want: "CoLink_1.2.7_x64-setup.exe"},
		{name: "windows arm64 installer", platform: "windows", architecture: updateArchitectureARM64, want: "CoLink_1.2.7_arm64-setup.exe"},
		{name: "linux historical amd64", platform: "linux", architecture: updateArchitectureX64, want: "CoLink_1.2.7_amd64.deb"},
		{name: "linux arm64 fallback", platform: "linux", architecture: updateArchitectureARM64, want: "CoLink_1.2.7_arm64.AppImage"},
		{name: "android split", platform: "android", architecture: updateArchitectureARM64V8A, want: "colink-arm64-v8a-release.apk"},
		{name: "android universal fallback", platform: "android", architecture: updateArchitectureAndroidX8664, want: "colink-universal-release.apk"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asset, err := selectUpdateAsset(test.platform, test.architecture, test.legacyAndroid, assets)
			if err != nil {
				t.Fatal(err)
			}
			if asset == nil || asset.FileName != test.want {
				t.Fatalf("selected asset = %#v, want %q", asset, test.want)
			}
		})
	}
}

func TestSelectUpdateAssetLegacyAndroidSkipsSplitReleases(t *testing.T) {
	assets := []model.ReleaseAsset{
		{FileName: "colink-arm64-v8a-release.apk"},
		{FileName: "colink-universal-release.apk"},
	}
	asset, err := selectUpdateAsset("android", "", true, assets)
	if err != nil || asset != nil {
		t.Fatalf("selectUpdateAsset() = %#v, %v; want no update", asset, err)
	}
}

func TestSelectUpdateAssetLegacyAndroidUsesUniversalRelease(t *testing.T) {
	assets := []model.ReleaseAsset{{FileName: "colink-universal-release.apk"}}
	asset, err := selectUpdateAsset("android", "", true, assets)
	if err != nil || asset == nil || asset.FileName != "colink-universal-release.apk" {
		t.Fatalf("selectUpdateAsset() = %#v, %v; want universal APK", asset, err)
	}
}

func TestSelectUpdateAssetRejectsAmbiguousCandidates(t *testing.T) {
	assets := []model.ReleaseAsset{
		{FileName: "CoLink_1.2.7_x64-setup.exe"},
		{FileName: "CoLink_1.2.7_x64-full.exe"},
	}
	asset, err := selectUpdateAsset("windows", updateArchitectureX64, false, assets)
	if err == nil || asset != nil {
		t.Fatalf("selectUpdateAsset() = %#v, %v; want ambiguity error", asset, err)
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "1.2.4", current: "1.2.3", want: true},
		{latest: "1.2.3", current: "1.2.3", want: false},
		{latest: "1.2.3", current: "1.2.4", want: false},
	}

	for _, test := range tests {
		actual, err := isNewerVersion(test.latest, test.current)
		if err != nil {
			t.Fatalf("isNewerVersion(%q, %q): %v", test.latest, test.current, err)
		}
		if actual != test.want {
			t.Fatalf("isNewerVersion(%q, %q) = %t, want %t", test.latest, test.current, actual, test.want)
		}
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
