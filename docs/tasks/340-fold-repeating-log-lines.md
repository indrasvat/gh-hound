# Task 340 — Fold Repeating Log Lines

## Status

IMPLEMENTED — issue #58 (also closes #59), PR #61. The approach was proven in a CLI spike (labelled synthetic corpus, 99 public GitHub Actions logs, fuzzing, two adversarial review rounds) before this task; this task ports it into the product. `make ci`, `make vqa`, interaction audit and render hygiene pass; live-verified read-only on pytorch, cilium, docker/buildx, flutter and openclaw; an adversarial review and three QA rounds, each finding fixed with a test. Pre-existing defects found along the way are filed as #62. Remaining: the PR review loop (T11).

## Ownership Boundary

- **Primary area:** the full-log screen's compact view (fold rows for repeating lines, `c`, row-based scrolling, gutter, footer), the folding engine, and repeat-aware `log_excerpt` / `repeats` output.
- **Allowed files:** `internal/logs/` (new `fold` subpackage; parser only for #59), `internal/tui/screens/log/`, `internal/tui/app.go`, `internal/tui/loading.go`, `internal/tui/keys/`, `internal/tui/overlay/help/`, `internal/usecase/triage.go`, `internal/usecase/ports.go`, `internal/render/`, `internal/config/`, `internal/adapter/fake/`, `cmd/gh-hound/`, `.claude/automations/`, `docs/`, `skills/gh-hound/SKILL.md`, `README.md`, `pages/`.
- **Avoid touching:** theme tokens, other screens' layouts, the parser's tokenizer and failure-window rules, watch/live streaming.

## Decisions

- **Compact is the default.** `log_compact` (config) / `HOUND_LOG_COMPACT` (env) set the initial view; `c` toggles per log. When a log has nothing to fold, the header and title stay exactly as today.
- **Projection, never rewrite.** Folding produces a plan over `logs.Document.Lines`; line numbers stay the parser's numbers in both views, and expanding every fold reproduces the document.
- **Off the input path.** The plan is computed on the log and failure resolvers' goroutines, so it arrives with the log (no raw-then-compact flash); the failure view's `l` reuses that folded model instead of rebuilding one on the key path. `SafePlanContext` has a 2 s budget; on deadline, panic, failed self-check or the 5M-line limit the full log is shown and nothing is hidden. Range zooms (`t` ranges) plan just the range synchronously with a 50 ms budget, falling back to the full lines of that range.
- **Safety rules (checked on every plan, O(n)):** lossless expansion; pinned lines are never hidden; every hidden line has a visible line of the same shape earlier in its fold (the head copy is always kept); step-group lines and the first line after each group are pinned so a repeat never straddles a group boundary; lines over 16 KB are pinned.
- **What counts as "the same line":** a single-pass scanner masks times, hex/pointers, uuids, hashes and plain numbers; names with ordinals (`shard-7`, `TestFoo2`, `abc100`, `b200`) and short numbers right after outcome words (`exit`, `code`, `status`, `rc`, `state`, `phase`, `result`, `reason`, `http`, `signal`, `errno`, `conclusion`) stay literal.
- **What repeats:** back-to-back blocks of 1–32 lines repeating ≥3 times, and stretches built from ≤32 recurring shapes in any order (≥20 lines, ≥half hidden). Region by region the better explanation wins.
- **What stays visible inside a fold:** in a low-cardinality slot (≤4 values), a rare value (all but max(1, 10%) share one value) and the first line of each new plateau (≤ max(2, 10%) changes). Refinement repeats until stable (≤8 rounds; leftover objections drop the fold).
- **Error-looking lines are not pinned** (they are mostly `0 failed` summaries and the repeated errors users want folded); a fold that hides them says `⚠ N error-like inside` in fail colour.
- **Excerpt:** when the failure window or the tail fallback contains folded repeats, `log_excerpt` renders them as one `⋯ <summary>` line and extends backwards until it has the same amount of visible context. `repeats` is additive and optional in JSON/MD/XML: up to 5 folds, largest first, with start/end line, cycles, block size, hidden lines, first/last clock and summary.
- **Gutter** pads to the widest line number in the document (minimum 3 digits) so rows stay aligned past line 999.
- **`z` closes every fold**, including the one holding the current search match; `n`/`N` reopen it. (Before, a collapsed group still drew its lines when the offset sat inside it.)
- **A view near the end fills the screen** from earlier rows; `Offset` stays the jump target.
- **Footer** is width-aware: bindings carry a drop priority; at narrow widths the lowest-priority hints drop before `⎋ back` (which never drops). `w wrap` wraps long lines onto continuation rows (dim `↪` gutter); scrolling stays in logical rows.

## Fold Row

```
0008 ⋯ ×720 2-line block · 16:12:11 → 18:12:02 · 3 changing ids       1,436 lines hidden
1203 ⋯ 2 recurring lines · 09:58:02 → 10:11:21 · 1 changing id          797 lines hidden
     ▾ ×12 · 0 → 11 (12 values)                                                 10 lines
```

Distinct from group folds so they read as "repeats" at a glance: an info-tinted band, an info `⋯` marker (`▾` when opened), the shape in bold info, the summary in fg-soft, and the hidden-line count dim and right-aligned. The gutter number is the first hidden line; an opened fold leaves it to that line and its count reads `N lines`. Summary order: shape (`×N k-line block` or `K recurring lines`), time span, low-cardinality values (`0|137`), numeric ranges (`10 → 99 (90 values)`), changing-id count, `⚠ N error-like inside`. Width goes to the shape, the count (`N lines hidden`, then `N hidden`, then `N`), the warning, and last the values, which truncate with `…`. Runs hiding fewer than 5 lines stay as lines.

## Header And Title

| State | Header (row 0) | Title bar right |
| --- | --- | --- |
| compact, folds present | `log · compact · 1,446 lines folded in 2 places` + gauge | `1,448 lines → 13 rows` |
| full (after `c`) or nothing to fold | `log` + gauge | `1,448 lines` |
| search | `log · /q · match c/t · compact` | `match c/t` |
| time jump | `log · t→HH:MM:SS · compact` | as above |

## Keys

| Key | Compact behaviour |
| --- | --- |
| `c` | toggle compact/full, keeping the top line (or the fold that holds it) at the top |
| `z` / `Z` | close / open every fold: step groups and repeats |
| `]` / `[` | jump to the next / previous repeat fold |
| `/` `n` `N` | matches are line numbers; the current match opens the fold that holds it (repeat or group) |
| `t` | jumps and ranges land on folded lines the same way |
| `j` `k` `g` `G` `^d` `^u` | move by visible rows; `G` fills the screen; with `w`, `j`/`k` step through a line taller than the screen |
| `w` | wrap long lines |

## Task List

- [x] T1 Task doc, visual contract (⑥ log + ⑥b log compact), design reference (fold row, header/title states, footer at 80 columns).
- [x] T2 `internal/logs/fold`: interfaces (`Normalizer`, `Slotter`, `KeySpanner`, `Guard`, `Detector`, `Refiner`), `Shape`, `Cycle`/`Set`/`Consecutive`, `Structural`, `Outlier`, `Engine`, `Plan`, `SafePlanContext`, `Summarize`, registry, `ForDocument`. Invariant, fuzz, regression and benchmark tests; labelled synthetic corpus test with thresholds.
- [x] T2b Parser (#59): lines over the scanner limit no longer end parsing; the rest of the log is kept. Regression test with a 2 MiB line.
- [x] T3 Log screen: row projection (cached), compact state, fold rows, `c`, `z`/`Z`, search/time/range reveal, row-based scrolling, gutter, header/title states, wrap.
- [x] T4 App: plan on the resolver goroutine; failure-path fold load via `startLoad`; supersession; config default; footer priority and width; help overlay.
- [x] T5 Config: `log_compact` / `HOUND_LOG_COMPACT`.
- [x] T6 Triage + render: repeat-aware `log_excerpt`, additive `repeats` (JSON/MD/XML), schema, golden.
- [x] T7 Fakes and fixtures: `noisy` fake scenario (poll loop, retry block, interleaved workers, a rare exit code, a step group), `__screen` fixtures `log-compact` / `log-compact-open` / `log-full`, assertions, interaction scenarios (`c`, search into a fold, `z`/`Z`, `G`, time jump), vqa list.
- [x] T8 Docs: README, configuration, agent surface, skill, help.
- [x] T9 Verification: `make ci`, `make vqa`, interaction audit, render hygiene; shux sessions on fakes and real repos (read-only), 256-colour, 80x24/120x40/200x60 and live resizes; cold `tui-qa` rounds; adversarial review; each finding fixed with a test. The log screen's colours stay the dark palette under Bone, as before this task (#56).
- [x] T10 Website: "Best in show" card + iTerm2 retina shot of compact mode.
- [ ] T11 PR closing #58 with evidence comment (screenshots, demo video) and the review loop.

## Measurements

- `BenchmarkView10kLineViewport`: 6.0 µs/op (was 4.8; the projection lookup and per-row slices). `BenchmarkViewCompact300kViewport` (every fold open, 40 rows): 20 µs/op.
- `BenchmarkSafePlan` (200k lines, 14.7 MB): ~125 MB/s, 71 MB/op allocated.
- A 3M-line plan under a 30 ms deadline returns the raw view in ~58 ms.
- A closed 300k-line fold: ~12 µs per frame (`BenchmarkViewCompact300kClosed`); it was 127–399 ms when titles were summarized every frame.
- 60k lines of 3,000 bytes under a 2 s budget: falls back at 2.00 s (was 4.24 s before refinement polled the deadline).
- `FuzzViewport` (keys, jumps, ranges, search, random resizes against a flattened-screen oracle): 850k execs clean, then 120 s more on the final tree.

## Found Along The Way

- At 80 columns the existing log footer is 82 characters; `fitPlain` cut `⎋ back`. Fixed by the width-aware footer (T4).
- `w wrap` toggled `Model.Wrap` but `View` never read it. Implemented (T3).
- `maxTop` was counted in lines, so `G` left the screen short when group folds were collapsed. Fixed by row-based scrolling (T3).
- The `%03d` gutter misaligned past line 999. Fixed (T3).
- Failure → `l` landed on the failure anchor of a long log with most of the screen empty; the view now fills from earlier rows.
- `logs.Parse` stopped at the first line over 1 MiB without checking `scanner.Err()`, dropping the rest of the log (#59). Fixed here (T2b) because folding promises the whole log.
- Adversarial review (codex + agy), each fixed with a failing-first test: short-stamped `::error::` lines could fold (the guard and parser disagreed on timestamps; parsed commands and annotations are now pinned); repeat titles were summarized every frame; refinement ignored the deadline; model copies shared fold maps and the row cache; reveal opened repeats before their group; space/up/down could change the view without a repaint; narrow repeat rows dropped the warning; excerpts summarized past the failure window; empty `<repeats>` XML wrapper.
- `G` panicked in wrap mode when the last line wrapped taller than the screen, and such lines had unreachable continuations. Scroll position is now a (row, wrapped line) pair clamped against one bottom; `FuzzViewport` guards it.
- Live walk: open repeat headers said "lines hidden"; group glyphs were dim, not ok as in the mock; the failure title did not group digits. Fixed.
- `encoding/xml` writes an empty wrapper for `a>b,omitempty`; `next_commands` in the queue output has the same quirk (pre-existing, to file).
- Out of scope, filed: watch re-fetches every completed job's log on each tick (#60); follow-up PR.
