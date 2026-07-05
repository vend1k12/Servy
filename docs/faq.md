# FAQ

Answers to questions that keep coming up. If your question is not here, check [`docs/safety.md`](safety.md), [`docs/troubleshooting.md`](troubleshooting.md), and [`docs/architecture.md`](architecture.md) first, then open an issue.

## Positioning

### Is Servy a replacement for Ansible?

No. Servy runs on one host at a time, executed **inside** the target VPS. It has no inventory, no SSH transport, no push mode. If you provision fleets from a laptop, use Ansible or its `ansible-pull` cousin. The README's [compare table](../README.md#how-servy-compares) covers the axis.

### Is Servy a replacement for cloud-init?

No. Cloud-init runs first, from the cloud image, before you ever SSH in. Servy runs afterwards, on an existing host you can SSH into. The two are complementary, not competing.

### Does Servy replace `bash setup.sh`?

That is the workflow Servy is aimed at. If your setup script is under 200 lines and never runs twice on the same host, keep the script. If you re-run it, want dry-runs, or want a plan-before-apply gate, Servy is the trade.

### Why not just use Docker + Portainer / Coolify / dokploy?

Those manage containers **on** a host. Servy sets up the host itself — apt repositories, apt keyring pinning, Docker installation, UFW rules, SSH hardening, swap, deploy users. Once the host is set up, Servy steps out of the way. You can layer a container manager on top.

### Why not Ansible even for one host?

Ansible works for one host. Servy trades three Ansible strengths (fleets, roles, community modules) for three things Servy prioritises: no runtime toolchain on the target, a small typed plan model with explicit confirmations for dangerous steps, and a single static Go binary. Pick the tool whose trade-offs match your setup.

## Supported hosts

### Which distros work?

