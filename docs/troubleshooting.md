# Troubleshooting

Errors, common failure modes, and how to recover. If you are looking for the safety model behind these behaviours, read [`docs/safety.md`](safety.md) first.

## `apply` refuses to run

### `refusing to apply without --yes`

Full message:

```
refusing to apply without --yes: review the plan above, then re-run with --yes
to execute non-dangerous steps; --yes never overrides confirmations.*
(see docs/safety.md)
```

Expected. `apply` requires `--yes` to actually mutate the host. Run:

```sh
servy apply --config servy.yml --dry-run   # verify plan
servy apply --config servy.yml --yes       # execute
```

### `cannot apply plan: N blocking step(s)`

At least one dangerous step is not confirmed. Servy lists every blocker with the exact `confirmations.*` key it needs:

```
cannot apply plan: 1 blocking step(s)
  [needs-confirmation] firewall.enable: enable ufw only after SSH allow rule is in plan
      set `confirmations.enableFirewall: true` in your config to allow this step
      recovery: use provider console to run `ufw disable` if SSH access is lost
```

Fix: add the named key to your YAML under `confirmations:`. See [`docs/safety.md`](safety.md#confirmations-reference) for what every key does and why Servy blocks without it.

`--yes` does not override this. That is intentional; the two locks are independent.

### `config file "servy.yml" not found`

Full message:

```
config file "servy.yml" not found; run `servy init --output servy.yml` or pass an existing --config path
```

Servy looks for `servy.yml`, `servy.yaml`, or `.servy.yml` in the current directory when `--config` is omitted. Generate one with `servy init --output servy.yml`, or point `--config` at your file.

### `unsupported profile "..."`

Valid profiles: `base`, `docker-only`, `web-app`, `node` (deprecated alias for `web-app`). Anything else is rejected at validation. Run `servy init --list-presets` for the canonical list.

### `schemaVersion must be "..."`

Your config was written for a different Servy release. Regenerate with `servy init --output servy.yml.new`, port your customisations over, and replace `servy.yml`.

### `modules.base.packages contains unsafe apt package name "..."`

Base package and tool names are validated against the Debian policy regex `^[a-z0-9][a-z0-9+\-.]+$`. If your name is legitimate but rejected, it violates policy for a reason — file an issue with the exact package name.

## Doctor warnings

### `[warn] disk: ... free (... % of ...) — below 2 GiB / 10 % threshold`

Free space on `/` is under 2 GiB or under 10 % of total. Apt installs (Docker, Caddy, base packages) can easily consume 1 GiB; expand the disk or free space before applying.

### `[warn] memory: ... MiB available — below 512 MiB threshold`

Available memory (from `/proc/meminfo` `MemAvailable`) is under 512 MiB. Apt operations and dockerd start-up will thrash swap. Either provision a swap file first (`modules.swap.enabled: true` with `size: 2G`), or size up the host.

### `[ok] memory: meminfo unavailable (not linux)`

You are running `doctor` on macOS or another non-Linux host. Expected; Servy applies only on Debian/Ubuntu, but `doctor` is safe to run anywhere.

## SSH lockout scenarios

### After enabling UFW, my SSH session dropped

Servy detects the SSH port from `ss -tnp` and adds an allow rule before enabling UFW. If your session used a non-standard port that Servy did not see (for example, a jump host with iptables NAT), the allow rule can miss.

Recovery from provider console:

```sh
ufw disable
# fix modules.firewall.sshPort in servy.yml
servy apply --config servy.yml --dry-run   # verify the SSH allow port
servy apply --config servy.yml --yes
```

### After `hardening.disable-root` / `disable-password`, deploy user cannot log in

`WriteSSHDDropIn` runs `sshd -t` before reload, so a syntactically broken drop-in reverts automatically. Logical lockout (no valid deploy key, wrong AllowUsers list) is not caught by `sshd -t`.

Recovery from provider console:

```sh
rm /etc/ssh/sshd_config.d/99-servy-hardening.conf
systemctl reload ssh
```

Then re-run apply with `PasswordAuthentication yes` temporarily, verify key login works, and re-enable hardening.

## Update / install issues

### `checksum mismatch for servy_linux_amd64.tar.gz: got X want Y`

Emitted by `servy update` and by `install.sh`. Either:

- The download was corrupted — retry.
- A middlebox is rewriting the archive — retry from a different network.
- The published `checksums.txt` was tampered with — do not proceed. Verify the release page on GitHub, and if the mismatch persists, file a security issue.

### `release archive must contain exactly one regular member named servy`

`servy update` expects a specific archive shape. If the assertion fires, the release was built by a workflow that does not match `release.yml`. Do not install; report the release tag on the tracker.

### `install: ... permission denied`

`install.sh` needs write access to `--install-dir` (default `/usr/local/bin`). Either run as root, or:

```sh
SERVY_INSTALL_DIR="$HOME/.local/bin" ./install.sh
```

### `cosign verification failed`

Emitted when `SERVY_REQUIRE_COSIGN=1` or `servy update --require-cosign` is set and the `.sig` / `.pem` sidecar does not verify against the pinned `vend1k12/Servy/.github/workflows/release.yml` identity.

If `cosign` is not installed, the strict mode fails hard by design. Install cosign (`https://docs.sigstore.dev/system_config/installation/`) and retry, or remove the flag to fall back to sha256-only verification.

## Apt keyring failures

### `keyring fingerprint mismatch: wanted <fp>, got [...]`

The keyring downloaded from Docker or Caddy did not carry the pinned primary fingerprint. Almost always means:

- Upstream rotated the signing key and the new fingerprint is not yet in Servy.
- A middlebox served a different keyring.

Do not bypass this. File an issue with the observed fingerprints and the URL, and wait for a Servy release that re-pins.

### `gpg not available for keyring verification`

Servy relies on `gpg` from the target host to extract fingerprints. Install `gnupg`:

```sh
apt-get update && apt-get install -y gnupg
```

### `keyring exceeds 1048577 bytes; refusing`

Official Docker/Caddy keyrings are ~5 KiB. A 1 MiB response is a strong signal something else is served at the URL. Do not proceed.

## Runner errors

### `step X exited N: <stderr excerpt>`

Servy stops at the first failed step. The full command, stdout, stderr, and exit code are written to `/var/log/servy/<timestamp>.jsonl`. Read that log:

```sh
tail -n 1 /var/log/servy/*.jsonl | jq .
```

Common cases:

- `apt-get install ...` — package name typo, or unreachable repo. Re-run `apt-get update` by hand to reproduce.
- `usermod -aG docker <user>` — the user does not exist yet because `deploy-user.create` failed earlier. The plan orders deploy-user creation before docker-group; if you see this out of order, file a bug.
- `ufw allow <port>/tcp` — UFW not installed. Ensure the plan's `firewall.install` step ran.

## Log locations

- Apply logs: `/var/log/servy/<UTC timestamp>.jsonl` (mode 0600).
- Apt sources: `/etc/apt/sources.list.d/{docker,caddy-stable}.list`.
- Apt keyrings: `/etc/apt/keyrings/{docker,caddy}.asc` (mode 0644, root-owned).
- SSH hardening drop-in: `/etc/ssh/sshd_config.d/99-servy-hardening.conf`.
- sysctl drop-in: `/etc/sysctl.d/99-servy.conf`.

## Getting help

Before opening an issue, please attach:

- `servy --version`.
- The exact command you ran.
- The output of `servy doctor` and `servy plan --config <path>`.
- The relevant `/var/log/servy/*.jsonl` entry, secrets redacted.

Report issues at `https://github.com/vend1k12/Servy/issues`.
Security-sensitive reports go to the security advisory tracker instead (see [`SECURITY.md`](../SECURITY.md)).
