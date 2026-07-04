package safeops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// dockerFingerprint is Docker's Release (CE deb) primary key. It is
// duplicated here (and in internal/modules) so a regression in either side
// shows up as a test failure, not silent drift.
const dockerFingerprint = "9DC858229FC7DD38854AE2D88D81803C0EBFCD88"

func requireGPG(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg is not installed on the test host")
	}
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestVerifyAndInstallKeyring_HappyPath(t *testing.T) {
	requireGPG(t)
	body := loadFixture(t, "docker.asc")
	dest := filepath.Join(t.TempDir(), "docker.asc")

	if err := verifyAndInstallKeyring(body, dest, dockerFingerprint); err != nil {
		t.Fatalf("verify+install: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatal("installed keyring bytes differ from fixture")
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("expected mode 0644, got %o", info.Mode().Perm())
	}
	// No stray staging file was left behind.
	if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("staging tmp file must not survive rename; err=%v", err)
	}
}

func TestVerifyAndInstallKeyring_RejectsWrongFingerprint(t *testing.T) {
	requireGPG(t)
	body := loadFixture(t, "docker.asc")
	dest := filepath.Join(t.TempDir(), "docker.asc")

	err := verifyAndInstallKeyring(body, dest, "0000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected fingerprint mismatch to fail")
	}
	if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("expected fingerprint-mismatch error, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("keyring must not be installed on mismatch; got err=%v", err)
	}
}

func TestVerifyAndInstallKeyring_AcceptsLowerCaseAndSpaces(t *testing.T) {
	requireGPG(t)
	body := loadFixture(t, "docker.asc")
	dest := filepath.Join(t.TempDir(), "docker.asc")

	// GPG default text format is separated by spaces; users copy-pasting it
	// must still match.
	spaced := "9dc8 5822 9fc7 dd38 854a e2d8 8d81 803c 0ebf cd88"
	if err := verifyAndInstallKeyring(body, dest, spaced); err != nil {
		t.Fatalf("spaced/lower-case fingerprint must be accepted: %v", err)
	}
}

func TestInstallAptKeyring_ValidatesInputs(t *testing.T) {
	cases := []struct {
		name, url, dest, fp, want string
	}{
		{"empty url", "", "/etc/apt/keyrings/x.asc", dockerFingerprint, "required"},
		{"http rejected", "http://example.com/gpg", "/etc/apt/keyrings/x.asc", dockerFingerprint, "non-https"},
		{"relative dest", "https://example.com/gpg", "relative.asc", dockerFingerprint, "non-absolute"},
		{"short fingerprint", "https://example.com/gpg", "/etc/apt/keyrings/x.asc", "DEADBEEF", "40-hex"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := InstallAptKeyring(context.Background(), c.url, c.dest, c.fp)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q lacks %q", err.Error(), c.want)
			}
		})
	}
}

func TestNormaliseFingerprint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"9dc858229fc7dd38854ae2d88d81803c0ebfcd88", dockerFingerprint},
		{"  9DC858229FC7DD38854AE2D88D81803C0EBFCD88  ", dockerFingerprint},
		{"9DC8 5822 9FC7 DD38 854A  E2D8 8D81 803C 0EBF CD88", dockerFingerprint},
	}
	for _, c := range cases {
		if got := normaliseFingerprint(c.in); got != c.want {
			t.Errorf("normalise(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}
