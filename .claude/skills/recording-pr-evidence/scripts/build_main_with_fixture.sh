#!/usr/bin/env bash
# Build the default branch with this branch's fake fixture files copied
# in, so main and the branch can be recorded on the same fixture.
#
#   build_main_with_fixture.sh <out-binary> <fixture-file>...
#   e.g. build_main_with_fixture.sh /tmp/gh-hound-main \
#          internal/adapter/fake/fake.go internal/adapter/fake/live.go
#
# Only fixture files belong in the list: copying product code would
# record the branch twice and prove nothing. When a fixture needs an API
# main lacks, the build fails: keep the fixture to what main has (fake
# data shaped through existing types), or say in the evidence that main
# cannot reach the state at all.
#
# GO picks the toolchain (default: go on PATH; gh-hound pins
# ~/sdk/go1.26.0/bin/go).
set -euo pipefail

out="${1:?usage: build_main_with_fixture.sh <out-binary> <fixture-file>...}"
shift
[ "$#" -gt 0 ] || { echo "name at least one fixture file" >&2; exit 2; }

root="$(git rev-parse --show-toplevel)"
base="$(git -C "$root" symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || echo main)"
base="${base#origin/}"
tree="$(mktemp -d "${TMPDIR:-/tmp}/main-fixture.XXXXXX")"
cleanup() { git -C "$root" worktree remove --force "$tree" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# Detached: the default branch may already be checked out elsewhere.
git -C "$root" worktree add --quiet --detach "$tree" "$base"
for file in "$@"; do
  mkdir -p "$tree/$(dirname "$file")"
  cp "$root/$file" "$tree/$file"
done
(cd "$tree" && "${GO:-go}" build -o "$out" ./cmd/gh-hound)
printf 'built %s from %s @ %s with %d fixture file(s)\n' "$out" "$base" "$(git -C "$tree" rev-parse --short HEAD)" "$#"
