#!/usr/bin/env bash
set -euo pipefail

before="$(mktemp)"
after="$(mktemp)"
trap 'rm -f "$before" "$after"' EXIT

git diff --binary -- . ':(exclude)go.sum' >"$before"
go fix ./...
go mod tidy
git diff --binary -- . ':(exclude)go.sum' >"$after"

if ! cmp -s "$before" "$after"; then
  echo "go fix or go mod tidy changed files. Review the diff, then rerun the check." >&2
  diff -u "$before" "$after" || true
  exit 1
fi

echo "go fix check passed"
