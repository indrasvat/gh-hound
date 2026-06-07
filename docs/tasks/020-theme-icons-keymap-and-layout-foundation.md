# Task 020: theme, icons, keymap, focus, and responsive layout foundation

## Status
Done

## Ownership Boundary
- **Primary area:** `internal/theme`, `internal/tui/keys`, `internal/layout`, `internal/tui/components`.
- **Allowed files:** theme/icon/keymap/layout packages and their tests.
- **Avoid touching:** concrete screens beyond test fixture views.

## Depends On
- 000.
- 010 for config merge hooks where keybinding overrides are validated.

## Parallelizable With
- 030, 040 after 010.
- **Parallel contract:** expose stable APIs for screen tasks 100-145; avoid adding screen-specific behavior here.

## PRD / Design References
- `docs/gh-hound-PRD.md` §5 — Bramble/Bone tokens.
- `docs/gh-hound-PRD.md` §6 — icon system and no emoji.
- `docs/gh-hound-PRD.md` §7 — layered keymap and input model.
- `docs/gh-hound-PRD.md` §10 — responsive breakpoints.
- `docs/gh-hound-design.html` sections 07, 08, and 10.

## Problem
The product's quality depends on consistent tokens, no emoji, generated key help, input-mode safety, and deterministic layout. If each screen invents styling and key handling, the TUI will drift from the mock and become hard to test.

## Scope
- Implement `theme.Theme` for Bramble and Bone.
- Implement semantic status/conclusion to token mapping.
- Implement centralized text-presentation icons.
- Implement layered keymap primitives, collision checks, and footer/help generation data.
- Implement responsive breakpoint helpers for `80x24`, `120x40`, and `200x60`.
- Add no-emoji scanner target for strings and snapshots.

## Non-Goals
- Do not build final screens.
- Do not implement shux VQA harness beyond scanner hooks.

## Expected Artifacts
### Files to Create
- `internal/theme/theme.go`, `theme_test.go`.
- `internal/tui/icons/icons.go`, `icons_test.go`.
- `internal/tui/keys/keymap.go`, `keymap_test.go`.
- `internal/layout/breakpoints.go`, `breakpoints_test.go`.
- `scripts/check-no-emoji.sh`.

### Files to Modify
- `Makefile` — add `emoji-check`, include in `check`.
- `internal/config` — connect keybinding override validation if needed.

### Public Contracts
- `theme.ForMode`, `theme.SemanticForStatus`, `theme.SemanticForConclusion`.
- `icons.ForStatus`, `icons.ForConclusion`, and action glyph constants.
- Keymap data is the single source for footer and help.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add table tests for every token hex, status/conclusion mapping, and icon glyph.
- Add a test that intentionally colliding keymaps are rejected.
- Add input-mode tests proving printable keys do not trigger commands.
- Add breakpoint tests for narrow, medium, and wide layouts.

### Green: minimal implementation
- Implement tokens, icon constants, keymap layers, conflict detection, and breakpoints.

### Refactor: harden without changing behavior
- Keep styles composable and avoid hard-coded colors outside `internal/theme`.
- Document any intentional key shadowing in code comments/tests.

## Test Pyramid
### L0 Static / Architecture
- `make emoji-check`.
- `make arch-check`.

### L1 Unit Tests
- Token, icon, key collision, input-mode, and breakpoint tests.

### L2 Component / Adapter Tests
- Render small fixture components with both themes.

### L3 Integration Tests
- Config keybinding overrides merge and collisions are rejected.

### L4 Visual / Interaction Tests
- Minimal fixture snapshot once Task 150 exists.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [ ] Red tests fail first.
- [ ] All PRD tokens exist exactly.
- [ ] No color constants outside `internal/theme` except tests and docs.
- [ ] All glyphs centralized and emoji-free.
- [ ] Keymap conflict test covers globals, overlays, screens, and input mode.
- [ ] Footer/help can be generated from the same keymap data.
- [ ] Breakpoint behavior is deterministic.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/theme ./internal/tui/icons ./internal/tui/keys ./internal/layout
rtk make emoji-check
rtk make check
```

## Visual QA Checklist
- [ ] Fixture component uses Bramble/Bone tokens correctly.
- [ ] No emoji or VS16 appears in rendered text.

## Implementation Notes
- `✔`, `✗`, and `▶` are emoji-capable codepoints; never append VS16.
- Keep wide-rune display-width helpers in layout rather than ad hoc string length checks.
- Completion evidence: red tests failed on missing theme/icon/keymap/layout APIs first; focused `go test -race ./internal/theme ./internal/tui/icons ./internal/tui/keys ./internal/layout` passed; `make check` passed with emoji and architecture gates.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Write failing table tests.
2. Implement the foundation.
3. Run verification.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add theme icons keymap and layout foundations`
