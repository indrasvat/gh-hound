# Task 440 — Runs Chrome At 80 Columns, The Scoped Branch-Miss Notice

## Status

IN PROGRESS — issue #56, third of three PRs; closes it. #71 read config.toml, made flags beat `HOUND_*` and moved keys; #72 made every screen recolour and kept fills on 256- and 16-colour terminals. UTF-8 input and `?` closing help shipped in #66; the palette's selection fill in #72.

## Ownership Boundary

- **Primary area:** the runs list's chrome (header, footer) and its launch notice.
- **Allowed files:** `internal/tui/keys/footers.go`, `internal/tui/chrome.go` (header), `internal/tui/screens/runs/view.go`, `internal/usecase/context.go` (notice scope), tests, `docs/visual-contract.md`, `.claude/automations/`.

## Decisions

- **The runs footer keeps filter and help at 80 columns.** It is 86 cells; an 80-column frame has 78, so it ended `/ …`. It now drops by priority like the log footer: `s scope` first (the notices that need it say `s scope` themselves), then `l log`, `w watch`, `f status`; `/ filter` and `?` never drop.
- **A long branch no longer pushes the header's right side off.** When the context overflows, the part before its last ` · ` (the branch) is cut with `…`; the identity after it and the right side (run count, `live`) stay. A header that fits is unchanged.
- **A notice shows in the scope it describes.** The branch-miss fallback ("… Showing all branches now.") and the detached-HEAD notice are set when the launch falls back to repo scope, but the view showed notices only in branch scope, so the fallback appeared only after `s` returned to the branch, where it was false. The launch now records each notice's scope (`NoticeScope`) and the view shows it only there: the fallback explains the all-branches list at launch and leaves with `s`; the "repo activity · s scope" hint stays a branch-scope notice.

## Task List

- [x] T1 Footer priorities, `TestRunsFooterFitsEightyColumns` (fails on main), PTY scenario `runs-footer-80`.
- [x] T2 Header, `TestHeaderKeepsItsRightSideBesideALongBranch` (fails on main).
- [x] T3 Notice scope, `TestScopedNoticeShowsOnlyInItsScope` (fails with the old rule), the launch's `NoticeScope` asserted.
- [ ] T4 Verification: `make ci`, `make vqa`, shux live at 80 columns, takshaka, Codex, cold `tui-qa`, evidence.
