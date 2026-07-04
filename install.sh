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
