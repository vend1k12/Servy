# Servy architecture

## Core shape

Servy is a single Go binary. The CLI layer is thin; it loads config, detects platform state, asks modules for a plan, prints that plan, and only then runs eligible steps.

```text
cmd/servy/main.go
  -> internal/cli
      -> internal/config
      -> internal/platform
      -> internal/modules
      -> internal/plan
      -> internal/runner
      -> internal/logging
      -> internal/doctor
```

## Module contract

Each module implements:

```go
type Module interface {
    Name() string
    Plan(modules.Context) []plan.Step
}
```

A module must be idempotent by planning from observed state:

1. Inspect current state through `system.State`.
2. Return `already_ok` when no mutation is needed.
3. Return `will_run` only for necessary actions.
4. Return `needs_confirmation` or `dangerous` for lockout-risk actions.
5. Include recovery hints for steps with partial-failure risk.

Modules must not directly execute commands. They only produce `plan.Step` values.

## Plan statuses

- `will_run`: runner may execute during `apply --yes`.
- `already_ok`: state already satisfies the module.
- `will_skip`: module/option disabled.
- `needs_confirmation`: config must explicitly confirm before apply.
- `dangerous`: must not run from profile defaults.
- `unsupported`: host is outside supported OS/arch/package-manager matrix.
- `failed_precondition`: requested operation cannot safely proceed.

`runner.Apply` refuses any blocking status before executing the first command.

## Command execution policy

- Prefer argv commands (`exec.CommandContext`) over shell strings.
- Shell is allowed only where system tools require atomic redirection/appending; keep input fixed or safely quoted.
- Docker and Caddy use official apt repository flows, not remote convenience scripts.
- User-level tools (`nvm`, `pnpm`, `bun`) use explicit official URLs and run as the selected target user.
- Future hardening should reduce shell usage further by moving file writes into Go.

## Logging

Mutating `apply` runs write JSONL logs under `/var/log/servy/` with timestamp, command, profile, config path, OS info, step, output, exit code, and error. Steps that may contain SSH public key data are redacted.

## Safety invariants

- `init` writes YAML and previews; it never silently configures the host.
- `plan` and `apply --dry-run` never mutate.
- `--yes` is not a dangerous-action override.
- UFW enablement requires an SSH allow step in the same plan plus explicit confirmation.
- SSH root/password/restrict-users changes need independent confirmations and recovery hints.
- Existing Caddy config, UFW rules, SSH config, and `authorized_keys` must not be reset or overwritten.
