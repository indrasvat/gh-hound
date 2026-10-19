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
  runs-filter-edit
  runs-actor-me
  runs-actor-anon
  runs-actor-palette
  log-time-jump
  log-time-picker
  detail-nav
  detail-artifacts
  detail-artifact-confirm
  detail-artifact-download
  failure-log-refetch
  failure-actions
  log-search-fold
  log-search-input
  log-search-esc
  log-search-esc-back
  log-compact-open
  log-compact-toggle
  log-compact-search
  log-compact-folds
  log-compact-failure
  log-compact-warning
  log-compact-jump-echo
  watch-toggle
  pack-board
  dispatch-fill
  palette-caches
  caches-delete-confirm
  input-utf8
  input-wide
  palette-utf8-backspace
  help-toggle
  startup-frame
  failure-cut-scroll
  log-range-search
  log-match-grouped
  log-fold-count
  timejump-small
  caches-filter-clear
)

adapter_for() {
  case "$1" in
    runs-select) printf 'green' ;;
    runs-actor-me | runs-actor-palette) printf 'actors' ;;
    runs-actor-anon) printf 'actors_anon' ;;
    watch-toggle) printf 'running' ;;
    pack-board) printf 'pack' ;;
    failure-log-refetch) printf 'log_refetch' ;;
    log-compact-* | log-range-search | log-match-grouped | log-fold-count | timejump-small) printf 'noisy' ;;
    failure-cut-scroll) printf 'failing' ;;
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
    runs-filter-edit) printf '/ z z' ;;
    runs-actor-me | runs-actor-anon) printf '/ a c t o r : @ m e enter' ;;
    runs-actor-palette) printf ': M y space r u n s enter' ;;
    log-time-jump) printf 'enter enter l t 1 7 : 4 2 enter' ;;
    log-time-picker) printf 'enter enter l t' ;;
    detail-nav) printf 'enter tab n' ;;
    detail-artifacts) printf 'enter a j k' ;;
    detail-artifact-confirm) printf 'enter a enter' ;;
    detail-artifact-download) printf 'enter a d y' ;;
    failure-actions) printf 'enter enter' ;;
    failure-log-refetch) printf 'enter enter' ;;
    log-search-fold) printf 'enter enter l / t r a i l enter z' ;;
    log-search-input) printf 'enter enter l / t r a' ;;
    log-search-esc) printf 'enter enter l / t r a i l enter esc' ;;
    log-search-esc-back) printf 'enter enter l / t r a i l enter esc esc' ;;
    log-compact-open) printf 'enter l' ;;
    log-compact-toggle) printf 'enter l c' ;;
    log-compact-search) printf 'enter l / 1 8 : 0 0 : 0 1 enter' ;;
    log-compact-folds) printf 'enter l Z z' ;;
    log-compact-failure) printf 'enter enter l' ;;
    log-compact-warning) printf 'enter l G' ;;
    log-compact-jump-echo) printf 'enter l t 1 7 : 0 0 enter G' ;;
    watch-toggle) printf 'w f d' ;;
    pack-board) printf 'j w f' ;;
    dispatch-fill) printf 'D T v 0 . 1 2 . 0 tab right' ;;
    palette-caches) printf ': c a c h e s enter' ;;
    caches-delete-confirm) printf ': c a c h e s enter d' ;;
    input-utf8) printf '/ c a f é space ✓' ;;
    input-wide) printf '/ %s末' "$(printf '世%.0s' $(seq 40))" ;;
    palette-utf8-backspace) printf ': c é backspace ä' ;;
    help-toggle) printf '? : ? ?' ;;
    failure-cut-scroll) printf 'enter enter j j j j j' ;;
    log-range-search) printf 'enter l t 1 6 : 1 2 : 0 2 - 1 6 : 1 2 : 1 2 enter / r e q u e s t - i d enter' ;;
    log-match-grouped) printf 'enter l / 2 0 2 6 enter' ;;
    log-fold-count) printf 'enter l t 1 6 : 1 2 : 2 1 enter G' ;;
    timejump-small) printf 'enter l t' ;;
    caches-filter-clear) printf ': c a c h e s enter / n o d e enter' ;;
    *) printf '' ;;
  esac
}

