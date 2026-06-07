# Task 100: runs list, all-green variant, filters, and live home

## Status
TODO

## Ownership Boundary
- **Primary area:** home screen.
- **Allowed files:** `internal/tui/screens/runs`, runs-list usecase extensions, component tests, fake scenarios.
- **Avoid touching:** run detail, logs, dispatch, release workflows.

## Depends On
- 020.
- 040.
- 070.
- 080.
- 090.

## Parallelizable With
- 110, 120, 130 after root shell is stable.
- **Parallel contract:** owns only home screen; route requests to other screens through root interfaces.

## PRD / Design References
- `docs/gh-hound-PRD.md` §9.1 — runs list.
- `docs/gh-hound-PRD.md` §9.2 — all-green state.
- `docs/gh-hound-PRD.md` §10 — responsive layout.
- `docs/gh-hound-design.html` visual refs ① and ②.

## Problem
The home screen answers "is it green?" and "what's running?" instantly. It must render live branch-scoped runs, a calm all-green state, server-side filters, cache indicators, selection, footer, and error/cached resilience.

## Scope
- Implement runs table with status glyph, workflow, event, run number, duration sparkline, and age.
- Implement all-green summary band variant.
- Implement loading, empty, error-with-cache, and live states.
- Implement `/` filter mode and server-side filter request construction.
- Implement keybindings and footer generated from keymap.

## Non-Goals
- Do not implement detail/log/watch screens; emit route intents only.

## Expected Artifacts
### Files to Create
- `internal/tui/screens/runs/model.go`, `view.go`, `update.go`, tests.
- `internal/tui/components/sparkline`.
- Runs screen fixtures under `testdata/`.

### Files to Modify
- `internal/tui/app` route wiring.
- `internal/adapter/fake` runs scenarios.

### Public Contracts
- Runs screen emits route intents for detail, logs, watch, dispatch, browser, copy, and mutations.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add model tests for selection, filters, live refresh, all-green switch, cached error, and route intents.
- Add view tests for headers, summary counts, footer keys, and 80-column truncation.

### Green: minimal implementation
- Implement screen model/view/update using fake adapter data.

### Refactor: harden without changing behavior
- Extract reusable table row formatting and age/duration helpers.

## Test Pyramid
### L0 Static / Architecture
- `make emoji-check`.

### L1 Unit Tests
- Selection, filters, summaries, and view content.

### L2 Component / Adapter Tests
- Fake adapter and usecase integration.

### L3 Integration Tests
- Root app launches to runs screen and routes on keypress.

### L4 Visual / Interaction Tests
- shux snapshots for refs ① and ② at three breakpoints.
- Interaction audit for all runs-list keys.

### L5 Live / Smoke Tests
- Optional live `gh hound -R owner/repo --no-tui` parity check.

## Definition of Done
- [ ] Red tests fail first.
- [ ] Runs list matches §②.
- [ ] All-green variant triggers exactly when no failing/running visible runs exist.
- [ ] Filter mode uses input-mode rules and server-side filters.
- [ ] Footer equals active keymap.
- [ ] VQA passes for refs ① and ②.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/screens/runs ./internal/tui/... ./internal/usecase
rtk make vqa SCREEN=runs
rtk make check
```

## Visual QA Checklist
- [ ] Columns align at all breakpoints.
- [ ] Selection fill and green bar are correct.
- [ ] Live spinner does not tear.
- [ ] All-green band is visually calm and matches mock.

## Implementation Notes
- Render from cache; never block keystrokes on network.
- Keep footer generated from keymap.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing screen tests.
2. Implement.
3. Run focused tests and VQA.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add runs home screen`

