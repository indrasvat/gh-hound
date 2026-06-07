# Task 120: failure view and excerpt workflow

## Status
TODO

## Ownership Boundary
- **Primary area:** failure screen and excerpt copy/open flows.
- **Allowed files:** `internal/tui/screens/failure`, usecase wiring for failure data, fake scenarios.
- **Avoid touching:** full log viewer implementation except route intent.

## Depends On
- 050.
- 070.
- 080.
- 110.

## Parallelizable With
- 130, 140 after parser contracts exist.
- **Parallel contract:** owns de-noised failure presentation, not full log viewport.

## PRD / Design References
- `docs/gh-hound-PRD.md` §9.4 — failure view.
- `docs/gh-hound-PRD.md` §4.4 — de-noise correlation.
- `docs/gh-hound-design.html` visual ref ④.

## Problem
The failure view is the killer screen. It must surface annotations first, then a concise colored excerpt around the actual failure, with one-key rerun/log/browser/copy actions.

## Scope
- Implement failure screen model/view/update.
- Render annotations and failure window.
- Route `l` to full log viewer at the same offset.
- Implement copy excerpt intent.
- Implement rerun failed/job intents.

## Non-Goals
- Do not implement clipboard backend unless already present; route intent is enough if clipboard is in a later shared helper.

## Expected Artifacts
### Files to Create
- `internal/tui/screens/failure/model.go`, `view.go`, `update.go`, tests.

### Files to Modify
- Root route wiring.
- Fake failure scenarios.

### Public Contracts
- Failure screen consumes parsed failure model from Task 050.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add view tests for annotations, excerpt line numbers, exit pill, footer, and same-offset log route.
- Add update tests for rerun/copy/browser/log keys.

### Green: minimal implementation
- Implement failure view and actions.

### Refactor: harden without changing behavior
- Share log line rendering primitives with Task 130 if already present.

## Test Pyramid
### L0 Static / Architecture
- `make emoji-check`.

### L1 Unit Tests
- Action intents and formatting.

### L2 Component / Adapter Tests
- Failure usecase with parser/fake adapter.

### L3 Integration Tests
- Detail -> failure route flow.

### L4 Visual / Interaction Tests
- shux snapshots for ref ④ at all breakpoints.
- Keyboard audit for failure context.

### L5 Live / Smoke Tests
- Optional failing-run fixture behind env flag.

## Definition of Done
- [ ] Red tests fail first.
- [ ] Error window contains actual failure.
- [ ] Annotations render with path/line/message.
- [ ] `l` opens full log at same offset.
- [ ] Footer equals active keymap.
- [ ] VQA passes for ref ④.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/screens/failure ./internal/logs ./internal/usecase
rtk make vqa SCREEN=failure
rtk make check
```

## Visual QA Checklist
- [ ] Annotations block is first and readable.
- [ ] Error excerpt has correct color classes and no bleed.
- [ ] Footer fits at 80x24.

## Implementation Notes
- Preserve line numbers and collapsed context indicators.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing failure-screen tests.
2. Implement.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add failure diagnosis view`

