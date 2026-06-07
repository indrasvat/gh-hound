# Task 160: agent surface, skill package, and JSON workflows

## Status
DONE

## Ownership Boundary
- **Primary area:** structured non-TUI workflows and agent integration docs.
- **Allowed files:** `internal/render`, CLI command wiring, skill docs/package, README snippets.
- **Avoid touching:** TUI layout except parity fixes found by tests.

## Depends On
- 030.
- 040.
- 050.
- 060.
- 135 for watch fail-fast.

## Parallelizable With
- 150, 170 after command contracts are stable.
- **Parallel contract:** owns pipe/agent behavior; TUI remains separate render surface.

## PRD / Design References
- `docs/gh-hound-PRD.md` §13 — dual surface and agent integration.
- `docs/gh-hound-PRD.md` Appendix B — JSON schema.
- `docs/gh-hound-PRD.md` §18 phase 10.

## Problem
Coding agents need a structured, stable interface with correct exit codes and failure excerpts. This must be first-class, not screen scraping.

## Scope
- Complete `runs --no-tui --json`, markdown, and XML output.
- Include failure object with job, step, exit code, annotations, and log excerpt.
- Implement exit codes for ok/failing/error/pending.
- Implement `--watch` fail-fast for non-TUI.
- Add Agent Skill documentation/package or repo-local handoff bundle ready to copy to skills repo.

## Non-Goals
- Do not implement v3 JSON-RPC/MCP server.

## Expected Artifacts
### Files to Create
- `docs/agent-surface.md`.
- `skills/gh-hound/SKILL.md` or equivalent handoff bundle if repo policy prefers `docs/`.
- JSON schema/golden fixtures.

### Files to Modify
- CLI commands and renderers.
- README command examples in Task 170 if needed.

### Public Contracts
- Stable JSON schema from Appendix B.
- Exit code behavior documented and tested.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add CLI integration tests for ok, failure, pending, and API error fake scenarios.
- Add `jq` schema/golden tests.
- Add `--watch` fail-fast tests.

### Green: minimal implementation
- Complete renderers and command behavior.

### Refactor: harden without changing behavior
- Keep same usecase core for TUI and pipe output.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Renderer and exit-code tests.

### L2 Component / Adapter Tests
- Fake adapter scenario tests.

### L3 Integration Tests
- Built binary piped through `jq`.

### L4 Visual / Interaction Tests
- Not required.

### L5 Live / Smoke Tests
- Optional live `--no-tui --json` against authenticated gh.

## Definition of Done
- [x] Red tests fail first (`--fake-scenario` was missing and focused CLI tests failed).
- [x] JSON schema matches Appendix B (`internal/render/testdata/schema.json`) and golden fixture is checked.
- [x] Exit codes are correct for green, failure, pending, and API-error fake scenarios.
- [x] `--watch` exits fail-fast with failure details.
- [x] Agent Skill/handoff exists and is discoverable at `skills/gh-hound/SKILL.md`.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./cmd/gh-hound ./internal/render ./internal/usecase
rtk make build
rtk './bin/gh-hound runs --no-tui --json --fake-scenario failure | jq .'
rtk make check
```

## Visual QA Checklist
- [x] Not applicable.

## Completion Evidence
- Red: `rtk go test -run 'TestAgentSurface|TestWatchFailFast|TestJSONFlag' ./cmd/gh-hound` failed on unknown `--fake-scenario`.
- Green: `rtk go test -race ./cmd/gh-hound ./internal/render ./internal/usecase` passed.
- Schema/golden: added `internal/render/testdata/schema.json` and `internal/render/testdata/failure.golden.json`.
- Agent handoff: added `docs/agent-surface.md` and `skills/gh-hound/SKILL.md`.
- Smoke: `rtk make build` plus `jq` checks passed for green, failure, pending, API error exit `2`, and `watch --json --fake-scenario failure` exit `1`.
- Regression gate: `rtk make check` passed.

## Implementation Notes
- Do not leak credentials, headers, or raw token-bearing URLs in JSON/log output.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing agent-surface tests.
2. Implement.
3. Verify.
4. Commit and push.

## Commit Protocol
- Expected commit: `feat(agent): add structured ci surface`
