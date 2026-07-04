package modules

import (
	"strings"
	"testing"

	"github.com/vend1k12/servy/internal/config"
	"github.com/vend1k12/servy/internal/plan"
	"github.com/vend1k12/servy/internal/platform"
)

type fakeState struct {
	commands map[string]bool
	files    map[string]bool
	users    map[string]bool
	groups   map[string]map[string]bool
	swap     bool
	services map[string]bool
	apt      map[string]bool
}

func (f fakeState) CommandExists(name string) bool                { return f.commands[name] }
func (f fakeState) FileExists(path string) bool                   { return f.files[path] }
func (f fakeState) UserExists(name string) bool                   { return f.users[name] }
func (f fakeState) GroupContainsUser(group, username string) bool { return f.groups[group][username] }
func (f fakeState) SwapActive() bool                              { return f.swap }
func (f fakeState) ServiceActive(name string) bool                { return f.services[name] }
func (f fakeState) AptPackagesInstalled(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		if f.apt[n] {
			out[n] = true
		}
	}
	return out
}

func supportedOS() platform.Info {
	return platform.Info{ID: "ubuntu", VersionID: "24.04", VersionCodename: "noble", UbuntuCodename: "noble", Arch: "amd64", PackageManager: "apt", HasSystemd: true, IsRoot: true, HasSudo: true}
}

func TestBasePlansServerEssentialsAndGitHubCLI(t *testing.T) {
	cfg := config.Default("base")
	p := Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{commands: map[string]bool{}, services: map[string]bool{}}})
	assertStep(t, p, "base.packages", plan.WillRun)
	assertStep(t, p, "base.gh.keyring", plan.WillRun)
	assertStep(t, p, "base.gh.repo", plan.WillRun)
	assertStep(t, p, "base.gh.install", plan.WillRun)
	for _, pkg := range []string{"git", "jq", "unzip", "rsync", "tmux", "htop", "nano"} {
		if !hasCommandArg(p, pkg) {
			t.Fatalf("base packages should include %s", pkg)
		}
	}
}

func TestBaseSkipsGitHubCLIRepoWhenPresent(t *testing.T) {
	cfg := config.Default("base")
	p := Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{commands: map[string]bool{"gh": true}, services: map[string]bool{}}})
	assertStep(t, p, "base.gh.present", plan.AlreadyOK)
	if findStep(p, "base.gh.install") != nil {
		t.Fatal("existing GitHub CLI should not be reinstalled")
	}
}

func TestBaseMarksInstalledPackagesAsAlreadyOK(t *testing.T) {
	cfg := config.Default("base")
	// Simulate every hardcoded base package already installed via apt.
	allInstalled := map[string]bool{}
	for _, p := range []string{"ca-certificates", "curl", "gnupg", "lsb-release", "apt-transport-https", "git", "unzip", "jq", "htop", "tmux", "rsync", "nano"} {
		allInstalled[p] = true
	}
	p := Build(Context{
		Config: cfg,
		OS:     supportedOS(),
		State:  fakeState{commands: map[string]bool{"gh": true}, apt: allInstalled, services: map[string]bool{}},
	})
	assertStep(t, p, "base.packages.present", plan.AlreadyOK)
	if findStep(p, "base.apt.update") != nil {
		t.Fatal("apt-get update must be skipped when nothing needs installing")
	}
	if findStep(p, "base.packages") != nil {
		t.Fatal("apt-get install must be skipped when everything is already installed")
	}
}

func TestBaseInstallsOnlyMissingPackages(t *testing.T) {
	cfg := config.Default("base")
	// Simulate two packages missing.
	installed := map[string]bool{}
	for _, p := range []string{"ca-certificates", "curl", "gnupg", "lsb-release", "apt-transport-https", "git", "unzip", "jq", "htop", "rsync"} {
		installed[p] = true
	}
	p := Build(Context{
		Config: cfg,
		OS:     supportedOS(),
		State:  fakeState{commands: map[string]bool{"gh": true}, apt: installed, services: map[string]bool{}},
	})
	step := findStep(p, "base.packages")
	if step == nil {
		t.Fatal("base.packages missing")
	}
	// argv is [apt-get install -y <missing…>]. Only tmux + nano should be there.
	got := strings.Join(step.Command, " ")
	for _, m := range []string{"tmux", "nano"} {
		if !strings.Contains(got, " "+m) {
			t.Errorf("expected missing package %s in install cmd, got %q", m, got)
		}
	}
	for _, present := range []string{"curl", "jq", "htop"} {
		if strings.Contains(got, " "+present+" ") || strings.HasSuffix(got, " "+present) {
			t.Errorf("already-installed package %s must not appear in install cmd: %q", present, got)
		}
	}
}

