package modules

import (
	"github.com/vend1k12/servy/internal/manifest"
)

// ManifestRecorder is the optional interface a module implements when it
// wants to contribute to /var/lib/servy/manifest.json.
//
// `ran` is the set of step IDs from module.Plan(ctx) that reached WillRun
// and completed successfully in the current apply. The module returns a
// ModuleRecord reflecting exactly those effects — nothing planned that did
// not run.
type ManifestRecorder interface {
	Module
	Record(ctx Context, ran map[string]bool) manifest.ModuleRecord
}

// BuildManifestRecords walks every module and asks the ones that implement
// ManifestRecorder for their contribution. Only non-empty records are
// included; the returned map is safe to hand to manifest.Save.
func BuildManifestRecords(ctx Context, ran map[string]bool) map[string]manifest.ModuleRecord {
	out := map[string]manifest.ModuleRecord{}
	for _, m := range Registry() {
		rec, ok := m.(ManifestRecorder)
		if !ok {
			continue
		}
		r := rec.Record(ctx, ran)
		if !r.Empty() {
			out[m.Name()] = r
		}
	}
	return out
}

// RanSet builds a step-id set from a list of successful step IDs. Callers
// pass results from runner.Apply already filtered to res.Err == nil.
func RanSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// The Record methods below mirror the Plan() step IDs 1:1. If Plan gains or
// renames a step, the corresponding Record entry must move in lock-step; a
// module-level test asserting the round-trip lives in modules_test.go.

func (Base) Record(ctx Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	if ran["base.packages"] {
		// Reconstruct the missing-packages list: base.Plan computes it once,
		// and we need to know exactly what apt install was asked to touch
		// so revert can `apt-get remove --purge` the same set. Because
		// Plan() already ran during the apply loop, we re-derive it from
		// the current dpkg state minus the effective package set. Only the
		// packages Servy actually asked apt to install belong here.
		installedNow := ctx.State.AptPackagesInstalled(ctx.Config.Modules.Base.EffectivePackages())
		for _, p := range ctx.Config.Modules.Base.EffectivePackages() {
			if installedNow[p] {
				r.AptPackagesInstalled = append(r.AptPackagesInstalled, p)
			}
		}
	}
	if ran["base.gh.keyring"] {
		r.AptKeyrings = append(r.AptKeyrings, "/etc/apt/keyrings/githubcli-archive-keyring.gpg")
	}
	if ran["base.gh.repo"] {
		r.AptLists = append(r.AptLists, "/etc/apt/sources.list.d/github-cli.list")
	}
	if ran["base.gh.install"] {
		r.AptPackagesInstalled = append(r.AptPackagesInstalled, "gh")
	}
	return r
}

func (Docker) Record(_ Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	if ran["docker.keyring.install"] {
		r.AptKeyrings = append(r.AptKeyrings, "/etc/apt/keyrings/docker.asc")
	}
	if ran["docker.repo"] {
		r.AptLists = append(r.AptLists, "/etc/apt/sources.list.d/docker.sources")
	}
	if ran["docker.install"] {
		r.AptPackagesInstalled = []string{
			"docker-ce", "docker-ce-cli", "containerd.io",
			"docker-buildx-plugin", "docker-compose-plugin",
		}
	}
	if ran["docker.service"] || ran["docker.service.start"] {
		r.ServicesEnabled = append(r.ServicesEnabled, "docker")
	}
	return r
}

func (DeployUser) Record(ctx Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	name := ctx.Config.Modules.DeployUser.Name
	if name == "" {
		return r
	}
	if ran["deploy-user.create"] {
		r.UsersCreated = append(r.UsersCreated, name)
	}
	groupsJoined := map[string][]string{}
	if ran["deploy-user.sudo"] {
		groupsJoined["sudo"] = append(groupsJoined["sudo"], name)
	}
	if ran["deploy-user.docker-group"] {
		groupsJoined["docker"] = append(groupsJoined["docker"], name)
	}
	for _, group := range ctx.Config.Modules.DeployUser.Groups {
		if ran["deploy-user.group."+group] {
			groupsJoined[group] = append(groupsJoined[group], name)
		}
	}
	if len(groupsJoined) > 0 {
		r.GroupsJoined = groupsJoined
	}
	return r
}

