#!/usr/bin/env bash
# Render-hygiene audit wrapper: builds the binary and runs the
# lossless pty harness (see render_hygiene.py for the contract).
set -euo pipefail
cd "$(dirname "$0")/../.."
make build >/dev/null
exec python3 .claude/automations/render_hygiene.py