Ubuntu 22.04, 24.04 (26.04 planned once the codename is confirmed) and Debian 12, 13. Both `amd64` and `arm64`. Anything else is a permanent non-goal in v1 — see [roadmap `Non-goals`](roadmap.md#non-goals).

### Does Servy work on WSL / Docker containers / LXD?

`servy doctor` and `servy plan` work anywhere. Mutating apply is only expected to work on Ubuntu/Debian with a working systemd and apt. WSL2 and privileged systemd-in-docker containers usually meet that bar and are how we exercise the CI matrix; anything else is best-effort.

### Does Servy work on macOS / Fedora / Arch / Alpine?

`doctor` and `plan` do — they are read-only. Mutating `apply` refuses non-Ubuntu / non-Debian hosts.

## Safety and correctness

### What does `--yes` do?

Executes the plan. It does **not** override `confirmations.*`. See [`docs/safety.md#core-invariants`](safety.md#core-invariants) invariant 2.

### What if I run `apply` twice?

Idempotent by design. Packages already installed are marked `already_ok` and skipped. Drop-in files that already contain the target directive are a no-op. Users already in the target groups are skipped. The plan output tells you exactly what would run before you commit.

### Can I `Ctrl-C` in the middle of `apply`?

Yes. Each step is an `exec.CommandContext` with the apply-level context; on cancel, the running step gets `SIGKILL` and Servy exits. Partial state is expected — re-run `plan` and see what is left.

### Does Servy modify existing `Caddyfile` / `sshd_config` / `authorized_keys` / UFW rules?

No. Servy writes drop-ins and additive apt list files. `authorized_keys` is append-only, symlink-refused, and TOCTOU-safe. `sshd_config.d/99-servy-hardening.conf` is a new file; the main `sshd_config` is untouched. See [`SECURITY.md`](../SECURITY.md).

### Where are secrets logged?

Nowhere. Steps whose argv contains user-supplied secrets (currently: SSH public keys) are logged with the argv redacted. Logs live at `/var/log/servy/*.jsonl` mode `0600`. If you find a step leaking a secret into the log, that is a security bug — please report it via a GitHub Security Advisory.

## Installation and updates

### Do I need Go on the target host?

No. Servy ships as a single statically-linked binary.

### How do I install?

Recommended:

```sh
curl -fsSL https://raw.githubusercontent.com/vend1k12/Servy/main/install.sh -o install.sh
less install.sh                 # inspect before running
sh install.sh
```

The installer downloads the release archive, verifies `checksums.txt` under `LC_ALL=C` with `sha256sum -c`, and (best-effort) verifies the cosign signature if `cosign` is on `$PATH`. Set `SERVY_REQUIRE_COSIGN=1` to fail hard when cosign is missing or verification fails.

### How do I upgrade?

`servy update` fetches the latest GitHub release, verifies checksum, verifies cosign signature (opportunistic; strict with `--require-cosign`), and atomically replaces the binary. It never touches host state.

### How do I pin to a specific version?

```sh
SERVY_VERSION=v0.0.2 sh install.sh
```

or

```sh
servy update --version v0.0.2
```

### How do I verify a release archive by hand?

```sh
curl -fsSL https://github.com/vend1k12/Servy/releases/download/vX.Y.Z/checksums.txt -o checksums.txt
curl -fsSL https://github.com/vend1k12/Servy/releases/download/vX.Y.Z/servy_linux_amd64.tar.gz -o servy_linux_amd64.tar.gz
LC_ALL=C sha256sum -c checksums.txt --ignore-missing

# cosign (optional, but recommended)
cosign verify-blob \
  --certificate-identity "https://github.com/vend1k12/Servy/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --signature servy_linux_amd64.tar.gz.sig \
  --certificate servy_linux_amd64.tar.gz.pem \
  servy_linux_amd64.tar.gz
```

## Configuration

### Where does Servy look for its config?

In this order: `--config <path>`, then `servy.yml`, `servy.yaml`, `.servy.yml` in the current directory.

### Is there an example config?

Yes: `examples/*.yml`. Start with `examples/base.yml` for a minimal server, `examples/docker-only.yml` for a Docker host, and `examples/web-app.yml` for a Node/pnpm/Bun web-app host.

### How do I write a config from scratch?

`servy init --output servy.yml` prints an interactive prompt. `servy init --preset docker-only --output servy.yml` writes a preset without prompting. `servy init --list-presets` lists them.

### Can I drop the default apt packages?

Yes. `modules.base.packages: []` disables the curated set; `modules.base.tools: {}` disables the curated tools; `modules.base.installGitHubCLI: false` skips the `cli.github.com` apt repository. See [`examples/base-minimal.yml`](../examples/base-minimal.yml).

### Why is the `node` profile called `web-app` now?

Because it also installs pnpm and Bun, not just Node. `node` is a deprecated alias with full backward compatibility; it will remain callable through v1.x.

## Operations

### Where are logs?

Apply logs: `/var/log/servy/<UTC timestamp>.jsonl`, mode `0600`.

`servy logs list|show|tail` is planned for v1.x; today, `servy logs` just prints the log directory path.

### How do I roll back?

Servy does not have `servy revert` yet — it is a v1.0 blocker. Until it lands, [`docs/safety.md#you-need-to-disable-a-change-servy-made`](safety.md#you-need-to-disable-a-change-servy-made) lists the manual undo steps per module.

### Does Servy support `--json` for CI pipelines?

Not yet. `apply --json` is on the v1.x quality-of-life list. Today, all output is human-readable text.

### Does Servy run in a systemd timer / cron?

You can invoke it non-interactively:

```sh
servy apply --config /etc/servy.yml --yes
```

The exit code is non-zero on any failure. `--yes` still refuses to run a plan with unconfirmed dangerous steps.

## Contributing

### Where do I file bugs?

`https://github.com/vend1k12/Servy/issues`. Please attach `servy --version`, `servy doctor` output, and the offending `/var/log/servy/*.jsonl` entry with secrets redacted.

### Security reports?

Open a [private security advisory](https://github.com/vend1k12/Servy/security/advisories) on the repo. Do not file a public issue for a vulnerability.

### Where is the roadmap?

[`docs/roadmap.md`](roadmap.md).
