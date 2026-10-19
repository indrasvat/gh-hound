# Task 380 — Cut Failure Keys, Log Search In Range, True Counters

## Status

IMPLEMENTED — issue #65, second of two PRs, which closes it. The first (task 370, PR #66) shipped UTF-8 input, the live startup paint and the help toggle. `make ci`, `make vqa`, interaction audit and render hygiene pass; verified with shux 0.51 on fakes and live (read-only) on this repo.

## Ownership Boundary

- **Primary area:** the failure screen's keys at cut heights, the log screen's search, range and fold header, the time-jump picker's layout, and digit grouping.
- **Allowed files:** `internal/tui/screens/failure/`, `internal/tui/screens/log/`, `internal/tui/overlay/timejump/`, `internal/tui/screens/caches/view.go`, `internal/tui/app.go`, `internal/numfmt/`, the grouping call sites, tests, `.claude/automations/`, `docs/`.
- **Avoid touching:** screen layouts beyond the cut rules, the key decoder and startup (task 370).

## Decisions

- **The failure screen knows its height.** The app sets it before each key, like the log screen's. One function, `drawn`, counts the excerpt and evidence rows the view draws at that height; the view and the keys both use it. The excerpt scrolls by the rows drawn, so a 1-row window still reaches the last line, and j/k do nothing with no excerpt row drawn; tab focuses the flake panel only when an evidence row is drawn; the selection is clamped to drawn rows; a resize that cuts the panel gives focus back to the excerpt, and one that grows the view refills a scrolled excerpt (never above where it opened, so `l` still opens the full log there). An annotation takes one row whatever its message's newlines.
- **Search covers the lines in view.** With a range active, matches are only the range's lines, and the header shows both (`log · [range] · /q · match c/t`). Setting or clearing a range re-runs a committed search; the current match stays if still in view, else is the next after it, and the view shows it.
- **The fold header counts what is still folded.** `N lines folded in M places` counts the closed folds of the full-log plan, so a jump or a search hit that opens one updates it; with every fold open it reads `log · compact`. `FoldCount` still counts the plan (the `c` "nothing to fold" toast uses it).
- **One digit grouper.** `numfmt.Group` replaces five private copies (log, failure, fold summary, app, usecase diff) and groups negatives right. The match counters (header and frame), fold markers and rows, `N runs loaded` (`1 run loaded`), the runs filter's `N of M loaded`, the caches filter (`1 match`) and `dug up N caches` use it. The markdown export keeps plain numbers (agent-facing).
- **The time-jump picker fits its box, and the box the frame.** Short of rows, the picker drops the spacers, then the in-box hint (the footer keeps the keys), then the feedback; the input and the selected entry stay, and with one row the input line carries the selected entry. A scrolled list ends in `… ↑ N · ↓ M more`, as the palette does. Short of body rows, the box moves its title into its top border, then drops its borders (`overlayBox`, only when it would not fit), so the modal works down to the frame's minimum (80x7); the marker shows from 80x11.

## Task List

- [x] T1 Failure: a property test at every terminal height 8–24 (body 2–18) reading the view: j/k reaches every excerpt line drawn, tab focus only with evidence drawn, the selection on a drawn row; an app test that the height reaches the model.
- [x] T2 Log: search inside a range (and again after clearing it), the folded header after a jump, z and Z, grouped counters in the header and the frame; `FuzzViewport` checks matches against the lines in view after every event.
- [x] T3 Time-jump: a property test over widths 44/66/80, heights 1–20, three selections, with and without feedback; an app test that the modal fits the body with its input and selected entry at 48–80 × 7–16.
- [x] T4 PTY scenarios `failure-cut-scroll`, `log-range-search`, `log-match-grouped`, `log-fold-count`, `timejump-small`, `timejump-tiny`, `caches-filter-clear` (each fails on main), and `failure-scroll-grow` (a resize after a scroll; guards the refill).
- [x] T5 Verification: `make ci`, `make vqa`; shux before/after on fakes and live; adversarial review and a cold `tui-qa` round.

## Found Along The Way

- On main, j could move the excerpt window up: a failure that opens near the end of its lines starts past the last full window, and j clamped it back. The window now never moves up on j.
- An applied caches filter's line now ends `· ⎋ clear`, as the runs filter and log search do (the footer's `⎋ back` is what esc does after that).
- Adversarial review: setting or clearing a range left the current match off screen (or at a stale index); `FuzzViewport` now checks the current match is in view after either. Fold markers and rows, the caches count and runs counters were still ungrouped; `1 runs loaded` and `1 matches` read wrong.
- Cold `tui-qa` round 4 (02cdcb0, FAIL): after a scroll, growing the terminal left a 1–2 line excerpt over empty rows (live CI #149: `1 of 489`), and j/k moved an excerpt with no row drawn; below 80x11 the time modal's input was not drawn (also on main). All fixed, each with a test that failed first.
- Not changed: flake-panel focus cut away by a resize comes back if the view grows again before a key is pressed (a key commits the move to the excerpt). Bone contrast is #56.
- Not changed: the runs footer is cut at 80 columns (`/ …`); which key to drop there is a product call.
- `TestSafePlanContextDeadline` (fold) is timing-sensitive without `-race`: 50–240 ms against a 250 ms limit, the same on main.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; `make ci` and `make vqa` pass; evidence lives on the PR only.
