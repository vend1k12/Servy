package doctor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/vend1k12/servy/internal/platform"
	"github.com/vend1k12/servy/internal/safepath"
	"github.com/vend1k12/servy/internal/system"
)

type Check struct {
	Name    string
	Status  string
	Details string
	Fix     string
}

func Run(osInfo platform.Info, st system.State) []Check {
	var checks []Check
	add := func(name string, ok bool, details, fix string) {
		status := "ok"
		if !ok {
			status = "warn"
		}
		checks = append(checks, Check{Name: name, Status: status, Details: details, Fix: fix})
	}
	ok, reason := osInfo.Supported()
	add("os", ok, fmt.Sprintf("%s %s %s %s", osInfo.ID, osInfo.VersionID, osInfo.DockerCodename(), reason), "run Servy on supported Ubuntu/Debian releases")
	add("architecture", osInfo.Arch == "amd64" || osInfo.Arch == "arm64", osInfo.Arch, "use a linux/amd64 or linux/arm64 host")
	add("root-or-sudo", osInfo.IsRoot || osInfo.HasSudo, fmt.Sprintf("user=%s root=%t sudo=%t", osInfo.CurrentUser, osInfo.IsRoot, osInfo.HasSudo), "run as root or install sudo for the current user")
	add("apt", osInfo.PackageManager == "apt" && st.CommandExists("apt-get"), osInfo.PackageManager, "use an apt-based Ubuntu/Debian host")
	add("dns", dnsOK(), "resolve docker.com", "fix DNS resolution before installing remote repositories")
	add("docker-repo-network", tcpOK("download.docker.com:443"), "tcp download.docker.com:443", "allow outbound HTTPS to download.docker.com")
	add("github-cli-repo-network", tcpOK("cli.github.com:443"), "tcp cli.github.com:443", "allow outbound HTTPS to cli.github.com")
	add("systemd", osInfo.HasSystemd, strconv.FormatBool(osInfo.HasSystemd), "use a systemd-based VPS image")
	add("disk", diskOK(), diskDetails(), "free disk space on /")
	add("memory", memOK(), memDetails(), "add memory or swap before applying larger profiles")
	add("ports", true, occupiedPorts(), "")
	add("docker", st.CommandExists("docker"), commandVersion("docker", "--version"), "enable the docker module and run `servy apply`")
	add("caddy", st.CommandExists("caddy"), commandVersion("caddy", "version"), "set modules.caddy.mode to host if host-level Caddy is required")
	add("firewall", st.CommandExists("ufw"), commandVersion("ufw", "status"), "enable the firewall module if UFW is required")
	add("ssh", st.FileExists("/etc/ssh/sshd_config"), sshHints(), "install or repair OpenSSH server before SSH hardening")
	return checks
}

func dnsOK() bool {
	_, err := net.LookupHost("docker.com")
	return err == nil
}

func tcpOK(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func diskOK() bool { return true }
func diskDetails() string {
	out, err := safeOutput("df", "-h", "/")
	if err != nil {
		return err.Error()
	}
	return compact(string(out))
}

func memOK() bool { return true }
func memDetails() string {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return err.Error()
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > 2 {
		lines = lines[:2]
	}
	return strings.Join(lines, "; ")
}

func occupiedPorts() string {
	out, err := safeOutput("ss", "-tulpn")
	if err == nil {
		return compact(string(out))
	}
	out, err = safeOutput("netstat", "-tulpn")
	if err == nil {
		return compact(string(out))
	}
	return "ss/netstat unavailable"
}

func commandVersion(name string, args ...string) string {
	out, err := safeOutput(name, args...)
	if err != nil {
		return err.Error()
	}
	return compact(string(out))
}

func safeOutput(name string, args ...string) ([]byte, error) {
	path, err := safepath.LookPath(name)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	cmd.Env = safepath.Env()
	return cmd.CombinedOutput()
}

func sshHints() string {
	b, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		return err.Error()
	}
	var hints []string
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trim), "port ") || strings.HasPrefix(strings.ToLower(trim), "permitrootlogin ") || strings.HasPrefix(strings.ToLower(trim), "passwordauthentication ") {
			hints = append(hints, trim)
		}
	}
	if len(hints) == 0 {
		return "no explicit Port/PermitRootLogin/PasswordAuthentication in main sshd_config"
	}
	return strings.Join(hints, "; ")
}

func compact(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
