---
name: gh-hound-qa-ledger
description: Running QA failure/verification ledger for gh-hound TUI audits (rounds 4-6; tasks 220 async loading, 240 rerun-confirm debug toggle)
metadata:
  type: project
---

Round 4 (branch feat/220-async-loading, 2026-06-10):

- P1 found, FIXED in 956bb2f (round-5 re-audit PASS): detail at
  width < 100 dropped the shared loading line; now
  `renderStepsPane(..., standalone)` carries `m.LoadingLine` in the
  stacked layout — verified at 80/120/200 via fixture (same `ViewSize`
  path as live). Live at 80x24 the fake fetch resolves inside the 100ms
  grace, so only the skeleton placeholder frame is observable — correct
  no-flash behavior, not a regression.
- VQA weakness FIXED in 956bb2f: detail-loading.json now asserts
  "fetching jobs…" as wait+verify needle, and vqa.sh loops every screen
  at all 3 breakpoints, so an F1 regression fails the suite. Lesson
  stands: check assertion needles against visual-contract wording.
- Stale title-bar meta during failure/log loading FIXED in 956bb2f:
  right slot shows "fetching…" instead of old line counts.
- Accepted P3 pre-existing (do not re-file as new): runs footer
  truncation at 80 cols; filter input keeps generic footer; palette/help
  repeat their title inside the body.
- Verified fixed/working in round 4: SIGWINCH live resize works
  (f2b85a7) — the round-3 "terminal size read once" note is STALE;
  esc pops exactly one layer (modal→log→failure→detail→runs); esc
  mid-fetch cancels with no stale route flip; filter input swallows
  global keys ('q' appends); f status cycle failing→running→passed→all;
  filtered view keeps Event/Duration/Age columns; palette cold-open
  shows only generic dispatch entry, warms to `dispatch: <name>` after
  D; dispatch route flips only after workflows resolve.

Round 6 (branch feat/240-pipe-mutations, 3b4646b, 2026-06-10): PASS.
`rerun-confirm` fixture clean at 80/120/200; live `r` confirm opens
"debug nose: off" + `y confirm · d debug · enter/n/esc cancel` footer,
`d` toggles on/off/on; `x` cancel confirm correctly shows no debug line
and no d-footer, and `d` is inert there (overlay unchanged); esc and n
close cleanly; `y` fires accepted toast (`✔ accepted · CI #… ·
rerun_run`). Known style, not a 240 regression: accepted toast overlays
the column-header right edge (truncates "Age" to "A…") — established
toast placement.

**Why:** future audits must not re-litigate verified behavior and must
re-check the narrow-width loading gap until fixed.
**How to apply:** read before every gh-hound audit; update entries when a
finding lands or a fix is verified. See [[shux-capture-recipes]].
