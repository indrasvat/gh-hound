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
- **Esc peels one layer:** `/` input → range → committed search → back. The search is cleared, not just hidden.
- **The `t→` echo clears when the jumped-to line is no longer drawn on screen** after a key: `j`/`k`, `^d`/`^u`, `g`/`G`, `n`/`N`, `]`/`[`, `c`, `z` (folding it away) or a search. A move that keeps the line on screen keeps the echo.
- **Error-window header:** `error window · N of M lines`, N the excerpt lines drawn, M grouped like the title (`1,587`).
- **Time-jump hint wraps** at ` · ` to the modal's inner width instead of being cut; the modal gets the width it renders in.
- **Queue XML** omits `next_commands` when there are none, matching JSON's `omitempty` (named slice type with `MarshalXML`, as `Repeats`). Always-present wrappers (`groups`, `members`, `blockers`) stay: JSON always has them.

## Task List

- [ ] T1 Search hits: no prefix; hit and current-hit tints; the gutter column is identical with and without a search, wrapped or not. Unit tests on the rendered SGR (hit rows, current row, alignment at 60/80/120); `FuzzViewport` still green.
- [ ] T2 `/` footer: `ScreenLogSearch` in the keymap and visual contract; the log footer switches while `/` is open. Keymap contract test; app test.
- [ ] T3 Failure header: real count, grouped total. Tests for a long log with a short excerpt (`4 of 1,587`) and the existing fixture.
- [ ] T4 Esc: clears a committed search before leaving the log; one layer per press with range and search both set. Model and app tests.
- [ ] T5 `t→` echo: cleared when the jumped-to line leaves the screen, kept while it stays. Model tests (scroll off, small move, `G`, `c`, wrap) and an app test through the modal.
- [ ] T6 Time-jump hint fits at 80 columns (wraps, no `…`); 120/200 unchanged. Unit test and interaction assertion.
- [ ] T7 Queue XML: no `next_commands` element when empty; commands still render when present. Render tests; CLI check with `--fake-scenario queue --format xml`.
- [ ] T8 VQA: update `log-compact-search` assertions; interaction scenarios for the `/` footer, esc after a search, the echo clearing and the modal hint.
- [ ] T9 Verification: `make ci`, `make vqa`, interaction audit at 80x24/120x40/200x60; shux before/after on fakes and a live repo (read-only); `tui-qa` round (overlay dimming out of scope); adversarial review against this task.
- [ ] T10 PR referencing #62 (not closing it) with an evidence comment: screenshots and a demo video.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; no prefix text, footer or header claims something the screen does not show; `make ci` and `make vqa` pass; evidence lives on the PR only.
