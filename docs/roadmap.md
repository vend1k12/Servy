# Servy roadmap

Servy is intentionally a small, boring tool. This file records what we are working toward and what we deliberately will not do.

## Current milestone: v0.1.0 — first public preview

Public preview, not "production ready". Semver stays at `0.x` while the API and safety model are hardened based on real use.

Blockers for tagging `v0.1.0`:

1. `install.sh` checksum matching uses `awk` exact match (not free-form `grep`).
2. Docker and Caddy apt keyring downloads are verified against pinned GPG fingerprints.
3. `doctor` reports honest disk and memory status (not constant `true`).
4. `base` module uses `dpkg-query` to mark installed packages as `already_ok` instead of always `will_run`.
5. `apply` blocked-by-confirmation errors are actionable (name the exact `confirmations.*` key and link the docs page).
6. Release artifacts are signed with cosign (keyless OIDC) and `install.sh` / `servy update` verify signatures.
7. GitHub Actions are pinned by commit SHA.
8. CI runs `golangci-lint` (staticcheck, errcheck, gosec, revive).
9. `CHANGELOG.md` exists and follows Keep a Changelog.
10. `SECURITY.md` contains an explicit threat model.

Once the preview binary is signed and CI is hardened, the tag ships.

## Milestone: v1.0 — publicly usable

Blockers for `v1.0`:

- Full mutation matrix on real VPS images: Ubuntu 22.04, 24.04, 26.04 (once codename is confirmed) × amd64, arm64; Debian 12, 13 × amd64, arm64.
- `servy revert <module>` to roll back Servy-owned side effects (drop-ins, apt list files, swapfile line).
- `WriteSSHDDropIn` uses the same `Openat` / `NOFOLLOW` pattern as `AppendAuthorizedKey`.
- `--yes` failure message names the missing confirmation and links `docs/safety.md`.
- Docker-smoke runs the four base images as a matrix and uploads build logs on failure.
- `install.sh` is smoke-tested in CI against a local fake release.
- SLSA provenance and SBOM published alongside signed artifacts.
- `docs/troubleshooting.md`, `docs/faq.md`, `docs/safety.md` are written.

## Milestone: v1.x — quality of life

- Colored, grouped plan output with `--no-color`.
- `apply --json` for CI pipelines.
- `servy logs list|show|tail` instead of the current path-print stub.
- `servy explain <step-id>` and `servy explain err <code>`.
- Optional non-language modules: nginx (check-only + install), postgres (check-only + install), backup-hook.
- `modules.base.packages` / `modules.base.tools` config surface so minimal hosts can drop `gh`, `tmux`, `nano`, etc.
- Rename `node` profile to `web-app` with a deprecation alias.

## Milestone: v2.0 (possible, not committed)

- Opt-in `servy remote apply --host <ssh-target>` that reuses the same plan model.
- Additional non-language modules the community keeps asking for.
- Formal plugin path (evaluate carefully; still a possible non-goal — see below).

## Real VPS validation matrix (run per pre-release)

For each Ubuntu/Debian × amd64/arm64 combination on a clean disposable image:

```sh
servy doctor
servy validate --config examples/base.yml
servy apply --config examples/base.yml --dry-run
servy apply --config examples/docker-only.yml --dry-run
servy update check
```

Then in disposable snapshots, mutation tests:

1. `base` with swap disabled and enabled.
2. `docker-only` without deploy-user docker group.
3. `docker-only` with explicit `dockerGroupRootEquivalent` confirmation.
4. UFW enablement with detected SSH port.
5. Caddy `check-only` and `host` modes.
6. `node` profile with explicit `installUserTooling` confirmation.

## Repository / release readiness checklist

Governance items that need to be set up in the GitHub UI (not source-controlled):

1. Protect `main` and `v*` tags.
2. Require CI, CodeQL, dependency review, and Docker smoke on `main`.
3. Enable GitHub Security Advisories.
4. Create the `release` environment with required reviewers.
5. Set up recommended issue labels (`bug`, `enhancement`, `security`, `docs`, `good first issue`, `needs-triage`, `blocked`, `module/docker`, `module/caddy`, `module/firewall`, `module/node`, `breaking-change`).

## Non-goals

Servy is small on purpose. These are permanent non-goals unless there is an explicit, documented reason to reconsider.

- **No remote SSH orchestration in v1.** Reconsidered only if v2 explicitly opens the door.
- **No Ansible / Terraform / OpenTofu replacement.** Servy runs one host at a time.
- **No project-specific `docker-compose.yml` or `Caddyfile` generation.**
- **No DNS or domain management.**
- **No backup or monitoring subsystem.**
- **No TUI.**
- **No plugin system in v1.** Attack surface is not worth it until the core is stable.
- **No Fedora / Arch / Alpine.** Servy is Ubuntu/Debian only.
- **No cloud-init replacement.** Cloud-init runs first; Servy runs afterwards on the host.
