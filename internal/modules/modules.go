package modules

import (
	"fmt"
	"strings"

	"github.com/vend1k12/servy/internal/config"
	"github.com/vend1k12/servy/internal/plan"
	"github.com/vend1k12/servy/internal/platform"
	"github.com/vend1k12/servy/internal/system"
)

type Context struct {
	Config     config.Config
	OS         platform.Info
	State      system.State
	BinaryPath string
}

type Module interface {
	Name() string
	Plan(Context) []plan.Step
}

func Registry() []Module {
	return []Module{Base{}, Docker{}, DeployUser{}, Firewall{}, Swap{}, Hardening{}, Node{}, Caddy{}}
}

func Build(ctx Context) plan.Plan {
	p := plan.Plan{Profile: ctx.Config.Profile}
	if ok, reason := ctx.OS.Supported(); !ok {
		p.Add(plan.Step{ID: "platform.support", Module: "platform", Description: "verify supported Ubuntu/Debian host", Status: plan.Unsupported, Rationale: reason})
		return p
	}
	for _, module := range Registry() {
		for _, step := range module.Plan(ctx) {
			p.Add(step)
		}
	}
	return p
}

func ModuleNames() []string {
	mods := Registry()
	names := make([]string, 0, len(mods))
	for _, m := range mods {
		names = append(names, m.Name())
	}
	return names
}

type Base struct{}

func (Base) Name() string { return "base" }
func (Base) Plan(ctx Context) []plan.Step {
	steps := []plan.Step{
		{ID: "base.apt.update", Module: "base", Description: "update apt package index", Status: plan.WillRun, Command: []string{"apt-get", "update"}},
		{ID: "base.packages", Module: "base", Description: "install base server packages", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "ca-certificates", "curl", "gnupg", "lsb-release", "apt-transport-https", "git", "unzip", "jq", "htop", "tmux", "rsync", "nano"}},
	}
	if ctx.State.CommandExists("gh") {
		return append(steps, plan.Step{ID: "base.gh.present", Module: "base", Description: "GitHub CLI is already installed", Status: plan.AlreadyOK, Command: []string{"gh", "--version"}})
	}
	repo := fmt.Sprintf("deb [arch=%s signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n", ctx.OS.Arch)
	writeRepo := fmt.Sprintf("printf %%s %s > /etc/apt/sources.list.d/github-cli.list", shellArg(repo))
	steps = append(steps,
		plan.Step{ID: "base.gh.keyring.dir", Module: "base", Description: "create apt keyring directory for GitHub CLI", Status: plan.WillRun, Command: []string{"install", "-m", "0755", "-d", "/etc/apt/keyrings"}},
		plan.Step{ID: "base.gh.keyring", Module: "base", Description: "download and verify official GitHub CLI apt keyring", Status: plan.WillRun, Command: []string{"/bin/sh", "-c", "tmpdir=$(mktemp -d) && trap 'rm -rf \"$tmpdir\"' EXIT INT TERM && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o \"$tmpdir/githubcli-archive-keyring.gpg\" && printf '%s  %s\\n' 6084d5d7bd8e288441e0e94fc6275570895da18e6751f70f057485dc2d1a811b \"$tmpdir/githubcli-archive-keyring.gpg\" | sha256sum -c - && install -m 0644 \"$tmpdir/githubcli-archive-keyring.gpg\" /etc/apt/keyrings/githubcli-archive-keyring.gpg"}, Rationale: "GitHub CLI official docs publish this SHA256 for the binary keyring"},
		plan.Step{ID: "base.gh.repo", Module: "base", Description: "add official GitHub CLI apt repository", Status: plan.WillRun, Command: []string{"/bin/sh", "-c", writeRepo}},
		plan.Step{ID: "base.gh.apt.update", Module: "base", Description: "refresh apt after adding GitHub CLI repository", Status: plan.WillRun, Command: []string{"apt-get", "update"}},
		plan.Step{ID: "base.gh.install", Module: "base", Description: "install GitHub CLI", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "gh"}},
	)
	return steps
}

