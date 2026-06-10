# Task 200: artifacts — list and download on TUI and pipe surface

## Status
DONE

## Ownership Boundary
- **Primary area:** artifacts listing/download across usecase, adapter, fake, render, cmd, and TUI detail screen.
- **Allowed files:** `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/model/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/` (detail screen, keymap, palette, help), `docs/`, `skill/SKILL.md`, `README.md`, `pages/` (landing mention), vqa harness.
- **Avoid touching:** caches/runners administration (stays v2), deployment approvals, watch/dispatch internals beyond keymap/help registration.

## Depends On
- 040 (github adapter), 060 (actions usecases), 110 (run detail), 160 (agent surface).

## Parallelizable With
- None within this branch.

## PRD / Design References
- `docs/gh-hound-PRD.md` §18 — artifacts pulled forward from the v2 bucket by explicit decision (2026-06-10): it is the only v2 item that is a daily-loop action.
- GitHub REST (verified live 2026-06-10 against indrasvat/gh-hound and openclaw/openclaw):
  - `GET /repos/{owner}/{repo}/actions/runs/{run_id}/artifacts` — params `per_page` (max 100), `page`, `name`; returns `total_count` + `artifacts[]` with `id`, `name`, `size_in_bytes`, `expired`, `expires_at`, `created_at`, `updated_at`, `digest`, `workflow_run{head_branch, head_sha}`.
  - `GET /repos/{owner}/{repo}/actions/artifacts/{artifact_id}/zip` — 302 to a short-lived signed URL (≈1 min), 410 Gone when expired. The signed URL must never be logged, rendered, or emitted.

## Problem
Downloading a test-report or build artifact after a CI run is a daily-loop action that today forces a browser trip or `gh run download`. gh-hound covers triage but not artifact retrieval — the one daily-loop gap vs the Actions web UI.

## Scope
- `ArtifactsService` usecase: list artifacts for a run (paginated to completion), download one artifact with `gh run download`-style extraction.
- GitHub adapter: `ListArtifacts` (paginate, per_page=100) and `DownloadArtifact` (follow 302, stream zip to temp file, extract into `<dir>/<artifact-name>/`, zip-slip safe, 410 → typed expired error).
- Fake adapter: artifact fixtures for green/failing scenarios including one expired artifact.
- Pipe surface: `gh hound artifacts --run <id> --no-tui --json` (list) and `--download <name> [--dir <path>]`; `runs[].artifacts[]` metadata included ONLY when `--artifacts` is passed to `runs` (opt-in keeps the default path at zero extra API calls).
- TUI: artifacts section in run detail (name, human size, expiry/expired badge), `a` focuses artifacts / opens artifact list state, `Enter`/`d` downloads selected artifact with confirm + toast (success shows destination path; failure shows typed reason), palette entries, help + footer registration.
- Docs: agent-surface.md, SKILL.md, README, configuration if a default download dir option is added; landing page mention if warranted.

## Non-Goals
- No caches or runner administration. No artifact deletion. No multi-artifact bulk download in v1 of this feature. No artifact content preview in the TUI.

## Expected Artifacts
### Files to Create
- `internal/usecase/artifacts.go` + `_test.go`
- `internal/adapter/github/artifacts.go` + `_test.go`
- `internal/tui/screens/detail/` artifacts state additions (or new component file) + tests

### Files to Modify
- `internal/usecase/ports.go` (GitHub interface: ListArtifacts, DownloadArtifact)
- `internal/model/actions.go` (Artifact model)
- `internal/adapter/fake/fake.go`
- `internal/render/render.go` + `testdata/schema.json` (artifacts array)
- `cmd/gh-hound/main.go` (artifacts command, --artifacts flag)
- TUI keymap/palette/help, `docs/visual-contract.md`, vqa harness captures
- `docs/agent-surface.md`, `skill/SKILL.md`, `README.md`

### Public Contracts
- JSON artifact object: `{id, name, size_in_bytes, expired, expires_at, created_at, digest}` — no archive URLs (signed-URL guardrail).
- `artifacts` command exit codes: `0` success, `2` API/config error (incl. expired artifact on download), `3` never (no pending concept).
- Download destination: `<dir>/<artifact-name>/` (extracted), `--dir` defaults to cwd. Existing destination directory is an error unless `--force`.

## Red / Green / Refactor Plan
### Red: prove the missing behavior
- Usecase tests: list paginates past one page; download extracts nested paths; zip-slip entry rejected; expired artifact → typed error; fake-backed.
- Adapter tests (httptest): pagination loop, 302 follow without auth header leak to non-GitHub host, 410 mapping.
- CLI tests: `artifacts --json` shape + exit codes; `runs --artifacts` includes metadata; default `runs` makes zero artifact API calls.
- TUI tests: detail renders artifacts section from fake; keymap/footer/help register `a`/`d`; download confirm + toast states.

