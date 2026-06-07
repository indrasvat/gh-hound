#!/usr/bin/env bash
set -euo pipefail

if [[ ! -s docs/visual-contract.md ]]; then
  echo "missing docs/visual-contract.md" >&2
  exit 1
fi

for ref in "⓪" "①" "②" "③" "④" "⑤" "⑥" "⑦" "⑧" "⑩"; do
  if ! grep -Fq "$ref" docs/visual-contract.md; then
    echo "missing visual ref $ref in docs/visual-contract.md" >&2
    exit 1
  fi
done

go test -race ./internal/theme ./internal/tui/icons ./internal/tui/keys ./internal/layout