func TestDockerOnlyPlansOfficialDockerInstall(t *testing.T) {
	cfg := config.Default("docker-only")
	p := Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{commands: map[string]bool{}, services: map[string]bool{}}})
	assertStep(t, p, "docker.keyring.install", plan.WillRun)
	assertStep(t, p, "docker.repo", plan.WillRun)
	assertStep(t, p, "docker.install", plan.WillRun)
	if hasCommandArg(p, "snap") || hasCommandArg(p, "docker-compose") {
		t.Fatal("Docker plan must not use snap or docker-compose v1")
	}
}

func TestFirewallEnableRequiresExplicitConfirmation(t *testing.T) {
	cfg := config.Default("base")
	cfg.Modules.Firewall.Enabled = true
	p := Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{commands: map[string]bool{"ufw": true}}})
	assertStep(t, p, "firewall.allow.ssh.22", plan.WillRun)
	assertStep(t, p, "firewall.enable", plan.NeedsConfirmation)

	cfg.Confirmations.EnableFirewall = true
	p = Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{commands: map[string]bool{"ufw": true}}})
	assertStep(t, p, "firewall.enable", plan.WillRun)
}

func TestDangerousSSHHardeningRequiresOwnConfirmation(t *testing.T) {
	cfg := config.Default("base")
	cfg.Modules.Hardening.DisableRootSSHLogin = true
	p := Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{}})
	assertStep(t, p, "hardening.disable-root", plan.NeedsConfirmation)

	cfg.Confirmations.DisableRootSSHLogin = true
	p = Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{}})
	assertStep(t, p, "hardening.disable-root", plan.WillRun)
	if step := findStep(p, "hardening.disable-root"); step == nil || len(step.Command) == 0 {
		t.Fatal("confirmed hardening step must have an executable command")
	}
}

func TestExistingCaddyIsNotOverwritten(t *testing.T) {
	cfg := config.Default("base")
	cfg.Modules.Caddy.Mode = "host"
	p := Build(Context{Config: cfg, OS: supportedOS(), State: fakeState{commands: map[string]bool{"caddy": true}}})
	assertStep(t, p, "caddy.present", plan.AlreadyOK)
	if findStep(p, "caddy.install") != nil {
		t.Fatal("existing Caddy must not be reinstalled or overwritten")
	}
}

func TestUnsupportedOSBlocksBeforeMutation(t *testing.T) {
	cfg := config.Default("docker-only")
	osInfo := supportedOS()
	osInfo.ID = "fedora"
	p := Build(Context{Config: cfg, OS: osInfo, State: fakeState{}})
	assertStep(t, p, "platform.support", plan.Unsupported)
	if len(p.Steps) != 1 {
		t.Fatalf("unsupported OS should stop planning before module steps; got %d steps", len(p.Steps))
	}
}

func assertStep(t *testing.T, p plan.Plan, id string, status plan.Status) {
	t.Helper()
	step := findStep(p, id)
	if step == nil {
		t.Fatalf("missing step %s", id)
	}
	if step.Status != status {
		t.Fatalf("step %s status = %s, want %s", id, step.Status, status)
	}
}

func findStep(p plan.Plan, id string) *plan.Step {
	for i := range p.Steps {
		if p.Steps[i].ID == id {
			return &p.Steps[i]
		}
	}
	return nil
}

func hasCommandArg(p plan.Plan, forbidden string) bool {
	for _, step := range p.Steps {
		for _, arg := range step.Command {
			if arg == forbidden {
				return true
			}
		}
	}
	return false
}
