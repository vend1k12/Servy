<div align="center">

# Servy

**Safe, repeatable VPS setup for Ubuntu and Debian.**

Servy is a single-binary Go CLI that runs **inside** a server, detects the host, builds a readable execution plan, and applies only explicitly approved setup steps.

[![CI](https://shieldcn.dev/github/ci/vend1k12/Servy.svg?workflow=ci.yml&branch=main&variant=branded)](https://github.com/vend1k12/Servy/actions/workflows/ci.yml)
[![CodeQL](https://shieldcn.dev/github/checks/vend1k12/Servy/main/analyze.svg?variant=branded)](https://github.com/vend1k12/Servy/actions/workflows/codeql.yml)
[![Go](https://shieldcn.dev/badge/Go-1.26.3-00ADD8.svg?logo=go&logoColor=white&variant=branded)](https://go.dev/)
[![License](https://shieldcn.dev/github/license/vend1k12/Servy.svg?variant=branded)](LICENSE)
[![Status](https://shieldcn.dev/badge/status-pre--v1-F97316.svg?variant=branded)](#release-status)

[Quick start](#quick-start) · [Config generation](#config-generation) · [Profiles](#profiles) · [Safety model](#safety-model) · [Examples](#examples) · [Contributing](#contributing)

</div>

---

## Who Servy is for

Servy is aimed at **solo developers and small teams** managing Ubuntu or Debian VPS hosts by hand. It also fits small production hosts, provided you accept the caveats in [Release status](#release-status) and read the [safety model](#safety-model).

Use Servy if:

- you SSH into your own server(s) and want repeatable setup without learning Ansible;
- you want a clear plan and dry-run before anything mutates the host;
- you want a single static Go binary instead of a runtime toolchain.

Do not use Servy if:

- you provision fleets from a laptop and want push-style SSH orchestration — reach for Ansible;
- you provision fresh cloud VMs from image — cloud-init is a better fit;
- your workflow depends on Fedora/Arch/Alpine — Servy is Ubuntu/Debian only.

### How Servy compares

| | Servy | ad-hoc bash | ansible-pull | cloud-init |
| --- | --- | --- | --- | --- |
| Install | one binary | copy/paste | Python + Ansible | baked into cloud image |
| Dry-run before mutation | yes | no | limited | no |
| Idempotent | yes | depends | yes | one-shot |
| Runs on existing hosts | yes | yes | yes | no |
| Learning curve | low | low | medium | medium |
| Fleet from a laptop | no | no | yes | no |

## Why Servy exists

Fresh VPS setup is usually a choice between a one-off shell script, hand-written notes, or a full configuration-management stack. Servy sits in the middle:

- more structured and auditable than a large bash script;
- smaller and more local than Ansible/Terraform;
- safe enough to rerun;
- explicit about dangerous operations before they happen.

## What Servy does

| Area | What Servy handles |
| --- | --- |
| Host detection | Ubuntu/Debian version, codename, architecture, apt, systemd, current user, root/sudo, SSH ports |
| Planning | Typed execution plan with `will_run`, `already_ok`, `will_skip`, `needs_confirmation`, `unsupported`, and `failed_precondition` statuses |
| Base setup | apt refresh, server essentials, `git`, official GitHub CLI (`gh`), `jq`, `unzip`, `rsync`, `tmux`, `htop`, `nano` |
| Docker | Official Docker apt repository, Docker Engine, CLI, containerd, Buildx, Compose plugin |
| Optional modules | UFW, swapfile, deploy user, hardening, Caddy, Node/nvm/pnpm/Bun |
| Safety | dry-run, explicit confirmations, no silent SSH lockout-risk changes, bounded logs |

## What Servy does not do

Servy intentionally keeps v1 narrow:

- no remote SSH orchestration;
- no Ansible/Terraform/OpenTofu replacement;
- no project-specific `docker-compose.yml` generation;
- no project-specific `Caddyfile` generation;
- no DNS/domain management;
- no backups or monitoring;
- no Fedora/Arch/Alpine/cloud-init/plugin system.

## Release status

Servy is currently **pre-v1**. Public releases still lack signed artifacts, and full Ubuntu/Debian × amd64/arm64 mutation testing on real VPS images is not complete. The CLI, config schema, and safety model are usable for local testing and disposable hosts.

See [`docs/roadmap.md`](docs/roadmap.md) for the current milestone plan and non-goals.

## Supported hosts

| Distribution | Supported codenames | Notes |
| --- | --- | --- |
| Ubuntu LTS | `jammy` 22.04, `noble` 24.04, `resolute` 26.04 LTS | Docker official apt repository support checked |
| Debian | `bookworm` 12, `trixie` 13 | Docker official apt repository support checked |

Target architectures: `linux/amd64` and `linux/arm64`.

## Quick start

Build locally:

```sh
go build -o servy ./cmd/servy
```

Inspect the host without changing anything:

```sh
sudo ./servy doctor
```

Create a config from a preset or with the custom wizard:

```sh
./servy init --list-presets
./servy init --preset docker-only --output servy.yml
./servy init --custom --output servy.yml
```

Validate and preview (Servy auto-discovers `servy.yml`, `servy.yaml`, then `.servy.yml` in the current directory):

```sh
./servy validate
./servy plan
./servy apply --dry-run
```

Use `servy doctor --json` and `servy plan --json` when automation or issue reports need structured read-only output.

Apply after reviewing the plan:

```sh
sudo ./servy apply --yes
```

`--yes` only acknowledges a reviewed non-dangerous plan. Dangerous actions still require explicit YAML confirmations.

## Installation

### From source

```sh
git clone https://github.com/vend1k12/servy.git
cd servy
go build -o servy ./cmd/servy
sudo install -m 0755 servy /usr/local/bin/servy
servy doctor
```

### From release bootstrap script

`install.sh` is intentionally minimal: it installs the Servy binary and runs `servy doctor`. It does **not** configure the server.

```sh
curl -fsSL https://raw.githubusercontent.com/vend1k12/servy/main/install.sh | sh
```

For production use, prefer downloading a tagged release and verifying checksums manually until the first signed public release is published.

Update an installed binary:

```sh
servy update check
sudo servy update
```

## Commands

| Command | Purpose | Mutates host? |
| --- | --- | --- |
| `servy doctor [--json]` | Read-only prerequisite, compatibility, DNS/network, and fix-hint diagnostics | No |
| `servy status` | Read-only host/config/module state summary | No |
| `servy init [--list-presets] [--preset <name>] [--custom]` | List presets or write config YAML through a preset/custom wizard | Writes local YAML only |
| `servy validate [profile]` | Strict YAML validation using `--config` or default config discovery | No |
| `servy plan [profile] [--json]` | Build and print execution plan using `--config` or default config discovery | No |
| `servy apply [profile] --dry-run` | Print apply plan | No |
| `servy apply [profile] --yes` | Execute eligible steps | Yes |
| `servy update check` | Check the latest GitHub Release for a newer CLI | No |
| `servy update` | Download, verify, and install the latest GitHub Release CLI binary | Replaces the Servy binary |
| `servy completion bash` | Interactively install Bash completion | Writes a completion file |
| `servy completion bash --print` | Print Bash completion script for manual installation | No |
| `servy module list` | List built-in modules | No |
| `servy module status <name>` | Show module plan entries | No |
| `servy logs` | Print log directory | No |
| `servy version` | Print version metadata | No |

`validate`, `plan`, `apply`, `status`, and `module status` search `servy.yml`, `servy.yaml`, and `.servy.yml` when `--config` is omitted. A positional profile such as `servy apply base` verifies that the discovered config's `profile` matches before planning or applying.

## Config generation

`servy init` helps write YAML; it does not configure the host.

| Mode | Use when |
| --- | --- |
| `servy init --list-presets` | You want to see the built-in generation templates. |
| `servy init --preset base --output servy.yml` | You want minimal server prep YAML. |
| `servy init --preset docker-only --output servy.yml` | You want base setup plus Docker YAML. |
| `servy init --preset node --output servy.yml` | You want Docker plus optional host-level JavaScript tooling YAML. |
| `servy init --custom --output servy.yml` | You want the wizard to ask about each module option exposed by the current config schema. |

Presets are generation-time shortcuts only. They write ordinary strict-schema YAML that still goes through the normal `validate`, `plan`, `apply --dry-run`, and explicit `apply --yes` flow. After reviewing the plan for a generated config, apply it the same way as any hand-written config: `sudo servy apply --config servy.yml --yes`. Custom mode is for tailoring module options up front; lockout-risk and privilege-escalating actions still require explicit confirmation fields and default to safe values.

## Profiles

### `base`

Minimal server preparation without Docker:

- apt package refresh;
- base packages: `ca-certificates`, `curl`, `gnupg`, `lsb-release`, `apt-transport-https`;
- everyday server tools: `git`, `gh`, `jq`, `unzip`, `rsync`, `tmux`, `htop`, `nano`;
- optional UFW, swap, deploy user, and hardening.

`gh` is installed from the official GitHub CLI apt repository. The repository keyring is checked against the SHA256 published by the GitHub CLI maintainers.

### `docker-only`

Everything in `base`, plus Docker Engine for containerized deployments:

- Docker official apt repository;
- Docker Engine and CLI;
- containerd;
- Buildx plugin;
- Docker Compose plugin.

Servy does not install Docker Desktop, Docker via snap, Docker convenience scripts, or Compose v1.

### `node`

Everything in `docker-only`, plus optional host-level JavaScript tooling for projects that need non-containerized processes:

- nvm;
- Node.js through nvm;
- pnpm through Corepack;
- optional Bun.

The `node` profile is intentionally not the default. Docker-only hosts usually do not need host-level Node tooling.

## Safety model

Servy treats server setup as a dangerous operation and keeps the defaults conservative.

| Risk | Servy behavior |
| --- | --- |
| Accidental mutation | `plan` and `apply --dry-run` never mutate |
| SSH lockout | root-login/password-auth/restrict-users changes need separate confirmations |
| Firewall lockout | UFW enablement requires SSH allow rules in the plan and `confirmations.enableFirewall` |
| Docker group privilege | deploy user Docker group membership requires `confirmations.dockerGroupRootEquivalent` |
| Remote user tooling installers | nvm/pnpm/Bun steps require `confirmations.installUserTooling` |
| Existing SSH keys | `authorized_keys` is append-only and symlink-safe |
| Command execution | privileged commands resolve through a restricted system path |
| Logging | apply logs go to `/var/log/servy/` with bounded output and redaction for key material |

## Example config

```yaml
schemaVersion: v1
profile: docker-only
modules:
  docker:
    enabled: true
    channel: stable
    addDeployUserToGroup: false
  deployUser:
    enabled: true
    name: deploy
    sshAuthorizedKeys: []
    sudo: true
  firewall:
    enabled: false
    sshPort: 22
    allowWeb: false
  swap:
    enabled: true
    size: 2G
    path: /swapfile
confirmations:
  enableFirewall: false
  dockerGroupRootEquivalent: false
runtime:
  logDir: /var/log/servy
```

More examples:

- [`examples/base.yml`](examples/base.yml)
- [`examples/docker-only.yml`](examples/docker-only.yml)
- [`examples/node.yml`](examples/node.yml)

## Plan output example

```text
Profile: docker-only
 1. [will_run] base: update apt package index
 2. [will_run] base: install base server packages
 3. [will_run] base: download and verify official GitHub CLI apt keyring
 4. [will_run] docker: add Docker official apt repository
 5. [will_run] docker: install Docker Engine, CLI, containerd, buildx, compose plugin
 6. [needs_confirmation] firewall: enable ufw only after SSH allow rule is in plan
```

The real plan includes commands, rationale, recovery hints, and skipped/already-ok steps.

## Documentation

| Document | Audience |
| --- | --- |
| [`docs/roadmap.md`](docs/roadmap.md) | Current milestones, non-goals, and release checklist |
| [`docs/architecture.md`](docs/architecture.md) | Maintainers changing internals |
| [`docs/ai-agents.md`](docs/ai-agents.md) | AI agents working on the repo |
| [`docs/history/mvp-v0.md`](docs/history/mvp-v0.md) | Historical MVP plan (delivered) |
| [`SECURITY.md`](SECURITY.md) | Vulnerability reporting and security model |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Contribution rules and safety expectations |

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/servy
```

Run the Docker smoke matrix:

```sh
tests/docker/run.sh
```

The Docker smoke test validates CLI behavior on Ubuntu 22.04, Ubuntu 24.04, Debian 12, and Debian 13. It covers validation, planning, dry-run, module listing, and doctor checks. It does not replace real VPS mutating tests.

## Official upstream sources

Servy installation logic is based on official upstream docs:

- Docker Engine: https://docs.docker.com/engine/install/
- GitHub CLI Linux packages: https://github.com/cli/cli/blob/trunk/docs/install_linux.md
- Caddy Debian/Ubuntu/Raspbian packages: https://caddyserver.com/docs/install#debian-ubuntu-raspbian
- nvm: https://github.com/nvm-sh/nvm
- pnpm: https://pnpm.io/installation
- Bun: https://bun.sh/docs/installation

## Uninstall Servy

```sh
sudo rm -f /usr/local/bin/servy
sudo rm -rf /var/log/servy
```

Servy does not automatically uninstall packages, users, firewall rules, swapfiles, Docker data, or Caddy because those resources may be in active use. Use the apply logs and plan output to reverse specific module actions manually.

## Contributing

Issues and PRs are welcome after triage. Because Servy can mutate production servers, every change must preserve safety invariants and include tests for behavior that can break.

Start with:

- [`CONTRIBUTING.md`](CONTRIBUTING.md)
- [`SECURITY.md`](SECURITY.md)
- [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml)
- [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml)

## License

MIT — see [`LICENSE`](LICENSE).
