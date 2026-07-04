#!/bin/sh
set -eu

PATH='/usr/sbin:/usr/bin:/sbin:/bin'
export PATH

REPO="${SERVY_REPO:-vend1k12/servy}"
VERSION="${SERVY_VERSION:-latest}"
INSTALL_DIR="${SERVY_INSTALL_DIR:-/usr/local/bin}"
BIN="servy"
TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT INT TERM

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }
}

need uname
need tr
need curl
need sha256sum
need tar
need grep
need awk
need id
need install

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  linux) ;;
  *) echo "unsupported OS: $os (Servy installer only supports Linux)" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

base="https://github.com/$REPO/releases"
asset="servy_${os}_${arch}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  url="$base/latest/download/$asset"
  sums="$base/latest/download/checksums.txt"
else
  url="$base/download/$VERSION/$asset"
  sums="$base/download/$VERSION/checksums.txt"
fi

curl -fsSL "$url" -o "$TMP/$asset"
curl -fsSL "$sums" -o "$TMP/checksums.txt"

# Extract the exact `<sha256>  <asset>` line. Free-form grep would ambiguously
# match future sibling artifacts (for example `<asset>.sig`) or expansion in
# `$asset` metacharacters, so we require an exact filename match.
awk -v a="$asset" 'NF==2 && $2==a { print; found=1 } END { exit !found }' \
    "$TMP/checksums.txt" > "$TMP/expected.sha" || {
    echo "no sha256 entry for $asset in checksums.txt" >&2
    exit 1
}
(
  cd "$TMP"
  LC_ALL=C sha256sum -c expected.sha
)

# Optional cosign keyless verification. When cosign is installed and the release
# ships a `.sig` + `.pem` next to the archive, we require the signature to
# match the expected release-workflow identity/issuer. This is the second trust
# anchor: sha256 alone shares fate with the release storage, cosign identity is
# rooted in Sigstore's Fulcio + Rekor transparency log.
#
# Set SERVY_REQUIRE_COSIGN=1 to hard-fail when either cosign is missing or the
# signature files are absent. Default is best-effort so casual installs on
# machines without cosign still succeed while pre-signed releases roll out.
if command -v cosign >/dev/null 2>&1; then
  if [ "$VERSION" = "latest" ]; then
    sig_url="$base/latest/download/${asset}.sig"
    pem_url="$base/latest/download/${asset}.pem"
  else
    sig_url="$base/download/$VERSION/${asset}.sig"
    pem_url="$base/download/$VERSION/${asset}.pem"
  fi
  if curl -fsSL "$sig_url" -o "$TMP/${asset}.sig" \
     && curl -fsSL "$pem_url" -o "$TMP/${asset}.pem"; then
    identity_re="^https://github\\.com/${REPO}/\\.github/workflows/release\\.yml@"
    cosign verify-blob \
      --certificate       "$TMP/${asset}.pem" \
      --signature         "$TMP/${asset}.sig" \
      --certificate-identity-regexp "$identity_re" \
      --certificate-oidc-issuer     "https://token.actions.githubusercontent.com" \
      "$TMP/$asset" >/dev/null
    echo "cosign: signature verified for $asset" >&2
  else
    if [ "${SERVY_REQUIRE_COSIGN:-0}" = "1" ]; then
      echo "cosign signature files missing for $asset and SERVY_REQUIRE_COSIGN=1" >&2
      exit 1
    fi
    echo "cosign: no signature published for $VERSION yet (sha256 verified above)" >&2
  fi
else
  if [ "${SERVY_REQUIRE_COSIGN:-0}" = "1" ]; then
    echo "SERVY_REQUIRE_COSIGN=1 but cosign is not installed" >&2
    exit 1
  fi
  echo "cosign not installed; skipping signature verification (sha256 verified above)" >&2
fi

members="$(tar -tzf "$TMP/$asset")"
if [ "$members" != "servy" ]; then
  echo "release archive must contain exactly one regular member named servy" >&2
  exit 1
fi
tar -xzf "$TMP/$asset" -C "$TMP" servy
if [ ! -f "$TMP/servy" ] || [ ! -x "$TMP/servy" ]; then
  echo "release archive did not contain executable servy" >&2
  exit 1
fi

if [ "$(id -u)" -ne 0 ] && [ ! -w "$INSTALL_DIR" ]; then
  need sudo
  /usr/bin/sudo /usr/bin/install -m 0755 "$TMP/servy" "$INSTALL_DIR/$BIN"
else
  /usr/bin/install -m 0755 "$TMP/servy" "$INSTALL_DIR/$BIN"
fi

"$INSTALL_DIR/$BIN" version
"$INSTALL_DIR/$BIN" doctor || true
