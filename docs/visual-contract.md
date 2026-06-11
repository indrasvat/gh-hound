# gh-hound Visual Contract

Source of truth: `docs/gh-hound-design.html`. Re-read the matching mock before editing any screen and again before marking it done.

## Screen Refs

| Ref | Screen | Implementation Requirement |
|---|---|---|
| ⓪ | welcome/version | ANSI Shadow `HOUND` banner; ok-to-info-to-fg gradient; tagline; first-run cards; emoji-free |
| ① | all_green | green summary band; large success glyph; dim recent rows; footer biases watch and dispatch |
| ② | runs_list | focused panel; breadcrumb; rate/cache/live header; selected row uses surface-2 fill plus green left bar; f cycles status filter (all/failing/running/passed) through the server filter line |
| ③ | detail | side-by-side jobs and step timeline; failed step gets fail-tinted fill plus fail left bar; Tab moves focus across jobs, steps, and artifacts; artifacts block lists name, size, and expired badge with selected-row fill |
| ③b | detail (waiting run) | Deploy gate panel above artifacts: per environment the ◫ glyph, approvability ("you can open this gate" in ok / "not yours to open" in warn), wait timer, and reviewers right-aligned |
| ④ | failure | breadcrumb header; annotations first; de-noised colored error window with line count and expand affordance |
| ⑤ | watch | streaming badge; follow marker; completed steps colored; active step shows running cursor and append-only tail |
| ⑤b | watch_board (the hunt) | aggregate header `the hunt: N running · N home · N lost`; scent + follow state right-aligned; one row per run (glyph, workflow, #, state, fixed-width elapsed); header and rows share one column math; cursor bar; follow-worst pins the cursor to the first lost/running row |
| ⑥ | log | line-number gutter; fold rows; search hit tint; ANSI plus syntax accents; decorative scrollbar; t opens a time-jump input (header echoes t→HH:MM:SS) |
| ⑦ | toasts_dispatch | toast layer over dimmed base and dispatch form with typed inputs |
| ⑧ | palette | centered overlay; prompt row; filtered list; selected row surface-2 fill plus green left bar |
| ⑨ | approvals | deploy-gate overlay over the runs screen: env multi-pick rows ([x]/[ ]/[-] checkboxes, ◫ glyph, reviewers line), comment line with documented default, notice line for refusals; opened by A on a waiting run after the shared "checking the gate" load |
| ⑩ | help | three-column contextual keymap; legend; Canvas/Layer overlay |
| ⑪ | diff (the trail) | hound verdict line; ✔ last-good / ✗ first-bad boundary summary with attempt note; suspect rows with aligned sha/author/subject columns, selection bar, long-subject ellipsis; inconclusive state hints at diff_max_pages |
| ⑫ | caches | kennel header `kennel: 7.2/10 GB · N caches`; pressure gauge colored ok (<50%), run (50–90%), fail (>90%); past 90% the warn line `kennel's almost full — GitHub starts evicting at 10 GB.`; sortable key/ref/size/last-used table with selected-row fill; `/` filter line; empty state `the kennel's empty — nothing cached on this repo.`; delete confirms lead with the match count |
| ⑭ | flakes (the scent check) | hound verdict line colored by status (run-amber flaky, fg suspect, ok clean, dim thin trail); per-job score line `~ build · score 1.00 · flaky · 2 flips · 2 masked retries`; evidence rows with selection bar and run-number/kind/detail columns; failure screen grows the flake panel below the excerpt — focused pane marked with the selection bar, tab toggles, j/k drives the focused pane, enter on evidence opens that run; runs rows badge known flakers with `~` inside the label column |
| ⑬ | workflows (the pack) | one row per workflow: name, file, themed state badge (✔ active, ◌ asleep, ⊘ muzzled, ⊘ fork-disabled, ✗ deleted; unknown states verbatim with ◇); header columns share the row width math; e toggles only toggleable states (confirm-gated); fork-disabled/deleted carry a why-line instead of the toggle |

## Theme Tokens

| Theme | Token | Hex |
|---|---|---|
| bramble | bg | #0E0F0C |
| bramble | bg-elev | #141512 |
| bramble | surface | #1B1D17 |
| bramble | surface-2 | #24271E |
| bramble | line | #2E3227 |
| bramble | line-2 | #3D4233 |
| bramble | dim | #6B7060 |
| bramble | subtle | #8C9179 |
| bramble | muted | #AEB39B |
| bramble | fg-soft | #CFCDBB |
| bramble | fg | #EAE8D9 |
| bramble | ok | #4FD37A |
| bramble | ok-deep | #2E8E55 |
| bramble | fail | #E2564B |
| bramble | run | #E0A33E |
| bramble | info | #6E9CB5 |
| bramble | warn | #E8895A |
| bramble | neutral | #6B7060 |
| bramble | term-bg | #0B0C0A |
| bone | bg | #EFEDE1 |
| bone | fg | #23241C |
| bone | ok | #1F9E55 |
| bone | fail | #C24033 |
| bone | run | #B57A1E |
| bone | info | #3E7491 |
| bone | warn | #C2632F |
| bone | neutral | #8A8773 |
| bone | term-bg | #1B1D17 |

Semantic mapping: success uses `ok`; failure uses `fail`; in-progress uses `run`; queued/pending/waiting uses `info`; action-required/timed-out uses `warn`; cancelled/skipped/stale/neutral uses `neutral`.

## Glyphs

All glyphs are text presentation only. Do not append emoji variation selectors. The HTML mock uses `⎇` for branch and the shux snapshot font chain must render it without tofu before visual QA passes.

| Name | Glyph |
|---|---|
| success | ✔ |
| gate (waiting run) | ◫ |
| failure | ✗ |
| in_progress | ⠹ |
| queued | ◌ |
| cancelled | ⊘ |
| skipped | ⊝ |
| action_required | ▲ |
| timed_out | ⧗ |
| neutral | ◇ |
| selection_bar | ▌ |
| branch | ⎇ |
| breadcrumb | › |
| fold_open | ▾ |
| fold_closed | ▸ |
| prompt | ❯ |
| rerun | ↻ |
| dispatch | ▶ |
| enter | ⏎ |
| escape | ⎋ |
| flake badge | ~ |

## Footers

Footer text must be generated from keymap data, not copied into renderers.

| Screen | Footer |
|---|---|
| welcome | ⏎ continue · ? help · q quit |
| all_green | j/k move · s scope · ⏎ open · w watch · D dispatch · / filter · ? help |
| runs_list | j/k · s scope · ⏎ open · ↻ rerun · ✗ cancel · l log · w watch · f status · / filter · ? |
| detail | ⏎ expand · ↻ rerun job · R rerun failed · ✗ cancel · ⎋ back · ? |
| failure | ↻ rerun failed · r rerun job · l full log · o browser · y copy excerpt |
| watch | ✗ cancel · f follow · d debug · ⎋ detach |
| watch_board | j/k move · ⏎ drill in · f follow worst · ✗ cancel · ⎋ back |
| log | j/k scroll · g/G ends · / search · t time · n/N match · z/Z fold · w wrap · ⎋ back |
| dispatch | ⏎ run · ⇥ next · ⎋ cancel |
| diff | j/k move · ⏎ first bad · o compare · ⎋ back |
| flakes | j/k move · ⏎ open run · ⎋ back |
| caches | j/k move · s sort · / filter · d dig up · D dig up key · ⎋ back · ? |
| workflows | j/k move · e wake/muzzle · o browser · ⎋ back |
| palette | workflows · watch · diff · theme |
| help | : palette · ? close · ⎋ close |
| toasts | ⎋ dismiss · g dismiss all · r retry · ? help |
| approvals overlay | j/k move · space pick · y open gate · n keep shut · c comment · ⎋ close |

## Breakpoints

| Geometry | Rule |
|---|---|
| 80x24 | single column; detail is full-screen push; keep status workflow number age |
| 120x40 | master-detail side-by-side; full runs columns |
| 200x60 | side-by-side with extra padding; add actor and sha |

## Visual QA Gate

For each screen at `80x24`, `120x40`, and `200x60`: verify alignment, color mapping, selection fill and bar, focus treatment, footer/keymap parity, overlay layering, no tearing, no clipped glyphs, and recognizable fidelity to the HTML mock.

## Loading States (the Task 220 invariant)

> **No keystroke may block on the network.** The UI repaints within
> 50ms of the keystroke (previous content dimmed, or a skeleton). If
> the fetch is still in flight after a 100ms grace window, the shared
> loading indicator appears. Every fetch is esc-cancellable and stale
> results are dropped.

There is exactly **one** loading indicator in gh-hound
(`internal/tui/loading.go` + `icons.SpinnerFrames`). A bespoke
per-screen spinner in any later change is a review-blocking defect.

| Surface | Loading treatment |
|---|---|
| runs reload (`f` `/` `G`) | previous rows dimmed; loading line below the summary |
| hunt board open (`w`) | rows seed instantly from the runs list; dimmed with the loading line below while the first hunt tick lands |
| detail open (`⏎`) | skeleton with run header + repo breadcrumb; loading line in the jobs pane |
| failure open | shared loading body |
| log open (`l`) | shared loading body with byte progress (`▰▰▱▱▱ 2.1 MB/4.8 MB`) when Content-Length is known |
| dispatch open (`D`) | loading line on the originating runs screen; route flips when resolved |
| diff open (`:diff`) | shared loading body (`picking up the scent`) on the trail screen |
| caches open (palette `caches`) | shared loading body (`sniffing the kennel…`); deletes reuse the slot (`digging it up…`) |

Palette note: `:` opens with the generic dispatch entry only — the
per-workflow `dispatch: <name>` items appear once the workflow cache
is warm (after the first dispatch open), because enriching on the
keystroke would violate the invariant.

Spinner: braille cycle (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`), ~120ms/frame, run color;
labels are hound-voiced and passed per context. Fixture screens:
`runs-loading`, `detail-loading`, `failure-loading`, `log-progress`,
`caches`, `caches-pressure`, `caches-empty`.

Approvals: `A` on a waiting run fetches the gate list through the
shared loader (kind `approvals`, label "checking the gate") before the
overlay opens; the runs list shows the standard loading line meanwhile.
Waiting runs wear the ◫ gate badge in the status column and the summary
line advertises `◫ N gated · A review` only while one is on screen.
Approve and reject are both confirm-gated ("open the gate for …?" /
"keep the gate shut for …?"); verdict toasts: "gate's open." and
"gate stays shut.". Fixture screens: `runs-waiting`, `approvals`,
`detail-pending`.

## Render Hygiene (the flicker contract)

The TTY renderer diffs each frame against the previous one and flushes
the update as ONE write wrapped in synchronized-output guards
(`ESC[?2026h` … `ESC[?2026l`). After the first paint it never erases
the whole screen — `ESC[2J` between frames is the blank-flash flicker
this contract exists to prevent. Changed lines reposition absolutely
and erase their own tails (`ESC[K`); shrinking frames erase below
(`ESC[J`); resizes invalidate the diff for one full repaint. The app
lives on the alternate screen with the cursor hidden.

Enforced three ways: renderer unit pins (`cmd/gh-hound/render_test.go`),
the `render_hygiene.sh` PTY stream audit in `make vqa` (scroll burst →
raw bytes → no 2J, guards present), and the tui-qa agent's mandatory
render-hygiene check.
