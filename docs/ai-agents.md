# Notes for AI agents

Servy is a load-bearing server setup tool. Optimize for safety and idempotency over convenience.

## Before changing installation logic

Re-check official upstream docs:

- Docker Engine Ubuntu/Debian install docs.
- Caddy Debian/Ubuntu/Raspbian package docs.
- GitHub CLI Linux package docs.
- nvm official README.
- pnpm installation docs.
- Bun installation docs.
- Ubuntu/Debian supported release status.

Do not copy commands from blogs or Stack Overflow when official docs exist.

## Non-negotiable constraints

- Keep bootstrap install logic minimal; `install.sh` must only install the Servy binary and run/read next steps.
- Do not add remote SSH orchestration.
- Do not silently disable SSH password auth, root login, or change SSH ports.
- Do not overwrite existing Caddy, SSH, UFW, or `authorized_keys` content.
- Do not install Docker through snap, Docker Desktop, Docker convenience scripts, or Compose v1.
- Do not install GitHub CLI from distro community packages when the official `cli.github.com` apt repository is available.
- Do not make host-level Node tooling part of `docker-only` defaults.

## Testing expectations

Add tests for behavior, not command string snapshots:

- config unknown keys and invalid combinations;
- unsupported OS/codename stops before mutation;
- profile-to-module mapping;
- dry-run/no-mutation paths;
- dangerous confirmation gates;
- firewall SSH safety;
- existing state preservation.

## Documentation expectations

When adding a module or changing defaults, update:

- `README.md` user-facing behavior and safety model;
- `docs/architecture.md` module contract if it changes;
- `examples/*.yml` if config schema changes;
- comments only when they explain a safety invariant or non-obvious upstream constraint.
