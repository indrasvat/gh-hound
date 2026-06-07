#!/usr/bin/env bash
set -euo pipefail

need_file() {
  if [ ! -f "$1" ]; then
    echo "missing file: $1" >&2
    exit 1
  fi
}

need_exec() {
  if [ ! -x "$1" ]; then
    echo "missing executable: $1" >&2
    exit 1
  fi
}

need_grep() {
  local pattern="$1"
  local file="$2"
  if ! grep -q "$pattern" "$file"; then
    echo "$file missing pattern: $pattern" >&2
    exit 1
  fi
}

need_file .github/workflows/ci.yml
need_file .github/workflows/release.yml
need_file .github/dependabot.yml
need_exec scripts/build-release.sh
need_exec scripts/smoke-test.sh
need_exec install.sh

need_grep "make ci" .github/workflows/ci.yml
need_grep "make tools-ci" .github/workflows/ci.yml
need_grep "actions/setup-go@v5" .github/workflows/ci.yml
need_grep "golangci-lint/v2" Makefile
need_grep "cli/gh-extension-precompile@v2" .github/workflows/release.yml
need_grep "go_version_file: go.mod" .github/workflows/release.yml
need_grep "build_script_override: scripts/build-release.sh" .github/workflows/release.yml
need_grep "gh extension install indrasvat/gh-hound" README.md

bash -n scripts/build-release.sh scripts/smoke-test.sh install.sh

if command -v actionlint >/dev/null 2>&1; then
  actionlint .github/workflows/*.yml
fi

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck scripts/build-release.sh scripts/smoke-test.sh install.sh
fi

echo "release config valid"
