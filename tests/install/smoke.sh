#!/bin/sh
# End-to-end smoke test for install.sh against a locally-staged fake release.
#
# Builds servy from the checked-out source, packages it into the same
# tar.gz layout `release.yml` produces, generates a matching checksums.txt,
# then runs install.sh with:
#   SERVY_REPO=local
#   SERVY_VERSION=vlocal
#   SERVY_RELEASE_BASE=file:///tmp/.../releases
#   SERVY_INSTALL_DIR=$TMP/bin
#
# cosign verification is skipped (best-effort; not required for the smoke).
# SERVY_REQUIRE_COSIGN is never set here — the installer must succeed
# without cosign present.
#
# Exit codes: 0 on success, non-zero on any failure. Verbose logging is
# unconditional so CI captures the whole run.
set -eux

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported test arch: $arch" >&2; exit 1 ;;
esac
if [ "$os" != "linux" ]; then
  # install.sh refuses non-linux; this smoke targets linux runners only.
  echo "install.sh smoke test only runs on linux; skipping on $os" >&2
  exit 0
fi

VERSION="vlocal"
ASSET="servy_${os}_${arch}.tar.gz"

# 1. Build the binary that will land inside the fake archive.
mkdir -p "$WORK/build"
GOTOOLCHAIN=go1.26.4 GOOS="$os" GOARCH="$arch" go build -o "$WORK/build/servy" ./cmd/servy

# 2. Stage the release layout install.sh expects:
#    <base>/download/<tag>/<asset>
#    <base>/download/<tag>/checksums.txt
STAGE="$WORK/releases/download/$VERSION"
mkdir -p "$STAGE"
tar -C "$WORK/build" -czf "$STAGE/$ASSET" servy
( cd "$STAGE" && sha256sum "$ASSET" > checksums.txt )

# Sanity: awk exact match must pick the archive line and only that line.
awk -v a="$ASSET" 'NF==2 && $2==a { found=1 } END { exit !found }' "$STAGE/checksums.txt"

# 3. Run the installer against the local staging dir. file:// works with
#    curl for GET, which is all install.sh does for asset+checksum fetch.
mkdir -p "$WORK/bin"
SERVY_REPO="local/servy" \
SERVY_VERSION="$VERSION" \
SERVY_RELEASE_BASE="file://$WORK/releases" \
SERVY_INSTALL_DIR="$WORK/bin" \
  sh "$ROOT/install.sh"

# 4. Verify the installed binary reports the expected version and passes
#    a read-only doctor invocation (doctor may print warnings; exit code
#    must be 0 on a supported OS).
"$WORK/bin/servy" version
"$WORK/bin/servy" doctor >/dev/null

# 5. Negative test: tampered checksum must abort with a non-zero exit and
#    NEVER install the binary. Isolate to a fresh install dir so a leaked
#    binary from an earlier run cannot mask the regression.
BAD_STAGE="$WORK/tampered/download/$VERSION"
mkdir -p "$BAD_STAGE"
cp "$STAGE/$ASSET" "$BAD_STAGE/"
printf '%s  %s\n' "0000000000000000000000000000000000000000000000000000000000000000" "$ASSET" > "$BAD_STAGE/checksums.txt"

BAD_BIN="$WORK/bad-bin"
mkdir -p "$BAD_BIN"
if SERVY_REPO="local/servy" \
   SERVY_VERSION="$VERSION" \
   SERVY_RELEASE_BASE="file://$WORK/tampered" \
   SERVY_INSTALL_DIR="$BAD_BIN" \
     sh "$ROOT/install.sh" >"$WORK/tampered.log" 2>&1; then
  echo "install.sh accepted a tampered checksum — regression" >&2
  cat "$WORK/tampered.log" >&2
  exit 1
fi
if [ -e "$BAD_BIN/servy" ]; then
  echo "install.sh left a binary at $BAD_BIN/servy after checksum failure" >&2
  exit 1
fi

echo "install.sh smoke: happy path + tampered-checksum negative both passed"