type Docker struct{}

func (Docker) Name() string { return "docker" }
func (Docker) Plan(ctx Context) []plan.Step {
	if !ctx.Config.Modules.Docker.Enabled {
		return []plan.Step{{ID: "docker.skip", Module: "docker", Description: "Docker module disabled", Status: plan.WillSkip}}
	}
	if ctx.State.CommandExists("docker") {
		steps := []plan.Step{{ID: "docker.present", Module: "docker", Description: "Docker CLI is already installed", Status: plan.AlreadyOK, Command: []string{"docker", "--version"}}}
		if ctx.State.ServiceActive("docker") {
			steps = append(steps, plan.Step{ID: "docker.service", Module: "docker", Description: "Docker service is active", Status: plan.AlreadyOK})
		} else {
			steps = append(steps, plan.Step{ID: "docker.service.start", Module: "docker", Description: "start and enable Docker service", Status: plan.WillRun, Command: []string{"systemctl", "enable", "--now", "docker"}})
		}
		return steps
	}
	repo := fmt.Sprintf("Types: deb\nURIs: https://download.docker.com/linux/%s\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n", ctx.OS.ID, ctx.OS.DockerCodename(), ctx.OS.Arch)
	writeRepo := fmt.Sprintf("printf %%s %s > /etc/apt/sources.list.d/docker.sources", shellArg(repo))
	return []plan.Step{
		{ID: "docker.keyring.dir", Module: "docker", Description: "create apt keyring directory", Status: plan.WillRun, Command: []string{"install", "-m", "0755", "-d", "/etc/apt/keyrings"}},
		{ID: "docker.keyring.install", Module: "docker", Description: "download Docker apt keyring and verify pinned GPG fingerprint", Status: plan.WillRun, Command: []string{selfBinary(ctx), "internal", "install-apt-keyring", "--url", "https://download.docker.com/linux/" + ctx.OS.ID + "/gpg", "--dest", "/etc/apt/keyrings/docker.asc", "--fingerprint", dockerGPGFingerprint}, Rationale: "verify against pinned Docker Release (CE deb) primary key " + dockerGPGFingerprint},
		{ID: "docker.repo", Module: "docker", Description: "add Docker official apt repository", Status: plan.WillRun, Command: []string{"/bin/sh", "-c", writeRepo}},
		{ID: "docker.apt.update", Module: "docker", Description: "refresh apt after adding Docker repository", Status: plan.WillRun, Command: []string{"apt-get", "update"}},
		{ID: "docker.install", Module: "docker", Description: "install Docker Engine, CLI, containerd, buildx, compose plugin", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin"}, RollbackHint: "review Docker packages and /var/lib/docker before removing anything"},
		{ID: "docker.service", Module: "docker", Description: "enable and start Docker service", Status: plan.WillRun, Command: []string{"systemctl", "enable", "--now", "docker"}},
		{ID: "docker.compose.check", Module: "docker", Description: "verify Docker Compose plugin", Status: plan.WillRun, Command: []string{"docker", "compose", "version"}},
	}
}

type DeployUser struct{}

func (DeployUser) Name() string { return "deploy-user" }
func (DeployUser) Plan(ctx Context) []plan.Step {
	du := ctx.Config.Modules.DeployUser
	if !du.Enabled {
		return []plan.Step{{ID: "deploy-user.skip", Module: "deploy-user", Description: "deploy user module disabled", Status: plan.WillSkip}}
	}
	var steps []plan.Step
	if ctx.State.UserExists(du.Name) {
		steps = append(steps, plan.Step{ID: "deploy-user.present", Module: "deploy-user", Description: "deploy user already exists", Status: plan.AlreadyOK})
	} else {
		steps = append(steps, plan.Step{ID: "deploy-user.create", Module: "deploy-user", Description: "create deploy user", Status: plan.WillRun, Command: []string{"useradd", "-m", "-s", "/bin/bash", du.Name}})
	}
	for i, key := range du.SSHAuthorizedKeys {
		steps = append(steps, plan.Step{ID: fmt.Sprintf("deploy-user.ssh-key.%d", i+1), Module: "deploy-user", Description: "append deploy SSH public key if missing", Status: plan.WillRun, Command: []string{selfBinary(ctx), "internal", "append-authorized-key", "--user", du.Name, "--key", key}, RedactCommandInLogs: true})
	}
	if du.Sudo {
		steps = append(steps, plan.Step{ID: "deploy-user.sudo", Module: "deploy-user", Description: "add deploy user to sudo group", Status: plan.WillRun, Command: []string{"usermod", "-aG", "sudo", du.Name}})
	}
	for _, group := range du.Groups {
		if group != "" && !ctx.State.GroupContainsUser(group, du.Name) {
			steps = append(steps, plan.Step{ID: "deploy-user.group." + group, Module: "deploy-user", Description: "add deploy user to " + group + " group", Status: plan.WillRun, Command: []string{"usermod", "-aG", group, du.Name}})
		}
	}
	if ctx.Config.Modules.Docker.Enabled && ctx.Config.Modules.Docker.AddDeployUserToGroup && !ctx.State.GroupContainsUser("docker", du.Name) {
		status := plan.NeedsConfirmation
		if ctx.Config.Confirmations.DockerGroupRootEquivalent {
			status = plan.WillRun
		}
		steps = append(steps, plan.Step{ID: "deploy-user.docker-group", Module: "deploy-user", Description: "add deploy user to docker group", Status: status, Command: []string{"usermod", "-aG", "docker", du.Name}, Dangerous: true, Confirmation: "confirmations.dockerGroupRootEquivalent", Rationale: "membership in the docker group is root-equivalent on most hosts"})
	}
	return steps
}

type Firewall struct{}

func (Firewall) Name() string { return "firewall" }
func (Firewall) Plan(ctx Context) []plan.Step {
	fw := ctx.Config.Modules.Firewall
	if !fw.Enabled {
		return []plan.Step{{ID: "firewall.skip", Module: "firewall", Description: "firewall module disabled", Status: plan.WillSkip}}
	}
	sshPorts := append([]int{}, ctx.OS.SSHPorts...)
	if fw.SSHPort != 0 && !containsPort(sshPorts, fw.SSHPort) {
		sshPorts = append(sshPorts, fw.SSHPort)
	}
	if len(sshPorts) == 0 {
		sshPorts = []int{22}
	}
	steps := []plan.Step{}
	if !ctx.State.CommandExists("ufw") {
		steps = append(steps, plan.Step{ID: "firewall.install", Module: "firewall", Description: "install ufw", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "ufw"}})
	}
	for _, sshPort := range sshPorts {
		steps = append(steps, plan.Step{ID: fmt.Sprintf("firewall.allow.ssh.%d", sshPort), Module: "firewall", Description: fmt.Sprintf("allow detected/configured SSH port %d before enabling firewall", sshPort), Status: plan.WillRun, Command: []string{"ufw", "allow", fmt.Sprintf("%d/tcp", sshPort)}})
	}
	if fw.AllowWeb || ctx.Config.Modules.Caddy.Mode == "host" {
		steps = append(steps, plan.Step{ID: "firewall.allow.http", Module: "firewall", Description: "allow HTTP", Status: plan.WillRun, Command: []string{"ufw", "allow", "80/tcp"}})
		steps = append(steps, plan.Step{ID: "firewall.allow.https", Module: "firewall", Description: "allow HTTPS", Status: plan.WillRun, Command: []string{"ufw", "allow", "443/tcp"}})
	}
	status := plan.NeedsConfirmation
	if ctx.Config.Confirmations.EnableFirewall {
		status = plan.WillRun
	}
	steps = append(steps, plan.Step{ID: "firewall.enable", Module: "firewall", Description: "enable ufw only after SSH allow rule is in plan", Status: status, Command: []string{"ufw", "--force", "enable"}, Dangerous: true, Confirmation: "confirmations.enableFirewall", RollbackHint: "use provider console to run `ufw disable` if SSH access is lost"})
	return steps
}

