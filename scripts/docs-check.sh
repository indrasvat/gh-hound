#!/usr/bin/env bash
set -euo pipefail

bin="./bin/gh-hound"
if [ ! -x "$bin" ]; then
  make build >/dev/null
fi

required_readme=(
  "## Overview"
  "## Why gh-hound"
  "## Features"
  "## Install"
  "## Quick Start"
  "## Controls"
  "## Configuration"
  "## Agent Surface"
  "## Architecture"
  "## Development"
  "## Roadmap"
)

for needle in "${required_readme[@]}"; do
  if ! grep -qF "$needle" README.md; then
    echo "README missing required section: $needle" >&2
    exit 1
  fi
done

for path in docs/development.md docs/configuration.md docs/agent-surface.md assets/demo.tape; do
  if [ ! -f "$path" ]; then
    echo "missing docs artifact: $path" >&2
    exit 1
  fi
done

if grep -qiE 'placeholder|scaffold is ready' README.md docs/development.md docs/configuration.md; then
  echo "docs still contain placeholder/scaffold language" >&2
  exit 1
fi

set +e
failure_json="$("$bin" runs --no-tui --json --fake-scenario failure 2>/dev/null)"
failure_code=$?
pending_json="$("$bin" runs --no-tui --json --fake-scenario pending 2>/dev/null)"
pending_code=$?
api_json="$("$bin" runs --no-tui --json --fake-scenario api_error 2>/dev/null)"
api_code=$?
set -e

if [ "$failure_code" -ne 1 ]; then
  echo "failure scenario exit = $failure_code, want 1" >&2
  exit 1
fi
printf '%s\n' "$failure_json" | jq -e '.runs[0].failed[0].job == "build"' >/dev/null

if [ "$pending_code" -ne 3 ]; then
  echo "pending scenario exit = $pending_code, want 3" >&2
  exit 1
fi
printf '%s\n' "$pending_json" | jq -e '.runs[0].status == "in_progress"' >/dev/null

if [ "$api_code" -ne 2 ]; then
  echo "api_error scenario exit = $api_code, want 2" >&2
  printf '%s\n' "$api_json" >&2
  exit 1
fi

md_output="$("$bin" runs --no-tui --format md --fake-scenario green)"
xml_output="$("$bin" runs --no-tui --format xml --fake-scenario green)"
grep -q '# gh-hound' <<<"$md_output"
grep -q '<result' <<<"$xml_output"

echo "docs check passed"
