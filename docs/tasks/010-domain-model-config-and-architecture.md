# Task 010: domain model, config, and architecture contracts

## Status
Done

## Ownership Boundary
- **Primary area:** `internal/model`, `internal/config`, architecture checks.
- **Allowed files:** `internal/model/`, `internal/config/`, `internal/usecase/ports.go`, `internal/adapter/port.go`, `scripts/check-architecture.sh`, `Makefile` target wiring.
- **Avoid touching:** TUI screens, real HTTP adapter, release workflows.

## Depends On
- 000.

## Parallelizable With
- 020, 030 after shared package names from 000 exist.
- **Parallel contract:** do not change keymap/theme APIs owned by 020.

## PRD / Design References
- `docs/gh-hound-PRD.md` §2 — hexagonal core.
- `docs/gh-hound-PRD.md` §4.3 — exact status and conclusion enums.
- `docs/gh-hound-PRD.md` §14 — config precedence and TOML schema.
- `docs/gh-hound-PRD.md` Appendix A — data model.

## Problem
All future behavior depends on stable typed domain objects, config loading, fakeable ports, and enforced dependency direction. Without these contracts, parallel tasks will drift and adapters/TUI will couple incorrectly.

## Scope
- Implement exact `Run`, `Job`, `Step`, and `Annotation` models.
- Implement status/conclusion enum parsing, validation, and semantic helpers.
- Implement config defaults, TOML loading, env/flag merge hooks, and validation.
- Define the GitHub port interface consumed by usecases.
- Add an architecture check forbidding `adapter`/`usecase` from importing `tui`.

## Non-Goals
- Do not implement go-gh HTTP calls.
- Do not implement TUI rendering.

## Expected Artifacts
### Files to Create
- `internal/model/actions.go` — PRD Appendix A structs and enums.
- `internal/model/actions_test.go` — enum and helper tests.
- `internal/config/config.go` — defaults, load, merge, validate.
- `internal/config/config_test.go` — TOML/env/override tests.
- `internal/usecase/ports.go` — usecase-facing interfaces.
- `scripts/check-architecture.sh` — dependency direction gate.

### Files to Modify
- `Makefile` — add `arch-check` and include it in `check`.

### Public Contracts
- Go interfaces/types: stable model structs and `GitHub` port.
- Config keys: `default_scope`, `auto_watch`, `per_page`, `theme`, `poll_min_ms`, `poll_max_ms`, `[keybindings]`.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add tests for every enum value in §4.3 and invalid values.
- Add config tests for missing config, invalid TOML, env override, flag override, and keybinding conflict.
- Add architecture test/check that fails if a forbidden import exists.

### Green: minimal implementation
- Implement model enums, config defaults, TOML decode, precedence merge, and architecture script.

### Refactor: harden without changing behavior
- Keep validation errors structured and user-facing.
- Keep env parsing centralized and covered.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.
- `go vet ./...`.

### L1 Unit Tests
- Model enum parsing and JSON marshal behavior.
- Config defaults, invalid input, env and flag precedence.

### L2 Component / Adapter Tests
- Port fake compiles against the interface.

### L3 Integration Tests
- CLI can load config path and print clear error for invalid TOML.

### L4 Visual / Interaction Tests
- Not required.

### L5 Live / Smoke Tests
- Not required.

## Definition of Done
- [x] Red tests fail first.
- [x] Model matches Appendix A.
- [x] Exact enum values accepted; invented values rejected.
- [x] Config precedence is built-in defaults -> file -> env -> flags.
- [x] Keybinding conflicts can be represented for Task 020 validation.
- [x] Architecture check is part of `make check`.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/model ./internal/config ./internal/usecase
rtk make arch-check
rtk make check
```

## Visual QA Checklist
- [x] Not applicable.

## Implementation Notes
- Use `BurntSushi/toml` or the dependency chosen in 000.
- Avoid global mutable config.
- Keep config test fixtures in `internal/config/testdata/`.
- Completion evidence: red model/config tests failed on missing symbols first; focused `go test -race ./internal/model ./internal/config ./internal/usecase` passed; `make check` passed.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Write failing model/config/architecture tests.
3. Implement.
4. Run verification commands.
5. Commit and push.

## Commit Protocol
- Expected commit: `feat(core): add domain model and config contracts`
