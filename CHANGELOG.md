# Changelog

All notable changes to Servy will be recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Servy is pre-1.0, so breaking changes may still land in any `0.x` release; they will be called out here.

## [Unreleased]

### Added
- `docs/roadmap.md`: v0.1.0 / v1.0 / v1.x milestones, explicit non-goals, real-VPS validation matrix.
- `docs/history/`: `mvp-v0.md` (historical MVP plan) and `original-spec-ru.md` (original Russian brief). Both carry a HISTORICAL banner so new readers know they are archival.
- Explicit "Who Servy is for" / "Do not use Servy if" block in `README.md`, plus a compare table against ad-hoc bash, `ansible-pull`, and cloud-init.
- `safeops.InstallAptKeyring`: HTTPS-only, size-capped, atomic apt-keyring installer that verifies the pinned primary GPG fingerprint via `gpg --show-keys`.
- Pinned Docker (`9DC858229FC7DD38854AE2D88D81803C0EBFCD88`) and Caddy (`65760C51EDEA2017CEA2CA15155B6D79CA56EA34`) apt-keyring fingerprints.
- Hidden `servy internal install-apt-keyring` subcommand used by Docker and Caddy module planning to enforce keyring pinning.
- Cosign keyless signing in `.github/workflows/release.yml`. Every `servy_linux_*.tar.gz` and `checksums.txt` ships `.sig` + `.pem`. Signing identity is pinned to `vend1k12/Servy/.github/workflows/release.yml`. Reproducible-build tweaks: `SOURCE_DATE_EPOCH` from commit time, `tar --mtime/--sort/--owner/--group/--numeric-owner`. Release now validates `examples/*.yml` with the freshly built binary before signing.
- Cosign verification in `install.sh` (best-effort by default, hard-fail with `SERVY_REQUIRE_COSIGN=1`).
- Cosign verification in `servy update` (best-effort by default, hard-fail with `--require-cosign`).
- `internal/plan.BlockingError`: typed error carrying every blocker, printed with confirmation-key + rationale + recovery hints. Replaces the previous one-line generic message.
- `web-app` profile as the canonical successor to `node`. `node` remains a deprecated alias, both in `--preset` and in the `profile:` YAML field.
- `modules.base.packages` / `modules.base.tools` config surface so operators can drop `gh`, `tmux`, `nano` from the curated default set or replace it wholesale. Package names validated against Debian policy regex.
- `modules.base.installGitHubCLI: false` skips the `cli.github.com` apt-repository flow.
- New `examples/web-app.yml`. `examples/node.yml` retained as a deprecation showcase.
- `.golangci.yml` and `lint` job in `.github/workflows/ci.yml`. Enabled: `errcheck`, `gosec`, `govet`, `ineffassign`, `misspell`, `revive`, `staticcheck`, `unconvert`, `unused`. Every exclusion carries an inline comment explaining why.
- Doctor tests (`internal/doctor/doctor_test.go`) covering `parseMemInfo` and `humanBytes`.
- Runner tests now assert `*plan.BlockingError` is returned with confirmation-key text.
- Package-level doc comments on every file under `internal/`.
- `dependabot.yml` covers the docker ecosystem for `tests/docker`.
- `docs/safety.md`: safety invariants, per-key `confirmations.*` reference, SSH lockout playbook, manual rollback table until `servy revert` lands.
- `docs/troubleshooting.md`: catalog of user-visible error messages (apply refusals, doctor warnings, SSH lockouts, keyring failures, cosign / update / install issues) with recovery steps.
- `docs/faq.md`: positioning vs Ansible / cloud-init / bash / container managers, supported hosts, install and update flows, hand-verification recipe for release archives.

### Changed
- `README.md` "Release status" no longer implies pre-v1 is production-ready.
- `install.sh` checksum lookup switched from a free-form multi-branch `grep` to an `awk` exact-match against `$2`. Adds `awk` to the required tools and runs `sha256sum -c` under `LC_ALL=C`.
- `install.sh` now attempts cosign keyless verification when signatures are published and cosign is installed.
- `README.md` `curl … | sh` example replaced with the safer download-then-inspect pattern; documents `SERVY_REQUIRE_COSIGN` and `servy update --require-cosign`.
- `Base.Plan` queries `dpkg-query` and marks already-installed packages as `already_ok`. `apt-get update` and `apt-get install` are skipped entirely when nothing needs installing. Only missing packages appear in the `apt-get install` argv.
- `Docker` module: `docker.gpg` + `docker.gpg.perms` collapsed into a single `docker.keyring.install` step backed by the pinned-fingerprint installer.
- `Caddy` module: `caddy.key.install` (which shelled out to `gpg --dearmor`) replaced with the pinned-fingerprint installer. Apt reads armored keyrings natively on Ubuntu 22.04+ / Debian 12+.
- `runner.Apply` now returns `*plan.BlockingError` instead of a first-hit `fmt.Errorf`.
- `apply --yes` refusal message names the exact confirmation key required, references dry-run, and links `docs/safety.md`.
- All GitHub Actions in `.github/workflows/` are pinned to commit SHA with `# vN` comments; dependabot still opens bump PRs.

### Fixed
- `doctor` no longer reports `[ok]` unconditionally for the disk and memory checks. `diskStatus` uses `syscall.Statfs` on `/` (warns when free `< 2 GiB` OR ratio `< 10%`); `memStatus` parses `MemAvailable` from `/proc/meminfo` (warns when `< 512 MiB`). Non-Linux dev hosts fall back to `ok` with a "meminfo unavailable" note.
- The `--yes` refusal error no longer ends with a period (staticcheck `ST1005`).

### Removed
- `docs/mvp-plan.md` (moved to `docs/history/mvp-v0.md` with a HISTORICAL banner).
- `docs/next-actions.md` (content merged into `docs/roadmap.md`).
- `universal-vps-setup-cli-spec.md` from the repo root (moved to `docs/history/original-spec-ru.md`).

### Security
- Apt keyring downloads verify pinned GPG fingerprints; TLS alone is no longer the sole trust anchor for a keyring that later authorises root packages.
- Release archives are cosign-signed keyless via the `release.yml` workflow. `install.sh` and `servy update` verify signatures when cosign is installed.
- `SECURITY.md` now contains an explicit threat model.
- `WriteSSHDDropIn` now uses the same `Openat` / `O_NOFOLLOW` / `Renameat` pattern as `AppendAuthorizedKey`. `/etc/ssh` and `/etc/ssh/sshd_config.d` are opened with `O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC`, the drop-in is staged via a sibling tempfile and swapped in with `Renameat`, and `sshd -t` failure now reverts through the same directory FD via `Renameat` / `Unlinkat`. A symlink planted between validate and revert can no longer redirect the write outside the drop-in directory.

## [0.0.2] - 2026-06-15

Initial pre-release with strict YAML config, `doctor`, `plan`, `apply --dry-run`, `apply --yes`, per-module confirmation gates, and Docker + Caddy + UFW + swap + deploy-user + hardening + Node tooling planners. See git history for details.
