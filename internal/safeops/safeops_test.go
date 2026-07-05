package safeops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
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

func openDir(t *testing.T, path string) int {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	return fd
}

func TestReadDropIn_MissingIsNotAnError(t *testing.T) {
	dirFD := openDir(t, t.TempDir())
	body, existed, err := readDropIn(dirFD, "99-servy-hardening.conf")
	if err != nil {
		t.Fatalf("readDropIn: %v", err)
	}
	if existed {
		t.Fatalf("existed=true for missing file")
	}
	if len(body) != 0 {
		t.Fatalf("body=%q, want empty", body)
	}
}

func TestReadDropIn_ReadsRegularFile(t *testing.T) {
	dir := t.TempDir()
	want := []byte("PermitRootLogin no\n")
	if err := os.WriteFile(filepath.Join(dir, "99-servy-hardening.conf"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	body, existed, err := readDropIn(openDir(t, dir), "99-servy-hardening.conf")
	if err != nil {
		t.Fatalf("readDropIn: %v", err)
	}
	if !existed {
		t.Fatalf("existed=false for regular file")
	}
	if string(body) != string(want) {
		t.Fatalf("body=%q, want %q", body, want)
	}
}

func TestReadDropIn_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside")
	if err := os.WriteFile(target, []byte("PermitRootLogin no\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "99-servy-hardening.conf")); err != nil {
		t.Fatal(err)
	}
	_, _, err := readDropIn(openDir(t, dir), "99-servy-hardening.conf")
	if err == nil {
		t.Fatal("expected error for symlinked drop-in")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error %q does not mention symlink", err.Error())
	}
}

func TestReadDropIn_RefusesNonRegular(t *testing.T) {
	dir := t.TempDir()
	// FIFO — non-regular but not a symlink, so O_NOFOLLOW opens it.
	if err := unix.Mkfifo(filepath.Join(dir, "99-servy-hardening.conf"), 0o644); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	// Open in nonblocking mode to avoid readDropIn blocking on the FIFO.
	// readDropIn uses O_RDONLY without O_NONBLOCK, so open the writer side
	// first from a goroutine and close it — that yields EOF on the reader.
	go func() {
		w, err := os.OpenFile(filepath.Join(dir, "99-servy-hardening.conf"), os.O_WRONLY, 0)
		if err == nil {
			_ = w.Close()
		}
	}()
	_, _, err := readDropIn(openDir(t, dir), "99-servy-hardening.conf")
	if err == nil {
		t.Fatal("expected error for FIFO drop-in")
	}
	if !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("error %q does not mention non-regular", err.Error())
	}
}

func TestWriteDropInAtomic_CreatesFileWithMode0644(t *testing.T) {
	dir := t.TempDir()
	body := []byte("PermitRootLogin no\n")
	if err := writeDropInAtomic(openDir(t, dir), "99-servy-hardening.conf", ".99-servy-hardening.conf.tmp", body); err != nil {
		t.Fatalf("writeDropInAtomic: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "99-servy-hardening.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("body=%q, want %q", got, body)
	}
	info, err := os.Stat(filepath.Join(dir, "99-servy-hardening.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode=%o, want 0644", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, ".99-servy-hardening.conf.tmp")); !os.IsNotExist(err) {
		t.Fatalf("stage file survived rename; err=%v", err)
	}
}

func TestWriteDropInAtomic_ReplacesExistingContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "99-servy-hardening.conf"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte("PermitRootLogin no\n")
	if err := writeDropInAtomic(openDir(t, dir), "99-servy-hardening.conf", ".99-servy-hardening.conf.tmp", body); err != nil {
		t.Fatalf("writeDropInAtomic: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "99-servy-hardening.conf"))
	if string(got) != string(body) {
		t.Fatalf("body=%q, want %q", got, body)
	}
}

func TestWriteDropInAtomic_RefusesSymlinkedStage(t *testing.T) {
	// A pre-existing symlink at the stage path must be cleared safely and
	// the fresh stage must be created via O_NOFOLLOW|O_EXCL so a race that
	// re-plants the symlink between unlink and create fails cleanly.
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".99-servy-hardening.conf.tmp")); err != nil {
		t.Fatal(err)
	}
	// writeDropInAtomic unlinks the stale stage (symlink) then creates fresh.
	body := []byte("PermitRootLogin no\n")
	if err := writeDropInAtomic(openDir(t, dir), "99-servy-hardening.conf", ".99-servy-hardening.conf.tmp", body); err != nil {
		t.Fatalf("writeDropInAtomic: %v", err)
	}
	// Victim file outside the drop-in dir must be untouched.
	got, _ := os.ReadFile(outside)
	if string(got) != "original\n" {
		t.Fatalf("symlink target was clobbered; got %q", got)
	}
	inside, _ := os.ReadFile(filepath.Join(dir, "99-servy-hardening.conf"))
	if string(inside) != string(body) {
		t.Fatalf("drop-in content=%q, want %q", inside, body)
	}
}

func TestWriteSSHDDropIn_RejectsMultilineDirective(t *testing.T) {
	for _, bad := range []string{"", "  ", "PermitRootLogin\nno", "PermitRootLogin\rno"} {
		err := WriteSSHDDropIn(bad)
		if err == nil {
			t.Fatalf("expected error for %q", bad)
		}
		if !strings.Contains(err.Error(), "single SSH directive") {
			t.Fatalf("error %q lacks directive validation message", err.Error())
		}
	}
}
