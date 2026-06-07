#!/usr/bin/env python3
import json
import sys
from pathlib import Path

assertion = json.loads(Path(sys.argv[1]).read_text())
capture = Path(sys.argv[2]).read_text(errors="replace")
missing = [needle for needle in assertion.get("contains", []) if needle not in capture]
if missing:
    print(f"{sys.argv[2]} missing assertions:", file=sys.stderr)
    for needle in missing:
        print(f"  - {needle}", file=sys.stderr)
    raise SystemExit(1)
