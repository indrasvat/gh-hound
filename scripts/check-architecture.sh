#!/usr/bin/env bash
set -euo pipefail

bad="$(go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/... | awk '$1 ~ /(internal\/usecase|internal\/adapter)/ && $0 ~ /github.com\/indrasvat\/gh-hound\/internal\/tui/ {print}')"
if [ -n "$bad" ]; then
  echo "architecture violation: usecase/adapter must not import tui" >&2
  echo "$bad" >&2
  exit 1
fi

echo "architecture check passed"
