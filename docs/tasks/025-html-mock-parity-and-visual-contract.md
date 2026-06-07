# Task 025: HTML mock parity and visual contract

## Status
TODO

## Ownership Boundary
- **Primary area:** visual source-of-truth extraction and parity rules.
- **Allowed files:** `internal/theme`, `internal/layout`, `internal/tui/icons`, `docs/visual-contract.md`, VQA assertions, scripts that extract/compare mock metadata.
- **Avoid touching:** feature behavior and real API code.

## Depends On
- 020.

## Parallelizable With
- 030, 040.
- **Parallel contract:** this task owns visual contract data and assertions; screen tasks consume them.

## PRD / Design References
- `docs/gh-hound-PRD.md` §5, §6, §7, §9, §10, §16, §17.
- `docs/gh-hound-design.html` entire file, with special attention to CSS tokens and visual refs ⓪-⑩.

## Problem
Most TUI implementation failures happen when agents stop looking at the mock and approximate from memory. gh-hound must match the HTML mocks closely: CSS theme tokens, layouts, screens, keyboard navigation, logs, overlays, and ASCII banner all need an explicit parity contract before screen work starts.

## Scope
- Extract and document visual tokens from `gh-hound-design.html`.
- Document exact screen refs, expected components, key footers, and breakpoint behavior.
- Add tests/assertions that compare implementation tokens/glyphs/key footer data against the visual contract.
- Add a mandatory "re-read mock" checklist hook to screen-task verification.

## Non-Goals
- Do not implement all screens here.
- Do not replace human/agent visual inspection with a fake automatic pass.

## Expected Artifacts
### Files to Create
- `docs/visual-contract.md` — terse source-of-truth map from HTML mock to implementation.
- `scripts/extract-visual-contract.sh` or equivalent helper if useful.
- Tests/assertions tying tokens, glyphs, and key footer labels to the contract.

### Files to Modify
- `Makefile` — add `visual-contract-check`.
- `internal/theme`, `internal/tui/icons`, and `internal/tui/keys` tests if needed.

### Public Contracts
- Screen refs: ⓪ welcome/version, ① all-green, ② runs, ③ detail, ④ failure, ⑤ watch, ⑥ log viewer, ⑦ toasts/dispatch, ⑧ palette, ⑩ help.
- Breakpoints: `80x24`, `120x40`, `200x60`.
- Every screen task must re-read the matching HTML mock before edits and before done.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add `visual-contract-check` and token/glyph assertions that fail until contract data exists.
- Add at least one test that fails if a CSS token from the HTML is missing from `theme.Theme`.

### Green: minimal implementation
- Document the contract and wire tests against implementation constants.

### Refactor: harden without changing behavior
- Keep contract docs short enough that agents actually re-read them.
- Avoid duplicating the entire HTML file; cite anchors/refs and extract only operational requirements.

## Test Pyramid
### L0 Static / Architecture
- `make visual-contract-check`.
- `make emoji-check`.

### L1 Unit Tests
- Theme token and icon parity tests.
- Key footer label parity tests where keymap is available.

### L2 Component / Adapter Tests
- Fixture render checks for banner/log/palette/help labels.

### L3 Integration Tests
- VQA harness later consumes the same screen-ref list.

### L4 Visual / Interaction Tests
- This task defines the contract; Task 150 proves the actual pixels.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [ ] Red visual-contract tests fail first.
- [ ] `docs/visual-contract.md` maps every visual ref to implementation requirements.
- [ ] CSS token parity is tested.
- [ ] Glyph/no-emoji parity is tested.
- [ ] Key footer/help parity requirements are documented and testable.
- [ ] Every screen task includes the re-read-mock protocol.
- [ ] `make visual-contract-check` passes.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk make visual-contract-check
rtk go test -race ./internal/theme ./internal/tui/icons ./internal/tui/keys
rtk make check
```

## Visual QA Checklist
- [ ] Contract covers CSS theme, ASCII banner, all screens, logs, overlays, and keyboard navigation.
- [ ] Contract points to HTML mock refs rather than relying on memory.

## Implementation Notes
- Do not summarize away important CSS details such as selected-row fill, green left bar, gradient focus border, and log color classes.
- Use this task as the grounding pass before any TUI screen implementation.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the full `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing visual-contract tests/check.
3. Implement the contract docs and assertions.
4. Verify.
5. Commit and push.

## Commit Protocol
- Expected commit: `test(visual): add html mock parity contract`

