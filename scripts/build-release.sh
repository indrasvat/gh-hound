#!/usr/bin/env bash
set -euo pipefail

version="${VERSION:-${GITHUB_REF_NAME:-dev}}"
commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
date="${DATE:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
output="${1:-gh-hound}"

go build \
  -trimpath \
  -ldflags "-s -w -X main.version=${version} -X main.commit=${commit} -X main.date=${date}" \
  -o "${output}" \
  ./cmd/gh-hound
