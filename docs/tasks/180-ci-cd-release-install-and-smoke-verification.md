# Task 180: CI/CD, release, install, and smoke verification

## Status
TODO

## Ownership Boundary
- **Primary area:** GitHub Actions, release config, install script, smoke tests.
- **Allowed files:** `.github/`, `.goreleaser.yml` if used, `scripts/build-release.sh`, `install.sh`, `scripts/smoke-test.sh`, `Makefile`.
- **Avoid touching:** feature code except version/build metadata fixes.

## Depends On
- 000.
- 030.
- 150.
- 160.
- 170.

## Parallelizable With
- 190 only after release config shape is stable.
- **Parallel contract:** own CI/release mechanics; product behavior stays unchanged.

## PRD / Design References
- `docs/gh-hound-PRD.md` §3.3 — Makefile targets.
- `docs/gh-hound-PRD.md` §15 — distribution and gh-extension precompile.
- `docs/gh-hound-PRD.md` §18 phase 10.
- Reference CI/CD from `nidhi`, `vivecaka`, and gh-extension conventions from `gh-ghent`.

## Problem
The project must be installable as a `gh` extension and verified by CI before merge. Release config must be correct before tagging, and smoke tests must prove downloaded artifacts work.

## Scope
- Add `ci.yml` running `make ci`, coverage upload, build verification, and artifacts.
- Add `release.yml` using gh-extension precompile mechanics with `scripts/build-release.sh`.
- Add dependabot config if appropriate.
- Add `install.sh` for non-gh-extension installs if PRD requires it.
- Add `make release-check`, `make snapshot`, and `make release-prep`.
- Verify gh-extension topic instructions.

## Non-Goals
- Do not push tags unless explicitly requested during release.

## Expected Artifacts
### Files to Create
- `.github/workflows/ci.yml`.
- `.github/workflows/release.yml`.
- `.github/dependabot.yml`.
- `install.sh`.
- Release/smoke scripts and tests.

### Files to Modify
- `Makefile`.
- `README.md` install section if needed.

### Public Contracts
- CI runs same gates as local `make ci`.
- Release injects version/commit/date into `--version`.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add release-check target that fails before config exists.
- Add install/smoke tests that fail before scripts exist.

### Green: minimal implementation
- Implement workflows, release scripts, installer, and smoke tests.

### Refactor: harden without changing behavior
- Pin action versions where the referenced repo pattern does.
- Keep CI no-fallback: if tool install fails, CI fails.

## Test Pyramid
### L0 Static / Architecture
- Workflow syntax check with actionlint if available.
- Shellcheck scripts if available.

### L1 Unit Tests
- Installer platform detection tests if script supports test mode.

### L2 Component / Adapter Tests
- `scripts/build-release.sh` local dry run.

### L3 Integration Tests
- `make snapshot` or equivalent release dry-run.

### L4 Visual / Interaction Tests
- `make vqa` included in release-prep if runtime allows; otherwise documented as local gate.

### L5 Live / Smoke Tests
- `make smoke-test`.
- After an actual release, verify `gh extension install indrasvat/gh-hound` on macOS/Linux and downloaded artifact metadata.

## Definition of Done
- [ ] Red release/smoke checks fail first.
- [ ] `ci.yml` mirrors `make ci`.
- [ ] Release workflow uses gh-extension precompile with build script override.
- [ ] `install.sh` verifies checksums and macOS quarantine behavior if present.
- [ ] `make release-check`, `make snapshot`, and `make smoke-test` pass.
- [ ] README install docs are accurate.

## Verification Commands
```bash
rtk make release-check
rtk make snapshot
rtk make smoke-test
rtk make ci
```

## Visual QA Checklist
- [ ] Not applicable.

## Implementation Notes
- Do not release with stale owner/module/binary names.
- Verify assets and downloaded binary together after any real tag.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing release/smoke checks.
2. Implement workflows/scripts.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `ci(release): add gh extension release pipeline`