type Swap struct{}

func (Swap) Name() string { return "swap" }
func (Swap) Plan(ctx Context) []plan.Step {
	sw := ctx.Config.Modules.Swap
	if !sw.Enabled {
		return []plan.Step{{ID: "swap.skip", Module: "swap", Description: "swap module disabled", Status: plan.WillSkip}}
	}
	if ctx.State.SwapActive() {
		return []plan.Step{{ID: "swap.present", Module: "swap", Description: "swap is already active", Status: plan.AlreadyOK}}
	}
	persist := fmt.Sprintf("grep -q '^%s ' /etc/fstab || printf '%%s none swap sw 0 0\\n' %s >> /etc/fstab", shellArg(sw.Path), shellArg(sw.Path))
	return []plan.Step{
		{ID: "swap.allocate", Module: "swap", Description: "allocate swapfile", Status: plan.WillRun, Command: []string{"fallocate", "-l", sw.Size, sw.Path}},
		{ID: "swap.permissions", Module: "swap", Description: "restrict swapfile permissions", Status: plan.WillRun, Command: []string{"chmod", "600", sw.Path}},
		{ID: "swap.mkswap", Module: "swap", Description: "format swapfile", Status: plan.WillRun, Command: []string{"mkswap", sw.Path}},
		{ID: "swap.enable", Module: "swap", Description: "enable swapfile", Status: plan.WillRun, Command: []string{"swapon", sw.Path}},
		{ID: "swap.persist", Module: "swap", Description: "persist swapfile in /etc/fstab if absent", Status: plan.WillRun, Command: []string{"/bin/sh", "-c", persist}, RollbackHint: "remove the swapfile line from /etc/fstab and run `swapoff <path>`"},
	}
}

