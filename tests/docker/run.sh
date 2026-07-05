#!/bin/sh
# Local dev entry point: build the linux binary and smoke-test it across
# every supported Ubuntu/Debian base image. CI uses tests/docker/smoke-one.sh
# under a matrix for the same coverage with per-image logs.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
mkdir -p dist
GOTOOLCHAIN=go1.26.4 GOOS=linux GOARCH="$(go env GOARCH)" go build -o dist/servy ./cmd/servy

for image in ubuntu:22.04 ubuntu:24.04 debian:12 debian:13; do
  tests/docker/smoke-one.sh "$image"
done
