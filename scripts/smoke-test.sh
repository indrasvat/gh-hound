#!/usr/bin/env bash
set -euo pipefail

bin="${1:-./bin/gh-hound}"

test -x "$bin"
version_output="$("$bin" version)"
help_output="$("$bin" --help)"
green_output="$("$bin" runs --no-tui --json --fake-scenario green)"

grep -q "Hunt down your GitHub Actions CI" <<<"$version_output"
grep -q "Hunt down your GitHub Actions CI" <<<"$help_output"
grep -q '"conclusion": "success"' <<<"$green_output"

set +e
failure_output="$("$bin" runs --no-tui --json --fake-scenario failure)"
failure_code=$?
pending_output="$("$bin" runs --no-tui --json --fake-scenario pending)"
pending_code=$?
watch_output="$("$bin" watch --json --fake-scenario failure)"
watch_code=$?
set -e

if [ "$failure_code" -ne 1 ]; then
  echo "failure scenario exit = $failure_code, want 1" >&2
  exit 1
fi
grep -q '"job": "build"' <<<"$failure_output"

if [ "$pending_code" -ne 3 ]; then
  echo "pending scenario exit = $pending_code, want 3" >&2
  exit 1
fi
grep -q '"status": "in_progress"' <<<"$pending_output"

if [ "$watch_code" -ne 1 ]; then
  echo "watch failure exit = $watch_code, want 1" >&2
  exit 1
fi
grep -q '"step": "go test ./..."' <<<"$watch_output"

echo "smoke test passed"
