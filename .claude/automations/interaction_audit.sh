#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

if ! command -v shux >/dev/null 2>&1; then
  cat >&2 <<'MSG'
missing shux; install it before running interaction audit:
  curl -sSf https://shux.pages.dev/install.sh | sh
MSG
  exit 1
fi

bin="${BIN:-./bin/gh-hound}"
if [ "${SKIP_BUILD:-}" != "1" ]; then
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
  runs-status-cycle
  log-time-jump
  log-time-picker
  detail-nav
  detail-artifacts
  detail-artifact-confirm
  failure-log-refetch
  failure-actions
  log-search-fold
  watch-toggle
  dispatch-fill
)

adapter_for() {
  case "$1" in
    runs-select) printf 'green' ;;
    watch-toggle) printf 'running' ;;
    failure-log-refetch) printf 'log_refetch' ;;
    *) printf 'failing' ;;
  esac
}

welcome_for() {
  case "$1" in
    welcome-enter) printf 'true' ;;
    *) printf 'false' ;;
  esac
}

keys_for() {
  case "$1" in
    welcome-enter) printf 'enter' ;;
    global-help) printf '?' ;;
    global-palette) printf ':' ;;
    overlay-esc) printf '? : esc' ;;
    runs-select) printf 'j' ;;
    runs-filter) printf '/ f a i l enter' ;;
    runs-status-cycle) printf 'f' ;;
    log-time-jump) printf 'enter enter l t 1 7 : 4 2 enter' ;;
    log-time-picker) printf 'enter enter l t' ;;
    detail-nav) printf 'enter tab n' ;;
    detail-artifacts) printf 'enter a j k' ;;
    detail-artifact-confirm) printf 'enter a enter' ;;
    failure-actions) printf 'enter enter' ;;
    failure-log-refetch) printf 'enter enter' ;;
    log-search-fold) printf 'enter enter l / t r a i l enter z' ;;
    watch-toggle) printf 'w f d' ;;
    dispatch-fill) printf 'D T v 0 . 1 2 . 0 tab right' ;;
    *) printf '' ;;
  esac
}

send_key() {
  local session="$1"
  local key="$2"
  case "$key" in
    enter) shux pane send-keys -s "$session" --data DQ== >/dev/null ;;
    tab) shux pane send-keys -s "$session" --data CQ== >/dev/null ;;
    esc) shux pane send-keys -s "$session" --data Gw== >/dev/null ;;
    up) shux pane send-keys -s "$session" --data G1tB >/dev/null ;;
    down) shux pane send-keys -s "$session" --data G1tC >/dev/null ;;
    right) shux pane send-keys -s "$session" --data G1tD >/dev/null ;;
    left) shux pane send-keys -s "$session" --data G1tE >/dev/null ;;
    *) shux pane send-keys -s "$session" --text "$key" >/dev/null ;;
  esac
}

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
  adapter="$(adapter_for "$scenario")"
  welcome="$(welcome_for "$scenario")"
  cat >"$runner" <<RUNNER
#!/usr/bin/env bash
set -euo pipefail
export TERM=xterm-256color
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export HOUND_WELCOME="$welcome"
unset NO_COLOR
"$abs_bin" __vqa-tui --scenario "$adapter"
RUNNER
  chmod +x "$runner"
  shux session kill "$session" >/dev/null 2>&1 || true
  shux session create "$session" -d --cwd "$root" --title "hound ia ${scenario}" -- "$runner" >/dev/null
  cleanup() {
    shux session kill "$session" >/dev/null 2>&1 || true
    rm -f "$runner"
  }
  trap cleanup EXIT
  shux pane set-size -s "$session" --cols 80 --rows 24 >/dev/null
  shux pane wait-for -s "$session" --text 'hound' --timeout-ms 5000 >/dev/null
  sleep 0.1
  for key in $(keys_for "$scenario"); do
    send_key "$session" "$key"
    sleep 0.08
  done
  needle="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["contains"][0])' "$assertion")"
  shux pane wait-for -s "$session" --text "$needle" --timeout-ms 5000 >/dev/null
  sleep 0.15
  txt="$out_dir/${scenario}.txt"
  png="$out_dir/${scenario}.png"
  shux pane capture -s "$session" --lines 24 --format plain >"$txt"
  shux pane snapshot -s "$session" -o "$png" >/dev/null
  python3 .claude/automations/verify_capture.py "$assertion" "$txt" "$png" 80 24
  cleanup
  trap - EXIT
  printf "audited real-pty interaction %s -> %s\n" "$scenario" "$png"
done

echo "interaction audit passed"
