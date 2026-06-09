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
need_file codecov.yml
need_exec scripts/build-release.sh
need_exec scripts/smoke-test.sh
need_exec install.sh

need_grep "make ci" .github/workflows/ci.yml
need_grep "make tools-ci" .github/workflows/ci.yml
need_grep "make workflow-check" .github/workflows/ci.yml
need_grep "make shellcheck" .github/workflows/ci.yml
need_grep "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff" .github/workflows/ci.yml
need_grep "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5" .github/workflows/ci.yml
need_grep "golangci-lint/v2" Makefile
need_grep "actionlint/cmd/actionlint" Makefile
need_grep "shellcheck install.sh scripts/\\*.sh" Makefile
need_grep "cli/gh-extension-precompile@v2.1.0" .github/workflows/release.yml
need_grep "go_version_file: go.mod" .github/workflows/release.yml
need_grep "build_script_override: scripts/build-release.sh" .github/workflows/release.yml
need_grep "codecov/codecov-action@v5" .github/workflows/ci.yml
need_grep "fail_ci_if_error: false" .github/workflows/ci.yml
need_grep "use_oidc:" .github/workflows/ci.yml
need_grep "target: 60%" codecov.yml
need_grep "gh extension install indrasvat/gh-hound" README.md

bash -n scripts/build-release.sh scripts/smoke-test.sh install.sh

actionlint .github/workflows/*.yml
shellcheck install.sh scripts/*.sh

echo "release config valid"