# startup-frame holds the launch's runs listing: the frame must paint
# before it lands.
args_for() {
  case "$1" in
    startup-frame) printf -- '--latency 5s' ;;
    *) printf '' ;;
  esac
}

# The small-frame scenarios run where the frame cuts the screen.
size_for() {
  case "$1" in
    failure-cut-scroll) printf '80x12' ;;
    timejump-small) printf '80x14' ;;
    *) printf '80x24' ;;
  esac
}

send_key() {
  local session="$1"
  local key="$2"
  case "$key" in
    enter) shux pane send-keys -s "$session" --data DQ== >/dev/null ;;
    tab) shux pane send-keys -s "$session" --data CQ== >/dev/null ;;
    esc) shux pane send-keys -s "$session" --data Gw== >/dev/null ;;
    backspace) shux pane send-keys -s "$session" --data fw== >/dev/null ;;
    space) shux pane send-keys -s "$session" --text ' ' >/dev/null ;;
    up) shux pane send-keys -s "$session" --data G1tB >/dev/null ;;
    down) shux pane send-keys -s "$session" --data G1tC >/dev/null ;;
    right) shux pane send-keys -s "$session" --data G1tD >/dev/null ;;
    left) shux pane send-keys -s "$session" --data G1tE >/dev/null ;;
    *) shux pane send-keys -s "$session" --text "$key" >/dev/null ;;
  esac
}

out_dir=".claude/automations/screenshots/interactions"
mkdir -p "$out_dir"
# A fixed short download root keeps the 80-col path-subline assertion
# deterministic (mktemp -d paths overflow the row at 80 cols).
rm -rf /tmp/hound-ia-dl
mkdir -p /tmp/hound-ia-dl

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
  args="$(args_for "$scenario")"
  size="$(size_for "$scenario")"
  cols="${size%x*}"
  rows="${size#*x}"
  cat >"$runner" <<RUNNER
#!/usr/bin/env bash
set -euo pipefail
export TERM=xterm-256color
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export HOUND_WELCOME="$welcome"
export GH_HOUND_ARTIFACT_DIR=/tmp/hound-ia-dl
unset NO_COLOR
"$abs_bin" __vqa-tui --scenario "$adapter" $args
RUNNER
  chmod +x "$runner"
  shux session kill "$session" >/dev/null 2>&1 || true
  shux session create "$session" -d --cwd "$root" --title "hound ia ${scenario}" -- "$runner" >/dev/null
  cleanup() {
    shux session kill "$session" >/dev/null 2>&1 || true
    rm -f "$runner"
  }
  trap cleanup EXIT
  shux pane set-size -s "$session" --cols "$cols" --rows "$rows" >/dev/null
  shux pane wait-for -s "$session" --text 'hound' --timeout-ms 20000 >/dev/null
  sleep 0.1
  for key in $(keys_for "$scenario"); do
    send_key "$session" "$key"
    sleep 0.08
  done
  needle="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["contains"][0])' "$assertion")"
  shux pane wait-for -s "$session" --text "$needle" --timeout-ms 20000 >/dev/null
  sleep 0.15
  txt="$out_dir/${scenario}.txt"
  png="$out_dir/${scenario}.png"
  shux pane capture -s "$session" --lines "$rows" --format plain >"$txt"
  shux pane snapshot -s "$session" -o "$png" >/dev/null
  python3 .claude/automations/verify_capture.py "$assertion" "$txt" "$png" "$cols" "$rows"
  cleanup
  trap - EXIT
  printf "audited real-pty interaction %s -> %s\n" "$scenario" "$png"
done

rm -rf /tmp/hound-ia-dl
echo "interaction audit passed"
