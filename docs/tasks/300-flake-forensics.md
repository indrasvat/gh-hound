# Task 300: flake forensics — cross-run flaky job detection (v0.5.0)

## Status
PLANNED

## Ownership Boundary
- **Primary area:** flake scoring: jobs/steps that fail then pass across attempts and across runs of the same workflow+branch; auto-retry masking detection.
- **Allowed files:** `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/model/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/` (flake panel on failure/detail screens, palette), `docs/`, `skill/SKILL.md`, `README.md`, `pages/`, vqa harness.
- **Avoid touching:** test-level (per-test-case) parsing of arbitrary frameworks — job/step granularity only in this task.

## Depends On
- 210 (attempt forensics), 260 (history-walk iterator — reuse it). This is the v0.5.0 milestone anchor; land after the 220–290 wave.

## Parallelizable With
- None (touches the same surfaces as several v0.4 tasks).

## PRD / Design References
- PRD §18 v2 bucket: "Flake detection".
- API reality (pin in tests): attempts via `GET .../runs/{id}/attempts/{n}` + `.../attempts/{n}/jobs`; **check-run annotations are only retrievable for the latest attempt** (community #103026) — flake evidence must come from attempt job conclusions + logs, not annotations.
- Demand: paid products (BuildPulse, TestDino) exist solely for this; GitHub's own engineering blog documents 18x flaky-build reduction efforts. No terminal tool ships a flake verdict, and agents need exactly this to decide rerun-vs-investigate.

## Problem
The most expensive triage question is "real failure or flake?" — today it's answered by gut feeling and a rerun. gh-hound has attempt forensics and (after 260) a history walker; combine them into a scored, evidenced flake verdict.

## Scope
- Signals (per job name within workflow+branch, over a window of `flake_window` runs, default 50):
  1. **Attempt flips:** failed attempt N, succeeded attempt N+1 on the same run (the strongest signal).
  2. **Cross-run flapping:** same head_sha (or adjacent commits) alternating fail/pass. The commit range between flaps is SHOWN as evidence (compare API from 260) — file relevance is **not inferred** (no generic job-to-path mapping exists; explicitly out of scope).
  3. **Retry-action masking:** step logs matching known retry wrappers (`nick-fields/retry`, `Retrying in …` patterns) where a step eventually "succeeds" — surfaced as masked instability. **Budget guard:** log scanning is restricted to logs already in hand (the currently viewed failure) plus runs in the window that had >1 attempt — NEVER a blanket log download across the window; the windowed verdict is otherwise metadata-only (attempt/conclusion data), or the serial queue and the 5s budget are unmeetable.
- Score: per job `{flake_score: 0..1, evidence: [{run_id, attempt, kind, detail}], verdict: "flaky"|"suspect"|"clean"}` with documented thresholds (e.g., ≥2 attempt flips in window → flaky). The result carries a **top-level `status`**: `"flaky" | "suspect" | "clean" | "insufficient_data"` (worst job verdict wins; `insufficient_data` when the window is underfilled) — this is the field agents and the exit code branch on.
- Pipe: `gh hound flakes [--workflow <w>] [--branch <b>] --no-tui --json` → scored jobs; exit codes follow the global contract: `0` no action (verdict `clean` OR `insufficient_data` — JSON `status` is the source of truth), `1` action needed (any job verdict `flaky` OR `suspect` — rerun vs investigate is distinguished in JSON, both demand attention), `2` API error. `3` is NOT used.
- Documented limitation: retry-wrapper masking in runs that never had a failed attempt is NOT detected (their logs are never fetched under the budget); the verdict JSON carries `signals_evaluated` so agents know what was checked.
- Failure screen: when the failing job has a non-clean verdict, a flake panel shows `seen this one before: flaked 3 of last 50 runs` with evidence drill-down. Focus model (the failure screen is a scroll viewport per PRD §9.4, no selection concept): `tab` toggles focus between the excerpt viewport and the flake panel; `j/k` drives whichever pane has focus; `enter` on a focused evidence row jumps to that run's detail; focused pane is visually marked per theme.
- Runs list: optional flake glyph on rows whose failing job is a known flaker (config `flake_badges`, default on).
- Caching: verdicts cached per (workflow, branch, head window) for the session; recomputation is incremental as new runs land.

## Out of Scope
- Per-test-case flake attribution, quarantine automation, historical persistence across sessions (no local DB in v0.5), org dashboards.

## Public Contracts
- Flake verdict JSON schema in schema.json + agent-surface.md, including thresholds and the explicit annotations-latest-attempt caveat. SKILL.md teaches the rerun-vs-investigate agent decision: `verdict=flaky → rerun --failed-only; verdict=clean → investigate`.

## Red / Green / Refactor Plan
- **Red:** scoring tests on synthetic histories (each signal isolated, then combined; threshold boundaries; insufficient data; window sliding), retry-pattern log matcher table tests, attempt-walker tests reusing 260's iterator, CLI/exit-code tests, failure-panel and badge render tests.
- **Green:** signals → score → verdict → surfaces, layer by layer.
- **Refactor:** consolidate history iteration shared with 260; keep the matcher table data-driven (additions are one-line).

## Test Pyramid
- L0–L1: lint + race suite — the scoring matrix is the heart; aim for exhaustive table tests.
- L2: adapter httptest for attempt endpoints; schema validation; call-count tests pinning the API budget (window walk ≤ `flake_window`/per_page list calls + attempts only for runs with >1 attempt).
- L3: `make e2e` — fake scenario `flaky` (seeded attempt flips + a retry-masked step).
- L4: `make vqa` — failure-screen flake panel + badged runs list at 80x24/120x40/200x60, evidence drill-down; cold-context tui-qa `VERDICT: PASS`, pixel-level (panel borders, badge alignment, evidence row columns).
- L5: shux against **real repos**: openclaw/openclaw (large history — find genuine flakes or document a clean verdict honestly) and indrasvat/gh-hound (this repo had real attempt flips during v0.3 development — locate them). Cross-check one scored flake by hand against the GitHub UI attempt history. Screenshots of panel + drill-down in PR.

## Performance Budget (hard gates)
- `flakes` verdict on a 50-run window: **< 5s** warm, API calls within the pinned budget (trace-log evidence).
- Failure-screen panel: verdict computed async, never blocks the failure paint (Task 220 pattern); panel arrival < 3s warm.
- Zero extra API calls when `flake_badges=false` and the panel is not opened (call-count test).

## Voice (MUST)
The flake verdict is the hound recognizing a scent: `seen this one before.` Verdicts: flaky = `it's a squirrel` (chasing it is optional), clean = `fresh scent — worth chasing.` Keep every surface (TUI, JSON field VALUES stay sober, docs, site) in voice; jokes in prose, never in schema keys. No emoji.

## Website & Docs Updates (required)
- Landing: new trail row — q: `Flaky or real?` keys: `gh hound flakes` — and agents-section mention of the rerun-vs-investigate JSON decision. Gallery tab for the flake panel if it reads well (`.claude/automations/capture_fixture.py`). **Voice MUST hold.**
- `docs/agent-surface.md` (schema + thresholds + caveats), `skill/SKILL.md` (decision recipe), README feature table, `docs/configuration.md` (`flake_window`, `flake_badges`), `docs/visual-contract.md`.

## Definition of Done
- [ ] Red-first; scoring matrix exhaustively table-tested; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] All three signals implemented with documented thresholds; verdict JSON + exit codes shipped.
- [ ] Failure panel + badges + evidence drill-down shipped, async, within perf gates (shux timings in PR).
- [ ] Real-repo validation: at least one hand-verified verdict (flaky or clean) cross-checked against GitHub UI attempt history (evidence in PR).
- [ ] API budget pinned by call-count tests; trace-log evidence in PR.
- [ ] tui-qa cold-context `VERDICT: PASS` at all breakpoints; dootsabha (codex + gemini) converged.
- [ ] Landing row + docs live in hound voice; preview pixel-checked; production on merge.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound flakes --workflow CI --branch main --no-tui --json; echo $?
./bin/gh-hound flakes -R openclaw/openclaw --no-tui --json
HOUND_FAKE_SCENARIO=flaky ./bin/gh-hound           # failure panel + badges
```

## Session Protocol
1. Pin the annotations-latest-attempt-only caveat and the API budget in red tests before any scoring code.
2. Build signals one at a time, each red→green; shux + tui-qa; dootsabha; landing capture; push; PR; gh-ghent loop.
