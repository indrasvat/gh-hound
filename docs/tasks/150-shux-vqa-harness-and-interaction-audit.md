# Task 150: shux VQA harness and interaction audit

## Status
TODO

## Ownership Boundary
- **Primary area:** visual and interaction verification automation.
- **Allowed files:** `.claude/automations/`, `scripts/vqa*.sh`, `Makefile`, test fixtures.
- **Avoid touching:** feature code except small testability hooks.

## Depends On
- 080.
- 100.
- 110.
- 120.
- 130.
- 135.
- 140.

## Parallelizable With
- 160 after screen contracts stabilize.
- **Parallel contract:** owns automation and evidence capture, not product behavior.

## PRD / Design References
- `docs/gh-hound-PRD.md` §16 — visual verification with shux.
- `docs/gh-hound-PRD.md` §17 — per-unit DoD.
- `docs/gh-hound-design.html` all visual refs.

## Problem
Text tests cannot catch TUI layout failures. The hard gate requires shux snapshots at three breakpoints and a keyboard interaction audit for every screen before any screen is considered done.

## Scope
- Implement `make vqa` to run visual and interaction audits.
- Launch gh-hound against deterministic fake scenarios.
- Capture screenshots at `80x24`, `120x40`, `200x60`.
- Drive keybindings across every screen.
- Save text/state assertions for headers, footers, and route state.
- Add documented manual review checklist and artifact path.

## Non-Goals
- Do not make subjective visual comparison fully automatic unless practical; the agent must still inspect PNGs.

## Expected Artifacts
### Files to Create
- `.claude/automations/vqa.sh`.
- `.claude/automations/interaction_audit.sh`.
- `.claude/automations/assertions/*.json`.
- `.claude/automations/screenshots/.gitkeep`.
- `docs/development.md` VQA section if not already covered by README task.

### Files to Modify
- `Makefile` — real `vqa`, `vqa-screen`, and `vqa-clean` targets.

### Public Contracts
- VQA artifact layout under `.claude/automations/screenshots/<screen>/<breakpoint>.png`.
- Interaction audit exits non-zero on missing key behavior.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add `make vqa` test invocation that fails because harness/screenshots are absent.
- Add audit assertion for at least one known key before implementing the harness.

### Green: minimal implementation
- Implement harness with fake app mode and shux commands.

### Refactor: harden without changing behavior
- Make screen selection and breakpoint selection deterministic.
- Keep screenshots ignored unless deliberately committed as golden docs.

## Test Pyramid
### L0 Static / Architecture
- Shell lint for scripts where available.

### L1 Unit Tests
- Not primary.

### L2 Component / Adapter Tests
- Harness dry-run mode verifies command construction.

### L3 Integration Tests
- `make vqa SCREEN=runs` runs on local binary.

### L4 Visual / Interaction Tests
- This task implements L4 for every screen.

### L5 Live / Smoke Tests
- Optional live VQA can be a separate mode; default must use fake deterministic data.

## Definition of Done
- [ ] Red harness check fails first.
- [ ] `make vqa` captures screenshots for all v1 screens at all three breakpoints.
- [ ] Interaction audit drives every contextual keybinding.
- [ ] Footer hints are cross-checked against state/keymap.
- [ ] Agent visually confirms mock fidelity for all screenshots.
- [ ] `make check` and `make vqa` pass.

## Verification Commands
```bash
rtk make build
rtk make vqa
rtk make check
```

## Visual QA Checklist
- [ ] Alignment.
- [ ] Color mapping.
- [ ] No truncation/overflow.
- [ ] Selection fill and bar.
- [ ] Focus border.
- [ ] Footer equals keymap.
- [ ] Overlays layer cleanly.
- [ ] No tearing.
- [ ] Mock fidelity.

## Implementation Notes
- Use shux, not iTerm2, for this repo.
- If shux is missing, fail with an actionable install message, not a silent skip.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing VQA target/harness test.
2. Implement harness.
3. Run and inspect screenshots.
4. Commit and push.

## Commit Protocol
- Expected commit: `test(vqa): add shux visual audit harness`

