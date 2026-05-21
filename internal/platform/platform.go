package platform

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"github.com/vend1k12/servy/internal/safepath"
)

type Info struct {
	ID              string
	Name            string
	VersionID       string
	VersionCodename string
	UbuntuCodename  string
	Arch            string
	PackageManager  string
	HasSystemd      bool
	CurrentUser     string
	UID             string
	IsRoot          bool
	HasSudo         bool
	SSHPorts        []int
}

type Detector struct {
	OSReleasePath string
}

func Detect() (Info, error) { return Detector{OSReleasePath: "/etc/os-release"}.Detect() }

func (d Detector) Detect() (Info, error) {
	path := d.OSReleasePath
	if path == "" {
		path = "/etc/os-release"
	}
	vars, err := parseOSRelease(path)
	if err != nil {
		return Info{}, err
	}
	usr, _ := user.Current()
	uid := os.Geteuid()
	info := Info{
		ID:              strings.ToLower(vars["ID"]),
		Name:            vars["NAME"],
		VersionID:       vars["VERSION_ID"],
		VersionCodename: vars["VERSION_CODENAME"],
		UbuntuCodename:  vars["UBUNTU_CODENAME"],
		Arch:            detectArch(),
		HasSystemd:      exists("/run/systemd/system"),
		IsRoot:          uid == 0,
	}
	if usr != nil {
		info.CurrentUser = usr.Username
		info.UID = usr.Uid
	}
	if _, err := safepath.LookPath("apt-get"); err == nil {
		info.PackageManager = "apt"
	}
	if _, err := safepath.LookPath("sudo"); err == nil || info.IsRoot {
		info.HasSudo = true
	}
	info.SSHPorts = detectSSHPorts()
	return info, nil
}

func parseOSRelease(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read os-release: %w", err)
	}
	vars := make(map[string]string)
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		vars[parts[0]] = strings.Trim(parts[1], "\"")
	}
	return vars, s.Err()
}

func detectArch() string {
	if dpkg, err := safepath.LookPath("dpkg"); err == nil {
		if out, err := exec.Command(dpkg, "--print-architecture").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armhf"
	default:
		return runtime.GOARCH
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (i Info) DockerCodename() string {
	if i.ID == "ubuntu" && i.UbuntuCodename != "" {
		return i.UbuntuCodename
	}
	return i.VersionCodename
}

func detectSSHPorts() []int {
	seen := map[int]bool{}
	var ports []int
	add := func(port int) {
		if port > 0 && port <= 65535 && !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	if fields := strings.Fields(os.Getenv("SSH_CONNECTION")); len(fields) >= 4 {
		var port int
		if _, err := fmt.Sscanf(fields[3], "%d", &port); err == nil {
			add(port)
		}
	}
	if b, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			trim := strings.TrimSpace(line)
			if trim == "" || strings.HasPrefix(trim, "#") {
				continue
			}
			fields := strings.Fields(trim)
			if len(fields) == 2 && strings.EqualFold(fields[0], "Port") {
				var port int
				if _, err := fmt.Sscanf(fields[1], "%d", &port); err == nil {
					add(port)
				}
			}
		}
	}
	if len(ports) == 0 {
		ports = append(ports, 22)
	}
	return ports
}

func (i Info) Supported() (bool, string) {
	if i.PackageManager != "apt" {
		return false, "apt package manager is required"
	}
	if !i.HasSystemd {
		return false, "systemd is required"
	}
	if !i.HasSudo {
		return false, "root or sudo is required"
	}
	if !contains([]string{"amd64", "arm64"}, i.Arch) {
		return false, "only amd64 and arm64 are supported by this MVP"
	}
	switch i.ID {
	case "ubuntu":
		if contains([]string{"jammy", "noble", "resolute"}, i.DockerCodename()) {
			return true, ""
		}
		return false, "supported Ubuntu codenames are jammy (22.04), noble (24.04), resolute (26.04 LTS)"
	case "debian":
		if contains([]string{"bookworm", "trixie"}, i.VersionCodename) {
			return true, ""
		}
		return false, "supported Debian codenames are bookworm (12) and trixie (13)"
	default:
		return false, "only Ubuntu LTS and Debian stable are supported"
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
