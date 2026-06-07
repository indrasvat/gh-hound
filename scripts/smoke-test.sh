#!/usr/bin/env bash
set -euo pipefail

bin="${1:-./bin/gh-hound}"

test -x "$bin"
version_output="$("$bin" version)"
help_output="$("$bin" --help)"
runs_output="$("$bin" runs --no-tui --json)"

grep -q "Hunt down your GitHub Actions CI" <<<"$version_output"
grep -q "Hunt down your GitHub Actions CI" <<<"$help_output"
grep -q '"runs"' <<<"$runs_output"

echo "smoke test passed"
