# Task 260: first bad run — diff vs last green ("who broke main?")

## Status
PLANNED

## Ownership Boundary
- **Primary area:** regression locator: walk a workflow's run history to the last-green → first-red boundary and present the suspect commit range. This implements the `diff (v2)` palette stub.
- **Allowed files:** `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/model/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/` (new diff screen, palette wiring, keys/help), `docs/`, `skill/SKILL.md`, `README.md`, `pages/`, vqa harness.
- **Avoid touching:** flake scoring (Task 300), local git operations (everything comes from the API).

## Depends On
- 040 (adapter), 100 (runs list), 240 (verb conventions). The palette already advertises `diff (v2)` — this task makes the footer truthful.

## Parallelizable With
- 250.

## PRD / Design References
- PRD §18 v2 bucket: "Run diff vs last green".
- API: `GET /repos/{o}/{r}/actions/workflows/{id}/runs?branch=&status=&created=` (date-range filterable, paginated) + `GET /repos/{o}/{r}/compare/{base}...{head}` (commit range, files). No dedicated endpoint — client-side scan over existing history.
- Ecosystem fact: devs fall back to `git bisect` (re-running builds!) for an answer already present in run history. No terminal tool mines it.

## Problem
"Who broke main?" — when a workflow goes red, the question is which commit range turned it. gh-hound has the run history and etag-cached pagination to answer instantly, with a JSON verdict agents can branch on.

## Scope
- Usecase `LocateRegression(repo, workflow, branch)`: newest-first scan for the most recent completed-success run before the current failure streak → `{last_good_run, first_bad_run, suspect_commits[], compare_url}`; commits via compare API (cap: 50 commits rendered, total count reported). Mixed states (cancelled/skipped between) are skipped, documented.
- Attempt-aware: a run that failed then succeeded on rerun counts by its **latest** attempt conclusion (note interplay with Task 300; keep the rule explicit and tested).
- Pipe: `gh hound diff --workflow <name|id> [--branch <b>] --no-tui --json` → verdict object; exit codes follow the global contract: `0` no action derivable (verdict `green` OR `inconclusive` — JSON `status` is the source of truth), `1` boundary located (a regression exists — action needed), `2` API error. `3` is NOT used (it means pending/running repo-wide and nothing here is pending).
- TUI: palette `diff` entry opens a diff screen — boundary summary (last good #N ✔ → first bad #M ✗), suspect commit list (sha, author, subject), `o` opens compare URL in browser, `enter` on the first-bad run jumps to its detail screen.
- Page cap: configurable `diff_max_pages` (default 10 pages × per_page 100 runs) to bound API spend; hitting the cap → `inconclusive` verdict, never a hang.

## Out of Scope
- Per-job/per-step blame, log diffing between runs, flake classification (Task 300), local `git bisect` orchestration.

## Public Contracts
- JSON verdict schema in schema.json + agent-surface.md: `{workflow, branch, status: "located"|"green"|"inconclusive", last_good: {run...}, first_bad: {run...}, suspect_commits: [{sha, author, message}], total_suspects, compare_url}`.

## Red / Green / Refactor Plan
- **Red:** usecase tests on fake history fixtures: clean boundary; failure streak with cancelled runs interleaved; rerun-flipped attempt; all-green; inconclusive at cap; pagination across pages. Adapter httptest for compare. CLI verdict/exit-code tests. TUI screen render tests.
- **Green:** scan + compare + render + screen, layer by layer.
- **Refactor:** extract history-walk iterator reusable by Task 300.

## Test Pyramid
- L0–L1: lint + race suite (the scan logic is the core — exhaustive fixture matrix).
- L2: adapter httptest (workflow-runs filters, compare); schema validation; call-count tests pinning page budget.
- L3: `make e2e` — new fake scenario `regression` with a seeded boundary.
- L4: `make vqa` — diff screen fixtures at 80x24/120x40/200x60 (boundary header, commit list truncation, inconclusive state); cold-context tui-qa `VERDICT: PASS`, pixel-level (alignment of sha/author/subject columns, long-subject ellipsis).
- L5: shux against **real repos**: indrasvat/gh-hound (find a real historical boundary — this repo has them) and openclaw/openclaw (deep history, exercises the page cap + etag reuse). Verify the located boundary against `git log` ground truth for gh-hound. Screenshots of the full flow: palette → diff → enter-to-detail → o-to-browser.

## Performance Budget (hard gates)
- Verdict benchmark (defined state, serial queue): boundary within the first 2 history pages, warm etag cache → **≤ 3 API round-trips (2 list pages + 1 compare) in < 3s** on indrasvat/gh-hound (shux/hyperfine evidence). Deep-history capped case: a separate envelope of **< 1.5s per additional page**, measured on openclaw/openclaw.
- API spend bounded by `diff_max_pages`; etag-cached re-runs of the same query answer from 304s (trace-log evidence in PR).
- TUI diff screen paint < 50ms once data arrives; scan runs async with loading feedback (Task 220 pattern).

## Voice (MUST)
This is the marquee hound feature — naming and copy must land the metaphor: the hound **picks up the scent**. Screen title suggestion: `the trail`. Verdict line: `scent picked up: #N was clean, #M wasn't.` Inconclusive: `trail went cold after 1,000 runs.` Docs/site copy same register. No emoji.

## Website & Docs Updates (required)
- Landing: new trail row — q: `Who broke main?` keys: `:diff` — answer in hound voice (this question was a hero-copy candidate; it belongs here). New screens-gallery tab (`the trail`) captured via `.claude/automations/capture_fixture.py`. Agents section: mention the JSON verdict. **Voice MUST hold everywhere.**
- `docs/agent-surface.md` (verdict schema + exit codes), `skill/SKILL.md` (teach `diff --json` for agent loops), README, `docs/visual-contract.md`, `docs/configuration.md` (`diff_max_pages`).
- Palette footer: `diff (v2)` becomes `diff` — the stub is finally honest.

## Definition of Done
- [ ] Red-first across layers; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] Scan matrix fully covered incl. attempt-flips, interleaved cancels, cap behavior.
- [ ] Pipe verdict + exit codes shipped and documented; schema.json updated.
- [ ] TUI diff screen wired from palette; enter→detail and o→browser work; loading is async.
- [ ] Real-repo boundary located and cross-checked against `git log` ground truth (evidence in PR).
- [ ] openclaw/openclaw deep-history run stops at `diff_max_pages` and meets the < 1.5s-per-additional-page envelope (timings in PR); the < 3s gate applies only to first-two-page boundaries.
- [ ] tui-qa cold-context `VERDICT: PASS` at all breakpoints; dootsabha (codex + gemini) converged.
- [ ] Landing row + gallery tab + docs live on preview in hound voice; production on merge.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound diff --workflow CI --branch main --no-tui --json; echo $?
./bin/gh-hound diff --workflow CI -R openclaw/openclaw --no-tui --json
HOUND_FAKE_SCENARIO=regression ./bin/gh-hound     # palette → diff
```

## Session Protocol
1. Pin the attempt-conclusion rule and the cancelled/skipped skip-list in tests before any scan code.
2. Red → green → refactor; shux + tui-qa; dootsabha; landing capture; push; PR; gh-ghent loop.
