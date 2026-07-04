package safeops

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// maxAptKeyringBytes bounds the keyring download; official Docker/Caddy
// keyrings are ~5 KiB. A hard cap keeps a hostile upstream from streaming
// gigabytes into a root-writable temp file.
const maxAptKeyringBytes = 1 << 20 // 1 MiB

// InstallAptKeyring downloads an apt keyring over HTTPS, verifies that the
// resulting keyring contains the caller-provided primary key fingerprint, and
// atomically installs it at destPath with mode 0644 owned by root:root.
//
// The GPG fingerprint pin is the only trust anchor: TLS alone is not enough
// for a keyring that will later authorise root packages.
func InstallAptKeyring(ctx context.Context, url, destPath, expectedFingerprint string) error {
	if err := validateKeyringInputs(url, destPath, expectedFingerprint); err != nil {
		return err
	}
	body, err := fetchKeyring(ctx, url)
	if err != nil {
		return err
	}
	return verifyAndInstallKeyring(body, destPath, expectedFingerprint)
}

// validateKeyringInputs enforces the InstallAptKeyring input contract without
// touching the network. Tests use it directly to lock the contract in place.
func validateKeyringInputs(url, destPath, expectedFingerprint string) error {
	if url == "" || destPath == "" || expectedFingerprint == "" {
		return errors.New("url, destPath and expectedFingerprint are required")
	}
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing non-https keyring URL %q", url)
	}
	if !filepath.IsAbs(destPath) {
		return fmt.Errorf("refusing non-absolute destination %q", destPath)
	}
	if len(normaliseFingerprint(expectedFingerprint)) != 40 {
		return fmt.Errorf("expected fingerprint must be a 40-hex GPG v4 fingerprint, got %q", expectedFingerprint)
	}
	return nil
}

// verifyAndInstallKeyring runs the trust check on already-downloaded bytes and
// atomically installs them at destPath. Split from InstallAptKeyring so unit
// tests can exercise the trust and file-mode invariants without a real HTTPS
// peer.
func verifyAndInstallKeyring(body []byte, destPath, expectedFingerprint string) error {
	expected := normaliseFingerprint(expectedFingerprint)

	// Land the raw bytes in a private tempdir so gpg can read them without
	// racing whatever else touches /etc/apt/keyrings.
	tmpDir, err := os.MkdirTemp("", "servy-keyring-*")
	if err != nil {
		return fmt.Errorf("create keyring tempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	rawPath := filepath.Join(tmpDir, "keyring.raw")
	if err := os.WriteFile(rawPath, body, 0o600); err != nil {
		return fmt.Errorf("write keyring tempfile: %w", err)
	}

	seen, err := gpgFingerprints(rawPath)
	if err != nil {
		return err
	}
	found := false
	for _, fp := range seen {
		if fp == expected {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("keyring fingerprint mismatch: wanted %s, got %v", expected, seen)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create keyring dir: %w", err)
	}
	// Two-step atomic install: write next to the target so rename is same-fs.
	stagePath := destPath + ".tmp"
	if err := os.WriteFile(stagePath, body, 0o644); err != nil {
		return fmt.Errorf("stage keyring: %w", err)
	}
	if err := os.Chmod(stagePath, 0o644); err != nil {
		_ = os.Remove(stagePath)
		return fmt.Errorf("chmod keyring: %w", err)
	}
	if err := os.Rename(stagePath, destPath); err != nil {
		_ = os.Remove(stagePath)
		return fmt.Errorf("install keyring at %s: %w", destPath, err)
	}
	return nil
}

func fetchKeyring(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build keyring request: %w", err)
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// Explicit TLS min version — Docker/Caddy/GitHub all support 1.2+.
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download keyring: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keyring download returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAptKeyringBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read keyring body: %w", err)
	}
	if len(body) > maxAptKeyringBytes {
		return nil, fmt.Errorf("keyring exceeds %d bytes; refusing", maxAptKeyringBytes)
	}
	return body, nil
}

// gpgFingerprints returns every primary and subkey fingerprint reported by
// `gpg --show-keys --with-colons` for the given file.
func gpgFingerprints(path string) ([]string, error) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		return nil, fmt.Errorf("gpg not available for keyring verification: %w", err)
	}
	// Isolate from the current user's keyring so verification is deterministic.
	homedir, err := os.MkdirTemp("", "servy-gpg-*")
	if err != nil {
		return nil, fmt.Errorf("gpg homedir: %w", err)
	}
	defer os.RemoveAll(homedir)
	cmd := exec.Command(gpg,
		"--homedir", homedir,
		"--no-default-keyring",
		"--batch",
		"--show-keys",
		"--with-colons",
		path,
	)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpg --show-keys failed: %w", err)
	}
	var fprs []string
	for _, line := range strings.Split(out.String(), "\n") {
		if !strings.HasPrefix(line, "fpr:") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 10 {
			continue
		}
		if fp := normaliseFingerprint(parts[9]); fp != "" {
			fprs = append(fprs, fp)
		}
	}
	if len(fprs) == 0 {
		return nil, errors.New("no GPG fingerprints extracted from keyring")
	}
	return fprs, nil
}

