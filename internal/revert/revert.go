// Package revert plans and executes the inverse of a Servy module apply, driven
// by /var/lib/servy/manifest.json. It builds a typed list of Steps a caller
// can dry-run or apply through the same runner used for `apply`.
//
// v1 scope (matches the roadmap):
//   - apt list files under /etc/apt/sources.list.d/*.{list,sources}
//   - apt keyring files under /etc/apt/keyrings/*.{asc,gpg} and
//     /usr/share/keyrings/*.gpg (Caddy)
//   - sysctl drop-ins under /etc/sysctl.d/*.conf
//   - sshd drop-in directive lines (removed from the shared 99-servy file)
//   - swapfile + /etc/fstab line
//   - systemd services Servy enabled (disable --now, opt-in via
//     WithPurgePackages)
//   - apt packages Servy installed (apt-get remove --purge, opt-in via
//     WithPurgePackages)
//
// Out of scope for v1: users/groups (require --force), user-scope node/nvm/bun
// tooling under $HOME.
package revert

import (
	"fmt"

	"github.com/vend1k12/servy/internal/manifest"
	"github.com/vend1k12/servy/internal/plan"
)

// Options controls which side effects the revert plan includes.
type Options struct {
	// PurgePackages authorises `apt-get remove --purge` for
	// AptPackagesInstalled and `systemctl disable --now` for ServicesEnabled.
	// Without it, package removal is elided and the revert covers only
	// files, drop-in lines, and swap state.
	PurgePackages bool
}

