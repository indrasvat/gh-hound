# Task 135: watch live status and log append

## Status
Done

## Ownership Boundary
- **Primary area:** watch screen and live polling orchestration.
- **Allowed files:** `internal/tui/screens/watch`, usecase watch orchestration, fake running scenarios.
- **Avoid touching:** full log viewer static controls beyond shared components.

## Depends On
- 040.
- 050.
- 060.
- 080.
- 130 for shared log rendering if needed.

## Parallelizable With
- 140, 150 after root route contracts exist.
- **Parallel contract:** owns watch/follow/cancel/debug behavior only.

## PRD / Design References
- `docs/gh-hound-PRD.md` §9.5 — watch/live stream.
- `docs/gh-hound-PRD.md` §20 — no live-log socket.
- `docs/gh-hound-design.html` visual ref ⑤.

## Problem
Watch must be honest: GitHub does not expose a line-stream socket. The UI should show live step status, append completed step logs when available, follow the tail, and paint atomically without over-promising.

## Scope
- Implement watch screen.
- Poll job status adaptively.
- Append completed step log chunks.
- Implement follow toggle, cancel intent, debug logging toggle, detach.
- Implement `--watch` fail-fast behavior for pipe surface if not already complete.

## Non-Goals
- Do not fake unavailable live log streaming.

## Expected Artifacts
### Files to Create
- `internal/tui/screens/watch/model.go`, `view.go`, `update.go`, tests.
- `internal/usecase/watch.go`, tests.

### Files to Modify
- CLI `watch` command.
- Fake running scenarios.

### Public Contracts
- Watch usecase emits state updates: run status, step changes, appended chunks, terminal completion.
- `--watch` exits 0/1/2/3 per §13.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add fake-clock tests for poll cadence, step completion, log append, follow toggle, cancel, and fail-fast exit.

### Green: minimal implementation
- Implement watch usecase and screen.

### Refactor: harden without changing behavior
- Keep polling and rendering decoupled.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Watch state machine and follow behavior.

### L2 Component / Adapter Tests
- Fake adapter running-to-failed and running-to-success scenarios.

### L3 Integration Tests
- CLI `watch --no-tui --json` fail-fast scenario.

### L4 Visual / Interaction Tests
- shux snapshots for ref ⑤ at all breakpoints.
- Interaction audit for watch keys.

### L5 Live / Smoke Tests
- Optional live watch against a known repo behind env flag.

## Definition of Done
- [x] Red tests fail first.
- [x] Polling is adaptive and cache-first.
- [x] Completed step chunks append with follow on.
- [x] `f` toggles follow.
- [x] Cancel intent works.
- [x] `--watch` fail-fast exit codes are covered by the existing pipe/CLI watch path.
- [x] VQA command passes for ref ⑤ placeholder; screenshot VQA is owned by Task 150.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/screens/watch ./internal/usecase ./cmd/gh-hound
rtk make vqa SCREEN=watch
rtk make check
```

## Visual QA Checklist
- [x] Streaming badge content is present and bounded.
- [x] Follow indicator is accurate.
- [x] Incoming log area matches mock content contract.

## Verification Evidence
```bash
rtk go test -race ./internal/tui/screens/watch ./internal/usecase ./cmd/gh-hound ./internal/tui/...
# Go test: 55 passed in 16 packages

rtk make vqa SCREEN=watch
# VQA harness lands in Task 150; placeholder is intentionally explicit

rtk make check
# go fix check passed
# 0 issues.
# emoji check passed
# architecture check passed
# check passed
```

## Implementation Summary
- Added `WatchService` with adaptive polling, terminal detection, and completed-job log chunk append.
- Added the watch screen with follow/debug toggles, cancel/detach intents, streaming header, incoming log viewport, and generated footer.
- Wired the root watch route to a concrete watch component shell.

## Implementation Notes
- Use Bubble Tea synchronized output where available.
- The copy must not claim true line streaming.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing watch tests.
2. Implement.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add watch screen`
