package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectOSRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	if err := os.WriteFile(path, []byte("ID=ubuntu\nNAME=Ubuntu\nVERSION_ID=24.04\nVERSION_CODENAME=noble\nUBUNTU_CODENAME=noble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := Detector{OSReleasePath: path}.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "ubuntu" || info.DockerCodename() != "noble" {
		t.Fatalf("unexpected info: %#v", info)
	}
}

func TestSupportedRejectsUnsupportedCodename(t *testing.T) {
	info := Info{ID: "ubuntu", VersionCodename: "bionic", Arch: "amd64", PackageManager: "apt", HasSystemd: true, HasSudo: true}
	if ok, _ := info.Supported(); ok {
		t.Fatal("expected unsupported codename")
	}
}
