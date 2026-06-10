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
theme = "bramble"       # bramble | bone
log_level = "info"      # off | error | warn | info | debug
```

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `GH_REPO` | Repository override, `owner/name` |
| `HOUND_REPO` | Repository override, higher priority than `GH_REPO` |
| `HOUND_BRANCH` | Branch/ref override |
| `HOUND_STATUS` | Runs filter, such as `failure` or `in_progress` |
| `HOUND_ALL` | Show all branches when true |
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
