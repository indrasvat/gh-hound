# Task 030: CLI, logging, and pipe-surface foundation

## Status
TODO

## Ownership Boundary
- **Primary area:** `cmd/gh-hound`, `internal/render`, logging bootstrap.
- **Allowed files:** CLI command tree, renderer interfaces, JSON schema tests, logging config.
- **Avoid touching:** real adapter implementation and concrete TUI screens.

## Depends On
- 000.
- 010.

## Parallelizable With
- 020, 040.
- **Parallel contract:** depend on usecase ports and fake data; do not own adapter internals.

## PRD / Design References
- `docs/gh-hound-PRD.md` §3.2 — CLI conventions and env vars.
- `docs/gh-hound-PRD.md` §8.3 — overrides and subcommands.
- `docs/gh-hound-PRD.md` §13 — dual surface and exit codes.
- `docs/gh-hound-PRD.md` Appendix B — JSON schema.

## Problem
`gh hound` must serve humans in a TTY and agents through structured output from the same usecase core. The command tree, logging, env/flag wiring, and exit codes must be stable before feature work plugs in real behavior.

## Scope
- Implement cobra root and subcommands: root TUI launch placeholder, `runs`, `watch`, `dispatch`, `version`.
- Wire flags and matching `HOUND_*` env vars.
- Detect TTY vs pipe and `--no-tui`.
- Implement `render` package for JSON, markdown, and XML skeletons.
- Implement JSON schema validation tests for Appendix B shape.
- Configure `slog` JSON logging under XDG state dir with `--log-level` and `--trace-http` plumbing.

## Non-Goals
- Do not implement real data fetching.
- Do not implement agent skill package; Task 160 owns that.

## Expected Artifacts
### Files to Create
- `cmd/gh-hound/root.go` and command files.
- `internal/render/json.go`, `markdown.go`, `xml.go`.
- `internal/render/testdata/*.golden`.
- `internal/logging/logging.go`.

### Files to Modify
- `cmd/gh-hound/main.go`.
- `Makefile` if focused CLI test targets are useful.

### Public Contracts
- Exit codes: `0` ok, `1` action needed, `2` error, `3` pending.
- Flags/env: every Hound flag has an env var documented in help.
- JSON output conforms to Appendix B.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add CLI tests for `--help`, env override, `--no-tui --json`, and exit-code mapping.
- Add golden tests for JSON render shape.
- Add logging test proving JSON logs are written to XDG state path.

### Green: minimal implementation
- Implement command tree, renderers, logging bootstrap, and fake usecase output.

### Refactor: harden without changing behavior
- Keep command construction injectable so tests do not mutate process globals.

## Test Pyramid
### L0 Static / Architecture
- `make lint`.

### L1 Unit Tests
- Renderer golden tests and exit-code unit tests.

### L2 Component / Adapter Tests
- Cobra command tests with fake IO/env.

### L3 Integration Tests
- Built binary `--no-tui --json` through `jq`.

### L4 Visual / Interaction Tests
- Not required.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [ ] Red tests fail first.
- [ ] Every flag has matching env var.
- [ ] TTY/pipe detection is tested.
- [ ] JSON schema matches Appendix B.
- [ ] Exit code mapping is tested.
- [ ] Structured logs are valid JSON.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./cmd/gh-hound ./internal/render ./internal/logging
rtk make build
rtk './bin/gh-hound runs --no-tui --json | jq .'
rtk make check
```

## Visual QA Checklist
- [ ] CLI help is readable and lists env vars.

## Implementation Notes
- Keep root with no subcommand as TUI launch only when stdout is a TTY.
- In tests, inject env lookup and writer streams.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing CLI/render/logging tests.
2. Implement command tree and renderers.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(cli): add command and pipe surface foundation`