type Hardening struct{}

func (Hardening) Name() string { return "hardening" }
func (Hardening) Plan(ctx Context) []plan.Step {
	h := ctx.Config.Modules.Hardening
	var steps []plan.Step
	if h.Fail2Ban {
		steps = append(steps, plan.Step{ID: "hardening.fail2ban", Module: "hardening", Description: "install fail2ban", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "fail2ban"}})
	}
	if h.UnattendedUpgrades {
		steps = append(steps, plan.Step{ID: "hardening.unattended-upgrades", Module: "hardening", Description: "install unattended-upgrades", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "unattended-upgrades"}})
	}
	if h.BasicSysctl {
		steps = append(steps, plan.Step{ID: "hardening.sysctl", Module: "hardening", Description: "write basic sysctl hardening drop-in", Status: plan.WillRun, Command: []string{"/bin/sh", "-c", "printf '%s\n' 'net.ipv4.conf.all.rp_filter=1' 'net.ipv4.conf.default.rp_filter=1' > /etc/sysctl.d/99-servy.conf && sysctl --system"}})
	}
	if h.DisableRootSSHLogin {
		steps = append(steps, dangerousSSH("hardening.disable-root", "disable root SSH login", "confirmations.disableRootSSHLogin", ctx.Config.Confirmations.DisableRootSSHLogin, sshDropInCommand(ctx, "PermitRootLogin no")))
	}
	if h.DisablePasswordAuth {
		steps = append(steps, dangerousSSH("hardening.disable-password", "disable SSH password authentication", "confirmations.disablePasswordAuth", ctx.Config.Confirmations.DisablePasswordAuth, sshDropInCommand(ctx, "PasswordAuthentication no")))
	}
	if h.RestrictSSHUsers {
		allowed := ctx.Config.Modules.DeployUser.Name
		if allowed == "" {
			allowed = ctx.Config.Modules.Node.User
		}
		if allowed == "" {
			steps = append(steps, plan.Step{ID: "hardening.restrict-users", Module: "hardening", Description: "restrict SSH users", Status: plan.FailedPrecondition, Rationale: "restrictSSHUsers requires a deploy or node user"})
		} else {
			steps = append(steps, dangerousSSH("hardening.restrict-users", "restrict SSH users", "confirmations.restrictSSHUsers", ctx.Config.Confirmations.RestrictSSHUsers, sshDropInCommand(ctx, "AllowUsers "+allowed)))
		}
	}
	if len(steps) == 0 {
		steps = append(steps, plan.Step{ID: "hardening.skip", Module: "hardening", Description: "hardening module disabled", Status: plan.WillSkip})
	}
	return steps
}

