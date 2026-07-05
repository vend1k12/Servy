#!/bin/sh
# Smoke-test the servy binary against a single Ubuntu/Debian base image.
#
# Usage: tests/docker/smoke-one.sh <image>
#        image = ubuntu:22.04 | ubuntu:24.04 | debian:12 | debian:13
#
# Assumes ./dist/servy already exists (see tests/docker/run.sh or the CI
# docker-smoke job for the build step). Prints the docker build log to
# stdout and stderr; CI captures both.
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <image>" >&2
  exit 2
fi
image="$1"

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

if [ ! -x dist/servy ]; then
  echo "dist/servy not found; build it before running smoke-one.sh" >&2
  exit 3
fi

tag="servy-test-$(printf '%s' "$image" | tr ':/' '--')"
docker build --pull --build-arg BASE_IMAGE="$image" -f tests/docker/Dockerfile -t "$tag" .
