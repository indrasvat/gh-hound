# Task 380 — Cut Failure Keys, Log Search In Range, True Counters

## Status

IMPLEMENTED — issue #65, second of two PRs, which closes it. The first (task 370, PR #66) shipped UTF-8 input, the live startup paint and the help toggle. `make ci`, `make vqa`, interaction audit and render hygiene pass; verified with shux 0.51 on fakes and live (read-only) on this repo.

## Ownership Boundary

- **Primary area:** the failure screen's keys at cut heights, the log screen's search, range and fold header, the time-jump picker's layout, and digit grouping.
- **Allowed files:** `internal/tui/screens/failure/`, `internal/tui/screens/log/`, `internal/tui/overlay/timejump/`, `internal/tui/screens/caches/view.go`, `internal/tui/app.go`, `internal/numfmt/`, the grouping call sites, tests, `.claude/automations/`, `docs/`.
- **Avoid touching:** screen layouts beyond the cut rules, the key decoder and startup (task 370).

## Decisions

- **The failure screen knows its height.** The app sets it before each key, like the log screen's. One function, `drawn`, counts the excerpt and evidence rows the view draws at that height; the view and the keys both use it. The excerpt scrolls by the rows drawn, so a 1-row window still reaches the last line; tab focuses the flake panel only when an evidence row is drawn; the selection is clamped to drawn rows; a resize that cuts the panel gives focus back to the excerpt.
- **Search covers the lines in view.** With a range active, matches are only the range's lines, and the header shows both (`log · [range] · /q · match c/t`). Setting or clearing a range re-runs a committed search.
- **The fold header counts what is still folded.** `N lines folded in M places` counts the closed folds of the full-log plan, so a jump or a search hit that opens one updates it; with every fold open it reads `log · compact`. `FoldCount` still counts the plan (the `c` "nothing to fold" toast uses it).
- **One digit grouper.** `numfmt.Group` replaces five private copies (log, failure, fold summary, app, usecase diff) and groups negatives right. The match counters, in the header and the frame, use it.
- **The time-jump picker fits its box.** Short of rows, it drops the spacers, then the in-box hint (the footer keeps the keys), then the feedback; the input and the selected entry stay. A scrolled list ends in `… ↑ N · ↓ M more`, as the palette does.

## Task List

- [x] T1 Failure: a property test at every terminal height 8–24 (body 2–18) reading the view: j/k reaches every excerpt line drawn, tab focus only with evidence drawn, the selection on a drawn row; an app test that the height reaches the model.
- [x] T2 Log: search inside a range (and again after clearing it), the folded header after a jump, z and Z, grouped counters in the header and the frame; `FuzzViewport` checks matches against the lines in view after every event.
- [x] T3 Time-jump: a property test over widths 44/66/80, heights 2–20, three selections, with and without feedback; an app test that the box closes inside the body at 48–80 × 11–16.
- [x] T4 PTY scenarios `failure-cut-scroll`, `log-range-search`, `log-match-grouped`, `log-fold-count`, `timejump-small`, `caches-filter-clear` (each fails on main).
- [x] T5 Verification: `make ci`, `make vqa`; shux before/after on fakes and live; adversarial review and a cold `tui-qa` round.

## Found Along The Way

- On main, j could move the excerpt window up: a failure that opens near the end of its lines starts past the last full window, and j clamped it back. The window now never moves up on j.
- An applied caches filter's line now ends `· ⎋ clear`, as the runs filter and log search do (the footer's `⎋ back` is what esc does after that).
- Not changed: the runs footer is cut at 80 columns (`/ …`); which key to drop there is a product call.
- `TestSafePlanContextDeadline` (fold) is timing-sensitive without `-race`: 50–240 ms against a 250 ms limit, the same on main.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; `make ci` and `make vqa` pass; evidence lives on the PR only.
