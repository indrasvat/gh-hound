---
name: gh-hound-qa-ledger
description: Running QA failure/verification ledger for gh-hound TUI audits (rounds 4-17; tasks 220-300 — round 17 PASS, flake forensics verified in binary; P3s: pre-existing failure-fetch error-body transient, thin-trail plural slip, clean verdict fake-unreachable)
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

Round 10 (feat/260-first-bad-run, dd9da86, 2026-06-10): PASS — the
trail (diff) screen. Fixtures diff/diff-inconclusive clean at
80/120/200; runs/palette/approvals unregressed; palette has BOTH
`approvals` and `diff · who broke main? · the trail` (rebase
coexistence OK), footer `workflows · watch · diff · theme`. Live
`__vqa-tui --scenario regression`: `:` → diff → enter flips route
instantly (immediate snapshot shows trail skeleton + "sniffing…" title
meta — 220 invariant holds; fake is zero-latency so "picking up the
scent" body is grace-window-skipped, label confirmed in binary via
strings); located trail shows verdict line, ✔ #572 attempt 2, ✗ #573,
full-width selected-row fill (applied by app-level wrapper, NOT in
screens/diff/view.go commitRow — fill lives outside the screen pkg),
j/k AND arrows move, enter opens detail for #573/d3c4b5a (not stale),
esc returns to trail with selection preserved, esc again → runs.
Accepted P3s (do not re-file): diff footer static in trail-cold state
(⏎/o/j/k inert but guarded no-ops, model.go; contract+assertion pin
one footer per screen); docs/gh-hound-design.html:900 and PRD:568
still say `diff (v2)` (stale mock text; visual-contract.md is
updated and is the operative truth); runs fixture Age values sit
under Duration header (pre-existing, renderer untouched in 260).
No HTML mock exists for the trail — visual-contract row ⑪ is the
mock reference. Evidence: .shux/out/r10-*.png.

Round 11 (feat/290-cache-visibility, 9313beb, 2026-06-10): FAIL — the
kennel (caches). Two P2s in NEW 290 code: (1) cachesHeader()
(screens/caches/view.go:115) is a hand-written literal that drifts from
row() math at ALL breakpoints — Ref header 1 col left of values,
"Last used" header 4 cols left of age values and overlapping the
right-aligned size field (hdr Ref@44/64 vs values@45/65; Last@68/94 vs
values@72/98). The exact mined header-vs-row-math failure. (2) kennel
`/` filter input silently drops the space key: caches/model.go
updateInput has no `case "space"` (runs/model.go:158 maps it to " ");
keyName emits "space" (5 runes) → default single-rune append skips it.
No leak (routeInputMode covers RouteCaches; q correctly appends), just
a dead key. P3s: "dig up 1 caches keyed" plural bug (key-path confirm
lacks singular, app.go cacheDeleteConfirmMessage); fake kennel has no
two caches sharing a key so the multi-match D confirm/toast variants
are unreachable in fixtures AND interactive fake; shared error screen
titles "Runs unavailable" over "kennel unavailable: …" on the caches
route (screens/empty/view.go:33, pre-existing renderer); 200x60 keeps
keyWidth=60 truncating keys that would fit (~90 cols idle). Everything
else VERIFIED: fixtures caches/caches-pressure/caches-empty clean at
3 breakpoints, warn line gated (present at 97%, forbidden+absent at
31%/0%); gauge tri-band confirmed in pixels (31% green, 69% orange,
97% red); palette lists approvals AND diff AND caches (rebase stitch
OK); runs/approvals/diff/diff-inconclusive fixtures unregressed;
footer matches contract + visual_contract_test carries caches row;
palette→caches flips route instantly ("fetching…" meta, 220 invariant;
"sniffing the kennel"/"digging it up" grace-skipped in fake, confirmed
in binary, app.go:1367/1432 live paths); s sort toggles size↔stalest;
/ narrows with match count; d/D confirm-gated match-count-first, esc
backs out without mutating, y folds delete locally (5→4 caches,
gauge 97→89% drops warn line) + "dug that one up." toast (TTL ~5s —
capture within one chained command); esc layering exact (filter→clear,
kennel→runs, help→kennel); j/k AND arrows move; empty-state voice line
live via --scenario empty; error state via rate_limited; help overlay
contextual to kennel; confirm-on-blank-base matches established
rerun-confirm style (NOT a 290 regression — don't re-file). Evidence:
.shux/out/r11-*.png + regenerated .claude/automations/screenshots.

Round 12 (feat/290-cache-visibility, 69624e6, 2026-06-10): PASS —
targeted re-audit of round-11 P2s + P3s. (1) cachesHeader() now derives
from columnWidths() shared with row() (view.go:117); measured in
regenerated fixtures: Ref hdr@45/65/65 == values, Size right-edge
70/96/96 == values, "Last used" hdr@72/98/98 == age values at
80/120/200 (at 80 the header label truncates to "Last u…" at the
border — values fit; acceptable). Pinned by
TestViewHeaderColumnsAlignWithRows. (2) kennel filter space FIXED live:
raw 0x20 through real decoder → "/go m  0 matches" + "no caches match
/go m"; backspace x2 restores "/go  4 matches"; esc clears (model.go:123
case "space"). (3) multi-match D now REACHABLE: fake kennel caches
go-mod-Linux-x64-1f2e3d on main (2.0 GB) + refs/pull/7/merge (819.2 MB)
(fake.go:494/497); D → confirm "dig up 2 caches keyed
\"go-mod-Linux-x64-1f2e3d\" (2.8 GB)?"; y → both rows fold 5→3,
usage 9.7→6.9 GB, gauge 97% red → 69% orange, warn line correctly
disappears, plural toast "✔ dug up 2 caches.". (4) single-match D reads
"dig up 1 cache keyed …" (app.go:1410-1414 noun switch). NOTE: the
caches/caches-pressure FIXTURE (__screen) dataset still uses
go-mod-Linux-x64-stale99 as the 4th row — no shared key in fixtures;
the multi-match flow is rehearsable only via the fake adapter
(__vqa-tui). New P3 (unfiled, cosmetic): single-key confirm on the
long setup-go-… key truncates the message tail to "(3.5 GB…" at the
confirm-box edge at 120 cols — closing ")?" lost; key label already
ellipsized by cacheKeyLabel, then fit-truncated again. Toast overlays
headline right edge ("… ✔ dug up…") — established placement, don't
re-file. Evidence: .shux/out/r12-live-*.png + regenerated
.claude/automations/screenshots/caches{,-pressure,-empty}.

