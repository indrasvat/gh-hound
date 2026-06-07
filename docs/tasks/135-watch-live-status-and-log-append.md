# Task 135: watch live status and log append

## Status
TODO

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
- [ ] Red tests fail first.
- [ ] Polling is adaptive and cache-first.
- [ ] Completed step chunks append with follow on.
- [ ] `f` toggles follow.
- [ ] Cancel intent works.
- [ ] `--watch` fail-fast exit codes are correct.
- [ ] VQA passes for ref ⑤.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/screens/watch ./internal/usecase ./cmd/gh-hound
rtk make vqa SCREEN=watch
rtk make check
```

## Visual QA Checklist
- [ ] Streaming badge pulses without tearing.
- [ ] Follow indicator is accurate.
- [ ] Incoming log area matches mock.

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

