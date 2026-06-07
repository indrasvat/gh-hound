# Development

## Setup

```bash
make tools
make hooks
make build
```

Required local tools for the full gate: Go from `go.mod`, `golangci-lint`, `gotestsum`, `lefthook`, `jq`, and `shux`. `vhs` is needed only for demo recording.

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

- `welcome`, `all_green`, `runs`, `detail`, `failure`, `watch`, `log`, `dispatch`, `palette`, and `help`.
- Breakpoints: `80x24`, `120x40`, `200x60`.
- Interaction frames for global overlays and screen-specific key paths.

Generated artifacts are under `.claude/automations/screenshots/` and are ignored except for the directory policy files.

## Agent Surface QA

Use deterministic fake scenarios:

```bash
make build
./bin/gh-hound runs --no-tui --json --fake-scenario failure | jq '.runs[0].failed[0]'
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
