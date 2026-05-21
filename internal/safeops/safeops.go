package safeops

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

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