func (Firewall) Record(_ Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	if ran["firewall.install"] {
		r.AptPackagesInstalled = append(r.AptPackagesInstalled, "ufw")
	}
	// UFW allow rules and `ufw --force enable` are recorded as a service
	// side effect. Revert flips UFW state, not the allow list, since UFW
	// stores rules in its own db and Servy adds rules additively.
	if ran["firewall.enable"] {
		r.ServicesEnabled = append(r.ServicesEnabled, "ufw")
	}
	return r
}

func (Swap) Record(ctx Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	if !ran["swap.allocate"] {
		return r
	}
	sw := ctx.Config.Modules.Swap
	r.SwapFilePath = sw.Path
	if ran["swap.persist"] {
		// The exact line appended when the fstab entry was absent. Format
		// matches modules.go Swap.Plan swap.persist command output.
		r.FstabLine = sw.Path + " none swap sw 0 0"
	}
	return r
}

func (Hardening) Record(ctx Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	h := ctx.Config.Modules.Hardening
	if ran["hardening.fail2ban"] {
		r.AptPackagesInstalled = append(r.AptPackagesInstalled, "fail2ban")
	}
	if ran["hardening.unattended-upgrades"] {
		r.AptPackagesInstalled = append(r.AptPackagesInstalled, "unattended-upgrades")
	}
	if ran["hardening.sysctl"] {
		r.SysctlDropIns = append(r.SysctlDropIns, "/etc/sysctl.d/99-servy.conf")
	}
	if ran["hardening.disable-root"] {
		r.SSHDDropInLines = append(r.SSHDDropInLines, "PermitRootLogin no")
	}
	if ran["hardening.disable-password"] {
		r.SSHDDropInLines = append(r.SSHDDropInLines, "PasswordAuthentication no")
	}
	if ran["hardening.restrict-users"] {
		allowed := ctx.Config.Modules.DeployUser.Name
		if allowed == "" {
			allowed = ctx.Config.Modules.Node.User
		}
		if allowed != "" {
			r.SSHDDropInLines = append(r.SSHDDropInLines, "AllowUsers "+allowed)
		}
	}
	// Silence the unused-h warning when hardening ran nothing.
	_ = h
	return r
}

// Node deliberately does not implement ManifestRecorder. User-scope nvm/pnpm/bun
// installs live under the target user's $HOME; revert v1 does not touch them
// (see docs/safety.md manual rollback table).
var _ ManifestRecorder = (*Base)(nil)

func (Caddy) Record(_ Context, ran map[string]bool) manifest.ModuleRecord {
	var r manifest.ModuleRecord
	if ran["caddy.keyring.install"] {
		r.AptKeyrings = append(r.AptKeyrings, "/usr/share/keyrings/caddy-stable-archive-keyring.gpg")
	}
	if ran["caddy.repo"] {
		r.AptLists = append(r.AptLists, "/etc/apt/sources.list.d/caddy-stable.list")
	}
	if ran["caddy.install"] {
		r.AptPackagesInstalled = append(r.AptPackagesInstalled, "caddy")
	}
	return r
}

// Compile-time interface assertions — a rename in modules.go that drops a
// Record method surfaces as a build failure here rather than a silent no-op
// during revert. Node is intentionally absent.
var (
	_ ManifestRecorder = Base{}
	_ ManifestRecorder = Docker{}
	_ ManifestRecorder = DeployUser{}
	_ ManifestRecorder = Firewall{}
	_ ManifestRecorder = Swap{}
	_ ManifestRecorder = Hardening{}
	_ ManifestRecorder = Caddy{}
)
