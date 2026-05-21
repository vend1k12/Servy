package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const Value = "/usr/sbin:/usr/bin:/sbin:/bin"

var allowedAbsolute = map[string]bool{
	"/bin/sh":        true,
	"/usr/bin/sh":    true,
	"/proc/self/exe": true,
}

func LookPath(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		if !filepath.IsAbs(name) {
			return "", fmt.Errorf("refusing relative executable path %q", name)
		}
		if name == "/proc/self/exe" {
			return name, nil
		}
		if !allowedAbsolute[name] {
			return "", fmt.Errorf("refusing non-allowlisted absolute executable path %q", name)
		}
		return verifyExecutable(name)
	}
	for _, dir := range filepath.SplitList(Value) {
		candidate := filepath.Join(dir, name)
		if resolved, err := verifyExecutable(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("executable %q not found in safe PATH", name)
}

func Env() []string {
	allow := map[string]bool{"HOME": true, "LANG": true, "LC_ALL": true, "TERM": true, "USER": true, "LOGNAME": true, "SHELL": true, "SSH_CONNECTION": true}
	out := []string{"PATH=" + Value, "DEBIAN_FRONTEND=noninteractive"}
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if ok && allow[name] && name != "PATH" {
			out = append(out, kv)
		}
	}
	return out
}

func verifyExecutable(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.IsDir() || st.Mode()&0o111 == 0 || st.Mode()&0o022 != 0 {
		return "", fmt.Errorf("unsafe executable permissions for %s", path)
	}
	if err := verifyRootOwned(path); err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		if err := verifyRootOwned(resolved); err != nil {
			return "", err
		}
		for dir := filepath.Dir(resolved); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			if err := verifyRootOwned(dir); err != nil {
				return "", err
			}
		}
	}
	for dir := filepath.Dir(path); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if err := verifyRootOwned(dir); err != nil {
			return "", err
		}
	}
	return path, nil
}

func verifyRootOwned(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink == 0 && st.Mode()&0o022 != 0 {
		return fmt.Errorf("unsafe writable path component %s", path)
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if ok && stat.Uid != 0 {
		return fmt.Errorf("path component %s is not root-owned", path)
	}
	return nil
}
