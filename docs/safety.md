# Safety

Servy runs as root on a live VPS. This page collects the invariants the tool enforces, the confirmations it needs before doing anything dangerous, and the recovery levers you have when something goes wrong.

If you are looking for a specific error message, jump to [`docs/troubleshooting.md`](troubleshooting.md).

## Core invariants

Servy is built around a small set of promises. Every module, every step, every release path is expected to preserve them.

1. **`init`, `plan`, `validate`, and `apply --dry-run` never mutate the host.** They only read state and print. It is safe to run any of them on a production host at any time.
2. **`--yes` is not an override.** `--yes` executes non-dangerous, already-confirmed steps. It never bypasses a `confirmations.*` gate. A plan with an unconfirmed dangerous step exits with a `BlockingError` even under `--yes`.
3. **Every dangerous step needs its own confirmation.** Confirmations are per-effect, not global. Approving `enableFirewall` does not approve `disableRootSSHLogin`.
4. **No silent overwrite of user config.** Servy does not touch existing `Caddyfile`, `authorized_keys`, `sshd_config`, `ufw` rules, or `sysctl.conf`. It writes drop-ins (`/etc/ssh/sshd_config.d/99-servy-hardening.conf`, `/etc/sysctl.d/99-servy.conf`, `/etc/apt/sources.list.d/*.list`) that can be inspected and removed by hand.
5. **All apt keyrings are pinned by GPG fingerprint.** TLS alone is not the trust anchor. Docker (`9DC858…CD88`) and Caddy (`65760C…EA34`) fingerprints are baked into the binary; a rotated upstream key requires a Servy release.
6. **Release archives are cosign-signed keyless.** Every `servy_linux_*.tar.gz` and `checksums.txt` ship a `.sig` + `.pem`. `install.sh` verifies opportunistically; set `SERVY_REQUIRE_COSIGN=1` (or `servy update --require-cosign`) to hard-fail without cosign.
7. **Argv-based exec.** Servy prefers `exec.CommandContext` over shell strings. Where a shell is required (user-tooling installers), the argv passed to `/bin/sh -c` is either a fixed literal or built from `shellArg`-escaped user input.
8. **Log integrity.** Steps that carry user secrets (currently: SSH public keys) are logged with the argv redacted. Log files are created with mode `0600`.

## Confirmations reference

Every dangerous step names a `confirmations.*` key in its plan output. Set the matching key to `true` in your YAML to authorise the step. Servy still refuses to run without `--yes`; the confirmation gate and `--yes` are independent locks.

| Key | Blocks | Why |
|---|---|---|
| `confirmations.enableFirewall` | `firewall.enable` — running `ufw --force enable` | If the SSH allow rule is missing or misnumbered, enabling the firewall locks you out. Servy already refuses to enable UFW unless an SSH allow step is present in the same plan; the confirmation adds a second human check. |
| `confirmations.disableRootSSHLogin` | `hardening.disable-root` — writing `PermitRootLogin no` to `/etc/ssh/sshd_config.d/99-servy-hardening.conf` | If the deploy user cannot actually log in, disabling root SSH locks the host to console-only recovery. |
| `confirmations.disablePasswordAuth` | `hardening.disable-password` — writing `PasswordAuthentication no` to the same drop-in | Same lockout mode as above; make sure key-based login works first. |
| `confirmations.restrictSSHUsers` | `hardening.restrict-users` — writing `AllowUsers <deploy>` to the same drop-in | Denies everyone else, including any admin accounts the operator forgot about. |
| `confirmations.dockerGroupRootEquivalent` | `deploy-user.docker-group` — `usermod -aG docker <deploy>` | Membership in the `docker` group is root-equivalent on most hosts. This is a policy decision, not a technical one; Servy will not make it silently. |
| `confirmations.installUserTooling` | `node.nvm.install`, `node.node.install`, `node.pnpm`, `node.bun` — downloading and executing official installers as the deploy user | Third-party installer scripts (nvm, bun) are pinned but still remote code. The gate is on the class of operation, not on any single URL. |

Every dangerous step also carries an optional `Rationale` and `RollbackHint`; `apply` prints both when the step is blocked, and `docs/troubleshooting.md` mirrors the same hints.

## Config safety pattern

Recommended flow for a new server:

