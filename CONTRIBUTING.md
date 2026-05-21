# Contributing to Servy

Thanks for helping make server setup safer and more repeatable.

## Development setup

```sh
go test ./...
go vet ./...
go build ./cmd/servy
```

Docker smoke tests:

```sh
tests/docker/run.sh
```

## Safety rules

Servy mutates production servers. Changes must preserve these invariants:

- `plan` and `apply --dry-run` never mutate.
- No silent SSH lockout-risk changes.
- No overwriting existing SSH, Caddy, UFW, or `authorized_keys` files.
- No Docker install through snap, Docker Desktop, convenience scripts, or Compose v1.
- User-level tooling that downloads remote installers must remain separately confirmed.
- Commands must be argv-based where possible and run with a sanitized environment.

## Pull requests

Every PR should include:

1. What behavior changes.
2. Why the change is safe.
3. Tests or Docker smoke-test evidence.
4. Documentation updates when user-facing behavior changes.

## Adding or changing modules

A module must inspect state before planning mutations and must return one of the typed statuses from `internal/plan`.

Add tests for:

- unsupported OS/codename behavior;
- idempotency decisions;
- dangerous confirmation gates;
- preservation of existing host configuration;
- dry-run/no-mutation paths.
