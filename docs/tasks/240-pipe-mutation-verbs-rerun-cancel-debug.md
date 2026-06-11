# Task 240: pipe mutation verbs — rerun, rerun-failed, cancel, --debug

## Status
PLANNED

## Ownership Boundary
- **Primary area:** agent surface parity for mutations: `rerun`, `cancel` subcommands with `--no-tui` JSON results; `--debug` rerun flag exposed end-to-end.
- **Allowed files:** `cmd/gh-hound/`, `internal/render/`, `internal/usecase/`, `internal/adapter/github/mutations.go` + fake (signature extensions ONLY: `RerunFailedJobs`/`RerunJob` gain a `debug bool` matching the official body fields — verify live which endpoints accept it), `docs/agent-surface.md`, `skill/SKILL.md`, `README.md`, `pages/` (agents section), tests.
- **Avoid touching:** TUI mutation flows beyond the debug toggle, mutation pacing/queue semantics.

## Depends On
- 060 (mutations), 160 (agent surface).

## Parallelizable With
- 220, 230.

## PRD / Design References
- PRD §13 (agent surface): reads are pipe-first, but mutations are TUI-only today — an agent that diagnoses a flaky failure via `runs --json` cannot rerun it without shelling out to `gh run rerun`, losing gh-hound's typed errors and rate-limit pacing.
- API (already adapted): `POST .../runs/{id}/rerun` (body `enable_debug_logging`), `.../rerun-failed-jobs`, `.../cancel`, `.../force-cancel`. `ActionService.RerunRun(repo, runID, debug)` exists.

## Problem
The JSON surface can see everything and do nothing. Close the loop for agents: `gh hound rerun --run <id> [--failed-only] [--debug]`, `gh hound cancel --run <id> [--force]`, machine-readable results, documented exit codes.

## Scope
- `rerun` subcommand: `--run <id>` (required), `--failed-only`, `--debug`, `--job <id>` for single-job rerun. `--debug` combines with ALL rerun forms — **live-verified 2026-06-10 on API v2026-03-10**: `enable_debug_logging=true` returned `201 Created` on `runs/{id}/rerun-failed-jobs` (run 27245877203) and `jobs/{id}/rerun` (job 80701207312); full rerun long-documented. Request bodies pinned in adapter tests.
- `cancel` subcommand: `--run <id>`, `--force`.
- JSON result: `{repo, run_id, action, accepted: true, html_url}` where `action` enumerates **every** mutation path: `"rerun" | "rerun_failed" | "rerun_job" | "cancel" | "force_cancel"` (single-job rerun carries `job_id` too); typed error object on failure (existing `ActionErrorKind` taxonomy). `html_url` is **reconstructed** (`https://github.com/{repo}/actions/runs/{id}`), never fetched — the mutation endpoints return no body and the one-call budget holds.
- Exit codes follow the global contract (1 is reserved for actionable CI state, never command-local): `0` mutation accepted, `2` anything else (validation, conflict such as cancelling a completed run, permission, API) — agents branch on `error.kind` in the JSON, which the schema enumerates.
- Mutation pacing: the existing 1s serial spacing applies; document it.
- TUI gains nothing new except: rerun confirm overlay gets a `d` toggle for debug logging (currently only watch has `d`); footer/help updated.

## Out of Scope
- Bulk mutations, `--all-failed` across runs, watch auto-attach after rerun (Task 270).

## Public Contracts
- New schema entries in `internal/render/testdata/schema.json`; `docs/agent-surface.md` gains a Mutations section with exit-code table; `skill/SKILL.md` teaches the verbs.

## Red / Green / Refactor Plan
- **Red:** CLI tests: each verb + flag matrix → correct usecase call (spy fake), JSON shape, exit codes incl. conflict (cancel completed run via fake) and validation (missing --run). TUI test: debug toggle in rerun confirm.
- **Green:** cobra wiring + render result type.
- **Refactor:** share mutation-result rendering between verbs.

## Test Pyramid
- L0–L1: lint + race unit suite (flag matrix, exit codes).
- L2: render schema validation; fake call-count (exactly one mutation call per invocation).
- L3: `make e2e` — fake-scenario rerun/cancel round-trips.
- L4: `make vqa` — rerun confirm overlay with debug toggle captured at three breakpoints; tui-qa cold-context `VERDICT: PASS` (pixel: toggle state styling).
- L5: live against **real repo** indrasvat/gh-hound via shux + plain CLI: rerun a completed run with `--debug`, verify debug logs appear in the rerun attempt's log (`##[debug]` lines); cancel an in-flight dispatched run; confirm exit codes in shell.

## Performance Budget (hard gates)
- One API call per mutation verb (call-count test). No TUI startup cost change.
- CLI verb wall time = API round-trip + < 50ms overhead.

## Voice (MUST)
JSON stays sober (it's for agents); human-facing strings (confirm overlay, toasts, README, agent-surface prose) keep the hound voice — e.g., rerun confirm: `send it back out? (debug nose: on)`. No emoji.

## Website & Docs Updates
- Landing `#agents` section: extend the example or exit-table note to mention mutation verbs — copy MUST keep the hound voice ("the leash works both ways"). Recapture/redeploy via preview workflow if HTML changes.
- `docs/agent-surface.md`, `skill/SKILL.md`, README agent section: verbs, flags, exit codes, pacing.

## Definition of Done
- [ ] Red-first tests; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] `rerun`/`cancel` verbs shipped with documented JSON + exit codes; schema.json updated.
- [ ] `--debug` verified live: rerun attempt's logs contain `##[debug]` lines (evidence in PR).
- [ ] TUI rerun confirm debug toggle: tui-qa cold-context `VERDICT: PASS`.
- [ ] Dootsabha (codex + gemini) converged; no blocking findings.
- [ ] Landing agents section updated in hound voice; preview deployed and pixel-checked before merge.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound rerun --run <id> --debug --no-tui --json; echo $?
./bin/gh-hound cancel --run <id> --no-tui --json; echo $?
```

## Session Protocol
1. Verify live whether `rerun-failed-jobs` accepts `enable_debug_logging`; pin the answer in the adapter test.
2. Red → green → refactor; shux/tui-qa for the confirm overlay; dootsabha; push; PR; gh-ghent loop.