1. `servy init` to generate a config template.
2. `servy validate --config servy.yml` — schema check, no mutation.
3. `servy doctor` — read-only host preflight (disk, memory, OS, SSH port, systemd, package manager).
4. `servy plan --config servy.yml` — deterministic plan preview. Reads state, mutates nothing.
5. Review the plan. If any dangerous step matters, set the matching `confirmations.*` key.
6. `servy apply --config servy.yml --dry-run` — same plan, prints "would run" for every step. Still no mutation.
7. `servy apply --config servy.yml --yes` — mutating apply. Every dangerous step must be pre-confirmed, or Servy exits with a `BlockingError` naming the missing keys.

Under this flow, the first mutation on the host happens at step 7, not before.

## When something breaks

### `apply` refuses with a `BlockingError`

Servy prints every blocker in one message, along with the `confirmations.*` key it needs, a rationale, and (if applicable) a recovery hint. The fix is always the same:

1. Read the blocker list.
2. Understand the effect. If in doubt, re-read this page.
3. Set the named `confirmations.*` key in your YAML.
4. Re-run `apply --dry-run` to double-check.
5. Re-run `apply --yes`.

### SSH lockout risk

The three highest-risk operations Servy performs:

- Enabling UFW (`firewall.enable`).
- Disabling root SSH login (`hardening.disable-root`).
- Restricting SSH to a single user (`hardening.restrict-users`).

Before enabling any of them, confirm from a second terminal that:

- Your deploy user can `ssh deploy@host` with a key.
- `sudo -n true` succeeds for that user.
- If UFW is about to be enabled, the plan contains an explicit SSH allow rule on the port your session is currently using. Servy detects the port from `ss -tnp` state and reflects it in the plan; verify it matches your actual connection.

Servy writes SSH changes into `/etc/ssh/sshd_config.d/99-servy-hardening.conf` and runs `sshd -t` before reloading. If `sshd -t` fails, Servy reverts the drop-in through the same directory FD (Renameat back for pre-existing files, Unlinkat for new ones) and returns a non-zero exit. See [`SECURITY.md`](../SECURITY.md) for the TOCTOU model behind that revert.

### `apply` failed halfway through

Servy runs one step at a time and stops on the first failure. Partial state is expected and recoverable:

- The step log is under `/var/log/servy/` — a JSONL file per apply, containing every command with stdout, stderr, and exit code.
- Nothing Servy touches is destructive on top of an existing user config; drop-ins and apt list files are additive.
- Re-run `plan` to see the current delta. Steps that already succeeded show as `already_ok` (for `base` packages) or reappear as `will_run` (for module state Servy cannot cheaply detect).
- `servy revert <module>` reads `/var/lib/servy/manifest.json` and undoes what that module added on the last apply: apt list files, apt keyrings, sysctl drop-ins, sshd directive lines, swapfile + `/etc/fstab` entry. Add `--purge-packages` to also `apt-get remove --purge` the Servy-installed packages and `systemctl disable --now` the services it enabled. Deploy user removal and group memberships are out of scope in v1 and print as `will_skip` steps.

### You need to disable a change Servy made

If `servy revert` cannot help (older manifest, module out of scope, manifest deleted), the manual playbook:

| Change | Undo |
|---|---|
| Docker apt repo | `rm /etc/apt/sources.list.d/docker.list /etc/apt/keyrings/docker.asc && apt-get update` |
| Caddy apt repo | `rm /etc/apt/sources.list.d/caddy-stable.list /etc/apt/keyrings/caddy.asc && apt-get update` |
| SSH hardening drop-in | `rm /etc/ssh/sshd_config.d/99-servy-hardening.conf && systemctl reload ssh` |
| sysctl drop-in | `rm /etc/sysctl.d/99-servy.conf && sysctl --system` |
| Swapfile | `swapoff /swapfile && rm /swapfile && sed -i '\|/swapfile|d' /etc/fstab` |
| UFW enabled | `ufw disable` (from console or another SSH session) |
| Deploy user added to docker group | `gpasswd -d <user> docker` |

## Out-of-scope threats

Servy does not defend against:

- Compromised BIOS, firmware, or hypervisor.
- Physical access to the host.
- An attacker who already has write access to `/etc/apt/keyrings`, `/etc/ssh/sshd_config.d`, or `~deploy` before Servy runs.
- A malicious `sshd` binary at `/usr/sbin/sshd`.
- Configuration files edited by hand after Servy applies; Servy does not lock or fingerprint them.

See [`SECURITY.md`](../SECURITY.md) for the full threat model and mitigations table.