func normaliseFingerprint(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	// Accept the common "XXXX XXXX ..." human format too.
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func AppendAuthorizedKey(username, key string) error {
	if username == "" || key == "" || strings.ContainsAny(key, "\r\n") {
		return errors.New("user and single-line SSH public key are required")
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("parse uid: %w", err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("parse gid: %w", err)
	}
	home := filepath.Clean(u.HomeDir)
	if home == "/" || home == "." {
		return errors.New("unsafe home directory")
	}
	homeFD, err := unix.Open(home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open home safely: %w", err)
	}
	defer unix.Close(homeFD)

	sshFD, err := unix.Openat(homeFD, ".ssh", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("open .ssh safely: %w", err)
		}
		if err := unix.Mkdirat(homeFD, ".ssh", 0o700); err != nil {
			return fmt.Errorf("create .ssh: %w", err)
		}
		sshFD, err = unix.Openat(homeFD, ".ssh", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return fmt.Errorf("open created .ssh safely: %w", err)
		}
	}
	defer unix.Close(sshFD)
	if err := unix.Fchown(sshFD, uid, gid); err != nil {
		return fmt.Errorf("chown .ssh: %w", err)
	}
	if err := unix.Fchmod(sshFD, 0o700); err != nil {
		return fmt.Errorf("chmod .ssh: %w", err)
	}

	keyFD, err := unix.Openat(sshFD, "authorized_keys", unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open authorized_keys safely: %w", err)
	}
	f := os.NewFile(uintptr(keyFD), "authorized_keys")
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(keyFD, &st); err != nil {
		return fmt.Errorf("stat authorized_keys: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("refusing to modify non-regular authorized_keys")
	}
	if err := f.Chown(uid, gid); err != nil {
		return fmt.Errorf("chown authorized_keys: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod authorized_keys: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read authorized_keys: %w", err)
	}
	s := bufio.NewScanner(bytes.NewReader(content))
	for s.Scan() {
		if strings.TrimSpace(s.Text()) == key {
			return nil
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.Write([]byte("\n")); err != nil {
			return err
		}
	}
	_, err = f.Write([]byte(key + "\n"))
	return err
}

func WriteSSHDDropIn(line string) error {
	if strings.ContainsAny(line, "\r\n") || strings.TrimSpace(line) == "" {
		return errors.New("single SSH directive line is required")
	}
	dir := "/etc/ssh/sshd_config.d"
	path := filepath.Join(dir, "99-servy-hardening.conf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sshd_config.d: %w", err)
	}
	var original []byte
	existed := false
	if st, err := os.Lstat(path); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
			return errors.New("refusing to modify symlink or non-regular Servy sshd drop-in")
		}
		existed = true
		original, err = os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read existing sshd drop-in: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect sshd drop-in: %w", err)
	}
	for _, existing := range strings.Split(string(original), "\n") {
		if strings.TrimSpace(existing) == line {
			return reloadSSH()
		}
	}
	next := append([]byte{}, original...)
	if len(next) > 0 && next[len(next)-1] != '\n' {
		next = append(next, '\n')
	}
	next = append(next, []byte(line+"\n")...)
	if err := os.WriteFile(path, next, 0o644); err != nil {
		return fmt.Errorf("write sshd drop-in: %w", err)
	}
	if err := testSSHDConfig(); err != nil {
		if existed {
			_ = os.WriteFile(path, original, 0o644)
		} else {
			_ = os.Remove(path)
		}
		return err
	}
	return reloadSSH()
}

func testSSHDConfig() error {
	for _, path := range []string{"/usr/sbin/sshd", "/usr/bin/sshd", "/sbin/sshd"} {
		if _, err := os.Stat(path); err == nil {
			if out, err := exec.Command(path, "-t", "-f", "/etc/ssh/sshd_config").CombinedOutput(); err != nil {
				return fmt.Errorf("sshd config validation failed: %s", strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	return errors.New("sshd binary not found for config validation")
}

func reloadSSH() error {
	for _, systemctl := range []string{"/usr/bin/systemctl", "/bin/systemctl"} {
		if _, err := os.Stat(systemctl); err != nil {
			continue
		}
		for _, service := range []string{"ssh", "sshd"} {
			cmd := exec.Command(systemctl, "reload", service)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}
	return errors.New("failed to reload ssh/sshd service")
}
