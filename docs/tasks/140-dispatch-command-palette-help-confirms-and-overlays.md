# Task 140: dispatch, command palette, help, confirms, and overlays

## Status
Done

## Ownership Boundary
- **Primary area:** overlay surfaces and dispatch form.
- **Allowed files:** `internal/tui/overlay`, `internal/tui/screens/dispatch`, help/palette/confirm components.
- **Avoid touching:** existing screen internals except to connect overlay hooks.

## Depends On
- 020.
- 060.
- 070.
- 080.
- 100.

## Parallelizable With
- 135, 150 after shared route contracts exist.
- **Parallel contract:** owns overlays/forms; screens supply route/action metadata.

## PRD / Design References
- `docs/gh-hound-PRD.md` §7.5-§7.7 — overlays, confirms, help.
- `docs/gh-hound-PRD.md` §9.7 — dispatch.
- `docs/gh-hound-PRD.md` §9.8 — command palette.
- `docs/gh-hound-PRD.md` §9.9 — help modal.
- `docs/gh-hound-design.html` visual refs ⑧, ⑨, ⑩, and ⑦ toasts.

## Problem
The TUI needs high-confidence overlay behavior: command palette from anywhere, contextual help generated from keymaps, safe destructive confirms, and typed workflow dispatch input mode. This is where key conflicts and accidental destructive actions are most likely.

## Scope
- Implement dispatch form for workflow_dispatch-enabled workflows.
- Implement command palette overlay.
- Implement contextual help overlay from active keymap plus legend.
- Implement confirm modal for destructive actions.
- Integrate toast overlay visuals from Task 070.
- Ensure input mode captures printable keys.

## Non-Goals
- Do not implement v2 approval actions except as disabled/roadmap entries if present.

## Expected Artifacts
### Files to Create
- `internal/tui/screens/dispatch/`.
- `internal/tui/overlay/help/`, `palette/`, `confirm/`.
- Tests for every overlay.

### Files to Modify
- Root app overlay stack.
- Screen key handlers to invoke confirms/overlays.

### Public Contracts
- Help and footer are generated from keymap.
- Confirm modal defaults to No; force-cancel requires explicit `y`.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add tests for palette route selection, help context, `Esc` one-layer pop, confirm defaults, force-cancel explicit confirmation, and dispatch input suppression of global keys.

### Green: minimal implementation
- Implement overlays and dispatch form using fake workflow schema.

### Refactor: harden without changing behavior
- Keep overlay stack generic.
- Share input-mode handling with keymap foundation.

## Test Pyramid
### L0 Static / Architecture
- `make emoji-check`.

### L1 Unit Tests
- Overlay stack, confirms, help, palette filtering.

### L2 Component / Adapter Tests
- Dispatch form validation with workflow schema.

### L3 Integration Tests
- Open overlays from each screen and route through root app.

### L4 Visual / Interaction Tests
- shux snapshots for refs ⑧, ⑨, ⑩, and toast overlay.
- Full keyboard audit for overlays/input mode.

### L5 Live / Smoke Tests
- Optional dispatch dry-run only; live dispatch must be opt-in.

## Definition of Done
- [x] Red tests fail first.
- [x] Dispatch fields reflect workflow inputs.
- [x] Input mode suppresses single-letter commands.
- [x] Help opens from every screen via the root overlay stack contract.
- [x] `Esc` pops exactly one overlay/layer.
- [x] Destructive confirms are safe by default.
- [x] VQA command passes for overlay refs placeholder; screenshot VQA is owned by Task 150.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/overlay/... ./internal/tui/screens/dispatch ./internal/tui/...
rtk make vqa SCREEN=overlays
rtk make check
```

## Visual QA Checklist
- [x] Overlays render after the base view without garbling it.
- [x] Help uses grouped sections and active screen keymap data.
- [x] Palette selected row has the selection bar contract.
- [x] Dispatch cursor and radio/select controls match the mock content contract.

## Verification Evidence
```bash
rtk go test -race ./internal/tui/overlay/... ./internal/tui/screens/dispatch ./internal/tui/...
# Go test: 40 passed in 18 packages

rtk make vqa SCREEN=overlays
# VQA harness lands in Task 150; placeholder is intentionally explicit

rtk make check
# go fix check passed
# 0 issues.
# emoji check passed
# architecture check passed
# check passed
```

## Implementation Summary
- Added palette, help, and confirm overlay packages with filtering, keymap-generated help, and safe confirm defaults.
- Added the dispatch form model/view with typed inputs, required validation, submit/cancel intents, and generated footer.
- Wired dispatch, help, and palette rendering into the root app overlay stack.

## Implementation Notes
- Use `huh` where it fits; use Bubble Tea textinput if direct control is cleaner.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing overlay/dispatch tests.
2. Implement.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add overlays and dispatch flow`
