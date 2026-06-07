# Task 060: Actions mutations and confirmable usecases

## Status
Done

## Ownership Boundary
- **Primary area:** rerun, cancel, force-cancel, dispatch usecases and adapter writes.
- **Allowed files:** `internal/adapter/github/mutations.go`, `internal/usecase/actions.go`, fake mutation scenarios, tests.
- **Avoid touching:** TUI confirm modal implementation; Task 140 owns presentation.

## Depends On
- 040.
- 010.

## Parallelizable With
- 050, 100, 110.
- **Parallel contract:** expose mutation results and errors; do not own keybindings or modals.

## PRD / Design References
- `docs/gh-hound-PRD.md` §4.2 — write endpoints and mutation spacing.
- `docs/gh-hound-PRD.md` §7.5 — destructive confirmation behavior.
- `docs/gh-hound-PRD.md` §9.7 — workflow dispatch.
- `docs/gh-hound-PRD.md` §12 — mutation error toasts.

## Problem
The daily loop includes acting on CI: rerun failed jobs, rerun a job, rerun the run, cancel, force-cancel, and dispatch workflows. These actions must be rate-limit-safe, confirmable, fakeable, and precisely reported.

## Scope
- Implement adapter write endpoints.
- Enforce >=1s spacing between mutations with injectable clock.
- Implement usecase methods for rerun run, rerun failed jobs, rerun job, cancel, force-cancel, and dispatch.
- Add typed mutation results for toast rendering.
- Add fake mutation scenarios for success, 403, 409, and rate limits.

## Non-Goals
- Do not build dispatch form UI.
- Do not build confirm modal UI.

## Expected Artifacts
### Files to Create
- `internal/adapter/github/mutations.go`.
- `internal/usecase/actions.go`.
- `internal/usecase/actions_test.go`.
- HTTP fixture tests for write bodies and status handling.

### Files to Modify
- `internal/adapter/fake/fake.go`.
- `internal/usecase/ports.go`.

### Public Contracts
- Mutations return typed `ActionResult` with run/job/workflow context and API message.
- Dispatch validates required inputs before POST.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add tests that two mutations cannot fire less than one second apart.
- Add body tests for each POST endpoint.
- Add dispatch validation tests for required/missing inputs.

### Green: minimal implementation
- Implement write endpoints and usecase spacing.

### Refactor: harden without changing behavior
- Keep clock and sleeper injectable to avoid slow tests.
- Normalize API errors for Task 070 toasts.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Usecase validation, spacing, and error mapping.

### L2 Component / Adapter Tests
- HTTP method/path/body tests for every mutation.

### L3 Integration Tests
- CLI fake-mode mutation command returns expected JSON and exit code.

### L4 Visual / Interaction Tests
- Confirm modal and toast verification in Task 140.

### L5 Live / Smoke Tests
- Optional live mutation tests must be opt-in and safe.

## Definition of Done
- [x] Red tests fail first.
- [x] All v1 write endpoints implemented.
- [x] Mutation spacing is tested without sleeping.
- [x] Dispatch inputs are validated.
- [x] Error mapping distinguishes 403, 409, rate limit, and network errors.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/usecase ./internal/adapter/...
rtk make check
```

## Visual QA Checklist
- [ ] Not applicable here.

## Implementation Notes
- Force-cancel must require explicit confirmation at TUI layer; expose metadata so UI can enforce it.
- Space mutations in the queue, not in each screen.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing mutation tests.
3. Implement.
4. Verify.
5. Commit and push.

## Commit Protocol
- Expected commit: `feat(actions): add github actions mutation usecases`

## Completion Evidence
- Red: `rtk go test -race ./internal/usecase ./internal/adapter/...` failed on missing `ActionService`, `MutationLimiter`, `DispatchRequest`, action errors, and adapter mutation methods.
- Focused tests: `rtk go test -race ./internal/usecase ./internal/adapter/...`.
- Full gate: `rtk make check`.
