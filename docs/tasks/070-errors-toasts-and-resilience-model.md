# Task 070: error taxonomy, toasts, and resilience model

## Status
Done

## Ownership Boundary
- **Primary area:** error taxonomy and toast data/model behavior.
- **Allowed files:** `internal/usecase/errors.go`, `internal/tui/toast`, fake adapter error scenarios.
- **Avoid touching:** final screen layouts except fixture views.

## Depends On
- 020.
- 040.
- 060.

## Parallelizable With
- 100, 110 after shared toast model exists.
- **Parallel contract:** this task owns toast behavior and taxonomy, not screen-specific layout.

## PRD / Design References
- `docs/gh-hound-PRD.md` §12 — errors, toasts, resilience.
- `docs/gh-hound-design.html` visual ref ⑦ toasts.

## Problem
GitHub failures must never blank the screen. Users need cached data plus clear toasts, retry paths, and non-blocking behavior. Every error class needs deterministic fake reproduction.

## Scope
- Implement error taxonomy mapping for rate limit, network/render, log render, mutation rejected, and success.
- Implement toast model with severities, auto-dismiss timer, `Esc` top-dismiss, and `g` dismiss-all.
- Keep toasts non-focus-stealing.
- Add fake adapter scenarios reproducing each taxonomy row.

## Non-Goals
- Do not implement full Canvas overlay visuals; Task 140 integrates into root TUI.

## Expected Artifacts
### Files to Create
- `internal/usecase/errors.go`.
- `internal/tui/toast/model.go`, `view.go`, `toast_test.go`.
- `internal/tui/toast/testdata/`.

### Files to Modify
- `internal/adapter/fake` scenarios.
- `internal/tui/keys` if dismiss keys need formal binding.

### Public Contracts
- `Toast` data includes severity, title, message, retry action, timeout, and source error class.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add table tests for every §12.2 taxonomy row.
- Add Bubble Tea update tests for auto-dismiss, `Esc`, `g`, and key passthrough.

### Green: minimal implementation
- Implement taxonomy and toast model.

### Refactor: harden without changing behavior
- Keep toast rendering token-based and testable without terminal.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Error mapping and toast state transitions.

### L2 Component / Adapter Tests
- Fake adapter scenarios produce expected toast data.

### L3 Integration Tests
- Screen fixture can receive an error and keep cached rows.

### L4 Visual / Interaction Tests
- Full visual overlay gate in Task 140/150.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [x] Red tests fail first.
- [x] Every taxonomy row is reproduced by fake adapter.
- [x] Auto-dismiss, `Esc`, and `g` are tested.
- [x] Toasts do not steal focus.
- [x] Cached data remains renderable during errors.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/usecase ./internal/tui/toast
rtk make check
```

## Visual QA Checklist
- [x] Toast fixture uses severity accents and no emoji.

## Implementation Notes
- Severity mapping: `err`, `warn`, `info`, `ok`.
- Retry is data, not a UI-specific callback, so tests can assert it.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing taxonomy/toast tests.
3. Implement.
4. Verify.
5. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add resilient toast model`

## Completion Evidence
- Red: `rtk go test -race ./internal/usecase ./internal/tui/toast` failed on missing resilience taxonomy and toast model APIs.
- Focused tests: `rtk go test -race ./internal/usecase ./internal/tui/toast ./internal/adapter/fake`.
- Full gate: `rtk make check`.
