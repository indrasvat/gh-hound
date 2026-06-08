#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

if ! command -v shux >/dev/null 2>&1; then
  cat >&2 <<'MSG'
missing shux; install it before running interaction audit:
  npx skills add indrasvat/shux --global --yes
MSG
  exit 1
fi

bin="${BIN:-./bin/gh-hound}"
if [ ! -x "$bin" ]; then
  make build
fi
case "$bin" in
  /*) abs_bin="$bin" ;;
  *) abs_bin="$root/$bin" ;;
esac

go test -race ./internal/tui ./internal/tui/overlay/... ./internal/tui/screens/...

scenarios=(
  welcome-enter
  global-help
  global-palette
  overlay-esc
  runs-select
  runs-filter
  detail-nav
  failure-actions
  log-search-fold
  watch-toggle
  dispatch-fill
)

out_dir=".claude/automations/screenshots/interactions"
mkdir -p "$out_dir"

for scenario in "${scenarios[@]}"; do
  assertion=".claude/automations/interactions/${scenario}.json"
  if [ ! -f "$assertion" ]; then
    echo "missing interaction assertion file: $assertion" >&2
    exit 1
  fi
  session="hound-ia-${scenario}-$$"
  runner="$(mktemp "${TMPDIR:-/tmp}/hound-ia.XXXXXX.sh")"
  cat >"$runner" <<RUNNER
#!/usr/bin/env bash
set -euo pipefail
export TERM=xterm-256color
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
unset NO_COLOR
sleep 0.25
printf '\\033[?25l\\033[2J\\033[H'
"$abs_bin" __interact --scenario "$scenario" --width 80
sleep 30
printf '\\033[?25h'
RUNNER
  chmod +x "$runner"
  shux session kill "$session" >/dev/null 2>&1 || true
  shux session create "$session" -d --cwd "$root" --title "hound ia ${scenario}" --cmd "$runner" >/dev/null
  cleanup() {
    shux session kill "$session" >/dev/null 2>&1 || true
    rm -f "$runner"
  }
  trap cleanup EXIT
  shux pane set-size -s "$session" --cols 80 --rows 24 >/dev/null
  needle="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["contains"][0])' "$assertion")"
  shux pane wait-for -s "$session" --text "$needle" --timeout-ms 5000 >/dev/null
  sleep 0.15
  txt="$out_dir/${scenario}.txt"
  png="$out_dir/${scenario}.png"
  shux pane capture -s "$session" --lines 24 --format plain >"$txt"
  python3 .claude/automations/verify_capture.py "$assertion" "$txt"
  shux pane snapshot -s "$session" -o "$png" >/dev/null
  cleanup
  trap - EXIT
  printf "audited interaction %s -> %s\n" "$scenario" "$png"
done

echo "interaction audit passed"
