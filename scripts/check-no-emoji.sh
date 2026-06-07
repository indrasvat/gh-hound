#!/usr/bin/env bash
set -euo pipefail

files="$(find . -name '*.go' -not -path './.git/*')"
if [ -z "$files" ]; then
  exit 0
fi

if LC_ALL=C grep -n $'\xEF\xB8\x8F' $files >/tmp/gh-hound-emoji-vs16.txt 2>/dev/null; then
  echo "emoji variation selector U+FE0F found:" >&2
  cat /tmp/gh-hound-emoji-vs16.txt >&2
  exit 1
fi

python3 - "$@" <<'PY'
from pathlib import Path
bad = []
for path in Path(".").rglob("*.go"):
    if ".git" in path.parts:
        continue
    text = path.read_text(encoding="utf-8")
    for lineno, line in enumerate(text.splitlines(), 1):
        for ch in line:
            if ord(ch) > 0xFFFF:
                bad.append((path, lineno, ch))
if bad:
    for path, lineno, ch in bad:
        print(f"{path}:{lineno}: astral-plane codepoint U+{ord(ch):X} {ch}", file=__import__("sys").stderr)
    raise SystemExit(1)
PY

echo "emoji check passed"