### Green: minimal implementation
- Smallest code to turn each red test green, layer by layer (model → port → adapter → usecase → render/cmd → TUI).

### Refactor: harden without changing behavior
- Only after green: extraction helpers, shared human-size formatting, dedupe with existing detail-screen patterns.

## Test Pyramid
- L0: `make gofix-check fmt-check lint arch-check emoji-check`.
- L1: `make test` (race) — new unit tests above.
- L2: adapter httptest suites; render schema validation against updated schema.json.
- L3: `make e2e`.
- L4: `make vqa` with new detail-artifacts captures at 80x24/120x40/200x60; cold-context tui-qa agent audit returns `VERDICT: PASS`.
- L5: live verification against real repos (indrasvat/gh-hound release run artifacts; openclaw/openclaw stress) — list + download + extraction verified through shux-driven sessions.

## Performance Budget (hard gates)
- Default `gh hound runs` path: zero additional API calls vs v0.1.1 (verified by fake call-count test).
- `artifacts` list on a 100+ artifact run: single-digit API pages, < 2s wall on warm network.
- TUI detail open on openclaw/openclaw heavy run: artifacts fetch is async, never blocks first paint; scroll stays responsive.
- Download: streams to disk (no full-zip buffering in memory); 100MB artifact must not balloon RSS.
- Benchmark evidence (before/after `hyperfine` or timed runs against gh-hound + openclaw/openclaw) included in PR body.

## Definition of Done
- [x] All red tests written first and observed failing; then green; race-enabled suite passes.
- [x] `make ci`, `make e2e`, `make vqa`, `make docs-check` all pass.
- [x] Pipe surface: `artifacts` command + `runs --artifacts` shipped, schema.json updated, exit codes documented in agent-surface.md.
- [x] TUI: artifacts visible in detail, download flow with confirm + success/failure toasts, keymap/palette/help/footer all updated and truthful.
- [x] Zero extra API calls on default runs path (call-count test green).
- [x] Live download verified: real artifact from a real run extracts correctly, content checksum matches `digest` where available.
- [x] openclaw/openclaw stress run: listing + TUI responsiveness verified, timings recorded.
- [x] Benchmark results in PR body; no perf regression vs v0.1.1 baseline.
- [x] tui-qa cold-context audit: `VERDICT: PASS` with screenshot evidence at all three breakpoints.
- [x] Dootsabha review rounds (codex + gemini) converged: no remaining blocking findings.
- [x] README, SKILL.md, agent-surface.md updated; landing page updated + wrangler-deployed + verified if feature is mentioned there.
- [x] Branch pushed only after all of the above; PR created with evidence.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound artifacts --run <run-id> -R indrasvat/gh-hound --no-tui --json
./bin/gh-hound artifacts --run <run-id> --download <name> --dir /tmp/x --no-tui --json
./bin/gh-hound runs --artifacts --no-tui --json -R openclaw/openclaw --all
```

## Visual QA Checklist
- [x] Detail artifacts section at 80x24/120x40/200x60: no overlap, truncation handles long artifact names, expired badge styled per theme.
- [x] Confirm + toast layering over base view; no color bleed; footer/help truth after focus changes.

## Completion Evidence
- TDD: every layer red-first; final suite 235+ tests, race-enabled, green.
- tui-qa cold-context audits: 4 rounds (FAIL, FAIL, FAIL, PASS) — 80+ pixel-inspected screenshots across fixture/fake/live lenses incl. openclaw; final round PASS with /running fix verified live.
- Dootsabha: codex BLOCKED -> BLOCKED -> CONVERGED across 3 rounds; gemini round-1 findings addressed, final consult CONVERGED. 12 findings fixed incl. '.'-name RemoveAll escape, zip-bomb budget, toast TTL dead code, palette dead path.
- Live: real artifact downloads verified on indrasvat/gh-hound (coverage, 28KB and 250KB) and openclaw 25MB dist-runtime-build (6.5s, 38.9MB peak RSS, streamed).
- Benchmarks: default runs path +0.8% vs v0.1.1 interleaved A/B (noise); zero extra API calls pinned by test.
- Pre-existing scale bugs fixed in passing: server-tagged filter 0-matches, pagination HasMore latch, esc-clear, running vocabulary. Follow-ups filed: #11 (SIGWINCH), #12 (filtered column alignment), #13 (palette dispatch toast).

## Accepted Gaps
- `docs/gh-hound-design.html` (the static HTML mock) does not include the artifacts block; `docs/visual-contract.md` ③ is the authoritative visual contract for this feature. Regenerating the large static mock is deferred.
- Terminal resize (SIGWINCH) is unhandled app-wide; pre-existing on main, out of scope here (QA finding P2-3, non-gating).

## Session Protocol
1. Re-read PRD detail-screen contract and visual contract before TUI edits.
2. Red tests → green → refactor, committing per layer.
3. shux + tui-qa audits before push; dootsabha rounds; push; PR.
