<div align="center">

# Servy

**Safe, repeatable VPS setup for Ubuntu and Debian.**

Servy is a single-binary Go CLI that runs **inside** a server, detects the host, builds a readable execution plan, and applies only explicitly approved setup steps.

[![CI](https://github.com/vend1k12/servy/actions/workflows/ci.yml/badge.svg)](https://github.com/vend1k12/servy/actions/workflows/ci.yml)
[![CodeQL](https://github.com/vend1k12/servy/actions/workflows/codeql.yml/badge.svg)](https://github.com/vend1k12/servy/actions/workflows/codeql.yml)
[![Go](https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-pre--v1-orange)](#release-status)

[Quick start](#quick-start) · [Profiles](#profiles) · [Safety model](#safety-model) · [Examples](#examples) · [Contributing](#contributing)

</div>

---

## Why Servy exists

Fresh VPS setup is usually a choice between a one-off shell script, hand-written notes, or a full configuration-management stack. Servy sits in the middle:

- more structured and auditable than a large bash script;
- smaller and more local than Ansible/Terraform;
- safe enough to rerun;
- explicit about dangerous operations before they happen.

Servy is designed for personal servers, small production hosts, and repeatable developer infrastructure where you want a clear plan before touching SSH, firewall, Docker, users, or runtime tooling.

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

Servy is currently **pre-v1**. The CLI, config schema, and safety model are usable for local testing and disposable hosts, but a public production release should wait for real VPS validation across the supported OS matrix.

See [`docs/next-actions.md`](docs/next-actions.md) for the remaining release checklist.

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

Create a config interactively:

```sh
./servy init --output servy.yml
```

Validate and preview:

```sh
./servy validate --config servy.yml
./servy plan --config servy.yml
./servy apply --config servy.yml --dry-run
```

Apply after reviewing the plan:

```sh
sudo ./servy apply --config servy.yml --yes
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

## Commands

| Command | Purpose | Mutates host? |
| --- | --- | --- |
| `servy doctor` | Read-only host diagnostics | No |
| `servy init` | Interactive config wizard | Writes local YAML only |
| `servy validate --config servy.yml` | Strict YAML validation | No |
| `servy plan --config servy.yml` | Build execution plan | No |
| `servy apply --config servy.yml --dry-run` | Print apply plan | No |
| `servy apply --config servy.yml --yes` | Execute eligible steps | Yes |
| `servy status` | Read-only status summary | No |
| `servy module list` | List built-in modules | No |
| `servy module status <name> --config servy.yml` | Show module plan entries | No |
| `servy logs` | Print log directory | No |
| `servy version` | Print version metadata | No |

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
    enabled: true
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
| [`docs/mvp-plan.md`](docs/mvp-plan.md) | MVP scope and implementation plan |
| [`docs/architecture.md`](docs/architecture.md) | Maintainers changing internals |
| [`docs/ai-agents.md`](docs/ai-agents.md) | AI agents working on the repo |
| [`docs/next-actions.md`](docs/next-actions.md) | Release and real VPS validation checklist |
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
