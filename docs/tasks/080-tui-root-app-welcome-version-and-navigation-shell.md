# Task 080: TUI root app, welcome, version banner, and navigation shell

## Status
Done

## Ownership Boundary
- **Primary area:** root Bubble Tea app model and navigation shell.
- **Allowed files:** `internal/tui/app.go`, `internal/tui/screens/welcome`, shared routing/focus/modal stack.
- **Avoid touching:** detailed screen implementations beyond placeholders.

## Depends On
- 020.
- 030.
- 070.

## Parallelizable With
- 100, 110 after shell interfaces are merged.
- **Parallel contract:** root owns routing and overlay stack; individual screens own their models/views.

## PRD / Design References
- `docs/gh-hound-PRD.md` §7 — modal stack and global keys.
- `docs/gh-hound-PRD.md` §9.0 — screen flow.
- `docs/gh-hound-PRD.md` §9.0.1 — welcome and version banner.
- `docs/gh-hound-design.html` visual ref ⓪ and root chrome across screens.

## Problem
Every screen depends on a stable app shell: global key resolution, modal stack, route transitions, welcome/version behavior, theme cycling, and footer/help plumbing. Building screens before the shell risks inconsistent navigation.

## Scope
- Implement root Bubble Tea model with route stack.
- Implement theme cycling via `T`.
- Implement modal/overlay stack semantics: top overlay receives keys; `Esc` pops exactly one layer.
- Implement optional first-run welcome splash.
- Implement version banner function used by CLI and splash.
- Wire placeholder screens so routes compile.

## Non-Goals
- Do not implement real runs/detail/log views.
- Do not implement final help/palette overlays; Task 140 owns them.

## Expected Artifacts
### Files to Create
- `internal/tui/app.go`, `app_test.go`.
- `internal/tui/screens/welcome/model.go`, `view.go`, tests.
- `internal/tui/banner/banner.go`, tests.

### Files to Modify
- `cmd/gh-hound` root launch path.
- `internal/config` for welcome dismissed flag if needed.

### Public Contracts
- Route identifiers and screen interface.
- Root handles globals and delegates context keys.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add update tests for `T`, `?`, `:`, `Esc`, `q`, `Ctrl+C`, and welcome dismissal.
- Add tests proving input-mode blocks global printable keys.

### Green: minimal implementation
- Implement root app shell and placeholder route rendering.

### Refactor: harden without changing behavior
- Keep route stack and modal stack separate.
- Keep command effects injectable.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Root update transitions and banner output.

### L2 Component / Adapter Tests
- Welcome screen view tests.

### L3 Integration Tests
- Binary can launch fake TUI and quit cleanly in test mode.

### L4 Visual / Interaction Tests
- Welcome/banner snapshot in Task 150.

### L5 Live / Smoke Tests
- `make smoke-test` launches and exits without raw-mode errors.

## Definition of Done
- [x] Red tests fail first.
- [x] Root route stack works.
- [x] `Esc` pops exactly one layer.
- [x] `T` recolors root view.
- [x] Welcome splash shows once and can be disabled.
- [x] Version banner is emoji-free and shared with CLI.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/tui/... ./cmd/gh-hound
rtk make smoke-test
rtk make check
```

## Visual QA Checklist
- [x] Welcome/banner uses PRD glyph constraints.
- [x] No broken layout at narrow terminal in placeholder shell.

## Verification Evidence
```bash
rtk go test -race ./internal/tui/... ./cmd/gh-hound
# Go test: 19 passed in 7 packages

rtk make smoke-test
# smoke test passed

rtk make check
# go fix check passed
# 0 issues.
# emoji check passed
# architecture check passed
# check passed
```

## Implementation Summary
- Added the root app shell with route stack, overlay stack, input-mode gating, global quit, help/palette routing, welcome dismissal, and theme toggle.
- Added a shared ASCII version banner used by both CLI and TUI welcome.
- Added the mock-aligned welcome screen content and footer plumbing.
- Added `welcome` config support through defaults, TOML, env, and overrides.
- Hardened `scripts/smoke-test.sh` so pipefail does not trip on banner output consumed by `grep -q`.

## Implementation Notes
- Keep altscreen/raw-mode launch out of non-interactive tests.
- Use Bubble Tea v2 primitives only.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing app shell tests.
2. Implement shell.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(tui): add root app shell`
