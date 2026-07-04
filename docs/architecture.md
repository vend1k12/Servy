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
      -> internal/update
```

## Config generation vs runtime config

`servy init` is a local YAML writer, not part of the apply path. Presets such as `base`, `docker-only`, and `node` are generation-time templates that produce ordinary config files; they are not alternate runtime schemas and do not bypass validation.

Custom init mode should enumerate the module options exposed by the current strict config schema and write the selected values into YAML. Confirmation fields remain explicit and safe by default, so generated configs still rely on `validate`, `plan`, `apply --dry-run`, and a reviewed `apply --yes` before anything mutates the host.

`plan --json` and `doctor --json` are read-only machine-output modes over the same planning and diagnostics data used by the human-readable commands.

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
- Apt keyrings are downloaded through `safeops.InstallAptKeyring`, which pins the primary GPG fingerprint. TLS is not sufficient trust for a keyring that authorises root packages.
- User-level tools (`nvm`, `pnpm`, `bun`) use explicit official URLs and run as the selected target user.
- Future hardening should reduce shell usage further by moving file writes into Go.

`servy update` is intentionally separate from host setup modules: it downloads GitHub Release assets, verifies `checksums.txt`, extracts an archive containing only the `servy` binary, and atomically replaces the target binary.

## Logging

Mutating `apply` runs write JSONL logs under `/var/log/servy/` with timestamp, command, profile, config path, OS info, step, output, exit code, and error. Steps that may contain SSH public key data are redacted.

## Safety invariants

- `init` writes generated YAML and previews; it never silently configures the host.
- `plan` and `apply --dry-run` never mutate.
- `--yes` is not a dangerous-action override.
- UFW enablement requires an SSH allow step in the same plan plus explicit confirmation.
- SSH root/password/restrict-users changes need independent confirmations and recovery hints.
- Existing Caddy config, UFW rules, SSH config, and `authorized_keys` must not be reset or overwritten.