Round 13 (feat/280-workflow-state, audited 75161cc then HEAD moved
mid-audit to 0c0175c — rebuilt + re-verified, 2026-06-11): FAIL — the
pack (workflows). P1 ROOT-CAUSED, answers the orchestrator's "palette
drops k" anomaly: palette.Update (overlay/palette/palette.go:51-58)
intercepts "j"/"k" as selection moves BEFORE the default text-append,
so those letters can NEVER be typed into the palette query. Burst
"workflows" → "worflows" (0 matches, dead enter); per-key w-o-r-k-f →
"worf". NOT shux, NOT keyDecoder (decoder verified byte-correct:
pending buffer pops one key per Read, burst-safe). Pre-existing since
c4dbfc5, but 280's own UX copy routes users into it: refusal toast
"wake it in :workflows before dispatching" + launch notice ":workflows
holds the leash" — and ":workflows" contains k. Arrows already cover
palette navigation; fix is dropping the j/k cases. P2 (new in 280):
toggle toasts stutter — usecase/workflow.go sets result.Message
("back on duty."/"muzzled.") and ResilienceForSuccess (errors.go:228)
uses Message for BOTH Title and Message when RunID==0 → "✔ back on
duty. · back on duty."; also never names the workflow. P3 fake-lens
unreachables: badged dispatch picker + off-duty refusal toast
unreachable in __vqa-tui (fake makes only ci.yml dispatchable —
deliberate per fake.go:138 comment; refusal pinned only by
TestDispatchPickerBadgesAndRefusesNonActiveWorkflows driving real
Update keys); workflows error/empty states unreachable (fake
ListWorkflows never errors/empties in any scenario); unknown-state ◇
badge absent from sampleWorkflows AND fake (app.go sampleWorkflows
comment "plus an unknown one" lies — slice has 5 documented states).
Everything else VERIFIED on 0c0175c: workflows + dispatch-picker
fixtures clean at 3 breakpoints, header/value columns EXACT (shared
columnWidths math, measured hdr State@64/104/184 == badges); palette
lists approvals+diff+caches+workflows w/ correct kennel/pack
descriptions; caches/approvals/diff fixtures unregressed; :→pac→enter
opens pack via startLoad ("counting…" meta flips instantly, 220
invariant; "counting the pack" grace-skipped in fake); badges ✔◌⊘⊘✗
colored per state; e on asleep→"wake workflow Nightly Sweep? it goes
back on duty"→y flips badge locally (summary recounts, no refetch);
e on active→"muzzle workflow CI? no runs until it is woken"; e INERT
on fork-disabled/deleted with why-lines ("the fork holds this
leash…"/"the workflow file is gone…"); asleep why-line "fell asleep
after 60 quiet days"; j/k+arrows+g/G all move; esc pops exactly one
layer (confirm→pack→runs); footer "j/k move · e wake/muzzle · o
browser · ⎋ back" truthful; help overlay contextual ("e wake/muzzle ·
o browser"); live resize 80/200 clean; empty-launch notice (0c0175c
names offenders: "off duty (1 asleep, 1 muzzled): Nightly Sweep,
Stale Patrol — :workflows holds the leash") wraps cleanly at 80+120;
pipe workflows --json exit 0, --disable 0 typed validation exit 2;
309 unit tests green. Stateless-fake note: reopening the pack
refetches the original roster (toggles don't persist in fake) — fresh
fetch by design, NOT stale state. 'o' browser not pressed live (real
`open` side effect); wiring verified in code. Evidence:
.shux/out/r13-*.png + regenerated screenshots/workflows,
dispatch-picker.

Round 14 (feat/280-workflow-state, 74acdba, 2026-06-11): PASS —
targeted re-audit of round-13 findings. F1 FIXED in binary: palette is
arrows-only (palette.go:51-58 drops j/k cases); burst ":workflows" and
":kennel" land every letter, items match, enter dispatches; arrows
move selection both ways; typed 'k' appends + filters + resets
selection to top. CAVEAT (P3, unfiled upstream): 23a491d's test
changes only swap "j"→"down" in 3 nav tests — NO test types j/k into
the palette query and asserts append; reintroducing `case "j",
"down":` stays green. F2 FIXED: toasts read "✔ muzzled. · workflow CI"
and "✔ back on duty. · workflow Nightly Sweep" — no stutter, workflow
named (errors.go:228 blanks echoed message; app.go:2545 injects
"workflow <name>"); badge+summary+e-verb flip locally, "5 off duty"
recounts. F3 partial FIXED: fixture roster has 6th row "Mystery Cron ·
◇ disabled_quarantine" (dim gray, real diamond glyph, no tofu);
alignment EXACT at 80/120/200 — at 80 the state text ends flush inside
the right border, no clip; summary has no "quarantined" bucket but
off-duty count includes it (matches assertion, fine). Fix 4 (74acdba
dispatch-picker stale-state refresh): code + both pinning tests
verified (workflows_app_test.go:248,279); NOT live-verifiable in fake —
the stateless fake's openDispatch refresh resurrects "active", so
in-app muzzle of CI then D still opens the form in __vqa-tui (fake-lens
artifact, NOT a build defect; orchestrator verified live both
directions). Remaining F3 unreachables from round 13 still stand
(refusal toast, workflows error/empty in fake). vqa.sh regenerated all
26 screens this audit, passed; dispatch-picker + palette fixtures
unregressed (badged picker entries ◌/⊘ intact). 519 unit tests green.
Evidence: .shux/out/r14-*.png (8) + regenerated
.claude/automations/screenshots.

Round 15 (feat/270-multi-watch, d8beceb, 2026-06-11): FAIL — the hunt
(multi-run watch board). P2: post-build rename (5dcedc2 "toasts …
all speak 'the hunt' now") MISSED the zero-lost settled toast — app.go
zero-lost branch said Title "pack's home." in the audited commit
(strings-verified in the tested binary), and "pack watch refresh
failed: " route error; the all-green settle is the COMMON real case
and is unreachable in the fake (pack scenario always loses Docs) —
fixture-fidelity gap, the buggy copy can never be rehearsed. Docs
residue: README:217 "Pack board", configuration.md:19/26/38 "pack
board", agent-surface.md:71-72, visual-contract.md:138 "pack board
open". MID-AUDIT DRIFT: working tree went dirty during the audit
(~02:18) with unaudited fixes for exactly these strings PLUS a
handoff-fence semantics change (handoffQuerySkew moved client→server
query only; strict client fence) and pack Tick Branch filter +
diff.go url.PathEscape — none built into any audited binary; gate the
NEXT commit fresh. Everything else VERIFIED on d8beceb: watch-board
fixture EXACT column math at 80/120/200 (hdr #@49/89/169 == rows,
ElapsedEnd@79/119/199, measured); all 27 vqa screens regenerated this
audit + passed; palette lists runs/--all/run:failed/watch("the hunt ·
watch the run's event group")/artifacts/approvals/diff/caches/
workflows("the pack")/dispatch; w on sibling run → board paints
INSTANTLY from on-screen rows (frame0: 3 running, queued rows "—")
before first pack tick (220 invariant; "rounding up the hunt"
grace-skipped in zero-latency fake, string confirmed in binary);
aggregate header matches rows; j/k+arrows move; f pins cursor to first
lost + follow ●; manual move drops follow; enter drills into THAT run
(Docs #603, watch footer); esc back with selection+follow kept; x →
"cancel run #603" confirm on blank base (established style) with
dedicated truthful footer; esc pops exactly one layer
(confirm→board→runs); settled toast fires ONCE at running→settled,
warn ▲ "the hunt's home — 1 run lost." (singular correct), TTL ~7s,
no re-fire; single-run group (Deploy Pages workflow_run) degrades to
classic watch; live resize 80/200 clean; help contextual to board;
pipe watch --group NDJSON: run-level transitions only, chained
workflow_run excluded, summary closes stream, exit 1. Fake-lens
unreachables: zero-lost toast, board error state, empty-hunt line,
elapsed wobble across two running ticks (settles too fast; column
edge stable across all 10 frames). 566 unit tests green. Evidence:
.shux/out/r15-*.png (22) + regenerated screenshots/watch-board.
NOTE: task doc 270 voice section still says "pack"/"pack's home." —
superseded by 5dcedc2's recorded rename, doc not updated.

Round 16 (feat/270-multi-watch, 4be4511, 2026-06-11): PASS — targeted
re-audit of round-15 findings + codex round-1 fixes, fresh build
gated. F1 FIXED IN BINARY: strings show "the hunt's home." (zero-lost)
+ "the hunt's home — " (lost) + "hunt refresh failed: "; ZERO hits for
"pack's home"/"pack watch refresh failed". Live __vqa-tui --scenario
pack at 120x40: settled warn toast "▲ the hunt's home — 1 run lost. ·
0 running · 2 home · 1 lost" captured in pixels; no re-fire after TTL
(~7s) across further refresh ticks; drill-in opens Docs #603 (correct
run), esc pops one layer. Zero-lost branch still fake-unreachable
(pack always loses Docs) — gated via strings + pin
TestRefreshPackPushesTheSettledToastOnce (real app.Refresh→refreshPack
→ViewSized path, asserts new string; no-repeat check greps "hunt's
home" — meaningful). Codex fixes verified on REAL paths: (a)
diff.go:47 url.PathEscape(workflow) in ListWorkflowRuns; workflow ids
ARE full file paths in prod (app.go:1220-1233 workflow.Path → main.go
:1266 DiscoverDispatchedRun); pin asserts EscapedPath
".../workflows/.github%2Fworkflows%2Fci.yml/runs" via real
Client+httptest. (b) handoff.go: handoffQuerySkew (5s) widens ONLY the
server CreatedAfter; newestRunSince keeps the strict client fence
(CreatedAt.Before(since) skip); pin feeds an inside-skew stale run
(since-2s) that must not attach. (c) pack.go:134 Tick filter carries
Branch: state.Branch; app packState() (app.go:2194) populates
board.Branch; pin asserts filter.Branch=="main" through real Tick.
Fixture watch-board UNCHANGED at 3 breakpoints: hdr#==rows at cell
48/88/169 (border-stripped), line widths 78/118/200 flush. 566 tests
green. PACK SCENARIO TIMING GOTCHA: packRuns() advances one tick PER
RUNS LISTING (fake.go:497, cap 4) — the runs screen's own poll burns
ticks, so idling on welcome/runs settles the pack before w; press w
within ~5s of the runs list to catch the live→settle transition.
FIXTURE SCROLL GOTCHA: __screen at 200x60 scrolls the top rows off
after the final newline — text captures race it; snapshot right after
wait-for (PNG fine), measure columns from __screen stdout instead.
RESIDUE (P3, unfiled — parent's "docs all speak hunt" claim partially
false): configuration.md:38 env-table still "Pack board size cap";
agent-surface.md "Pack size is capped by watch_group_max". Task doc
carries the post-build voice note (F4 closed). Evidence:
.shux/out/r16-*.png (6).

Round 17 (feat/300-flake-forensics, f7466a4, 2026-06-11): PASS — flake
forensics (the scent check), v0.5.0 anchor. All verified in the built
binary: fixtures flakes/failure-flaky/runs-flaky clean at 80/120/200
(amber verdict line, blue kind column, selection bar, ~ badge inside
label column, footer "j/k move · ⏎ open run · ⎋ back"); all 30 vqa
screens regenerated + passed (brief said 28 — counting drift in the
brief, not a defect); palette lists ALL 11 surfaces incl. "flakes ·
flaky or real? · the scent check"; watch-board/workflows/caches/diff/
approvals fixtures unregressed (rebase stitch clean). Live __vqa-tui
--scenario flaky: failure paints before the panel (panel arrives async
off the load slot, frame0 has no panel); tab toggles focus with ▌ mark
swapping between error-window header and panel header, in-pane hint
flips to "j/k move · ⏎ open run · ⇥ back to excerpt"; j/k drives the
focused pane (evidence cursor vs excerpt offset, both observed); enter
on evidence opens THAT run (#568/c3b2a19, #570/e5d4c3b — not stale);
esc layering exact with panel focus+cursor preserved; runs `~` badge
appears only after a verdict lands, 80-col column math intact; cold
:flakes flips route instantly ("the scent check · sniffing…" frame0,
220 invariant; "checking the scent" loading label grace-skipped in
zero-latency fake, string confirmed in binary); cached verdict answers
the second jump with no load. Verdict chrome: squirrel (flaky, amber)
+ thin trail (insufficient, dim — green scenario has only 3 completed
runs) seen live; "fresh scent"/"suspect" strings in binary, suspect +
insufficient live-verified by orchestrator on real repos. Guard toast
"no run selected — give the hound a trail to sniff" works (scenario
with no runs). HOUND_FLAKE_BADGES=false live: no panel after 3s, no
badge even after an explicit verdict, :flakes still works; pinned by
TestFailureScreenSpendsNoFlakeCallsWhenBadgesOff (real Update path,
counting resolver — right test shape; cache/rerun-invalidation pins
likewise real-path). Help on failure lists "⇥ flake panel (when
scented)" + "j/k scroll excerpt / evidence"; help on flakes contextual.
619 tests green. NEW P3s (unfiled): (1) PRE-EXISTING (c4dbfc5, NOT
300): transient "failure unavailable: select a failed job with live
GitHub data loaded" error body shows during the failure fetch —
unloadedRouteBody RouteFailure (app.go:3954) lacks the pending-load
guard its Diff/Flakes/Workflows/Watch siblings have and screenBody
checks it before the loading body (app.go:3852-3855); ~200ms in fake,
full fetch live; 300's RouteFlakes case guards correctly. (2) Plural
slip "only 1 completed runs" in thin-trail copy (usecase/flakes.go:530
+ screens/flakes/view.go:26), live-observed in failing scenario
(sample=1). (3) Clean/"fresh scent" verdict unreachable in fake lens
(green=3 completed → thin trail) and the clean VIEW branch ("nothing
wobbled" line, ok-green header, view.go:23) has no unit test —
strings-correct on inspection, but unrehearsable; ask for fixture
enrichment. (4) Help overlay shows an empty "Actions" section on
action-less screens (flakes). Fake-lens notes: live fake panel shows 4
evidence rows (retry_mask per flip run) vs fixture's 3 — different
datasets by design; fake retry_mask details render identical text for
both runs (shared fake log). Evidence: .shux/out/r17-*.png (23) +
regenerated .claude/automations/screenshots.

**Why:** future audits must not re-litigate verified behavior and must
re-check the narrow-width loading gap until fixed.
**How to apply:** read before every gh-hound audit; update entries when a
finding lands or a fix is verified. See [[shux-capture-recipes]].
