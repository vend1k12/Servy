// Package doctor doctor runs read-only host diagnostics before any mutation.
package doctor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
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
	diskOK, diskInfo := diskStatus()
	add("disk", diskOK, diskInfo, fmt.Sprintf("free at least %s and %.0f%% of / before applying", humanBytes(minFreeDiskBytes), minFreeDiskFraction*100))
	memOK, memInfo := memStatus()
	add("memory", memOK, memInfo, fmt.Sprintf("free at least %s of RAM (add swap for smaller VPS)", humanBytes(minAvailMemBytes)))
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
	_ = conn.Close()
	return true
}

// Doctor thresholds. Values are conservative: enough to say "you have a real
// problem" without turning small VPS images into constant warnings.
const (
	minFreeDiskBytes    = 2 * 1024 * 1024 * 1024 // 2 GiB
	minFreeDiskFraction = 0.10                   // 10% of /
	minAvailMemBytes    = 512 * 1024 * 1024      // 512 MiB
)

// diskStatus reports whether / has enough free space, plus a human-readable
// details string. Falls back to the previous "df -h /" line when statfs is
// not available.
func diskStatus() (bool, string) {
	var s syscall.Statfs_t
	if err := syscall.Statfs("/", &s); err != nil {
		return false, err.Error()
	}
	// s.Bsize is signed on Linux (int64 on amd64, int32 on 32-bit archs).
	// A non-positive block size would be a kernel bug; refuse to fabricate a
	// disk-usage number from it instead of doing a hostile int -> uint cast.
	if s.Bsize <= 0 {
		return false, fmt.Sprintf("statfs reported non-positive block size %d", s.Bsize)
	}
	blockSize := uint64(s.Bsize)
	free := s.Bavail * blockSize
	total := s.Blocks * blockSize
	ok := free >= minFreeDiskBytes
	if total > 0 && float64(free)/float64(total) < minFreeDiskFraction {
		ok = false
	}
	details := fmt.Sprintf("/: free=%s total=%s", humanBytes(free), humanBytes(total))
	return ok, details
}

// memStatus reads MemAvailable from /proc/meminfo. Linux-only file; if it is
// missing we return true with a "unknown" note so non-Linux dev hosts running
// doctor for testing do not fail the check.
func memStatus() (bool, string) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return true, "meminfo unavailable: " + err.Error()
	}
	avail, total, ok := parseMemInfo(b)
	if !ok {
		return true, "meminfo unparseable"
	}
	return avail >= minAvailMemBytes, fmt.Sprintf("MemAvailable=%s MemTotal=%s", humanBytes(avail), humanBytes(total))
}

// parseMemInfo extracts MemAvailable and MemTotal (in bytes). Both fields are
// kB in /proc/meminfo. Returns ok=false if either field is missing.
func parseMemInfo(b []byte) (avail, total uint64, ok bool) {
	var haveAvail, haveTotal bool
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemAvailable":
			avail = val * 1024
			haveAvail = true
		case "MemTotal":
			total = val * 1024
			haveTotal = true
		}
	}
	return avail, total, haveAvail && haveTotal
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
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
