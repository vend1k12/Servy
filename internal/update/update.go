package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultAPIBaseURL = "https://api.github.com"

type Options struct {
	Repo           string
	Version        string
	CurrentVersion string
	InstallDir     string
	BinaryName     string
	BaseURL        string
	OS             string
	Arch           string
	Client         *http.Client
}

type CheckResult struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseURL      string
}

type Result struct {
	Version string
	Path    string
	Updated bool
	Bytes   int64
}

type release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Check(ctx context.Context, opts Options) (CheckResult, error) {
	rel, err := fetchRelease(ctx, opts, "")
	if err != nil {
		return CheckResult{}, err
	}
	current := opts.CurrentVersion
	return CheckResult{
		CurrentVersion:  current,
		LatestVersion:   rel.TagName,
		UpdateAvailable: updateAvailable(current, rel.TagName),
		ReleaseURL:      rel.HTMLURL,
	}, nil
}

func Install(ctx context.Context, opts Options) (Result, error) {
	if opts.OS == "" {
		opts.OS = runtime.GOOS
	}
	if opts.Arch == "" {
		opts.Arch = runtime.GOARCH
	}
	if opts.OS != "linux" {
		return Result{}, fmt.Errorf("servy update supports linux release assets only; current OS is %s", opts.OS)
	}

	rel, err := fetchRelease(ctx, opts, opts.Version)
	if err != nil {
		return Result{}, err
	}
	if opts.Version == "" && !updateAvailable(opts.CurrentVersion, rel.TagName) {
		path, pathErr := targetPath(opts)
		if pathErr != nil {
			return Result{}, pathErr
		}
		return Result{Version: rel.TagName, Path: path, Updated: false}, nil
	}

	binaryName := opts.BinaryName
	if binaryName == "" {
		binaryName = "servy"
	}
	archiveName := fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, opts.OS, opts.Arch)
	archiveAsset, ok := findAsset(rel.Assets, archiveName)
	if !ok {
		return Result{}, fmt.Errorf("release %s has no asset %s", rel.TagName, archiveName)
	}
	checksumsAsset, ok := findAsset(rel.Assets, "checksums.txt")
	if !ok {
		return Result{}, fmt.Errorf("release %s has no checksums.txt asset", rel.TagName)
	}

	checksums, err := download(ctx, opts, checksumsAsset.BrowserDownloadURL)
	if err != nil {
		return Result{}, fmt.Errorf("download checksums: %w", err)
	}
	expected, err := checksumFor(checksums, archiveName)
	if err != nil {
		return Result{}, err
	}
	archiveBytes, err := download(ctx, opts, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", archiveName, err)
	}
	if err := verifySHA256(archiveBytes, expected, archiveName); err != nil {
		return Result{}, err
	}
	binary, err := extractBinary(archiveBytes, binaryName)
	if err != nil {
		return Result{}, err
	}
	path, err := installBinary(opts, binary)
	if err != nil {
		return Result{}, err
	}
	return Result{Version: rel.TagName, Path: path, Updated: true, Bytes: int64(len(binary))}, nil
}

func fetchRelease(ctx context.Context, opts Options, version string) (release, error) {
	repo := opts.Repo
	if repo == "" {
		repo = "vend1k12/servy"
	}
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	path := fmt.Sprintf("/repos/%s/releases/latest", strings.Trim(repo, "/"))
	if version != "" && version != "latest" {
		path = fmt.Sprintf("/repos/%s/releases/tags/%s", strings.Trim(repo, "/"), version)
	}
	body, err := get(ctx, opts, baseURL+path)
	if err != nil {
		return release{}, err
	}
	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return release{}, fmt.Errorf("decode GitHub release response: %w", err)
	}
	if rel.TagName == "" {
		return release{}, errors.New("GitHub release response did not include tag_name")
	}
	return rel, nil
}

func download(ctx context.Context, opts Options, url string) ([]byte, error) {
	if url == "" {
		return nil, errors.New("asset download URL is empty")
	}
	return get(ctx, opts, url)
}

func get(ctx context.Context, opts Options, url string) ([]byte, error) {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "servy-updater")
	req.Header.Set("Accept", "application/octet-stream, application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("GET %s returned %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func findAsset(assets []asset, name string) (asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return asset{}, false
}

func checksumFor(checksums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimLeft(fields[len(fields)-1], "*")
		if filepath.Base(name) == assetName {
			if len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("checksum for %s is not SHA256", assetName)
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", fmt.Errorf("checksum for %s is not hex: %w", assetName, err)
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt does not contain %s", assetName)
}

func verifySHA256(b []byte, expected, name string) error {
	sum := sha256.Sum256(b)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", name, actual, expected)
	}
	return nil
}

func extractBinary(archive []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var binary []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Name != binaryName {
			return nil, fmt.Errorf("release archive must contain exactly one regular member named %s", binaryName)
		}
		if binary != nil {
			return nil, fmt.Errorf("release archive contains multiple %s entries", binaryName)
		}
		binary, err = io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s from release archive: %w", binaryName, err)
		}
	}
	if len(binary) == 0 {
		return nil, fmt.Errorf("release archive did not contain executable %s", binaryName)
	}
	return binary, nil
}

func installBinary(opts Options, binary []byte) (string, error) {
	path, err := targetPath(opts)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("install %s: %w; rerun with appropriate permissions or --install-dir", path, err)
	}
	return path, nil
}

func targetPath(opts Options) (string, error) {
	binaryName := opts.BinaryName
	if binaryName == "" {
		binaryName = "servy"
	}
	if opts.InstallDir != "" {
		return filepath.Join(opts.InstallDir, binaryName), nil
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "", errors.New("cannot determine current executable; pass --install-dir")
	}
	return exe, nil
}

func updateAvailable(current, latest string) bool {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	if latest == "" {
		return false
	}
	if current == "" || current == "dev" || current == "unknown" {
		return true
	}
	return current != latest
}
