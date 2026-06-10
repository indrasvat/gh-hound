# Task 110: run detail master-detail and step timeline

## Status
Done

## Ownership Boundary
- **Primary area:** run detail screen.
- **Allowed files:** `internal/tui/screens/detail`, supporting components, fake detail scenarios.
- **Avoid touching:** failure view/log viewer except route intents.

## Depends On
- 020.
- 040.
- 080.
- 100 for route integration.

## Parallelizable With
- 120, 130 after shared screen interfaces exist.
- **Parallel contract:** owns job/step master-detail UI only.

## PRD / Design References
- `docs/gh-hound-PRD.md` §9.3 — run detail.
- `docs/gh-hound-PRD.md` §10 — narrow collapse.
- `docs/gh-hound-design.html` visual ref ③.

## Problem
Users need to drill from a run into jobs and steps without hunting. The failed step must be pre-highlighted, focus must move predictably, and narrow layouts must collapse without overlap.

## Scope
- Implement jobs pane and step timeline pane.
- Implement `Tab` focus cycling between jobs and steps.
- Implement failed-step highlight and `n` jump.
- Implement sibling run navigation intents via `J/K`.
- Implement narrow layout push/collapse.

## Non-Goals
- Do not implement failure/log content; route to Tasks 120/130.

## Expected Artifacts
### Files to Create
- `internal/tui/screens/detail/model.go`, `view.go`, `update.go`, tests.

### Files to Modify
- Root route wiring.
- Fake adapter detail scenarios.

### Public Contracts
- Detail screen route intents: failure, log, watch, rerun job, rerun failed, cancel, browser.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add tests for focus cycling, failed-step selection, `n`, narrow collapse, and footer keys.

### Green: minimal implementation
- Implement model/update/view with fake jobs/steps.

### Refactor: harden without changing behavior
- Extract job row and step row renderers.

## Test Pyramid
### L0 Static / Architecture
- `make emoji-check`.

### L1 Unit Tests
- Focus, selection, route intents.

### L2 Component / Adapter Tests
- View tests with partial/queued jobs.

### L3 Integration Tests
- Runs -> detail navigation through root app.

### L4 Visual / Interaction Tests
- shux snapshots for ref ③ at all breakpoints.
- Keyboard audit for detail context.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [x] Red tests fail first.
- [x] Two panes align and share height at medium/wide.
- [x] Narrow layout collapses correctly.
- [x] Failed step is highlighted and `n` lands on it.
- [x] `Tab` focus is visibly represented.
- [x] VQA command passes for ref ③ placeholder; screenshot VQA is owned by Task 150.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/screens/detail ./internal/tui/...
rtk make vqa SCREEN=detail
rtk make check
```

## Visual QA Checklist
- [x] Focus marker moves with `Tab`.
- [x] Failed step highlight is unambiguous in the bounded text view.
- [x] Breadcrumb and footer fit at 80x24.

## Verification Evidence
```bash
rtk go test -race ./internal/tui/screens/detail ./internal/tui/...
# Go test: 24 passed in 10 packages

rtk make vqa SCREEN=detail
# VQA harness lands in Task 150; placeholder is intentionally explicit

rtk make check
# go fix check passed
# 0 issues.
# emoji check passed
# architecture check passed
# check passed
```

## Implementation Summary
- Added the run detail model with job/step focus, failed-step preselection, `Tab`, `n`, sibling-run intents, and action/log route intents.
- Added the master-detail view with breadcrumb, jobs pane, step timeline, selected failed-step marker, detail footer, and narrow collapse.
- Wired the root detail route to render the detail component shell.

## Implementation Notes
- Use display width, not byte length, for truncation.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing detail tests.
2. Implement.
3. Verify with VQA.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add run detail screen`
