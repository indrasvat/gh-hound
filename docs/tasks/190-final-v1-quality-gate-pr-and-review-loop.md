# Task 190: final v1 quality gate, PR, and review loop

## Status
DONE

## Ownership Boundary
- **Primary area:** final integration, verification evidence, PR creation, review loop.
- **Allowed files:** only fixes required by failed gates or review feedback.
- **Avoid touching:** speculative features and v2 roadmap items.

## Depends On
- 000.
- 010.
- 020.
- 030.
- 040.
- 050.
- 060.
- 070.
- 080.
- 090.
- 100.
- 110.
- 120.
- 130.
- 135.
- 140.
- 150.
- 160.
- 170.
- 180.

## Parallelizable With
- None. This is the final gate.
- **Parallel contract:** all feature tasks must be complete first.

## PRD / Design References
- `docs/gh-hound-PRD.md` §17 — per-unit and product/v1 DoD.
- `docs/gh-hound-PRD.md` §18 — phase completion.
- `docs/gh-hound-design.html` all screens.

## Problem
The product is not done until all local gates, visual gates, smoke checks, docs, CI expectations, and PR review loops are complete. This task prevents calling a partial implementation done.

## Scope
- Run complete local verification.
- Fix any integration regressions.
- Push final branch state.
- Create a brief, relevant, terse, focused one-page PR.
- Enter review-monitor loop and address actionable feedback.
- Keep PR ready-for-review unless a real blocker requires draft.

## Non-Goals
- Do not add v2/v3 features.
- Do not widen scope beyond PRD v1.

## Expected Artifacts
### Files to Create
- PR description only, unless verification creates final evidence files intended for docs.

### Files to Modify
- Any code/docs needed to pass gates or review.

### Public Contracts
- PR states what changed, how it was tested, and known limitations.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Treat any failing gate as red and fix it before PR.

### Green: minimal implementation
- Bring all gates green.

### Refactor: harden without changing behavior
- Only small cleanup justified by failed checks or review feedback.

## Test Pyramid
### L0 Static / Architecture
- `make gofix-check`, `make fmt-check`, `make lint`, `make arch-check`, `make emoji-check`.

### L1 Unit Tests
- `make test`.

### L2 Component / Adapter Tests
- Adapter, TUI component, config, render tests.

### L3 Integration Tests
- `make e2e`.

### L4 Visual / Interaction Tests
- `make vqa`; inspect screenshots.

### L5 Live / Smoke Tests
- `make smoke-test`.
- Optional release/install smoke only after tag/release exists.

## Definition of Done
- [x] Every task file is complete or explicitly superseded with reason.
- [x] `make ci` passes.
- [x] `make e2e` passes.
- [x] `make vqa` passes and screenshots are inspected.
- [x] `make release-prep` passes or documents any external-only release gate.
- [x] Branch pushed.
- [x] PR created with focused one-page body.
- [x] Review loop monitored.
- [x] Actionable review comments addressed with tests.
- [x] CI green.

## Verification Commands
```bash
rtk make gofix-check
rtk make ci
rtk make e2e
rtk make vqa
rtk make release-prep
rtk git status --short
```

## Visual QA Checklist
- [x] All screen screenshots inspected against HTML mock.
- [x] No overlap, truncation, color bleed, broken focus, or stale footer hints.

## Completion Evidence
- Task status: every task file from `000` through `190` is now marked done.
- Final gate: `rtk make gofix-check && rtk make ci && rtk make e2e && rtk make vqa && rtk make release-prep && rtk git status --short` passed.
- Visual: regenerated `.claude/automations/screenshots/contact-sheet.png` from latest shux captures and inspected the combined screen/interaction sheet.
- Release prep: `make release-prep` passed including CI, e2e, docs, VQA, smoke, release-check, and snapshot.
- Branch: `create-gh-hound` pushed to origin.
- PR/review: PR created and review loop monitored; no actionable review comments were present at completion.

## Implementation Notes
- PR body should be concise: summary, test evidence, risk/rollback, links to VQA artifacts if useful.
- Use `gh ghent status --pr <N> --await-review --solo --logs --format json --no-tui` if available for review monitoring.

## Session Protocol
1. Re-read every relevant PRD section and HTML mock, then inspect the latest VQA screenshots against the mock before final fixes.
2. Run all gates.
2. Fix failures.
3. Push.
4. Create PR.
5. Monitor review/CI.
6. Address feedback until ready.

## Commit Protocol
- Expected commit(s): `fix(...)` or `test(...)` only as needed for final gate failures.
