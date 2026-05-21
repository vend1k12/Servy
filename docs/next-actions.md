# Next actions before public v1

## Release readiness

1. Protect `main` and `v*` tags in GitHub settings.
2. Require CI, CodeQL, dependency review, and Docker smoke tests before merge.
3. Enable GitHub Security Advisories.
4. Create the `release` environment and require manual approval for release jobs.
5. Tag the first pre-release only after Docker smoke tests and at least one real VPS dry-run pass.

## Real VPS validation matrix

Run on clean servers or disposable VMs:

- Ubuntu 22.04 amd64/arm64
- Ubuntu 24.04 amd64/arm64
- Debian 12 amd64/arm64
- Debian 13 amd64/arm64

For each:

```sh
servy doctor
servy validate --config examples/base.yml
servy apply --config examples/base.yml --dry-run
servy apply --config examples/docker-only.yml --dry-run
```

Then run mutating tests in snapshots only:

1. `base` with swap disabled/enabled.
2. `docker-only` without deploy user docker group.
3. `docker-only` with explicit docker group confirmation.
4. firewall enablement with detected SSH port.
5. Caddy `check-only` and `host` modes.
6. node profile with explicit `installUserTooling` confirmation.

## High-value engineering follow-ups

1. Move Docker/Caddy repository file writes from shell to Go safe file operations.
2. Add fingerprint checks for Docker and Caddy apt GPG keys.
3. Add integration tests for `safeops.AppendAuthorizedKey` in a Linux container.
4. Add `servy doctor --json` for issue reports and automation.
5. Add JSON output for `plan` to support CI policy checks.
6. Add shell completions after CLI stabilizes.
7. Add release artifact signing with a maintainer-controlled public key before recommending `install.sh` for production.

## Issue labels

Recommended initial labels:

- `bug`
- `enhancement`
- `security`
- `docs`
- `good first issue`
- `needs-triage`
- `blocked`
- `module/docker`
- `module/caddy`
- `module/firewall`
- `module/node`
- `breaking-change`
