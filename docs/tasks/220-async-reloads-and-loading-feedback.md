# Task 220: async fetches everywhere — spinners and progress for every blocking transition

## Status
PLANNED

## Ownership Boundary
- **Primary area:** TUI message loop — make **every** server-backed fetch asynchronous with visible loading feedback:
  - runs-list reloads (status cycle `f`, filter `/`, scope `s`, load-more `G`) — issue #21;
  - `enter` → detail (jobs fetch is synchronous today: `app.go loadDetail` calls the resolver inline);
  - failure screen open (log + annotations fetch);
  - `l` → full log — the slowest path (multi-MB download + parse): spinner MUST include byte progress (`◔ fetching log… 2.1 MB`), and parse runs off the paint path.
  The artifacts fetch is the existing async reference pattern; this task brings every other transition up to it.
- **Allowed files:** `internal/tui/` (app, runs/detail/failure/log screens, toasts, keys), `internal/usecase/` (resolver entry points only), `cmd/gh-hound/main.go` (resolver wiring), tests, vqa harness, `docs/visual-contract.md`.
- **Avoid touching:** adapter request semantics, pipe surface, dispatch/watch internals.

## Depends On
- 100 (runs list), 210 (status cycle). Closes GitHub issue #21.

## Parallelizable With
- 230, 240 (disjoint files).

## PRD / Design References
- PRD §9.2 (runs home is cache-first; network never blocks a keystroke) — currently violated: `f` and `/` reloads are synchronous and freeze the UI 3+ seconds on slow repos (issue #21, QA round 3).
- Existing async pattern to copy: artifacts fetch in `internal/tui/app.go` (message-passing, spinner state).

## Problem
gh-hound promises "born to run" speed, but most keystrokes that need the network block the whole TUI with zero feedback. The artifacts pane already does this right; everything else must follow the same message-passing pattern.

## The Invariant (the actual deliverable)
> **No keystroke may block on the network. Any operation that can exceed 100ms paints a themed spinner (or byte/percent progress when a size is knowable) within 50ms, stays cancellable with `esc`, and drops stale results.**

This is a standing contract: every current fetch is converted in this task, and every FUTURE task (250 approvals, 260 diff scan, 270 pack watch, 290 caches, 300 flake verdicts — all already reference "the Task 220 pattern") inherits it. Add the invariant to `docs/visual-contract.md` and `docs/development.md` so reviews can cite it.

## Scope — full inventory of blocking fetches to convert
- **Runs-list reloads** (`f` status cycle, `/` filter, `s` scope, `G` load-more) — issue #21; previous list stays visible, dimmed, with a loading line.
- **`enter` → detail**: jobs fetch goes async; skeleton detail (run header from cached data) paints immediately with spinner in the jobs pane.
- **Failure screen open**: log + annotations fetch async; header paints first.
- **`l` → full log**: byte-progress spinner (`◔ fetching log… 2.1 MB`) — content-length is known after redirect; parse runs off the paint path too (multi-MB logs must not freeze on parse).
- **Dispatch form open**: workflows/default-branch fetches (coordinates with Task 230) — form paints with placeholders.
- Common machinery: a single loading-state component + generation counter (stale responses dropped) + `esc` cancels interest and restores the prior view instantly.
- All loading/error states join the visual contract.

## Out of Scope
- Parallel API calls (serial queue stays), prefetching, watch-screen polling (already async), progress for sub-100ms cached paths (no spinner flash — a 100ms grace delay before showing the spinner).

## Public Contracts
- No JSON/pipe changes. Keybindings unchanged. New visual states only.

## Red / Green / Refactor Plan
- **Red:** per transition (runs reload, detail, failure, log, dispatch): keypress returns immediately with loading state set; slow fake adapter (injected delay) proves no blocking; generation test: stale result dropped; esc-cancel test; log byte-progress updates test; 100ms grace-delay test (cached path shows no spinner flash).
- **Green:** wrap each resolver in a `tea.Cmd`; one shared loading-state component (generation counter, spinner, optional progress); skeleton paints per screen.
- **Refactor:** fold the artifacts async helper into the shared component; resolvers become uniformly async.

## Test Pyramid
- L0: `make gofix-check fmt-check lint arch-check emoji-check`.
- L1: `make test` (race) — new async/generation/cancel tests.
- L2: fake adapter with injectable latency; call-count tests prove no duplicate fetches per keystroke.
- L3: `make e2e`.
- L4: `make vqa` — new captures: runs-loading, detail-skeleton, failure-loading, log-byte-progress states at 80x24/120x40/200x60; cold-context tui-qa agent audit returns `VERDICT: PASS` with pixel-level screenshot review (no color bleed in dimmed lists, spinner alignment, progress text stability).
- L5: shux-driven live sessions against **real repos**: indrasvat/gh-hound (small) and openclaw/openclaw (large, slow — multi-MB logs). Drive every converted transition; capture pixel screenshots before/during/after each fetch; verify the UI accepts input during every fetch; open a >5MB real log and watch byte progress climb.

## Performance Budget (hard gates)
- Keystroke-to-first-repaint for **every** converted transition: **< 50ms**, measured via shux timestamps on openclaw/openclaw.
- Spinner grace delay: cached/fast paths (< 100ms) show no spinner at all (no flash).
- Multi-MB log: parse off the paint path; scroll responsive within 100ms of content arrival.
- Zero additional API calls vs current behavior (call-count test).
- No goroutine leak across 50 rapid transition cycles (race suite + goroutine count assertion).

## Voice (MUST)
All new user-facing strings keep the funny/nerdy hound voice (e.g., loading: `◌ sniffing out failing runs…`). No corporate filler, no emoji (emoji-check enforces), glyphs from the existing theme set only.

## Website & Docs Updates
- None expected (behavioral fix). If the loading state is visible in any landing screenshot recapture, regenerate via `.claude/automations/capture_fixture.py` and deploy preview.
- `docs/visual-contract.md`: add reload loading/dimmed states.

## Definition of Done
- [ ] Red tests written first and observed failing; then green; race suite passes.
- [ ] `make ci && make e2e && make vqa && make docs-check` all pass.
- [ ] The Invariant holds for every fetch in the inventory (runs reloads, detail, failure, log, dispatch open); stale results dropped; esc cancels; grace delay prevents spinner flash.
- [ ] Log fetch shows byte progress on a real >5MB log (shux evidence).
- [ ] < 50ms keystroke-to-repaint verified for every transition on openclaw/openclaw via shux, timings in PR body.
- [ ] The Invariant documented in `docs/visual-contract.md` + `docs/development.md`.
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
