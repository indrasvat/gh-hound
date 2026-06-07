#!/usr/bin/env bash
set -euo pipefail

bin="${1:-./bin/gh-hound}"

test -x "$bin"
"$bin" version | grep -q "Hunt down your GitHub Actions CI"
"$bin" --help | grep -q "Hunt down your GitHub Actions CI"
"$bin" runs --no-tui --json | grep -q '"runs"'

echo "smoke test passed"
