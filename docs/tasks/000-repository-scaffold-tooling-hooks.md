# Task 000: repository scaffold, tooling, hooks, and modern Go guardrails

## Status
Done

## Ownership Boundary
- **Primary area:** repository root, build tooling, developer guardrails.
- **Allowed files:** `go.mod`, `go.sum`, `cmd/gh-hound/`, `main.go` if used, `Makefile`, `lefthook.yml`, `.golangci.yml`, `.gitignore`, `.claude/`, `scripts/`, `internal/` package stubs, `.github/` placeholders only when needed for local gates.
- **Avoid touching:** screen implementations beyond compile-safe stubs; GitHub API behavior beyond interfaces/fakes.

## Depends On
- None.

## Parallelizable With
- None. This is the root dependency for all implementation tasks.
- **Parallel contract:** complete and merge this before any feature branch changes code structure.

## PRD / Design References
- `docs/gh-hound-PRD.md` §2 — architecture and package layout.
- `docs/gh-hound-PRD.md` §3 — Go 1.26, Charm v2, CLI conventions, Makefile targets.
- `docs/gh-hound-PRD.md` §9.0.1 — version banner contract.
- `docs/gh-hound-PRD.md` §15 — gh extension distribution constraints.
- `docs/gh-hound-PRD.md` §18 phase 1 — scaffold done criteria.

## Problem
The repository only contains docs and assets. Every later task needs a compilable Go module, a gh-extension entrypoint, consistent Makefile targets, lefthook hooks, `go fix` checks, linting, tests, and release metadata injection from day one.

## Scope
- Initialize `module github.com/indrasvat/gh-hound` with `go 1.26`.
- Create the binary entrypoint that builds as `gh-hound` and runs as `gh hound`.
- Add compile-safe package stubs for `adapter`, `config`, `layout`, `model`, `render`, `theme`, `tui`, and `usecase`.
- Add an awesome colorful Makefile inspired by `nidhi`, `vivecaka`, and `shux`.
- Configure lefthook early so local development is grounded by the same gates agents must pass.
- Configure `go fix` as both an apply target and a check target.
- Add minimal `--help` and `--version` behavior with build metadata wired through ldflags.

## Non-Goals
- Do not implement real GitHub API calls.
- Do not implement full TUI screens.
- Do not publish a release.

## Expected Artifacts
### Files to Create
- `go.mod` / `go.sum` — Go 1.26 module and dependencies.
- `cmd/gh-hound/main.go` — binary entrypoint.
- `internal/...` package stubs — compile-safe architecture seams.
- `Makefile` — grouped colorful help and all standard targets.
- `lefthook.yml` — pre-commit and pre-push gates.
- `.golangci.yml` — golangci-lint v2 schema.
- `.claude/settings.json` — optional local guardrails for commit/test reminders.
- `scripts/gofix-check.sh` — fails if `go fix ./...` would alter the tree.
- `scripts/build-release.sh` — ldflags-aware build script skeleton for gh-extension precompile.
- `scripts/smoke-test.sh` — local binary smoke skeleton.

### Files to Modify
- `.gitignore` — Go, coverage, build, dist, VQA, and local artifact ignores.
- `README.md` — can remain minimal here; full rewrite is Task 170.

### Public Contracts
- CLI flags/env: `--help`, `--version`, `--log-level`, `--trace-http`, `--no-tui`, `--format`, `-R/--repo`.
- Make targets: `help`, `build`, `install`, `run`, `fmt`, `fmt-check`, `gofix`, `gofix-check`, `lint`, `test`, `coverage`, `coverage-check`, `check`, `ci`, `e2e`, `vqa`, `demo`, `smoke-test`, `release-check`, `snapshot`, `release-prep`, `tools`, `tools-ci`, `hooks`, `hooks-run`, `clean`.
- Hook commands must call Makefile targets, not duplicate raw commands.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add a CLI smoke test that expects `gh-hound --version` to include version, commit, date, and tagline.
- Run `make ci` and confirm it fails because the scaffold is absent.
- Run `make gofix-check` and confirm the target is absent or failing before implementation.

### Green: minimal implementation
- Add module, entrypoint, Makefile, hooks, lint config, scripts, and package stubs.
- Make `go build ./...`, `make gofix-check`, `make check`, and `lefthook run pre-commit` pass.

### Refactor: harden without changing behavior
- Remove duplicated shell fragments by routing all checks through Makefile targets.
- Ensure scripts are POSIX-safe, executable, and shellcheck-clean if shellcheck is available.

## Test Pyramid
### L0 Static / Architecture
- `make fmt-check`, `make gofix-check`, `make lint`.
- `go list` dependency check placeholder exists for future architecture enforcement.

### L1 Unit Tests
- CLI version/help tests.
- Build metadata formatting tests if split into a helper.

### L2 Component / Adapter Tests
- Not required beyond package compile stubs.

### L3 Integration Tests
- `go build ./...`.
- `./bin/gh-hound --version`.
- `./bin/gh-hound --help`.

### L4 Visual / Interaction Tests
- `make vqa` target exists and fails with a clear "harness not implemented yet" only until Task 150; after Task 150 it must run real checks.

### L5 Live / Smoke Tests
- `make smoke-test` verifies local build, `--version`, `--help`, and clean non-interactive exit behavior.

## Definition of Done
- [x] Red CLI/tooling tests fail for the intended missing behavior first.
- [x] `go build ./...` passes.
- [x] `make help` renders grouped colorful output.
- [x] `make gofix-check` passes.
- [x] `make check` passes.
- [x] `lefthook install` succeeds.
- [x] `lefthook run pre-commit` succeeds.
- [x] `lefthook run pre-push` succeeds.
- [x] `gh-hound --version` prints banner/build metadata/tagline.
- [x] Hooks and CI paths call Makefile targets.

## Verification Commands
```bash
rtk make help
rtk make gofix-check
rtk make check
rtk make smoke-test
rtk lefthook install
rtk lefthook run pre-commit
rtk lefthook run pre-push
```

## Visual QA Checklist
- [x] `make help` is readable, grouped, and colorized.
- [x] `--version` banner degrades without emoji or broken glyphs.

## Implementation Notes
- Use `github.com/spf13/cobra` with `SilenceUsage` and `SilenceErrors` configured sanely.
- Use `log/slog` and XDG-compatible state paths, but defer full logging behavior to Task 030.
- `go fix` must run before `go mod tidy` in the apply target.
- `go fix` check should copy or diff the worktree safely and fail if changes are needed.
- Completion evidence: focused version test, `make check`, `make ci`, `make smoke-test`, `lefthook run pre-commit`, and `lefthook run pre-push` passed before commit/push.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Create the red tests first and prove they fail.
3. Implement the scaffold.
4. Run focused tests, then all verification commands above.
5. Commit once green.
6. Push the branch immediately after the commit.

## Commit Protocol
- Expected commit: `chore(scaffold): add project tooling and guardrails`
- Commit early once this foundation is green; do not mix feature logic into this commit.
