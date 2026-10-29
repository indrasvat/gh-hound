# Configuration

`gh-hound` reads configuration from:

1. Built-in defaults.
2. `$XDG_CONFIG_HOME/gh-hound/config.toml`, or `~/.config/gh-hound/config.toml` when `XDG_CONFIG_HOME` is unset (on macOS too).
3. Environment variables.
4. CLI flags.

Later layers win: a flag you pass beats its variable, and a variable beats the file. A missing file means defaults. A file that does not parse, has a key gh-hound does not know, or holds an out-of-range value stops every command but `version` with an error naming the file (pipe commands report it as `error.kind: validation`, `field: config`); so does a path that is not a regular file (a broken symlink, a FIFO) or is over 1 MiB. A relative `XDG_CONFIG_HOME` is ignored, as the XDG spec says.

## Config File

```toml
default_scope = "branch" # branch | repo
auto_watch = false
per_page = 30
diff_max_pages = 10     # history pages (x100 runs) the diff scan may walk
watch_group_max = 10    # runs one hunt board watches (1-50)
flake_window = 50       # recent runs the flake scan reads per workflow+branch
flake_badges = true     # mark known flakers on the runs list; off = zero flake calls
log_compact = true      # open full logs with repeating lines folded; c toggles
theme = "bramble"       # bramble | bone
welcome = true          # show the welcome banner on launch
poll_min_ms = 2000      # fastest live poll (1-3600000)
poll_max_ms = 30000     # slowest live poll (1-3600000)
```

`diff_max_pages` bounds the regression scan behind `gh hound diff` and the TUI trail screen: at most `diff_max_pages` pages of 100 runs are read before the hound declares the trail cold (an `inconclusive` verdict, never a hang). Accepted range 1-100.

`watch_group_max` caps how many runs one hunt board (or `watch --group` stream) follows at once. The group poll budget stays one runs-list call per tick regardless of hunt size. Accepted range 1-50.
`flake_window` (accepted range 1-500, default 50) is how many recent runs `gh hound flakes` and the TUI scent check read per workflow+branch before issuing a verdict. `flake_badges` (default on) controls two things: the `~` flake badge on runs-list rows whose workflow has a known flaker, and the automatic failure-screen scent check. Verdicts are cached per workflow+branch for the session and recomputed after a rerun; badges never spend API calls of their own. With `flake_badges = false` the failure screen spends zero flake calls — the `:flakes` jump and the `flakes` verb stay available.

`log_compact` (default on) opens the full-log screen with runs of repeating lines folded into one row each. `c` switches between the compact and the full log either way; with `log_compact = false` logs open full and `c` folds them. It does not change pipe output.

## Keybindings

A `[keybindings]` table moves an action to another key. The new key acts wherever the action does, footers and help show it, and the old key stops acting there. Text inputs (filters, search, dispatch fields, the palette) always type the key you press; once `help` moves, `?` in the palette is typed too.

```toml
[keybindings]
cancel = "c"        # runs, detail, watch, hunt board (default x)
help = "h"          # everywhere (default ?)
rerun_failed = "F"  # runs, detail, failure (default R)
```

| Action | Default | Acts on |
| --- | --- | --- |
| `help` · `quit` · `theme` | `?` · `q` · `T` | every screen; `help` and `quit` also in help and approvals |
| `rerun` · `logs` · `status_cycle` | `r` · `l` · `f` | runs |
| `watch` | `w` | runs, detail |
| `cancel` · `force_cancel` | `x` · `X` | runs, detail; `cancel` also watch and the hunt board |
| `rerun_failed` | `R` | runs, detail, failure |
| `rerun_job` | `r` | detail, failure |
| `full_log` · `copy_excerpt` | `l` · `y` | failure |
| `browser` | `o` | failure, workflows |
| `follow` · `debug` | `f` · `d` | watch |
| `follow_worst` | `f` | hunt board |
| `time_jump` · `search` · `compact` · `wrap` | `t` · `/` · `c` · `w` | full log |
| `compare` | `o` | diff |
| `dig_up` · `dig_up_key` | `d` · `D` | caches |
| `copy_json` | `y` | queue |

A key is one printable character. Navigation keys and keys that in-screen hints name (`j/k`, `g/G`, `s`, `/` on runs, `:`, Enter, Esc, Tab) stay fixed. One key may serve two actions that never share a screen (`rerun = "e"` and `rerun_job = "e"`). A key that would mean two things on one screen, an unknown action, or a key that is not one printable character stops the launch with an error naming the file, the screen and both actions.

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `GH_REPO` | Repository override, `owner/name` |
| `HOUND_REPO` | Repository override, higher priority than `GH_REPO` |
| `HOUND_BRANCH` | Branch/ref override |
| `HOUND_STATUS` | Runs filter, such as `failure` or `in_progress` |
| `HOUND_ACTOR` | Runs started by a login, or `@me` for the authenticated user; `--actor` wins, and `--run` ignores it |
| `HOUND_ALL` | Show all branches when true |
| `HOUND_DIFF_MAX_PAGES` | Page budget for the `diff` regression scan (default 10) |
| `HOUND_WATCH_GROUP_MAX` | Hunt board size cap for multi-run watch (default 10) |
| `HOUND_FLAKE_WINDOW` | Run window for the `flakes` scan (default 50) |
| `HOUND_FLAKE_BADGES` | Flake badges + automatic failure-screen scent check (default true) |
| `HOUND_LOG_COMPACT` | Open full logs with repeating lines folded (default true) |
| `HOUND_NO_TUI` | Force structured output when true |
| `HOUND_JSON` | Force JSON output when true |
| `HOUND_FORMAT` | `json`, `md`, or `xml` |
| `HOUND_LOG_LEVEL` | `off`, `error`, `warn`, `info`, or `debug` |
| `HOUND_TRACE_HTTP` | Trace GitHub API calls to JSON logs when true |
| `HOUND_FAKE_SCENARIO` | Deterministic local scenario for docs/tests |

## Logging

Structured logs use `slog` JSON under:

```text
~/.local/state/gh-hound/gh-hound.log
```

Use `--log-level debug` for local diagnosis and `--trace-http` when adapter behavior needs inspection. Logs must not include credentials or authorization headers.

## Output Formats

```bash
gh hound runs --no-tui --json
gh hound runs --no-tui --format md
gh hound runs --no-tui --format xml
```

`--json` always wins over `--format` and disables the TUI. `HOUND_JSON` yields to an explicit `--json`, and its format to an explicit `--format` (the TUI stays off).
