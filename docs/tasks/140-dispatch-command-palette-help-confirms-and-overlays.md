# Task 140: dispatch, command palette, help, confirms, and overlays

## Status
TODO

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
- [ ] Red tests fail first.
- [ ] Dispatch fields reflect workflow inputs.
- [ ] Input mode suppresses single-letter commands.
- [ ] Help opens from every screen.
- [ ] `Esc` pops exactly one overlay/layer.
- [ ] Destructive confirms are safe by default.
- [ ] VQA passes for overlay refs.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/overlay/... ./internal/tui/screens/dispatch ./internal/tui/...
rtk make vqa SCREEN=overlays
rtk make check
```

## Visual QA Checklist
- [ ] Overlays dim base and do not garble underlying view.
- [ ] Help uses three columns and active screen keymap.
- [ ] Palette selected row has fill and green bar.
- [ ] Dispatch cursor and radio/select controls match mock.

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

