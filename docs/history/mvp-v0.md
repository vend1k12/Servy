> **Historical planning document.** All MVP phases below were delivered; kept here for provenance.
> For current direction see [`docs/roadmap.md`](../roadmap.md).

# Servy MVP plan

## Decisions

- Binary and public command name: `servy`.
- Language: Go, single static-friendly binary.
- CLI framework: Cobra.
- Config: strict YAML via `gopkg.in/yaml.v3`; unknown keys fail validation.
- Distribution: GitHub Releases with `linux/amd64` and `linux/arm64` archives plus SHA256 checksums.
- Bootstrap: `install.sh` installs/verifies the binary only, then runs `servy doctor`; it never configures the server.

## Architecture

1. `internal/cli`: command wiring and UX.
2. `internal/config`: schema, defaults, validation, YAML load/write.
3. `internal/platform`: OS, codename, architecture, apt, systemd, user, root/sudo detection.
4. `internal/modules`: idempotent module planners.
5. `internal/plan`: typed execution steps and statuses.
6. `internal/runner`: argv-based execution for `will_run` steps only.
7. `internal/logging`: JSONL mutation logs under `/var/log/servy/`.
8. `internal/doctor`: read-only host diagnostics.
9. `internal/system`: state abstraction for real host and tests.

## MVP phases

### Phase 1: CLI skeleton

- Cobra root and commands: `doctor`, `init`, `validate`, `plan`, `apply`, `status`, `version`, `module list`, `module status`, `logs`.
- Product name isolated in `internal/app` for future rename safety.
- CI with `go test ./...` and `go vet ./...`.

### Phase 2: config and planner

- Strict YAML schema with profiles: `base`, `docker-only`, `node`.
- Module options for Docker, Caddy, UFW, swap, deploy user, hardening, Node tooling.
- Plan statuses: `will_run`, `already_ok`, `will_skip`, `needs_confirmation`, `dangerous`, `unsupported`, `failed_precondition`.
- `plan` and `apply --dry-run` must never mutate.

### Base profile

- Install base server utilities: `git`, `gh`, `jq`, `unzip`, `rsync`, `tmux`, `htop`, `nano`.
- Install GitHub CLI from the official `cli.github.com` apt repository with published keyring SHA256 verification.

### Phase 3: docker-only profile

- Official Docker apt repository flow.
- Install `docker-ce`, `docker-ce-cli`, `containerd.io`, `docker-buildx-plugin`, `docker-compose-plugin`.
- No snap, Docker Desktop, convenience script, or Compose v1.
- Preserve existing Docker installs and only start service if needed.

### Phase 4: optional safety modules

- UFW with SSH allow rule before enablement and explicit `confirmations.enableFirewall`.
- Swapfile idempotency.
- Deploy user creation and append-only SSH keys.
- Hardening split into independent options with separate confirmations for lockout-risk changes.

### Phase 5: node profile

- User-level nvm install.
- Node.js via nvm.
- Optional pnpm and Bun using official sources.
- Never enabled by `docker-only` defaults.

### Phase 6: Caddy

- Optional host-level Caddy install using official Cloudsmith apt package flow.
- `none`, `host`, and `check-only` modes.
- No project Caddyfile generation or overwrite.

### Phase 7: open-source polish

- README, architecture docs, AI-agent notes, examples, install script, license, CI, release workflow.
- User docs must explain what Servy does not do, safety model, supported OS, dry-run, profiles, and uninstall.

## Verification strategy

- Unit tests for config validation and unknown YAML keys.
- Unit tests for profile defaults and module planning.
- Unit tests for unsupported OS blocking before mutation.
- Unit tests for dangerous confirmation gates and firewall safety.
- Unit tests for runner refusing blocking plans and skipping non-`will_run` steps.
- Build binary and validate example configs.

## Current MVP status

This repository now contains the first implementation slice: CLI skeleton, config schema, planner, core module plans, doctor, runner, logging, examples, docs, installer, CI, release workflow, and targeted tests. Full end-to-end mutation testing on real Ubuntu/Debian VPS images remains required before tagging a public v1 release.
