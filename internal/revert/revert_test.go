package revert

import (
	"strings"
	"testing"

	"github.com/vend1k12/servy/internal/manifest"
	"github.com/vend1k12/servy/internal/plan"
)

func TestBuild_EmptyRecordYieldsSingleSkip(t *testing.T) {
	p := Build("docker", manifest.ModuleRecord{}, Options{})
	if len(p.Steps) != 1 {
		t.Fatalf("Steps=%d, want 1 skip step", len(p.Steps))
	}
	if p.Steps[0].Status != plan.WillSkip {
		t.Errorf("status=%s, want WillSkip", p.Steps[0].Status)
	}
	if p.HasRunnableSteps() {
		t.Error("empty record must have no runnable steps")
	}
}

func TestBuild_DockerRevertOmitsPackagesByDefault(t *testing.T) {
	rec := manifest.ModuleRecord{
		AptKeyrings:          []string{"/etc/apt/keyrings/docker.asc"},
		AptLists:             []string{"/etc/apt/sources.list.d/docker.sources"},
		AptPackagesInstalled: []string{"docker-ce", "docker-ce-cli"},
		ServicesEnabled:      []string{"docker"},
	}
	p := Build("docker", rec, Options{PurgePackages: false})
	if hasCommand(p, "apt-get", "remove") {
		t.Error("default revert must not run apt-get remove")
	}
	if hasCommand(p, "systemctl", "disable") {
		t.Error("default revert must not disable services")
	}
	if !hasCommandArg(p, "/etc/apt/keyrings/docker.asc") {
		t.Error("default revert must remove the keyring file")
	}
	if !hasCommandArg(p, "/etc/apt/sources.list.d/docker.sources") {
		t.Error("default revert must remove the apt list file")
	}
	if !hasCommand(p, "apt-get", "update") {
		t.Error("default revert must refresh apt after removing lists")
	}
}

func TestBuild_DockerRevertPurgesPackages(t *testing.T) {
	rec := manifest.ModuleRecord{
		AptPackagesInstalled: []string{"docker-ce", "docker-ce-cli", "containerd.io"},
		ServicesEnabled:      []string{"docker"},
	}
	p := Build("docker", rec, Options{PurgePackages: true})
	if !hasCommand(p, "systemctl", "disable") {
		t.Error("--purge-packages must disable services")
	}
	if !hasCommand(p, "apt-get", "remove") {
		t.Error("--purge-packages must run apt-get remove")
	}
	if !hasCommandArg(p, "docker-ce") || !hasCommandArg(p, "containerd.io") {
		t.Error("purge must include every recorded package")
	}
}

func TestBuild_HardeningRevertRemovesDropInsAndSysctl(t *testing.T) {
	rec := manifest.ModuleRecord{
		SysctlDropIns:   []string{"/etc/sysctl.d/99-servy.conf"},
		SSHDDropInLines: []string{"PermitRootLogin no", "PasswordAuthentication no"},
	}
	p := Build("hardening", rec, Options{})
	if !hasCommandArg(p, "/etc/sysctl.d/99-servy.conf") {
		t.Error("must remove sysctl drop-in file")
	}
	if !hasCommand(p, "sysctl", "--system") {
		t.Error("must reload sysctl after removing drop-in")
	}
	// sshd revert routes through the hidden internal subcommand.
	foundSSH := false
	for _, s := range p.Steps {
		if len(s.Command) >= 3 && s.Command[1] == "internal" && s.Command[2] == "remove-sshd-dropin-lines" {
			foundSSH = true
			// exact directive lines must reach the subcommand argv.
			joined := strings.Join(s.Command, " ")
			if !strings.Contains(joined, "PermitRootLogin no") || !strings.Contains(joined, "PasswordAuthentication no") {
				t.Errorf("sshd step missing directive lines: %v", s.Command)
			}
		}
	}
	if !foundSSH {
		t.Error("hardening revert must produce a remove-sshd-dropin-lines step")
	}
}

func TestBuild_SwapRevertOffAndRemove(t *testing.T) {
	rec := manifest.ModuleRecord{
		SwapFilePath: "/swapfile",
		FstabLine:    "/swapfile none swap sw 0 0",
	}
	p := Build("swap", rec, Options{})
	if !hasCommand(p, "swapoff", "/swapfile") {
		t.Error("must swapoff first")
	}
	// swapoff must precede rm of the file.
	swapOffIdx := indexOfCommand(p, "swapoff")
	rmIdx := indexOfCommandArg(p, "/swapfile")
	if swapOffIdx < 0 || rmIdx < 0 || swapOffIdx > rmIdx {
		t.Errorf("expected swapoff (%d) before rm (%d) of swapfile", swapOffIdx, rmIdx)
	}
	// fstab step goes through the hidden internal subcommand.
	found := false
	for _, s := range p.Steps {
		if len(s.Command) >= 3 && s.Command[1] == "internal" && s.Command[2] == "remove-fstab-line" {
			found = true
		}
	}
	if !found {
		t.Error("swap revert must produce a remove-fstab-line step")
	}
}

func TestBuild_UsersAndGroupsAreSkipped(t *testing.T) {
	rec := manifest.ModuleRecord{
		UsersCreated: []string{"deploy"},
		GroupsJoined: map[string][]string{"docker": {"deploy"}},
	}
	p := Build("deploy-user", rec, Options{PurgePackages: true})
	skipCount := 0
	for _, s := range p.Steps {
		if s.Status == plan.WillSkip {
			skipCount++
		}
	}
	if skipCount < 2 {
		t.Errorf("expected 2 skip steps (users + groups), got %d", skipCount)
	}
	if p.HasRunnableSteps() {
		t.Error("v1 revert of user-only record must have no runnable steps")
	}
}

func TestSafeName_StripsSlashesAndDots(t *testing.T) {
	cases := map[string]string{
		"/etc/apt/keyrings/docker.asc":            "etc-apt-keyrings-docker-asc",
		"/etc/sysctl.d/99-servy.conf":             "etc-sysctl-d-99-servy-conf",
		"/etc/apt/sources.list.d/github-cli.list": "etc-apt-sources-list-d-github-cli-list",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := safeName(""); got != "unknown" {
		t.Errorf("safeName(\"\") = %q, want %q", got, "unknown")
	}
}

// hasCommand reports whether any WillRun step's argv starts with the given
// prefix.
func hasCommand(p plan.Plan, prefix ...string) bool {
	for _, s := range p.Steps {
		if s.Status != plan.WillRun || len(s.Command) < len(prefix) {
			continue
		}
		match := true
		for i, w := range prefix {
			if s.Command[i] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// hasCommandArg reports whether any step's argv contains the given argument.
func hasCommandArg(p plan.Plan, arg string) bool {
	for _, s := range p.Steps {
		for _, a := range s.Command {
			if a == arg {
				return true
			}
		}
	}
	return false
}

func indexOfCommand(p plan.Plan, head string) int {
	for i, s := range p.Steps {
		if len(s.Command) > 0 && s.Command[0] == head {
			return i
		}
	}
	return -1
}

func indexOfCommandArg(p plan.Plan, arg string) int {
	for i, s := range p.Steps {
		for _, a := range s.Command {
			if a == arg {
				return i
			}
		}
	}
	return -1
}
