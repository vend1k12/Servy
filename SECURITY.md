# Security policy

## Supported versions

Servy is pre-v1. Until the first public release, only the default branch is supported.

## Reporting vulnerabilities

Please do not open public issues for vulnerabilities that could help compromise servers.

Report privately through GitHub Security Advisories once the repository is public, or contact the maintainer directly if advisories are not yet enabled.

Include:

- affected commit or release;
- host OS and architecture;
- exact config and command used, with secrets removed;
- expected vs actual behavior;
- impact and reproduction steps.

## Security model

Servy runs locally on the server and may execute privileged commands. Its safety model relies on:

- explicit plans before mutation;
- strict config validation;
- confirmation gates for dangerous actions;
- official upstream installation sources;
- no silent SSH lockout-risk changes;
- preserving existing host configuration by default.

## Release integrity

Since v0.1.0-preview:

- Every release archive and `checksums.txt` is signed by cosign keyless OIDC via `.github/workflows/release.yml`. The signing identity is `https://github.com/vend1k12/Servy/.github/workflows/release.yml@refs/tags/vX.Y.Z`, issuer `https://token.actions.githubusercontent.com`.
- `install.sh` and `servy update` verify the signature when cosign is installed; `SERVY_REQUIRE_COSIGN=1` and `servy update --require-cosign` make the check mandatory.
- `checksums.txt` is signed alongside the archives so downstream automation can pin trust to the signed checksums file and derive per-archive sha256 from it.

Release tags should be created from a protected branch and signed. The release workflow lives in the `release` environment with required-reviewer protection so that pushing a `v*` tag alone does not publish artifacts.

## Threat model

Servy is a CLI that runs on the target server, usually as root or via sudo. It downloads and adds apt repositories, writes to `/etc/apt/keyrings`, `/etc/sysctl.d`, `/etc/ssh/sshd_config.d`, `/etc/fstab`, `/var/log/servy`, and `~deploy/.ssh/authorized_keys`, and can update itself from GitHub Releases. The table below is the honest boundary of what Servy defends against and where the maintainer's expectations end.

| Asset | Threat | Mitigations in the repo today | Out of scope |
| --- | --- | --- | --- |
| Root shell on the target host | Compromised GitHub release | sha256 in `checksums.txt` + cosign keyless signature pinned to this workflow. `install.sh` and `servy update` verify both. `SERVY_REQUIRE_COSIGN=1` hard-fails without cosign. | Compromise of the maintainer's GitHub account credentials, or of Sigstore Fulcio/Rekor infrastructure itself. |
| Root shell on the target host | Compromised GitHub Actions token | Release workflow uses only first-party actions checked out via `git init` + `git fetch`. `contents: write` scoped to the `release` environment; every other workflow is `contents: read`. Actions in other workflows are SHA-pinned so a moved tag cannot silently swap the code. | A malicious PR that convinces a reviewer to merge a workflow change. |
| Root shell on the target host | Hostile apt keyring served over TLS | `safeops.InstallAptKeyring` HTTPS-downloads the keyring, verifies against the pinned Docker (`9DC858…CD88`) and Caddy (`65760C…EA34`) primary GPG fingerprints, then atomically installs at mode 0644. | Upstream project rotating the signing key without publishing the new fingerprint. Rotation requires a Servy PR. |
| Deploy-user account | Hostile user-tooling installer (nvm/bun) | Confirmation-gated (`confirmations.installUserTooling`) and executed as the deploy user, not as root. Pinned nvm tag. | The installers themselves — the maintainers of nvm/bun are the ultimate trust anchor. |
| SSH access | Firewall lockout | UFW enable requires an SSH allow step in the same plan plus `confirmations.enableFirewall`. `--yes` never overrides confirmations. | Users editing `sshd_config` outside Servy after apply. |
| SSH access | Hardening lockout (`PermitRootLogin`, `PasswordAuthentication`, `AllowUsers`) | Each option needs its own `confirmations.disable*` / `restrictSSHUsers`. `WriteSSHDDropIn` runs `sshd -t` before reload; failure reverts the drop-in file. | The maintainer console access on the VPS if `sshd -t` passes but the deploy user cannot actually log in. |
| `authorized_keys` files | Symlink attack, TOCTOU | `AppendAuthorizedKey` uses `O_NOFOLLOW`, `Openat`, `Fstat`, `Fchown`, `Fchmod`. Append-only, refuses non-regular files. | An attacker who already has write access to `~deploy` before Servy runs. |
| `PATH` and argv[0] resolution | Command-hijack via poisoned `/usr/local/bin/apt` | `safepath.LookPath` restricts argv[0] to a fixed `/usr/sbin:/usr/bin:/sbin:/bin` and requires root-owned, non-world-writable executables + parents. | Attacker who has already compromised `/usr/sbin` (root already). |
| Log integrity | Secret leak in `/var/log/servy` | `RedactCommandInLogs` is set for steps whose argv contains user-supplied secrets (currently: SSH public keys). Log files are created with mode 0600. | Anything Servy does not know is a secret. Do not put secrets in module descriptions. |

### Explicitly out of scope for v1

- Compromised BIOS, firmware, or hypervisor.
- Physical access to the host.
- Attackers who already have root on the target machine.
- Denial of service against `download.docker.com`, `cli.github.com`, or `dl.cloudsmith.io`.
- Configuration files edited by hand after Servy applies; Servy does not lock or fingerprint them.
