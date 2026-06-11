# Configuration

`gh-hound` reads configuration from:

1. Built-in defaults.
2. `~/.config/gh-hound/config.toml`.
3. Environment variables.
4. CLI flags.

Later layers win.

## Config File

```toml
default_scope = "branch" # branch | repo
auto_watch = false
per_page = 30
diff_max_pages = 10     # history pages (x100 runs) the diff scan may walk
watch_group_max = 10    # runs one hunt board watches (1-50)
theme = "bramble"       # bramble | bone
log_level = "info"      # off | error | warn | info | debug
```

`diff_max_pages` bounds the regression scan behind `gh hound diff` and the TUI trail screen: at most `diff_max_pages` pages of 100 runs are read before the hound declares the trail cold (an `inconclusive` verdict, never a hang). Accepted range 1-100.

`watch_group_max` caps how many runs one hunt board (or `watch --group` stream) follows at once. The group poll budget stays one runs-list call per tick regardless of hunt size. Accepted range 1-50.

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `GH_REPO` | Repository override, `owner/name` |
| `HOUND_REPO` | Repository override, higher priority than `GH_REPO` |
| `HOUND_BRANCH` | Branch/ref override |
| `HOUND_STATUS` | Runs filter, such as `failure` or `in_progress` |
| `HOUND_ALL` | Show all branches when true |
| `HOUND_DIFF_MAX_PAGES` | Page budget for the `diff` regression scan (default 10) |
| `HOUND_WATCH_GROUP_MAX` | Pack board size cap for multi-run watch (default 10) |
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

`--json` always wins over `--format` and disables the TUI.