func dangerousSSH(id, desc, confirmation string, confirmed bool, command []string) plan.Step {
	status := plan.NeedsConfirmation
	if confirmed {
		status = plan.WillRun
	}
	return plan.Step{ID: id, Module: "hardening", Description: desc, Status: status, Command: command, Dangerous: true, Confirmation: confirmation, Rationale: "SSH lockout risk; Servy cannot verify external SSH login from inside the host", RollbackHint: "keep provider console access and remove the matching line from /etc/ssh/sshd_config.d/99-servy-hardening.conf if login fails"}
}

func sshDropInCommand(ctx Context, line string) []string {
	return []string{selfBinary(ctx), "internal", "write-sshd-dropin", "--line", line}
}

type Node struct{}

func (Node) Name() string { return "node" }
func (Node) Plan(ctx Context) []plan.Step {
	n := ctx.Config.Modules.Node
	if !n.Enabled {
		return []plan.Step{{ID: "node.skip", Module: "node", Description: "node tooling module disabled", Status: plan.WillSkip}}
	}
	userName := n.User
	if userName == "" {
		userName = ctx.Config.Modules.DeployUser.Name
	}
	version := n.Version
	if version == "" || version == "lts" {
		version = "--lts"
	}
	status := plan.NeedsConfirmation
	if ctx.Config.Confirmations.InstallUserTooling {
		status = plan.WillRun
	}
	steps := []plan.Step{
		{ID: "node.nvm.install", Module: "node", Description: "install nvm for target user using official pinned nvm installer", Status: status, Command: []string{"sudo", "-u", userName, "bash", "-lc", "tmp=$(mktemp) && curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.4/install.sh -o \"$tmp\" && bash \"$tmp\" && rm -f \"$tmp\""}, Dangerous: true, Confirmation: "confirmations.installUserTooling", Rationale: "downloads and executes an official remote installer as the target user"},
		{ID: "node.node.install", Module: "node", Description: "install selected Node.js version through nvm", Status: status, Command: []string{"sudo", "-u", userName, "bash", "-lc", fmt.Sprintf(". ~/.nvm/nvm.sh && nvm install %s && nvm alias default %s", shellArg(version), shellArg(version))}, Dangerous: true, Confirmation: "confirmations.installUserTooling"},
	}
	if n.InstallPNPM {
		steps = append(steps, plan.Step{ID: "node.pnpm", Module: "node", Description: "enable pnpm with Corepack after Node installation", Status: status, Command: []string{"sudo", "-u", userName, "bash", "-lc", ". ~/.nvm/nvm.sh && npm install --global corepack@latest && corepack enable pnpm"}, Dangerous: true, Confirmation: "confirmations.installUserTooling"})
	}
	if n.InstallBun {
		steps = append(steps, plan.Step{ID: "node.bun", Module: "node", Description: "install Bun with official installer", Status: status, Command: []string{"sudo", "-u", userName, "bash", "-lc", "tmp=$(mktemp) && curl -fsSL https://bun.com/install -o \"$tmp\" && bash \"$tmp\" && rm -f \"$tmp\""}, Dangerous: true, Confirmation: "confirmations.installUserTooling", Rationale: "downloads and executes an official remote installer as the target user"})
	}
	return steps
}

type Caddy struct{}

