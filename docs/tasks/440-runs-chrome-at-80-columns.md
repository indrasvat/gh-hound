# Task 440 — Runs Chrome At 80 Columns, The Scoped Branch-Miss Notice

## Status

IMPLEMENTED — issue #56, third of three PRs; closes it. #71 read config.toml, made flags beat `HOUND_*` and moved keys; #72 made every screen recolour and kept fills on 256- and 16-colour terminals. UTF-8 input and `?` closing help shipped in #66; the palette's selection fill in #72.

## Ownership Boundary

- **Primary area:** the runs list's chrome (header, footer) and its launch notice.
- **Allowed files:** `internal/tui/keys/footers.go`, `internal/tui/chrome.go` (header), `internal/tui/screens/runs/view.go`, `internal/usecase/context.go` (notice scope), tests, `docs/visual-contract.md`, `.claude/automations/`.

## Decisions

- **The runs footer keeps filter and help at 80 columns.** It is 86 cells; an 80-column frame has 78, so it ended `/ …`. It now drops by priority like the log footer: `s scope` first (the notices that need it say `s scope` themselves), then `f status`, `w watch`, `l log`; `/ filter` and `?` never drop.
- **A long branch no longer pushes the header's right side off.** When the context overflows, the part before its last ` · ` (the branch) is cut with `…`; the identity after it and the right side (run count, `live`) stay. A header that fits is unchanged.
- **A notice shows in the scope it describes.** The branch-miss fallback ("… Showing all branches now.") and the detached-HEAD notice are set when the launch falls back to repo scope, but the view showed notices only in branch scope, so the fallback appeared only after `s` returned to the branch, where it was false. The launch now records each notice's scope (`NoticeScope`) and the view shows it only there: the fallback explains the all-branches list at launch and leaves with `s`; the "repo activity · s scope" hint stays a branch-scope notice.

## Task List

- [x] T1 Footer priorities, `TestRunsFooterFitsEightyColumns` (fails on main), PTY scenario `runs-footer-80`.
- [x] T2 Header, `TestHeaderKeepsItsRightSideBesideALongBranch` (fails on main).
- [x] T3 Notice scope, `TestScopedNoticeShowsOnlyInItsScope` (fails with the old rule), the launch's `NoticeScope` asserted.
- [x] T4 Verification: `make ci` (1,232 tests), `make vqa` (3 new PTY scenarios), shux live at 80/120/200 columns, Codex, cold `tui-qa` (FAIL, then PASS on 627fcdc), evidence.

## Found Along The Way

- Codex (1 major, 5 minor, fixed with a test): the header cut kept any context's last ` · ` segment, so a scent-check header lost its workflow name; it now keeps only an identity (` · @login`), and reserves it before a long branch when a long login meets a wide right side. A `branch:` query in repo scope still showed "Showing all branches now"; a branch query hides it. From a detached local checkout, `-R` another repo carried the detached-HEAD notice; it needs the same repo. The footer's documented drop order was the reverse of the code's (equal priorities drop rightmost first): `f status`, `w watch`, `l log`. The evidence recorder's `cwd` now stops the run when the directory is gone instead of recording whatever checkout it ran in.
- The live evidence uses a temporary worktree on a long local-only branch, read-only (`--actor @me`): no fixture can give a branch with no runs plus the fallback.
- Cold `tui-qa` round 1 (1d417bd, FAIL on two P2s; all three claims worked): a cut header put its right side in the frame's last cell, against the border, where every other header keeps one; the budget now counts that cell. `__vqa-tui` ignored `--branch`, so no fixture could reach the long-branch header or the fallback; it takes `--branch` now, and PTY scenarios `runs-branch-miss` and `runs-branch-miss-scope` pin both (on a branch the fake has no runs on). Also: the fallback notice no longer advises "try repo-wide discovery" in a list that is already repo-wide (the agent JSON diagnostic keeps it). Recorded, not changed: without a server query, `s` on an empty branch stays put (by design, as on main), and the 80-column footer no longer shows `s` there.
- Cold `tui-qa` round 2 (627fcdc, PASS): a cut header keeps its border cell at 80/120/200 columns, live and in the fixture; flakes and diff headers keep their workflow name; the 80x24 sweep matches main on every screen but the runs footer line.
