# Task 270: multi-run watch — event groups and dispatch→watch handoff

## Status
PLANNED

## Ownership Boundary
- **Primary area:** watch a group of runs (same push/PR event) as one board; auto-attach watch after dispatch and rerun.
- **Allowed files:** `internal/usecase/` (watch, context, ports, `ActionResult` — gains optional `WorkflowRunID`/`RunURL`/`HTMLURL` from the dispatch `200` body), `internal/adapter/github/mutations.go` (consume the dispatch response body; pin 200-body AND 204-fallback adapter tests), `internal/adapter/fake/`, `internal/tui/screens/watch/`, `internal/tui/` (app routing), `internal/render/`, `cmd/gh-hound/`, `docs/`, `skill/SKILL.md`, `README.md`, `pages/`, vqa harness.
- **Avoid touching:** log streaming internals (per-step append contract stays as PRD §9.5), request pacing/etag semantics.

## Depends On
- 135 (watch), 140 (dispatch), 220 (async pattern), 240 (rerun verbs for handoff).

## Parallelizable With
- 280, 290.

## PRD / Design References
- PRD §9.5 watch contract (poll min/max, step-append "streaming").
- Ecosystem demand: gh CLI's most-upvoted Actions issues — log streaming in watch (cli#3484), watch multiple runs (cli#10719), `workflow run --watch` (cli#12967). watchgha's loved feature is exactly event-grouped multi-run watch.

## Problem
A push typically triggers several workflows. gh-hound watches one run; the human alt-tabs (or worse, opens a browser) to see the rest. Group by `head_sha`, watch the pack together, and after `dispatch`/`rerun` drop straight into watch.

## Scope
- Group model: runs sharing `head_sha` + event from the current scope; board shows one row per run (workflow name, status glyph, conclusion, elapsed), aggregate header (`3 running · 1 passed · 0 failing`). Job/step detail appears only for the drilled-in run — the runs-list API carries no job data, and the poll budget (below) forbids per-run job fetches on the board.
- Watch board keys: `j/k` select run, `enter` drill into single-run watch (existing screen), `esc` back to board, `x` cancel selected (confirm-gated), `f` follow worst-status run automatically.
- Entry points: `w` on a runs-list selection watches that run's event group (single-run group degrades to today's behavior — zero regression); post-dispatch handoff (config `auto_watch` honored) — **live-verified 2026-06-10**: on API version 2026-03-10 (which the adapter pins) the dispatches endpoint returns `200` with `{workflow_run_id, run_url, html_url}` — attach watch directly to that run id. Fallback ONLY for `204 No Content` responses (older GHES/API versions): bounded discovery poll (runs list filtered by workflow + ref + `event=workflow_dispatch`, `created_at` after the dispatch timestamp; max 10 polls / 30s, then a `couldn't pick up the scent` toast and graceful return to runs). Rerun handoff is DIFFERENT — no discovery: rerun reuses the existing `run_id`; attach to it and poll until `run_attempt` advances or status leaves `completed`.
- Pipe: `gh hound watch --group --no-tui` emits NDJSON state transitions per run until the group settles (documented for agents); exit code = worst outcome (existing exit-code semantics).
- Polling: ONE serial queue, group polling budget ≤ existing single-watch budget × 1.5 via shared run-list poll (one list call covers all group members) — NOT per-run polling. Cap group size (config `watch_group_max`, default 10).

## Out of Scope
- Cross-repo groups, log multiplexing on the board (logs remain in single-run watch), desktop notifications.

## Public Contracts
- NDJSON event schema documented in agent-surface.md — **group events are run-level only** (`{ts, run_id, workflow, status, conclusion}`), consistent with the board's no-job-fetch budget; `job`/`step` fields appear ONLY in single-run drill-in watch events. Terminal summary object closes the stream.

## Red / Green / Refactor Plan
- **Red:** grouping tests (same sha different events; single-run group; cap overflow), board state-machine tests (selection, drill-in/out, follow-worst), poll-budget call-count test (N runs, one list call per tick), dispatch→watch handoff test, NDJSON shape tests.
- **Green:** group usecase + board screen + handoff wiring.
- **Refactor:** unify board/single-watch status row rendering.

## Test Pyramid
- L0–L1: lint + race suite.
- L2: fake adapter latency/ordering races (run completes between ticks; new run joins group mid-watch).
- L3: `make e2e` — fake multi-run scenario (`pack`: 3 workflows, staggered completion).
- L4: `make vqa` — watch-board fixtures at 80x24/120x40/200x60 (aggregate header, per-run rows, worst-follow highlight); cold-context tui-qa `VERDICT: PASS`, pixel-level (row alignment, elapsed column jitter across repaints — MUST be stable, no column wobble).
- L5: shux against **real repos**: push a commit to indrasvat/gh-hound (CI + CodeQL + Pages preview all trigger) and watch the real pack converge through the actual binary; dispatch→watch handoff live; openclaw/openclaw for a long-running group. Record a full session (shux capture series) for the PR.

## Performance Budget (hard gates)
- Poll cost: one `runs` list call per tick regardless of group size (call-count test) + per-run job calls only for the drilled-in run.
- Board repaint < 16ms per tick at 200x60 (no flicker — pixel-diff consecutive shux frames, only expected cells change).
- Rate budget: a 10-run group watched 10 minutes stays under the etag-304 profile documented in PRD §20 (trace-log evidence).

## Voice (MUST)
Pack metaphor: a group is a `pack`. Header: `the pack: 3 running · 1 home · 0 lost`. Settled toast: `pack's home.` All copy in the hound voice, theme glyphs, no emoji.

## Website & Docs Updates (required)
- Landing: watch gallery tab recaptured to show the pack board (`.claude/automations/capture_fixture.py`); watch caption updated; "Sit. Stay. Triage." watch row mentions the pack if it reads naturally. **Voice MUST hold.**
- `docs/agent-surface.md` (NDJSON contract), `skill/SKILL.md`, README, `docs/configuration.md` (`watch_group_max`), `docs/visual-contract.md`.

## Definition of Done
- [ ] Red-first; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] Board + drill-in + handoffs shipped; single-run behavior unchanged (regression tests).
- [ ] Poll budget pinned by call-count tests; trace-log rate evidence in PR.
- [ ] Pixel-diff stability: consecutive board frames differ only in expected cells (shux evidence).
- [ ] Live pack watched to settlement on real pushes (gh-hound + openclaw), session captures in PR.
- [ ] tui-qa cold-context `VERDICT: PASS`; dootsabha (codex + gemini) converged.
- [ ] Landing watch tab + docs updated in hound voice; preview pixel-checked; production on merge.

## Verification Commands
```bash
make ci && make e2e && make vqa
HOUND_FAKE_SCENARIO=pack ./bin/gh-hound            # board via fake
./bin/gh-hound watch --group --no-tui              # NDJSON until settled
./bin/gh-hound -R openclaw/openclaw --all          # shux: w on a fresh event group
```

## Session Protocol
1. Pin the one-list-call-per-tick budget in a red test before building the board.
2. Red → green → refactor; shux pixel-diff pass; tui-qa; dootsabha; landing capture; push; PR; gh-ghent loop.
