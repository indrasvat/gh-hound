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
| ⑥ | log | line-number gutter; fold rows; search hit tint; ANSI plus syntax accents; decorative scrollbar; t opens a time-jump input (header echoes t→HH:MM:SS) |
| ⑦ | toasts_dispatch | toast layer over dimmed base and dispatch form with typed inputs |
| ⑧ | palette | centered overlay; prompt row; filtered list; selected row surface-2 fill plus green left bar |
| ⑨ | approvals | deploy-gate overlay over the runs screen: env multi-pick rows ([x]/[ ]/[-] checkboxes, ◫ glyph, reviewers line), comment line with documented default, notice line for refusals; opened by A on a waiting run after the shared "checking the gate" load |
| ⑩ | help | three-column contextual keymap; legend; Canvas/Layer overlay |

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
| log | j/k scroll · g/G ends · / search · t time · n/N match · z/Z fold · w wrap · ⎋ back |
| dispatch | ⏎ run · ⇥ next · ⎋ cancel |
| palette | workflows · watch · diff (v2) · theme |
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
| detail open (`⏎`) | skeleton with run header + repo breadcrumb; loading line in the jobs pane |
| failure open | shared loading body |
| log open (`l`) | shared loading body with byte progress (`▰▰▱▱▱ 2.1 MB/4.8 MB`) when Content-Length is known |
| dispatch open (`D`) | loading line on the originating runs screen; route flips when resolved |

Palette note: `:` opens with the generic dispatch entry only — the
per-workflow `dispatch: <name>` items appear once the workflow cache
is warm (after the first dispatch open), because enriching on the
keystroke would violate the invariant.

Spinner: braille cycle (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`), ~120ms/frame, run color;
labels are hound-voiced and passed per context. Fixture screens:
`runs-loading`, `detail-loading`, `failure-loading`, `log-progress`.

Approvals: `A` on a waiting run fetches the gate list through the
shared loader (kind `approvals`, label "checking the gate") before the
overlay opens; the runs list shows the standard loading line meanwhile.
Waiting runs wear the ◫ gate badge in the status column and the summary
line advertises `◫ N gated · A review` only while one is on screen.
Approve and reject are both confirm-gated ("open the gate for …?" /
"keep the gate shut for …?"); verdict toasts: "gate's open." and
"gate stays shut.". Fixture screens: `runs-waiting`, `approvals`,
`detail-pending`.
