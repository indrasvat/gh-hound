# Task 220: async run reloads with loading feedback

## Status
PLANNED

## Ownership Boundary
- **Primary area:** TUI message loop — make every server-backed reload (status cycle `f`, filter `/`, scope `s`, load-more `G`) asynchronous with visible loading feedback.
- **Allowed files:** `internal/tui/` (app, runs screen, toasts, keys), `internal/usecase/context.go` (reload entry points only), tests, vqa harness, `docs/visual-contract.md`.
- **Avoid touching:** adapter request semantics, pipe surface, dispatch/watch internals.

## Depends On
- 100 (runs list), 210 (status cycle). Closes GitHub issue #21.

## Parallelizable With
- 230, 240 (disjoint files).

## PRD / Design References
- PRD §9.2 (runs home is cache-first; network never blocks a keystroke) — currently violated: `f` and `/` reloads are synchronous and freeze the UI 3+ seconds on slow repos (issue #21, QA round 3).
- Existing async pattern to copy: artifacts fetch in `internal/tui/app.go` (message-passing, spinner state).

## Problem
gh-hound promises "born to run" speed, but on large repos a status-cycle or filter keystroke blocks the whole TUI while the server-filtered list loads. The artifacts pane already does this right; run reloads must follow the same message-passing pattern.

## Scope
- All run-list reloads return a `tea.Cmd`; UI repaints immediately with the previous list dimmed plus a themed loading line (`◌ fetching failing runs…` — hound voice, see Voice).
- Stale-response guard: a reload result carries a generation counter; late responses for a superseded query are dropped.
- `esc` during an in-flight reload cancels interest (result discarded) and restores the prior view instantly.
- Loading/error states for reloads join the visual contract.

## Out of Scope
- Parallel API calls (serial queue stays), prefetching, watch-screen polling (already async).

## Public Contracts
- No JSON/pipe changes. Keybindings unchanged. New visual states only.

## Red / Green / Refactor Plan
- **Red:** app-level test: `f` keypress returns immediately with loading state set; slow fake adapter (injected delay) proves no blocking; generation test: stale result dropped; esc-cancel test.
- **Green:** wrap reload calls in commands; add generation counter + loading flag to runs model; render dimmed list + loading line.
- **Refactor:** unify with artifacts async helper into one loading-state component.

## Test Pyramid
- L0: `make gofix-check fmt-check lint arch-check emoji-check`.
- L1: `make test` (race) — new async/generation/cancel tests.
- L2: fake adapter with injectable latency; call-count tests prove no duplicate fetches per keystroke.
- L3: `make e2e`.
- L4: `make vqa` — new captures: runs-loading state at 80x24/120x40/200x60; cold-context tui-qa agent audit returns `VERDICT: PASS` with pixel-level screenshot review (no color bleed in the dimmed list, loading line alignment).
- L5: shux-driven live sessions against **real repos**: indrasvat/gh-hound (small) and openclaw/openclaw (large, slow). Drive `f` through the full cycle and `/` filters; capture pixel screenshots before/during/after each reload; verify the UI accepts input during fetch.

## Performance Budget (hard gates)
- Keystroke-to-repaint after `f` or `/`: **< 50ms** (loading frame), measured via shux timestamps on openclaw/openclaw.
- Zero additional API calls vs current behavior (call-count test).
- No goroutine leak across 50 rapid filter cycles (race suite + goroutine count assertion).

## Voice (MUST)
All new user-facing strings keep the funny/nerdy hound voice (e.g., loading: `◌ sniffing out failing runs…`). No corporate filler, no emoji (emoji-check enforces), glyphs from the existing theme set only.

## Website & Docs Updates
- None expected (behavioral fix). If the loading state is visible in any landing screenshot recapture, regenerate via `.claude/automations/capture_fixture.py` and deploy preview.
- `docs/visual-contract.md`: add reload loading/dimmed states.

## Definition of Done
- [ ] Red tests written first and observed failing; then green; race suite passes.
- [ ] `make ci && make e2e && make vqa && make docs-check` all pass.
- [ ] `f`/`/`/`s`/`G` reloads never block; loading feedback visible; stale results dropped; esc cancels.
- [ ] < 50ms keystroke-to-repaint verified on openclaw/openclaw via shux, timings in PR body.
- [ ] tui-qa cold-context audit `VERDICT: PASS` at all three breakpoints with screenshot evidence.
- [ ] Dootsabha review rounds (codex + gemini) converged: no blocking findings.
- [ ] GitHub issue #21 closed by the PR.

## Verification Commands
```bash
make ci && make e2e && make vqa
HOUND_FAKE_SCENARIO=failure ./bin/gh-hound        # then f/f/f rapid-cycle
./bin/gh-hound -R openclaw/openclaw --all          # shux-driven, real repo
```

## Session Protocol
1. Re-read PRD §9.2 + artifacts async pattern before edits.
2. Red → green → refactor, committing per layer.
3. shux + tui-qa audits before push; dootsabha rounds; push; PR; gh-ghent review loop.
