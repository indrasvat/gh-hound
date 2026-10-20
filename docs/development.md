# Development

## Setup

```bash
make tools
make hooks
make build
```

Required local tools for the full gate: Go from `go.mod`, `golangci-lint`, `gotestsum`, `lefthook`, `jq`, and `shux`. `vhs` is needed only for demo recording.

`make tools` installs shux too (`make tools-shux` alone, e.g. in a fresh
cloud sandbox). It runs the official installer:

```bash
curl -sSfL https://shux.pages.dev/install.sh | sh
shux --version
```

The installer always takes the latest release, resolved through GitHub's
`releases/latest` web redirect rather than the rate-limited API (which
it uses only as a fallback), verifies the tarball's sha256, and installs
to `~/.local/bin` (`--dir` to change; put it on `PATH`), then installs
the shux agent skill through `npx skills add` (skipped with a notice
when `npx` is missing). Re-run it to upgrade both.

## Workflow

Use red/green/refactor for every task:

1. Re-read the relevant task, PRD section, and mock before editing.
2. Add a failing test or verification check.
3. Implement the smallest behavior that turns it green.
4. Refactor only after the gate is green.
5. Run focused tests, then `make check`.
6. Commit and push the task.

## Make Targets

| Target | Purpose |
| --- | --- |
| `make build` | Build `bin/gh-hound` with version metadata |
| `make run` | Run the CLI from source |
| `make fmt` / `make fmt-check` | Format or verify Go formatting |
| `make gofix` / `make gofix-check` | Apply or verify modern Go fixes |
| `make lint` / `make vet` | Static analysis |
| `make test` | Race-enabled test suite with coverage |
| `make check` | Local green-bar gate |
| `make ci` | CI gate: `check` plus build |
| `make docs-check` | Verify docs sections and documented commands |
| `make vqa` | shux visual and interaction audit |
| `make vqa-screen SCREEN=runs` | Focused visual audit for one screen |
| `make demo` | Record `assets/demo.gif` from `assets/demo.tape` |
| `make smoke-test` | Local binary smoke test |
| `make release-prep` | Release preparation gate |

## Hooks

`lefthook` is installed with:

```bash
make hooks
```

Pre-commit runs formatting, `go fix` verification, lint, and short tests. Pre-push runs `make ci`.

## Visual QA

`make vqa` uses shux to capture:

- `welcome`, `all_green`, `runs`, `detail`, `failure`, `watch`, `watch-board`, `log`, `dispatch`, `dispatch-picker`, `workflows`, `diff`, `diff-inconclusive`, `flakes`, `queue`, `caches`, `palette`, and `help`.
- Breakpoints: `80x24`, `120x40`, `200x60`.
- Interaction frames for global overlays and screen-specific key paths.

Generated artifacts are under `.claude/automations/screenshots/` and are ignored except for the directory policy files.

## PR Evidence

A PR that changes what the TUI shows carries an evidence page: main and
the branch recorded side by side on the same fake fixture, each change
marked in the video, plus stills, tests and the review record, linked
from the PR body. The `recording-pr-evidence` project skill
(`.claude/skills/recording-pr-evidence/`) holds the workflow, the
marking style and the tools; each task keeps its spec in
`docs/tasks/<task>-demo.json`. Requires `uv`, `ffmpeg` and `shux`.

## Async Loading Pattern

Every server-backed fetch in the TUI goes through `App.startLoad` /
`startLoadProgress` (internal/tui/loading.go): work runs in a
goroutine, the result applies on a poll-tick drain, supersession and
esc-cancel work by pointer identity, and `PollInterval` drops to the
spinner frame rate while a load is pending. Never call a resolver on
the keypress path. Tests and fixtures settle async state with
`App.SettleLoads(timeout)` — the interactive loop must never call it.
See docs/visual-contract.md "Loading States" for the user-facing
contract.

## Agent Surface QA

Use deterministic fake scenarios:

```bash
make build
./bin/gh-hound runs --no-tui --json --fake-scenario failure | jq '.runs[0].failed[0]'
./bin/gh-hound runs --branch release/v1 --no-tui --json --fake-scenario empty | jq '.diagnostic'
./bin/gh-hound watch --json --fake-scenario failure | jq '.runs[0].failed[0].step'
```

Expected exit codes are documented in `docs/agent-surface.md`.

## Release Prep

Before a release:

```bash
make ci
make docs-check
make vqa
make smoke-test
make release-check
```

Release workflow wiring and install verification are tracked in Task 180.

## Flicker-free rendering

`cmd/gh-hound/render.go` owns the TTY paint path (the same line-diff
strategy as Bubble Tea's standard renderer and the cellbuf screen
behind its v2 Cursed Renderer): keep the previous frame's lines, write
only the rows that changed, one buffered write per frame, mode-2026
synchronized-update guards around every flush. Never emit `ESC[2J`
outside the initial alt-screen entry — a steady-state clear is visible
as flicker. If you touch the render path, `make vqa` runs
`render_hygiene.sh`, which scrolls the real binary under shux and
fails on any full-screen erase in the raw PTY stream.
