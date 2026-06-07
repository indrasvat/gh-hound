# Task 090: launch context resolution and empty/error states

## Status
Done

## Ownership Boundary
- **Primary area:** launch usecases and contextual fallback behavior.
- **Allowed files:** `internal/usecase/context.go`, `internal/adapter/repository`, CLI flag wiring, fake scenarios, minimal empty-state components.
- **Avoid touching:** full runs-list rendering beyond empty/error state contracts.

## Depends On
- 030.
- 040.
- 070.
- 080.

## Parallelizable With
- 110, 120 after route contracts are stable.
- **Parallel contract:** this task owns context resolution; Task 100 owns normal runs-list screen layout.

## PRD / Design References
- `docs/gh-hound-PRD.md` §8 — launch and context resolution.
- `docs/gh-hound-PRD.md` §9.1 and §9.2 state handling.
- `docs/gh-hound-design.html` section 03.

## Problem
The default launch must always land somewhere useful: current branch when possible, repo-wide fallback when branch has no runs, and explicit empty/error states when context cannot resolve. Blank screens are not allowed.

## Scope
- Implement resolution order for flags, env, gh repo, git remote, branch, detached HEAD.
- Implement fallback behaviors from §8.2.
- Connect launch to fake adapter scenarios.
- Expose user-facing notices for widened scope and errors.

## Non-Goals
- Do not implement full runs table.
- Do not implement live polling UI beyond usecase state.

## Expected Artifacts
### Files to Create
- `internal/usecase/context.go`, `context_test.go`.
- `internal/adapter/repository/repository.go` and tests/fakes.
- Empty/error state component tests.

### Files to Modify
- `cmd/gh-hound` launch flags.
- `internal/tui/app` route initialization.

### Public Contracts
- `LaunchContext` includes repo, branch, scope, actor/ref metadata, and fallback notice.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add tests for every fallback row: no runs, no workflows, not git repo, detached HEAD, unpushed SHA.
- Add CLI tests for `-R`, `GH_REPO`, `-A`, and `watch` routing.

### Green: minimal implementation
- Implement context resolver and launch usecase.

### Refactor: harden without changing behavior
- Keep git/gh operations behind interfaces for tests.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Resolution and fallback tests.

### L2 Component / Adapter Tests
- Fake repository detector tests.

### L3 Integration Tests
- CLI launch path uses fake context and routes correctly.

### L4 Visual / Interaction Tests
- Empty/error state snapshots in Task 150.

### L5 Live / Smoke Tests
- Optional live smoke inside a real repo.

## Definition of Done
- [x] Red tests fail first.
- [x] Every §8.2 fallback row is reproduced.
- [x] `-R`, `GH_REPO`, `-A`, and `watch` route correctly.
- [x] No blank screen states remain.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/usecase ./internal/adapter/repository ./cmd/gh-hound
rtk make check
```

## Visual QA Checklist
- [x] Empty/error text fits at 80x24.

## Verification Evidence
```bash
rtk go test -race ./internal/usecase ./internal/adapter/repository ./internal/tui/screens/empty ./cmd/gh-hound
# Go test: 24 passed in 4 packages

rtk make check
# go fix check passed
# 0 issues.
# emoji check passed
# architecture check passed
# check passed
```

## Implementation Summary
- Added `LaunchService` and `LaunchContext` contracts for repo/branch scope, notices, route state, all-green, watch, empty, and error outcomes.
- Added repository detection behind `Detector`, with `GH_REPO`/`HOUND_REPO`, GitHub remote parsing, branch/head metadata, and detached-HEAD signaling.
- Added minimal empty/error screen rendering with 80-column wrapping guarantees.
- Tightened CLI fake routing so `--all`, `GH_REPO`, `-R`, and `watch` reflect the PRD launch semantics in structured output.

## Implementation Notes
- Do not silently ignore unresolved remotes.
- Detached HEAD falls back branch -> repo-wide per PRD.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing launch fallback tests.
2. Implement resolver.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(launch): resolve repository and branch context`
