# Task 050: log fetch, de-noise, annotations, and color model

## Status
Done

## Ownership Boundary
- **Primary area:** log retrieval/parsing and failure extraction.
- **Allowed files:** `internal/adapter/github/logs.go`, `internal/logs/`, `internal/usecase/failure.go`, fixtures.
- **Avoid touching:** final log viewer TUI rendering beyond parser-friendly data structures.

## Depends On
- 040.
- 020 for token/icon constants where parser labels need semantic classes.

## Parallelizable With
- 060, 100 after adapter contracts are stable.
- **Parallel contract:** own parsed log data and failure excerpt; Task 130 owns viewport UI.

## PRD / Design References
- `docs/gh-hound-PRD.md` §4.1 — job logs and annotations.
- `docs/gh-hound-PRD.md` §4.4 — log parsing and de-noise.
- `docs/gh-hound-PRD.md` §9.4 — failure view.
- `docs/gh-hound-PRD.md` §9.6 — color log viewer.
- `docs/gh-hound-PRD.md` §20 — 60s log link expiry and zip-vs-text gotchas.

## Problem
The core wedge is the failure view: the error comes to the user instead of forcing them through thousands of lines. This requires reliable job-log fetching, expired redirect recovery, annotations, group folding metadata, ANSI parsing, and a deterministic de-noised excerpt.

## Scope
- Fetch single-job plain-text logs via the job logs endpoint.
- Detect and recover from expired 302 log links by refetching.
- Parse GitHub group markers and fold ranges.
- Extract first meaningful failure window with surrounding context.
- Correlate annotations from `check_run_url`.
- Produce tokenized lines for later colored rendering.
- Add 10k-line fixture and performance test.

## Non-Goals
- Do not implement full log viewer UI.
- Do not implement run-log zip export unless needed for fixtures.

## Expected Artifacts
### Files to Create
- `internal/logs/parser.go`, `failure.go`, `folds.go`, `tokens.go`.
- `internal/logs/testdata/*.log`.
- `internal/logs/parser_test.go`, `failure_test.go`, `folds_test.go`.
- `internal/usecase/failure.go`.

### Files to Modify
- `internal/adapter/github/logs.go`.
- `internal/adapter/fake` scenarios for failed logs and annotations.

### Public Contracts
- Parsed log model with lines, display tokens, folds, search-ready text, failure window, and annotations.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add failing tests for Go test failure, shell exit failure, `##[error]`, nested groups, no failure, and expired link recovery.
- Add benchmark or timed test for 10k-line parse.

### Green: minimal implementation
- Implement parser and failure extraction.
- Wire adapter/usecase methods to return parsed results.

### Refactor: harden without changing behavior
- Keep parsing streaming-friendly.
- Separate tokenization from UI styling.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Parser, fold, token, and failure-window tests.

### L2 Component / Adapter Tests
- HTTP tests for log endpoint redirect and expiry retry.

### L3 Integration Tests
- `--no-tui --json --logs` style path includes failed step, annotations, and excerpt once CLI supports it.

### L4 Visual / Interaction Tests
- Covered by Tasks 120 and 130.

### L5 Live / Smoke Tests
- Optional live failing-run fixture behind env flag.

## Definition of Done
- [x] Red tests fail first.
- [x] Failure window points to the actual failure in all fixtures.
- [x] Annotations are present and path/line/message correct.
- [x] Expired log link recovery is tested.
- [x] Group folds are correct and nested groups behave.
- [x] 10k-line parse is within the performance target.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/logs ./internal/adapter/... ./internal/usecase/...
rtk go test -bench=. ./internal/logs
rtk make check
```

## Visual QA Checklist
- [x] Not applicable here; visual color fidelity is Task 130.

## Implementation Notes
- Prefer job logs over run zip logs for drill-down.
- Never cache a redirected log URL.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing log fixtures/tests.
3. Implement parser/fetch/retry.
4. Verify.
5. Commit and push.

## Commit Protocol
- Expected commit: `feat(logs): add failure extraction and annotations`

## Completion Evidence
- Red: `rtk go test -race ./internal/logs ./internal/adapter/... ./internal/usecase/...` failed on missing `logs.Parse`, `FetchJobLog`, and `FailureService`.
- Focused tests: `rtk go test -race ./internal/logs ./internal/adapter/... ./internal/usecase/...`.
- Benchmark: `rtk go test -bench=. ./internal/logs` reported `BenchmarkParse10kLines-10` at about `25.8ms/op`.
- Full gate: `rtk make check`.
