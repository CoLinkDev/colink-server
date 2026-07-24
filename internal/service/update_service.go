package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"colink-server/internal/config"
	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

const updateDownloadPath = "/api/v1/update/download"

var updatePlatforms = map[string]struct{}{
	"android": {},
	"windows": {},
	"linux":   {},
}

type UpdateCheckResult struct {
	HasUpdate bool           `json:"hasUpdate"`
	Latest    *UpdateRelease `json:"latest"`
}

type UpdateRelease struct {
	Version      string        `json:"version"`
	ReleaseNotes string        `json:"releaseNotes"`
	PublishedAt  time.Time     `json:"publishedAt"`
	Assets       []UpdateAsset `json:"assets"`
}

type UpdateAsset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"downloadUrl"`
}

type TauriManifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	PubDate   string `json:"pub_date"`
	Signature string `json:"signature"`
	URL       string `json:"url"`
}

type UpdateService struct {
	releaseRepo *repository.ReleaseRepository
	cfg         config.UpdateConfig
	log         *zap.Logger
	httpClient  *http.Client
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func NewUpdateService(releaseRepo *repository.ReleaseRepository, cfg config.UpdateConfig, log *zap.Logger) *UpdateService {
	return &UpdateService{
		releaseRepo: releaseRepo,
		cfg:         cfg,
		log:         log,
		httpClient:  newGitHubHTTPClient(cfg.Proxy),
	}
}

func (s *UpdateService) CheckForUpdates(ctx context.Context) {
	for _, repo := range s.cfg.GitHub.Repos {
		if err := s.checkRepo(ctx, repo); err != nil {
			s.log.Error(
				"check update repository",
				zap.String("owner", repo.Owner),
				zap.String("repo", repo.Repo),
				zap.String("platform", repo.Platform),
				zap.Error(err),
			)
		}
	}
}

func (s *UpdateService) GetLatestRelease(platform string, currentVersion string) (*UpdateCheckResult, error) {
	platform, err := normalizeUpdatePlatform(platform)
	if err != nil {
		return nil, err
	}

	release, err := s.releaseRepo.FindLatestByPlatform(platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &UpdateCheckResult{HasUpdate: false, Latest: nil}, nil
		}
		return nil, pkg.InternalError(err)
	}

	if strings.TrimSpace(currentVersion) != "" {
		if _, err := parseSemver(currentVersion); err != nil {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		hasUpdate, err := isNewerVersion(release.Version, currentVersion)
		if err != nil {
			return nil, pkg.InternalError(err)
		}
		if !hasUpdate {
			return &UpdateCheckResult{HasUpdate: false, Latest: nil}, nil
		}
	}

	result, err := s.releaseResult(release)
	if err != nil {
		return nil, err
	}
	return &UpdateCheckResult{HasUpdate: true, Latest: result}, nil
}

func (s *UpdateService) GetTauriManifest(target, arch, currentVersion string) (*TauriManifest, error) {
	if !strings.EqualFold(strings.TrimSpace(target), "windows") || strings.TrimSpace(arch) != "x86_64" {
		return nil, nil
	}

	release, err := s.releaseRepo.FindLatestByPlatform("windows")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, pkg.InternalError(err)
	}

	if _, err := parseSemver(currentVersion); err != nil {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	hasUpdate, err := isNewerVersion(release.Version, currentVersion)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	if !hasUpdate {
		return nil, nil
	}

	assets, err := s.releaseRepo.FindAssetsByReleaseID(release.ID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	archive, signature := selectTauriAssets(assets)
	if archive == nil || signature == nil {
		return nil, nil
	}
	archiveExists, err := cachedAssetExists(archive.FilePath)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	signatureExists, err := cachedAssetExists(signature.FilePath)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	if !archiveExists || !signatureExists {
		return nil, nil
	}

	signatureText, err := os.ReadFile(signature.FilePath)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	if strings.TrimSpace(string(signatureText)) == "" {
		return nil, nil
	}

	return &TauriManifest{
		Version:   release.Version,
		Notes:     release.ReleaseNotes,
		PubDate:   release.PublishedAt.UTC().Format(time.RFC3339Nano),
		Signature: strings.TrimSpace(string(signatureText)),
		URL:       buildDownloadURL(release.Platform, release.Version, archive.FileName),
	}, nil
}

func (s *UpdateService) GetAssetFilePath(platform, version, filename string) (string, error) {
	platform, err := normalizeUpdatePlatform(platform)
	if err != nil {
		return "", err
	}
	version, err = normalizeVersion(version)
	if err != nil {
		return "", pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", pkg.NewAppError(http.StatusNotFound, pkg.CodeUpdateAssetNotFound, "asset not found")
	}

	release, err := s.releaseRepo.FindByPlatformAndVersion(platform, version)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", pkg.NewAppError(http.StatusNotFound, pkg.CodeUpdateReleaseNotFound, "release not found")
		}
		return "", pkg.InternalError(err)
	}

	asset, err := s.releaseRepo.FindAssetByReleaseIDAndFileName(release.ID, filename)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", pkg.NewAppError(http.StatusNotFound, pkg.CodeUpdateAssetNotFound, "asset not found")
		}
		return "", pkg.InternalError(err)
	}

	return asset.FilePath, nil
}

func (s *UpdateService) checkRepo(ctx context.Context, repo config.GitHubRepoConfig) error {
	platform, err := normalizeUpdatePlatform(repo.Platform)
	if err != nil {
		return err
	}
	release, err := s.fetchLatestRelease(ctx, repo)
	if err != nil {
		return err
	}
	version, err := normalizeVersion(release.TagName)
	if err != nil {
		return fmt.Errorf("normalize release tag %q: %w", release.TagName, err)
	}

	existingAssets, err := s.releaseAssets(platform, version)
	if err != nil {
		return pkg.InternalError(err)
	}
	if len(release.Assets) == 0 {
		s.log.Warn("release has no assets", zap.String("platform", platform), zap.String("version", version))
		return nil
	}

	releaseAssets := filterReleaseAssets(platform, release.Assets)
	if len(releaseAssets) == 0 {
		s.log.Warn("release has no platform assets", zap.String("platform", platform), zap.String("version", version))
		return nil
	}

	assets, err := s.cacheAssets(ctx, platform, version, releaseAssets, existingAssets)
	if err != nil {
		return err
	}
	appRelease := &model.AppRelease{
		Platform:     platform,
		Version:      version,
		ReleaseNotes: release.Body,
		PublishedAt:  release.PublishedAt,
	}
	created, staleAssets, err := s.releaseRepo.CreateOrUpdateWithAssets(appRelease, assets)
	if err != nil {
		return pkg.InternalError(err)
	}
	s.removeStaleAssets(staleAssets)

	s.log.Info("synced app release", zap.String("platform", platform), zap.String("version", version), zap.Int("assets", len(assets)), zap.Bool("created", created))
	return nil
}

func (s *UpdateService) releaseAssets(platform, version string) (map[string]model.ReleaseAsset, error) {
	release, err := s.releaseRepo.FindByPlatformAndVersion(platform, version)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]model.ReleaseAsset{}, nil
	}
	if err != nil {
		return nil, err
	}

	assets, err := s.releaseRepo.FindAssetsByReleaseID(release.ID)
	if err != nil {
		return nil, err
	}
	assetsByName := make(map[string]model.ReleaseAsset, len(assets))
	for _, asset := range assets {
		assetsByName[asset.FileName] = asset
	}
	return assetsByName, nil
}

func filterReleaseAssets(platform string, assets []githubAsset) []githubAsset {
	filtered := make([]githubAsset, 0, len(assets))
	for _, asset := range assets {
		if matchesPlatformAsset(platform, asset.Name) {
			filtered = append(filtered, asset)
		}
	}
	return filtered
}

func matchesPlatformAsset(platform, name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	switch platform {
	case "android":
		return extension == ".apk"
	case "windows":
		return extension == ".exe" || extension == ".msi" ||
			strings.HasSuffix(strings.ToLower(name), ".nsis.zip") ||
			strings.HasSuffix(strings.ToLower(name), ".nsis.zip.sig")
	case "linux":
		return extension == ".deb" || extension == ".appimage"
	default:
		return false
	}
}

func (s *UpdateService) fetchLatestRelease(ctx context.Context, repo config.GitHubRepoConfig) (*githubRelease, error) {
	owner := strings.TrimSpace(repo.Owner)
	name := strings.TrimSpace(repo.Repo)
	if owner == "" || name == "" {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	s.authorizeGitHub(req)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "colink-server")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github latest release status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *UpdateService) cacheAssets(ctx context.Context, platform, version string, assets []githubAsset, existingAssets map[string]model.ReleaseAsset) ([]model.ReleaseAsset, error) {
	targetDir := filepath.Join(s.cfg.StoragePath, platform, version)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}

	cached := make([]model.ReleaseAsset, 0, len(assets))
	for _, asset := range assets {
		fileName, err := cleanAssetFileName(asset.Name)
		if err != nil {
			return nil, err
		}
		filePath := filepath.Join(targetDir, fileName)
		existingAsset, exists := existingAssets[fileName]
		if err := s.ensureAssetCached(ctx, asset, filePath, assetNeedsRefresh(asset, existingAsset, exists)); err != nil {
			return nil, err
		}
		info, err := os.Stat(filePath)
		if err != nil {
			return nil, err
		}
		cached = append(cached, model.ReleaseAsset{
			FileName: fileName,
			FileSize: info.Size(),
			FilePath: filePath,
			SourceUpdatedAt: &asset.UpdatedAt,
		})
	}

	return cached, nil
}

func assetNeedsRefresh(asset githubAsset, existing model.ReleaseAsset, exists bool) bool {
	return !exists || existing.SourceUpdatedAt == nil || !asset.UpdatedAt.Equal(*existing.SourceUpdatedAt)
}

func (s *UpdateService) ensureAssetCached(ctx context.Context, asset githubAsset, filePath string, refresh bool) error {
	if !refresh {
		if info, err := os.Stat(filePath); err == nil && asset.Size >= 0 && info.Size() == asset.Size {
			return nil
		}
	}

	tmpPath := filePath + ".tmp"
	if err := s.downloadAsset(ctx, asset, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if asset.Size >= 0 {
		info, err := os.Stat(tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		if info.Size() != asset.Size {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("asset size mismatch for %s: got %d want %d", asset.Name, info.Size(), asset.Size)
		}
	}
	_ = os.Remove(filePath)
	return os.Rename(tmpPath, filePath)
}

func (s *UpdateService) removeStaleAssets(assets []model.ReleaseAsset) {
	for _, asset := range assets {
		if err := os.Remove(asset.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("remove stale release asset", zap.String("path", asset.FilePath), zap.Error(err))
		}
	}
}

func (s *UpdateService) downloadAsset(ctx context.Context, asset githubAsset, tmpPath string) error {
	if strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		return fmt.Errorf("asset %q has no download url", asset.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	s.authorizeGitHub(req)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "colink-server")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download asset %q status %d", asset.Name, resp.StatusCode)
	}

	output, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer output.Close()
	_, err = io.Copy(output, resp.Body)
	return err
}

func (s *UpdateService) releaseResult(release *model.AppRelease) (*UpdateRelease, error) {
	assets, err := s.releaseRepo.FindAssetsByReleaseID(release.ID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	items := make([]UpdateAsset, 0, len(assets))
	for _, asset := range assets {
		items = append(items, UpdateAsset{
			Name:        asset.FileName,
			Size:        asset.FileSize,
			DownloadURL: buildDownloadURL(release.Platform, release.Version, asset.FileName),
		})
	}

	return &UpdateRelease{
		Version:      release.Version,
		ReleaseNotes: release.ReleaseNotes,
		PublishedAt:  release.PublishedAt,
		Assets:       items,
	}, nil
}

func (s *UpdateService) authorizeGitHub(req *http.Request) {
	if token := strings.TrimSpace(s.cfg.GitHub.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func normalizeUpdatePlatform(platform string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(platform))
	if _, ok := updatePlatforms[normalized]; !ok {
		return "", pkg.NewAppError(http.StatusBadRequest, pkg.CodeUpdatePlatformNotSupported, "platform not supported")
	}
	return normalized, nil
}

func normalizeVersion(version string) (string, error) {
	normalized := strings.TrimSpace(version)
	normalized = strings.TrimPrefix(normalized, "v")
	normalized = strings.TrimPrefix(normalized, "V")
	if normalized == "" {
		return "", fmt.Errorf("empty version")
	}
	if _, err := parseSemver(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func isNewerVersion(latestVersion, currentVersion string) (bool, error) {
	latest, err := parseSemver(latestVersion)
	if err != nil {
		return false, err
	}
	current, err := parseSemver(currentVersion)
	if err != nil {
		return false, err
	}
	return latest.compare(current) > 0, nil
}

type semverValue struct {
	major int
	minor int
	patch int
}

func parseSemver(version string) (semverValue, error) {
	normalized := strings.TrimSpace(version)
	normalized = strings.TrimPrefix(normalized, "v")
	normalized = strings.TrimPrefix(normalized, "V")
	if index := strings.IndexAny(normalized, "+-"); index >= 0 {
		normalized = normalized[:index]
	}
	parts := strings.Split(normalized, ".")
	if len(parts) != 3 {
		return semverValue{}, fmt.Errorf("invalid semver %q", version)
	}

	values := [3]int{}
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return semverValue{}, fmt.Errorf("invalid semver %q", version)
		}
		values[index] = value
	}

	return semverValue{major: values[0], minor: values[1], patch: values[2]}, nil
}

func (v semverValue) compare(other semverValue) int {
	if v.major != other.major {
		return v.major - other.major
	}
	if v.minor != other.minor {
		return v.minor - other.minor
	}
	return v.patch - other.patch
}

func buildDownloadURL(platform, version, fileName string) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s",
		updateDownloadPath,
		url.PathEscape(platform),
		url.PathEscape(version),
		url.PathEscape(fileName),
	)
}

func selectTauriAssets(assets []model.ReleaseAsset) (*model.ReleaseAsset, *model.ReleaseAsset) {
	var archive *model.ReleaseAsset
	var signature *model.ReleaseAsset
	for index := range assets {
		name := strings.ToLower(assets[index].FileName)
		switch {
		case strings.HasSuffix(name, ".nsis.zip"):
			archive = &assets[index]
		case strings.HasSuffix(name, ".nsis.zip.sig"):
			signature = &assets[index]
		}
	}
	return archive, signature
}

func cachedAssetExists(filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func cleanAssetFileName(name string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	fileName := path.Base(normalized)
	if fileName == "." || fileName == "/" || fileName == "" {
		return "", fmt.Errorf("invalid asset file name %q", name)
	}
	return fileName, nil
}

func newGitHubHTTPClient(proxy config.ProxyConfig) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxyFunc(proxy)
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Minute,
	}
}

func proxyFunc(proxy config.ProxyConfig) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		if req.URL == nil || proxyBypass(req.URL.Hostname(), proxy.NoProxy) {
			return nil, nil
		}

		proxyURL := proxy.HTTP
		if req.URL.Scheme == "https" && strings.TrimSpace(proxy.HTTPS) != "" {
			proxyURL = proxy.HTTPS
		}
		if strings.TrimSpace(proxyURL) == "" {
			return nil, nil
		}
		return url.Parse(proxyURL)
	}
}

func proxyBypass(host string, noProxy string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, entry := range strings.Split(noProxy, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == "*" || entry == host {
			return true
		}
		if strings.HasPrefix(entry, ".") && strings.HasSuffix(host, entry) {
			return true
		}
		if strings.HasPrefix(host, entry+".") {
			return true
		}
	}
	return false
}