func (Caddy) Name() string { return "caddy" }
func (Caddy) Plan(ctx Context) []plan.Step {
	c := ctx.Config.Modules.Caddy
	switch c.Mode {
	case "", "none":
		return []plan.Step{{ID: "caddy.skip", Module: "caddy", Description: "host-level Caddy disabled", Status: plan.WillSkip}}
	case "check-only":
		status := plan.FailedPrecondition
		if ctx.State.CommandExists("caddy") {
			status = plan.AlreadyOK
		}
		return []plan.Step{{ID: "caddy.check", Module: "caddy", Description: "check existing Caddy installation", Status: status, Command: []string{"caddy", "version"}}}
	case "host":
		if ctx.State.CommandExists("caddy") {
			return []plan.Step{{ID: "caddy.present", Module: "caddy", Description: "Caddy already installed; no config overwrite", Status: plan.AlreadyOK, Command: []string{"caddy", "version"}}}
		}
		return []plan.Step{
			{ID: "caddy.prereqs", Module: "caddy", Description: "install Caddy apt repository prerequisites", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl", "gnupg"}},
			{ID: "caddy.keyring.install", Module: "caddy", Description: "download Caddy stable apt keyring and verify pinned GPG fingerprint", Status: plan.WillRun, Command: []string{selfBinary(ctx), "internal", "install-apt-keyring", "--url", "https://dl.cloudsmith.io/public/caddy/stable/gpg.key", "--dest", "/usr/share/keyrings/caddy-stable-archive-keyring.gpg", "--fingerprint", caddyGPGFingerprint}, Rationale: "verify against pinned Caddy Cloudsmith primary key " + caddyGPGFingerprint},
			{ID: "caddy.repo", Module: "caddy", Description: "download Caddy stable apt source list", Status: plan.WillRun, Command: []string{"curl", "-1sLf", "https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt", "-o", "/etc/apt/sources.list.d/caddy-stable.list"}},
			{ID: "caddy.perms", Module: "caddy", Description: "make Caddy repository source list readable", Status: plan.WillRun, Command: []string{"chmod", "o+r", "/etc/apt/sources.list.d/caddy-stable.list"}},
			{ID: "caddy.apt.update", Module: "caddy", Description: "refresh apt after adding Caddy repository", Status: plan.WillRun, Command: []string{"apt-get", "update"}},
			{ID: "caddy.install", Module: "caddy", Description: "install Caddy host package without generating project Caddyfile", Status: plan.WillRun, Command: []string{"apt-get", "install", "-y", "caddy"}},
		}
	default:
		return []plan.Step{{ID: "caddy.invalid", Module: "caddy", Description: "invalid Caddy mode", Status: plan.FailedPrecondition}}
	}
}

func selfBinary(ctx Context) string {
	if ctx.OS.ID == "ubuntu" || ctx.OS.ID == "debian" {
		return "/proc/self/exe"
	}
	if ctx.BinaryPath != "" {
		return ctx.BinaryPath
	}
	return "servy"
}

func containsPort(ports []int, port int) bool {
	for _, existing := range ports {
		if existing == port {
			return true
		}
	}
	return false
}

func shellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// Pinned GPG primary fingerprints for apt keyrings we install.
//
// These values are the authoritative trust anchor for any package
// eventually installed from these repositories. They are hard-coded on
// purpose: TLS alone is not enough for a keyring that authorises root
// packages, and letting the fingerprint float would defeat the pin.
//
// When rotating (for example after upstream key change) verify against
// the official Docker / Caddy documentation, then update these constants
// and note the reason in CHANGELOG.md and docs/roadmap.md.
const (
	// dockerGPGFingerprint is Docker's Release (CE deb) primary key.
	// Source: https://download.docker.com/linux/ubuntu/gpg and .../debian/gpg
	// Verified 2026-07-04.
	dockerGPGFingerprint = "9DC858229FC7DD38854AE2D88D81803C0EBFCD88"

	// caddyGPGFingerprint is the Caddy stable release signing primary key
	// distributed via Cloudsmith.
	// Source: https://dl.cloudsmith.io/public/caddy/stable/gpg.key
	// Verified 2026-07-04.
	caddyGPGFingerprint = "65760C51EDEA2017CEA2CA15155B6D79CA56EA34"
)
