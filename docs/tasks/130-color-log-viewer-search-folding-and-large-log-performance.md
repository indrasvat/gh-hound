# Task 130: color log viewer, search, folding, and large-log performance

## Status
TODO

## Ownership Boundary
- **Primary area:** full log viewer screen.
- **Allowed files:** `internal/tui/screens/log`, log viewport components, parser integration tests.
- **Avoid touching:** watch screen and failure excerpt beyond shared renderer helpers.

## Depends On
- 050.
- 080.
- 120 for route integration if needed.

## Parallelizable With
- 140 after parser contracts exist.
- **Parallel contract:** owns static/full log viewport; Task 135 owns watch behavior.

## PRD / Design References
- `docs/gh-hound-PRD.md` §9.6 — color log viewer.
- `docs/gh-hound-PRD.md` §11 — 10k-line performance.
- `docs/gh-hound-design.html` visual ref ⑥.

## Problem
Logs must be terminal-native, fast, colored, searchable, and foldable. Rendering every styled line eagerly will jank on real logs, so the viewer must render only the visible window and maintain correct fold/search state.

## Scope
- Implement log viewer screen with line gutter, folds, search, match navigation, wrap toggle, and scroll keys.
- Render ANSI/token colors from parsed log model.
- Lazily style visible viewport lines.
- Add performance tests for 10k-line logs.

## Non-Goals
- Do not implement live watch append behavior.

## Expected Artifacts
### Files to Create
- `internal/tui/screens/log/model.go`, `view.go`, `update.go`, tests.
- `internal/tui/components/logview/`.

### Files to Modify
- Root route wiring.

### Public Contracts
- Log screen can open at a requested line offset.
- Search state exposes current match and total matches.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add tests for fold/unfold, search counts, match navigation, wrap toggle, open-at-offset, and viewport-only rendering.
- Add large log performance test.

### Green: minimal implementation
- Implement log model/view/update.

### Refactor: harden without changing behavior
- Separate fold model from viewport render.
- Cache styled visible windows safely.

## Test Pyramid
### L0 Static / Architecture
- `make emoji-check`.

### L1 Unit Tests
- Fold/search/scroll/wrap state.

### L2 Component / Adapter Tests
- View tests with ANSI and token classes.

### L3 Integration Tests
- Failure -> full log route starts at same offset.

### L4 Visual / Interaction Tests
- shux snapshots for ref ⑥ at all breakpoints.
- Keyboard audit for log context.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [ ] Red tests fail first.
- [ ] ANSI colors preserved.
- [ ] Folds collapse/expand and counts are correct.
- [ ] Search highlights and match counts are correct.
- [ ] 10k-line log scroll is smooth under target.
- [ ] VQA passes for ref ⑥.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/screens/log ./internal/tui/components/logview ./internal/logs
rtk go test -bench=. ./internal/tui/screens/log
rtk make vqa SCREEN=log
rtk make check
```

## Visual QA Checklist
- [ ] Line-number gutter is dim and aligned.
- [ ] Search hit line is highlighted.
- [ ] Folds use `▾`/`▸` and counts.
- [ ] No color bleed across lines.

## Implementation Notes
- Prefer viewport rendering by visible range, not full string concat of large logs.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing log-viewer tests.
2. Implement.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add color log viewer`

