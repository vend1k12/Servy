package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckReportsAvailableRelease(t *testing.T) {
	server := releaseServer(t, []byte("binary"))
	defer server.Close()

	res, err := Check(context.Background(), Options{Repo: "vend1k12/servy", CurrentVersion: "v0.0.1", BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpdateAvailable {
		t.Fatal("expected update to be available")
	}
	if res.LatestVersion != "v0.0.2" {
		t.Fatalf("latest = %q", res.LatestVersion)
	}
}

func TestInstallDownloadsVerifiesAndInstallsRelease(t *testing.T) {
	payload := []byte("servy-test-binary")
	server := releaseServer(t, payload)
	defer server.Close()
	dir := t.TempDir()

	res, err := Install(context.Background(), Options{Repo: "vend1k12/servy", CurrentVersion: "v0.0.1", InstallDir: dir, BinaryName: "servy", BaseURL: server.URL, OS: "linux", Arch: "amd64", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated || res.Version != "v0.0.2" {
		t.Fatalf("result = %#v", res)
	}
	installed, err := os.ReadFile(filepath.Join(dir, "servy"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, payload) {
		t.Fatalf("installed payload = %q", installed)
	}
}

func releaseServer(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	archive := releaseArchive(t, payload)
	sum := sha256.Sum256(archive)
	checksums := []byte(fmt.Sprintf("%s  servy_linux_amd64.tar.gz\n", hex.EncodeToString(sum[:])))

	mux := http.NewServeMux()
	var baseURL string
	mux.HandleFunc("/repos/vend1k12/servy/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.0.2","html_url":"%s/releases/tag/v0.0.2","assets":[{"name":"servy_linux_amd64.tar.gz","browser_download_url":"%s/download/servy_linux_amd64.tar.gz"},{"name":"checksums.txt","browser_download_url":"%s/download/checksums.txt"}]}`, baseURL, baseURL, baseURL)
	})
	mux.HandleFunc("/download/servy_linux_amd64.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/download/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksums)
	})
	server := httptest.NewServer(mux)
	baseURL = server.URL
	return server
}

func releaseArchive(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "servy", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
