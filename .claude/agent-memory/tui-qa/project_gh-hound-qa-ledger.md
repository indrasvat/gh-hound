---
name: gh-hound-qa-ledger
description: Running QA failure/verification ledger for gh-hound TUI audits (rounds 4-9; tasks 220, 240, 230, 250 approvals — round 9 PASS, space key fix verified live)
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

Round 7 (branch fix/230-dispatch-ref-foreign-repos, 9054d7e,
2026-06-10): PASS. Dispatch fixture unchanged at 80/120/200. Live
against real repos: `dispatch -R openclaw/openclaw` opens the workflow
chooser (palette pre-filtered to dispatch: entries, real list, clean
ellipsis truncation) and the form pre-fills ref `main` — NOT the local
fix/230 branch. Own-repo (`-R indrasvat/gh-hound`) pre-fills the local
branch, which IS the task-230 contract ("target == local origin →
local branch") — do not file that as a leak. HOUND_WELCOME=true +
dispatch verb: enter on welcome opens the chooser with workflows
loaded (9054d7e). Pipe `dispatch --no-tui --json` exits 2 with
"dispatch is interactive…" and empty stdout. Untested: bogus-ref typed
validation error; actual dispatch submission (mutating, skipped by
design). P3 transient: dispatch-launch backdrop shows "no runs match /"
with an empty filter token while workflows fetch.

Round 7 addendum (orchestrator L5, 15ca847, 2026-06-10): both
round-7 gaps closed live. Bogus ref: submitting the dispatch form with
the unpushed local branch as ref produced the typed red toast
`Mutation rejected · ref "…" isn't in this yard — pass an existing
branch or tag` and fired no mutation. Real submission: foreign-repo
dispatch (`--repo indrasvat/shux`, form pre-filled `ref main ▾`)
created run 27320888708 (Deploy Pages, event workflow_dispatch,
head_branch main). Note: the accepted toast TTLs out within ~5s, so
delayed snapshots miss it — verify submission via the API run record.
Ref field is render-only in the form (`m.Workflow.Ref`); it is set by
context resolution and not editable in-form.

Round 8 (branch feat/250-deployment-approvals, f49f88f worktree,
2026-06-10): FAIL — P1: `space pick` in the deploy-gate overlay is DEAD
in the built binary. `keyName()` (cmd/gh-hound/main.go:1346 default
branch) emits `" "` for byte 0x20; `approvals.Update` matches only
`case "space"` (overlay/approvals/approvals.go:99,123). Unit tests
drive `KeyMsg{Key: "space"}` (approvals_app_test.go:221) — green in a
key name the real TTY never produces; the EXACT dead-code-path pattern
from the claim-verification rules. Consequences: [x] never toggles,
locked-env refusal notice ("not yours to open — …") unreachable,
footer/help/visual-contract "space pick" all lie. Comment mode space
works only by accident (default single-rune append). app.go:2406 also
lists "space" (dispatchHandled) — same dead name, check dispatch
toggles whenever space handling is fixed. Everything else in round 8
passed: 3 new fixtures + assertions OK at all breakpoints, ◫ badge +
gated summary + notice line on runs, A → shared loader (app.go:1432
"checking the gate") → overlay async, y/n confirm-gated ("open the
gate for production?" / "keep the gate shut for production?"), c
comment mode with truthful footer, detail pending panel live, help "A
approvals (waiting runs)", palette "approvals · review the deploy
gate", runs footer static, runs/detail/dispatch fixtures unregressed.

Round 9 (feat/250-deployment-approvals, 1808bc9, 2026-06-10): PASS —
round-8 P1 (dead space key) verified FIXED in the built binary, live
`__vqa-tui --scenario waiting` 120x40. keyName() now maps 0x20 →
"space" (cmd/gh-hound/main.go:1346); pinned end-to-end by
TestSpaceByteTogglesApprovalsPickThroughRealDecoder (cli_test.go:374,
drives raw " " through production keyDecoder.Next into app.Update —
the right test shape). Verified live: space toggles [x]→[ ]→[x] on
production; space on locked staging → "not yours to open — staging
needs another reviewer" and stays [-]; y with nothing picked → "pick a
gate first — space marks an environment", no confirm; runs filter
accepts "a b" and narrows; palette query accepts space; dispatch text
field accepts "v1 rc" AND space still cycles choice fields (round-8
app.go:2406 follow-up closed). e273dfc also verified live: c → type
draft → esc restores default comment line ("reviewed from gh-hound
(default)"), esc pops only the comment layer, and y afterwards opens
the confirm with no draft carried; comment mode has dedicated truthful
footer "type comment · ⏎ done · ⎋ cancel". Confirm box itself never
displays the comment text — by design, the overlay comment line is the
source of truth. Waiting-scenario nav note: `A`'s wait-for needle must
be overlay-specific ("space pick"), since "production" matches the
runs row text. 253 unit tests green; 12 screenshots in .shux/out/r9-*.

**Why:** future audits must not re-litigate verified behavior and must
re-check the narrow-width loading gap until fixed.
**How to apply:** read before every gh-hound audit; update entries when a
finding lands or a fix is verified. See [[shux-capture-recipes]].
