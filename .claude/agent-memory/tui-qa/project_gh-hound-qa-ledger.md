---
name: gh-hound-qa-ledger
description: Running QA failure/verification ledger for gh-hound TUI audits (round 4, task 220 async loading)
metadata:
  type: project
---

Round 4 (branch feat/220-async-loading, 2026-06-10):

- P1 found: detail screen at width < 100 cols renders steps-pane-only
  (`internal/tui/screens/detail/view.go` `if width < 100` branch), so the
  shared loading line ("fetching jobs…" + spinner, injected only into the
  Jobs pane via `Model.LoadingLine`) is dropped at 80x24. Only static dim
  "the hound is on its way back…" shows. Live-confirmed in built binary.
- VQA assertion `.claude/automations/assertions/detail-loading.json`
  asserts the placeholder text, NOT the loading line — vqa stays green
  while the documented treatment is missing at 80x24. Check assertion
  needles against the visual-contract wording every audit.
- Verified fixed/working in round 4: SIGWINCH live resize works
  (f2b85a7) — the round-3 "terminal size read once" note is STALE;
  esc pops exactly one layer (modal→log→failure→detail→runs); esc
  mid-fetch cancels with no stale route flip; filter input swallows
  global keys ('q' appends); f status cycle failing→running→passed→all;
  filtered view keeps Event/Duration/Age columns; palette cold-open
  shows only generic dispatch entry, warms to `dispatch: <name>` after
  D; dispatch route flips only after workflows resolve.

**Why:** future audits must not re-litigate verified behavior and must
re-check the narrow-width loading gap until fixed.
**How to apply:** read before every gh-hound audit; update entries when a
finding lands or a fix is verified. See [[shux-capture-recipes]].
