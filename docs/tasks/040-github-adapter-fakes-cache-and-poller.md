# Task 040: GitHub adapter, fakes, cache, request queue, and poller

## Status
Done

## Ownership Boundary
- **Primary area:** `internal/adapter`, `internal/usecase` data-loading orchestration.
- **Allowed files:** adapter port implementation, fake adapter, cache, poller, fixtures, usecase load methods.
- **Avoid touching:** TUI screen rendering and release workflows.

## Depends On
- 010.
- 030 for pipe integration hooks where useful.

## Parallelizable With
- 020, 050 after model contracts are stable.
- **Parallel contract:** expose typed usecases but do not own TUI presentation.

## PRD / Design References
- `docs/gh-hound-PRD.md` §4 — GitHub API integration.
- `docs/gh-hound-PRD.md` §11 — performance and rate limits.
- `docs/gh-hound-PRD.md` §20 — API caveats.
- `docs/gh-hound-design.html` section 04 and 05.

## Problem
The app must be grounded in real GitHub Actions endpoints while remaining fakeable and deterministic for tests. It needs ETag caching, serial requests, mutation spacing, and adaptive polling before screens can reliably render live CI.

## Scope
- Implement go-gh REST client wrapper.
- Resolve repo through `GH_REPO`, `-R`, and current repository hooks.
- Implement read endpoints for runs, workflows, jobs, single run/job, and annotations.
- Implement ETag/304 cache with trace metadata.
- Implement one serial request queue.
- Implement adaptive poller state and tests.
- Implement fake adapter with deterministic scenarios for all screen tasks.

## Non-Goals
- Do not implement log de-noising; Task 050 owns it.
- Do not implement mutations beyond interface stubs; Task 060 owns them.

## Expected Artifacts
### Files to Create
- `internal/adapter/github/client.go`.
- `internal/adapter/github/runs.go`, `jobs.go`, `workflows.go`, `annotations.go`.
- `internal/adapter/github/cache.go`, `queue.go`, `poller.go`.
- `internal/adapter/fake/fake.go`.
- `internal/adapter/github/testdata/*.json`.
- `internal/usecase/runs.go`.

### Files to Modify
- `internal/usecase/ports.go`.
- `Makefile` if adapter-focused target is useful.

### Public Contracts
- `GitHub` port read methods return typed models and cache metadata.
- Trace HTTP emits method, path, status, duration, ETag, remaining rate.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Add fixture tests for each REST response shape.
- Add `304 Not Modified` test proving cached value is reused.
- Add serial queue test proving requests do not run concurrently.
- Add poller tests for fast interval while in progress and slow/backoff when idle.

### Green: minimal implementation
- Implement fixture-backed decode, cache, queue, and poller.
- Add go-gh client path with tests via `httptest` or injectable HTTP transport.

### Refactor: harden without changing behavior
- Separate transport, decode, and usecase orchestration.
- Keep rate-limit/backoff logic testable without sleeping.

## Test Pyramid
### L0 Static / Architecture
- `make arch-check`.

### L1 Unit Tests
- Cache, queue, poller, enum decode.

### L2 Component / Adapter Tests
- HTTP fixture tests for every read endpoint.
- Fake adapter scenario tests.

### L3 Integration Tests
- `runs --no-tui --json` uses fake adapter and emits expected data.

### L4 Visual / Interaction Tests
- Not required.

### L5 Live / Smoke Tests
- Optional live smoke behind `HOUND_LIVE_TEST=1`; never required in CI.

## Definition of Done
- [x] Red tests fail first.
- [x] All read endpoints decode typed models.
- [x] ETag/304 behavior is verified.
- [x] Serial queue prevents concurrent API calls.
- [x] Adaptive poller is deterministic and testable.
- [x] Fake adapter supports green, failing, running, empty, rate-limited, network-error scenarios.
- [x] `make check` passes.

## Verification Commands
```bash
rtk go test -race ./internal/adapter/... ./internal/usecase/...
rtk make check
```

## Visual QA Checklist
- [x] Not applicable.

## Implementation Notes
- Use REST as the backbone. GraphQL is optional and must not block v1.
- Never prompt for or store tokens.
- Follow 302s through the client; do not reconstruct redirect URLs.

## Session Protocol
1. Re-read this task, the referenced PRD sections, and the relevant `docs/gh-hound-design.html` mock immediately before editing.
2. Add failing fixture/cache/queue/poller tests.
3. Implement adapter and fake.
4. Verify.
5. Commit and push.

## Commit Protocol
- Expected commit: `feat(adapter): add github actions data layer`

## Completion Evidence
- Grounding: re-read PRD §4/§11/§20 and HTML sections 04/05; checked current GitHub REST Actions workflow-runs and workflow-jobs docs.
- Red: `rtk go test -race ./internal/adapter/... ./internal/usecase/...` failed on missing `NewClient`, `APIVersion`, queue/poller, fake scenarios, and `RunsService`.
- Focused tests: `rtk go test -race ./internal/adapter/... ./internal/usecase/...`.
- Full gate: `rtk make check`.
