#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

if ! command -v shux >/dev/null 2>&1; then
  cat >&2 <<'MSG'
missing shux; install it before running VQA:
  npx skills add indrasvat/shux --global --yes
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

screen_filter="${SCREEN:-all}"
case "$screen_filter" in
  all) screens=(welcome all_green runs detail failure watch log dispatch palette help) ;;
  overlays) screens=(dispatch palette help) ;;
  *) screens=("$screen_filter") ;;
esac

breakpoints=(80x24 120x40 200x60)
out_dir=".claude/automations/screenshots"
mkdir -p "$out_dir"

for screen in "${screens[@]}"; do
  assertion=".claude/automations/assertions/${screen}.json"
  if [ ! -f "$assertion" ]; then
    echo "missing assertion file: $assertion" >&2
    exit 1
  fi
  mkdir -p "$out_dir/$screen"
  for bp in "${breakpoints[@]}"; do
    cols="${bp%x*}"
    rows="${bp#*x}"
    session="hound-vqa-${screen}-${bp}-$$"
    runner="$(mktemp "${TMPDIR:-/tmp}/hound-vqa.XXXXXX.sh")"
    cat >"$runner" <<RUNNER
#!/usr/bin/env bash
set -euo pipefail
export TERM=xterm-256color
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
unset NO_COLOR
sleep 0.25
printf '\\033[?25l\\033[2J\\033[H'
"$abs_bin" __screen --screen "$screen" --width "$cols" --height "$rows"
sleep 30
printf '\\033[?25h'
RUNNER
    chmod +x "$runner"
    shux session kill "$session" >/dev/null 2>&1 || true
    shux session create "$session" -d --cwd "$root" --title "hound ${screen} ${bp}" -- "$runner" >/dev/null
    cleanup() {
      shux session kill "$session" >/dev/null 2>&1 || true
      rm -f "$runner"
    }
    trap cleanup EXIT
    shux pane set-size -s "$session" --cols "$cols" --rows "$rows" >/dev/null
    needle="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["contains"][0])' "$assertion")"
    shux pane wait-for -s "$session" --text "$needle" --timeout-ms 5000 >/dev/null
    sleep 0.15
    txt="$out_dir/$screen/${bp}.txt"
    png="$out_dir/$screen/${bp}.png"
    shux pane capture -s "$session" --lines "$rows" --format plain >"$txt"
    python3 .claude/automations/verify_capture.py "$assertion" "$txt"
    shux pane snapshot -s "$session" -o "$png" >/dev/null
    cleanup
    trap - EXIT
    printf "captured %s %s -> %s\n" "$screen" "$bp" "$png"
  done
done

echo "vqa passed"
