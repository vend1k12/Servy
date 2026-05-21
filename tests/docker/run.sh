#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
mkdir -p dist
GOTOOLCHAIN=go1.26.3 GOOS=linux GOARCH="$(go env GOARCH)" go build -o dist/servy ./cmd/servy

for image in ubuntu:22.04 ubuntu:24.04 debian:12 debian:13; do
  tag="servy-test-$(printf '%s' "$image" | tr ':/' '--')"
  docker build --pull --build-arg BASE_IMAGE="$image" -f tests/docker/Dockerfile -t "$tag" .
done
