# Task 150: shux VQA harness and interaction audit

## Status
DONE

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
- [x] Red harness check fails first (`make vqa-screen SCREEN=runs` initially failed on missing/incorrect capture assertions).
- [x] `make vqa` captures screenshots for all v1 screens at all three breakpoints.
- [x] Interaction audit fixture covers welcome, global help, global palette, overlay pop, runs selection/filter, detail nav, failure actions, log search/fold, watch follow toggle, and dispatch input flows.
- [x] Footer hints are cross-checked by per-screen assertion JSON against the rendered deterministic frame.
- [x] Agent visually confirms mock fidelity for all screenshots via `.claude/automations/screenshots/contact-sheet.png` plus focused welcome/runs/failure screenshots.
- [x] `make check` and `make vqa` pass.

## Verification Commands
```bash
rtk make build
rtk make vqa
rtk make check
```

## Visual QA Checklist
- [x] Alignment.
- [x] Color mapping.
- [x] No truncation/overflow.
- [x] Selection fill and bar.
- [x] Focus border.
- [x] Footer equals keymap.
- [x] Overlays layer cleanly.
- [x] No tearing.
- [x] Mock fidelity.

## Completion Evidence
- Red: `rtk make vqa-screen SCREEN=runs` failed before the shux runner fix because required table/footer assertions were absent from the captured frame.
- Green: `rtk make vqa-screen SCREEN=runs` passed for `80x24`, `120x40`, and `200x60`.
- Full visual matrix: `rtk make vqa` passed for `welcome`, `all_green`, `runs`, `detail`, `failure`, `watch`, `log`, `dispatch`, `palette`, and `help` at all three breakpoints.
- Interaction matrix: `rtk ./.claude/automations/interaction_audit.sh` and `rtk make vqa` passed 11 shux post-key scenarios plus race-enabled TUI key tests.
- Visual inspection: generated `.claude/automations/screenshots/contact-sheet.png` and inspected the combined visual/interaction contact sheet plus focused screenshots for `welcome/120x40.png`, `runs/120x40.png`, and `failure/120x40.png`.
- Regression gate: `rtk make check` passed.

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
