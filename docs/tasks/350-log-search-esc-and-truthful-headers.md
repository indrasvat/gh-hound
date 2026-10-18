# Task 350 — Log Search Tint, Esc Layers And Truthful Headers

## Status

IN PROGRESS — issue #62, first of two PRs. This PR fixes the log screen, failure header, time-jump modal and queue XML items; it references #62 without closing it. The overlay dimming item (help, palette, confirm, time-jump and approvals all replace the base view today) is the second PR, which closes #62.

## Ownership Boundary

- **Primary area:** the full-log screen's search hits, `/` input footer, esc layering and `t→` echo; the failure screen's error-window header; the time-jump modal hint; queue XML.
- **Allowed files:** `internal/tui/screens/log/`, `internal/tui/screens/failure/`, `internal/tui/overlay/timejump/`, `internal/tui/keys/`, `internal/tui/app.go`, `internal/tui/*_test.go`, `internal/render/queue.go`, `internal/render/*_test.go`, `.claude/automations/`, `docs/`.
- **Avoid touching:** overlay compositing (second PR), theme tokens, other screens, the fold engine, the parser.

## Decisions

- **Hits are tint only.** Every hit row keeps the mock's warn tint (`.logln.hit`); the current hit (`match c/t`) gets a stronger warn tint and a warn line number. Nothing is prefixed, so the gutter and wrap widths are the same with or without a search.
- **`/` input footer** is `type to search · ⏎ find · ⎋ cancel`, generated from the keymap like `runs_filter`.
- **Esc peels one layer:** `/` input → range → committed search → back. The search is cleared, not just hidden; its header ends `· ⎋ clear`, as a range's does, so the footer's `⎋ back` is not the only hint.
- **The `t→` echo clears when the jumped-to line is no longer drawn on screen** after a key: `j`/`k`, `^d`/`^u`, `g`/`G`, `n`/`N`, `]`/`[`, `c`, `z` (folding it away) or a search. A move that keeps the line on screen keeps the echo.
- **Error-window header:** `error window · N of M lines`, N the excerpt lines on screen, M grouped like the title (`1,587`). The failure view gets the body height and draws only the excerpt lines that fit; the flake panel follows only a whole excerpt and is itself cut to what is left (ending `…`, as the frame would), so neither annotations above nor the panel below can make the count wrong.
- **Time-jump hint wraps** at ` · ` to the modal's inner width instead of being cut; the modal gets the width it renders in, and a picker longer than the screen scrolls with the selection so the input, the hint and the box border stay visible.
- **Queue XML** omits `next_commands` when there are none, matching JSON's `omitempty` (named slice type with `MarshalXML`, as `Repeats`). Always-present wrappers (`groups`, `members`, `blockers`) stay: JSON always has them.

## Task List

- [x] T1 Search hits: no prefix; hit and current-hit tints; the gutter column is identical with and without a search, wrapped or not. Unit tests on the rendered SGR (hit rows, current row, alignment at 60/80/120); `FuzzViewport` still green.
- [x] T2 `/` footer: `ScreenLogSearch` in the keymap and visual contract; the log footer switches while `/` is open. Keymap contract test; app test.
- [x] T3 Failure header: real count, grouped total. Tests for a 1,587-line log (the count is read off the drawn rows), a height that cuts the excerpt (unit, app and the `failure` fixtures) and the existing fixture.
- [x] T4 Esc: clears a committed search before leaving the log; one layer per press with range and search both set (model test); search then back (app test).
- [x] T5 `t→` echo: cleared when the jumped-to line leaves the screen, kept while it stays. Model tests (scroll off, small move, `G`, `c`, wrap) and an app test through the modal.
- [x] T6 Time-jump hint fits at 80 columns (wraps, no `…`); 120/200 unchanged; a long picker scrolls to fit 80x24. Unit and app tests, interaction assertion.
- [x] T7 Queue XML: no `next_commands` element when empty; commands still render when present. Render tests; CLI check with `--fake-scenario queue --format xml`.
- [x] T8 VQA: update `log-compact-search` assertions; interaction scenarios for the `/` footer, esc after a search (stays, then goes back), the echo clearing and the modal hint.
- [x] T9 Verification: `make ci`, `make vqa`, interaction audit at 80x24/120x40/200x60; shux before/after on fakes and a live repo (read-only); `tui-qa` round (overlay dimming out of scope); adversarial review against this task.
- [ ] T10 PR referencing #62 (not closing it) with an evidence comment: screenshots and a demo video.

## Found Along The Way

- Live dogfooding (CI #149): a time jump can land on a stamped `##[group]` line, which draws as a fold row; the echo cleared on the first key. Group headers now count as the line (a collapsed repeat still hides it), with a test that failed first. `FuzzViewport` missed it because its echo oracle shared the row-kind rule; it now reads the rendered gutter and finds the reinjected bug in seconds.
- `FuzzViewport` also fuzzes esc on a committed search and checks the echo after every key against the frame.
- Cold `tui-qa` round (0e8f7f6, FAIL on P2s): the failure header still counted the excerpt when annotations pushed it off screen (live CI #149 at 80x24); `⎋ back` was the only esc hint once a search was committed; a long picker pushed the time-jump hint and border off screen at 80x24. All three fixed above, each with a test that failed first. Re-gate (Round 25, e288c95): with a flake panel at the exact-fit height the frame's clip still replaced the last excerpt row (`3 of 7`, 2 drawn); the panel is now budgeted inside the view, tested at every height from 8 to 24 on the unit model and the `failure`, `failure-flaky` and `failure-actions` fixtures. Removing the `LastJump` guard from `Changed` (suggested by review) would skip the repaint when a resize then an unbound key clears the echo; kept, with a test.
- Tabs and escapes in the failure excerpt pushed its right border out (pre-existing, but the height fix draws such rows at 80x24 on live CI #149): excerpt rows now go through the full log's `TerminalSafe`, with a test.
- Pre-existing, now visible because the excerpt is cut to fit rather than clipped by the frame: with a cut excerpt, `j`/`k` still scrolls a 5-line window (its last lines can be unreachable at small heights), and `tab` still focuses the flake panel that is not drawn (or, in a partly cut panel, `j` can move its selection onto an undrawn evidence row). The header stays truthful; left as is, a candidate for the second PR now that the view knows the room.
- Pre-existing, not fixed here: search ignores an active time range; match counters are not digit-grouped; the compact header's "N lines folded" count goes stale after a jump or match opens a fold; the time-jump modal is clipped below 80x16 and its scrolled list shows no more-above/below marker.
- shux `pane snapshot` PNGs drop the frame's right border column (the text capture has it); evidence uses window snapshots.
- Out of scope, unchanged: confirm and approvals overlays still render at `width-20`; `groupDigits` now has three private copies (log, fold, failure).

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; no prefix text, footer or header claims something the screen does not show; `make ci` and `make vqa` pass; evidence lives on the PR only.