// Build assembles a typed revert plan for the module identified by name using
// the manifest record. An empty record produces an empty plan; the caller
// prints a "nothing to revert" message.
func Build(module string, rec manifest.ModuleRecord, opts Options) plan.Plan {
	p := plan.Plan{Profile: "revert:" + module}
	if rec.Empty() {
		p.Add(plan.Step{ID: module + ".revert.nothing", Module: module, Description: "no Servy-owned side effects recorded for " + module, Status: plan.WillSkip})
		return p
	}
	// Order matters. We tear down in the reverse of install: stop services,
	// remove packages (only when authorised), then delete apt list files,
	// keyrings, drop-ins, and finally undo swap. Anything file-oriented
	// tolerates "already gone" — the corresponding step goes AlreadyOK.
	if opts.PurgePackages {
		for _, svc := range rec.ServicesEnabled {
			p.Add(plan.Step{
				ID:           fmt.Sprintf("%s.revert.service.%s", module, svc),
				Module:       module,
				Description:  fmt.Sprintf("disable and stop %s.service", svc),
				Status:       plan.WillRun,
				Command:      []string{"systemctl", "disable", "--now", svc},
				Dangerous:    true,
				RollbackHint: "if a workload depends on " + svc + ", restart it with `systemctl enable --now " + svc + "`",
			})
		}
		if len(rec.AptPackagesInstalled) > 0 {
			cmd := append([]string{"apt-get", "remove", "--purge", "-y"}, rec.AptPackagesInstalled...)
			p.Add(plan.Step{
				ID:           module + ".revert.packages",
				Module:       module,
				Description:  fmt.Sprintf("apt purge %d Servy-installed package(s)", len(rec.AptPackagesInstalled)),
				Status:       plan.WillRun,
				Command:      cmd,
				Dangerous:    true,
				Confirmation: "confirmations.purgePackages",
				Rationale:    "apt purge removes package data; Servy will not touch /var/lib/docker or existing container images",
				RollbackHint: "re-run `servy apply` to reinstall from the same pinned repositories",
			})
		}
	}
	// File removals go through `rm -f -- <path>`. The path list is the
	// exact set Servy authored. rm -f treats missing files as a no-op so
	// re-running revert is idempotent.
	for _, path := range rec.AptLists {
		p.Add(plan.Step{
			ID:          fmt.Sprintf("%s.revert.apt-list.%s", module, safeName(path)),
			Module:      module,
			Description: "remove apt list file " + path,
			Status:      plan.WillRun,
			Command:     []string{"rm", "-f", "--", path},
		})
	}
	for _, path := range rec.AptKeyrings {
		p.Add(plan.Step{
			ID:          fmt.Sprintf("%s.revert.apt-keyring.%s", module, safeName(path)),
			Module:      module,
			Description: "remove apt keyring " + path,
			Status:      plan.WillRun,
			Command:     []string{"rm", "-f", "--", path},
		})
	}
	if len(rec.AptLists) > 0 || len(rec.AptKeyrings) > 0 {
		p.Add(plan.Step{
			ID:          module + ".revert.apt.update",
			Module:      module,
			Description: "refresh apt after removing repositories",
			Status:      plan.WillRun,
			Command:     []string{"apt-get", "update"},
		})
	}
	for _, path := range rec.SysctlDropIns {
		p.Add(plan.Step{
			ID:          fmt.Sprintf("%s.revert.sysctl.%s", module, safeName(path)),
			Module:      module,
			Description: "remove sysctl drop-in " + path,
			Status:      plan.WillRun,
			Command:     []string{"rm", "-f", "--", path},
		})
	}
	if len(rec.SysctlDropIns) > 0 {
		p.Add(plan.Step{
			ID:          module + ".revert.sysctl.reload",
			Module:      module,
			Description: "reapply system sysctl configuration",
			Status:      plan.WillRun,
			Command:     []string{"sysctl", "--system"},
		})
	}
	if len(rec.SSHDDropInLines) > 0 {
		// Argv-based invocation via the hidden internal subcommand. This
		// mirrors how WriteSSHDDropIn is called from module planners and
		// keeps the O_NOFOLLOW/Renameat safety pattern inside safeops.
		args := []string{"/proc/self/exe", "internal", "remove-sshd-dropin-lines"}
		for _, line := range rec.SSHDDropInLines {
			args = append(args, "--line", line)
		}
		p.Add(plan.Step{
			ID:           module + ".revert.sshd-dropin",
			Module:       module,
			Description:  fmt.Sprintf("remove %d Servy-authored sshd directive line(s) and reload sshd", len(rec.SSHDDropInLines)),
			Status:       plan.WillRun,
			Command:      args,
			Dangerous:    true,
			RollbackHint: "keep provider console access; if the removal is wrong, re-run `servy apply` to restore the drop-in",
		})
	}
	if rec.SwapFilePath != "" {
		p.Add(plan.Step{
			ID:          module + ".revert.swapoff",
			Module:      module,
			Description: "swapoff " + rec.SwapFilePath,
			Status:      plan.WillRun,
			Command:     []string{"swapoff", rec.SwapFilePath},
		})
		p.Add(plan.Step{
			ID:          module + ".revert.swap.remove",
			Module:      module,
			Description: "remove swapfile " + rec.SwapFilePath,
			Status:      plan.WillRun,
			Command:     []string{"rm", "-f", "--", rec.SwapFilePath},
		})
	}
	if rec.FstabLine != "" {
		args := []string{"/proc/self/exe", "internal", "remove-fstab-line", "--line", rec.FstabLine}
		p.Add(plan.Step{
			ID:          module + ".revert.fstab",
			Module:      module,
			Description: "remove Servy-authored /etc/fstab line",
			Status:      plan.WillRun,
			Command:     args,
		})
	}
	// Users/groups intentionally not in v1. Leave a visible skip so revert
	// output is unambiguous.
	if len(rec.UsersCreated) > 0 {
		p.Add(plan.Step{
			ID:          module + ".revert.users.skip",
			Module:      module,
			Description: fmt.Sprintf("deploy user removal is out of scope in v1 (recorded: %v)", rec.UsersCreated),
			Status:      plan.WillSkip,
			Rationale:   "user deletion is destructive and requires --force in a future release",
		})
	}
	if len(rec.GroupsJoined) > 0 {
		p.Add(plan.Step{
			ID:          module + ".revert.groups.skip",
			Module:      module,
			Description: "group memberships are not reverted in v1",
			Status:      plan.WillSkip,
			Rationale:   "membership changes are typically fine to leave; use `gpasswd -d <user> <group>` by hand if needed",
		})
	}
	return p
}

// safeName produces a plan-step-ID-safe segment from an absolute path so the
// step ID is stable and readable. Slashes and dots become dashes.
func safeName(path string) string {
	out := make([]byte, 0, len(path))
	for i := range len(path) {
		b := path[i]
		switch b {
		case '/', '.':
			out = append(out, '-')
		default:
			out = append(out, b)
		}
	}
	// Trim leading dashes so an absolute path does not produce "-etc-...".
	for len(out) > 0 && out[0] == '-' {
		out = out[1:]
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}
