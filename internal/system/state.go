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
