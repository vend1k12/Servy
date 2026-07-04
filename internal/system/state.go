package system

import (
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/vend1k12/servy/internal/safepath"
)

type State interface {
	CommandExists(name string) bool
	FileExists(path string) bool
	UserExists(name string) bool
	GroupContainsUser(group, username string) bool
	SwapActive() bool
	ServiceActive(name string) bool
	// AptPackagesInstalled reports which of the given apt package names are
	// currently installed. Missing entries mean "not installed" (or unknown to
	// dpkg on this host). One batched dpkg-query per call.
	AptPackagesInstalled(names []string) map[string]bool
}

type RealState struct{}

func (RealState) CommandExists(name string) bool {
	_, err := safepath.LookPath(name)
	return err == nil
}

func (RealState) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (RealState) UserExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

func (RealState) GroupContainsUser(groupName, username string) bool {
	grp, err := user.LookupGroup(groupName)
	if err != nil {
		return false
	}
	b, err := os.ReadFile("/etc/group")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 4 && parts[2] == grp.Gid {
			for _, member := range strings.Split(parts[3], ",") {
				if member == username {
					return true
				}
			}
		}
	}
	return false
}

func (RealState) SwapActive() bool {
	b, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return len(lines) > 1
}

func (RealState) ServiceActive(name string) bool {
	systemctl, err := safepath.LookPath("systemctl")
	if err != nil {
		return false
	}
	cmd := exec.Command(systemctl, "is-active", "--quiet", name)
	return cmd.Run() == nil
}

func (RealState) AptPackagesInstalled(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	if len(names) == 0 {
		return out
	}
	dpkg, err := safepath.LookPath("dpkg-query")
	if err != nil {
		return out // no dpkg (not an apt host) — treat everything as unknown/missing.
	}
	args := append([]string{"-W", "-f", "${Package} ${db:Status-Status}\n"}, names...)
	cmd := exec.Command(dpkg, args...)
	// dpkg-query exits non-zero when any requested package is unknown to dpkg.
	// The stdout still contains rows for the ones it does know about, which is
	// exactly what we want. Discard stderr and the exit code.
	stdout, _ := cmd.Output()
	for _, line := range strings.Split(string(stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == "installed" {
			out[fields[0]] = true
		}
	}
	return out
}
