# Task 170: README, user docs, demo, and development guide

## Status
TODO

## Ownership Boundary
- **Primary area:** user-facing documentation and demo assets.
- **Allowed files:** `README.md`, `docs/development.md`, `docs/configuration.md`, `docs/agent-surface.md`, `assets/`, `vhs` tapes.
- **Avoid touching:** product code except command output fixes discovered by docs verification.

## Depends On
- 000.
- 100.
- 110.
- 120.
- 130.
- 135.
- 140.
- 160.

## Parallelizable With
- 180 after CLI and screen contracts are stable.
- **Parallel contract:** own docs and examples; code changes only to correct inaccurate docs.

## PRD / Design References
- `docs/gh-hound-PRD.md` §1 — executive summary and wedge.
- `docs/gh-hound-PRD.md` §13 — agent surface.
- `docs/gh-hound-PRD.md` §15 — install/distribution.
- `docs/gh-hound-design.html` visual language.
- Reference README layout from `nidhi`, `vivecaka`, and `shux`.

## Problem
README is currently a placeholder. The finished project needs a polished, accurate one-pager-style front door with install, usage, controls, architecture, development, and agent-surface details matching the quality of the referenced repos.

## Scope
- Rewrite README with centered logo/title, badges, nav, overview, why, features, install, quick start, controls, config, JSON/agent surface, architecture, development, and roadmap.
- Add demo GIF workflow via VHS or documented placeholder until demo is generated.
- Add development guide covering Makefile targets, hooks, TDD, VQA, and release prep.
- Add configuration docs.
- Verify every command in docs.

## Non-Goals
- Do not invent features not shipped.
- Do not claim live log streaming beyond PRD's honest mechanism.

## Expected Artifacts
### Files to Create
- `docs/development.md`.
- `docs/configuration.md`.
- `assets/demo.tape` or `docs/demo.tape`.

### Files to Modify
- `README.md`.
- `Makefile` demo target if needed.

### Public Contracts
- README commands are runnable.
- Badges point to correct repo/workflows.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add a docs command verification script or markdown link/check target that fails on placeholder README and broken commands.

### Green: minimal implementation
- Rewrite docs and add verification target.

### Refactor: harden without changing behavior
- Keep docs concise but complete; remove duplicate stale details.

## Test Pyramid
### L0 Static / Architecture
- Markdown lint/link check where available.

### L1 Unit Tests
- Not applicable.

### L2 Component / Adapter Tests
- Docs command verifier.

### L3 Integration Tests
- README quick-start commands run against fake scenario.

### L4 Visual / Interaction Tests
- Demo/VQA screenshots can be embedded after Task 150.

### L5 Live / Smoke Tests
- Install command documented; actual release install verified in Task 180.

## Definition of Done
- [ ] Red docs check fails first.
- [ ] README matches referenced repos' completeness and polish.
- [ ] Install, quick start, controls, config, architecture, dev, and agent sections exist.
- [ ] Commands are verified.
- [ ] Demo target is documented and works or has a tracked follow-up blocked only on release assets.
- [ ] `make check` passes.

## Verification Commands
```bash
rtk make docs-check
rtk make demo
rtk make check
```

## Visual QA Checklist
- [ ] README logo renders.
- [ ] Screenshots/GIFs are real product output when available.

## Implementation Notes
- Keep claims aligned with shipped implementation, not roadmap.
- Mention `gh extension install indrasvat/gh-hound` prominently once release task is complete.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing docs check.
2. Write docs.
3. Verify commands.
4. Commit and push.

## Commit Protocol
- Expected commit: `docs(readme): document gh-hound workflows`

