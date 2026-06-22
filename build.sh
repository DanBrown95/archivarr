#!/usr/bin/env bash
# Build/run Archivarr via docker compose, stamping the version from git.
#
# The app version is derived from `git describe` (tags are the source of truth):
#   - on a tagged commit:      v0.2.0
#   - between tags:            v0.2.0-3-gabc1234
#   - dirty working tree:      ...-dirty
#   - no tags yet:            <short-sha>
# It's exported as VERSION and consumed by the compose files' build arg
# (VERSION: ${VERSION:-dev}), which threads it into the Go binary via ldflags.
#
# Usage:
#   ./build.sh                         # docker compose up --build
#   ./build.sh up -d --build           # pass any compose args through
#   ./build.sh -f compose.local.yml up --build
#
# (.git is excluded from the Docker build context, so the version must be
#  computed here on the host and passed in — it can't be read inside the image.)
set -euo pipefail

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
export VERSION
echo "Building Archivarr version: ${VERSION}"

if [ "$#" -eq 0 ]; then
  set -- up --build
fi

exec docker compose "$@"
