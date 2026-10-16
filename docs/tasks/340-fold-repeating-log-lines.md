# Task 340 — Fold Repeating Log Lines

## Status

IN PROGRESS — issue #58 (also closes #59). The approach was proven in a CLI spike (labelled synthetic corpus, 99 public GitHub Actions logs, fuzzing, two adversarial review rounds) before this task; this task ports it into the product.

## Ownership Boundary

- **Primary area:** the full-log screen's compact view (fold rows for repeating lines, `c`, row-based scrolling, gutter, footer), the folding engine, and repeat-aware `log_excerpt` / `repeats` output.
- **Allowed files:** `internal/logs/` (new `fold` subpackage; parser only for #59), `internal/tui/screens/log/`, `internal/tui/app.go`, `internal/tui/loading.go`, `internal/tui/keys/`, `internal/tui/overlay/help/`, `internal/usecase/triage.go`, `internal/usecase/ports.go`, `internal/render/`, `internal/config/`, `internal/adapter/fake/`, `cmd/gh-hound/`, `.claude/automations/`, `docs/`, `skills/gh-hound/SKILL.md`, `README.md`, `pages/`.
- **Avoid touching:** theme tokens, other screens' layouts, the parser's tokenizer and failure-window rules, watch/live streaming.

## Decisions

- **Compact is the default.** `log_compact` (config) / `HOUND_LOG_COMPACT` (env) set the initial view; `c` toggles per log. When a log has nothing to fold, the header and title stay exactly as today.
- **Projection, never rewrite.** Folding produces a plan over `logs.Document.Lines`; line numbers stay the parser's numbers in both views, and expanding every fold reproduces the document.
- **Off the input path.** The plan is computed by the log resolver's goroutine (runs/detail paths) or a `startLoad` fold job (failure path, which reuses the already-parsed report). The screen renders the full log first and swaps to compact when the plan lands; a superseded or failed plan is dropped. `SafePlanContext` has a 2 s budget; on deadline, panic, failed self-check or the 5M-line limit the full log is shown and nothing is hidden. Range zooms (`t` ranges) plan just the range synchronously with a 50 ms budget, falling back to the full lines of that range.
- **Safety rules (checked on every plan, O(n)):** lossless expansion; pinned lines are never hidden; every hidden line has a visible line of the same shape earlier in its fold (the head copy is always kept); step-group lines and the first line after each group are pinned so a repeat never straddles a group boundary; lines over 16 KB are pinned.
- **What counts as "the same line":** a single-pass scanner masks times, hex/pointers, uuids, hashes and plain numbers; names with ordinals (`shard-7`, `TestFoo2`, `abc100`, `b200`) and short numbers right after outcome words (`exit`, `code`, `status`, `rc`, `state`, `phase`, `result`, `reason`, `http`, `signal`, `errno`, `conclusion`) stay literal.
- **What repeats:** back-to-back blocks of 1–32 lines repeating ≥3 times, and stretches built from ≤32 recurring shapes in any order (≥20 lines, ≥half hidden). Region by region the better explanation wins.
- **What stays visible inside a fold:** in a low-cardinality slot (≤4 values), a rare value (all but max(1, 10%) share one value) and the first line of each new plateau (≤ max(2, 10%) changes). Refinement repeats until stable (≤8 rounds; leftover objections drop the fold).
- **Error-looking lines are not pinned** (they are mostly `0 failed` summaries and the repeated errors users want folded); a fold that hides them says `⚠ N error-like inside` in fail colour.
- **Excerpt:** when the failure window or the tail fallback contains folded repeats, `log_excerpt` renders them as one `⋯ <summary>` line and extends backwards until it has the same amount of visible context. `repeats` is additive and optional in JSON/MD/XML: up to 5 folds, largest first, with start/end line, cycles, block size, hidden lines, first/last clock and summary.
- **Gutter** pads to the widest line number in the document (minimum 3 digits) so rows stay aligned past line 999.
- **Footer** is width-aware: bindings carry a drop priority; at narrow widths the lowest-priority hints drop before `⎋ back` (which never drops). `w wrap` wraps long lines onto continuation rows (dim `↪` gutter); scrolling stays in logical rows.

## Fold Row

```
0008 ▸ ×720 2-line block · 16:12:11 → 18:12:02 · 3 changing ids  1,436 lines
1203 ▸ 2 recurring lines · 09:58:02 → 10:11:21 · 1 changing id  797 lines
0412 ▾ ×12 · 0 → 11 (12 values)  10 lines          (opened: lines follow)
```

Same surface, glyphs and count styling as group folds (`renderFold`). The gutter number is the first hidden line. Summary order: shape (`×N k-line block` or `K recurring lines`), time span, low-cardinality values (`0|137`), numeric ranges (`10 → 99 (90 values)`), changing-id count, `⚠ N error-like inside`. It truncates with `…` from the right; the count never truncates.

## Header And Title

| State | Header (row 0) | Title bar right |
| --- | --- | --- |
| compact, folds present | `log · compact` + gauge | `1,448 lines → 13 rows` |
| full (after `c`) or nothing to fold | `log` + gauge | `1,448 lines` |
| search | `log · /q · match c/t · compact` | `match c/t` |
| time jump | `log · t→HH:MM:SS · compact` | as above |
| folding in progress | `log` | `1,448 lines · folding…` |

## Keys

| Key | Compact behaviour |
| --- | --- |
| `c` | toggle compact/full, keeping the top line (or the fold that holds it) at the top |
| `z` / `Z` | close / open every fold: step groups and repeats |
| `/` `n` `N` | matches are line numbers; the current match opens the fold that holds it (repeat or group) |
| `t` | jumps and ranges land on folded lines the same way |
| `j` `k` `g` `G` `^d` `^u` | move by visible rows; `G` fills the screen |
| `w` | wrap long lines |

## Task List

- [ ] T1 Task doc, visual contract (⑥ log + ⑥b log compact), design reference (fold row, header/title states, footer at 80 columns).
- [ ] T2 `internal/logs/fold`: interfaces (`Normalizer`, `Slotter`, `KeySpanner`, `Guard`, `Detector`, `Refiner`), `Shape`, `Cycle`/`Set`/`Consecutive`, `Structural`, `Outlier`, `Engine`, `Plan`, `SafePlanContext`, `Summarize`, registry, `ForDocument`. Invariant, fuzz, regression and benchmark tests; labelled synthetic corpus test with thresholds.
- [ ] T2b Parser (#59): lines over the scanner limit no longer end parsing; the rest of the log is kept. Regression test with a 2 MiB line.
- [ ] T3 Log screen: row projection (cached), compact state, fold rows, `c`, `z`/`Z`, search/time/range reveal, row-based scrolling, gutter, header/title states, wrap.
- [ ] T4 App: plan on the resolver goroutine; failure-path fold load via `startLoad`; supersession; config default; footer priority and width; help overlay.
- [ ] T5 Config: `log_compact` / `HOUND_LOG_COMPACT`.
- [ ] T6 Triage + render: repeat-aware `log_excerpt`, additive `repeats` (JSON/MD/XML), schema, golden.
- [ ] T7 Fakes and fixtures: `noisy` fake scenario (poll loop, retry block, interleaved workers, a rare exit code, a step group), `__screen` fixtures `log-compact` / `log-compact-open` / `log-full`, assertions, interaction scenarios (`c`, search into a fold, `z`/`Z`, `G`, time jump), vqa list.
- [ ] T8 Docs: README, configuration, agent surface, skill, help.
- [ ] T9 Verification: `make ci`, `make vqa`, interaction audit, render hygiene; shux sessions on fakes and real repos (read-only), both themes, 256-colour, 80x24/120x40/200x60; cold `tui-qa` rounds; adversarial review; each finding fixed with a test.
- [ ] T10 Website: "Best in show" card + iTerm2 retina shot of compact mode.
- [ ] T11 PR closing #58 with evidence comment (screenshots, demo video) and the review loop.

## Found Along The Way

- At 80 columns the existing log footer is 82 characters; `fitPlain` cut `⎋ back`. Fixed by the width-aware footer (T4).
- `w wrap` toggled `Model.Wrap` but `View` never read it. Implemented (T3).
- `maxTop` was counted in lines, so `G` left the screen short when group folds were collapsed. Fixed by row-based scrolling (T3).
- The `%03d` gutter misaligned past line 999. Fixed (T3).
- `logs.Parse` stopped at the first line over 1 MiB without checking `scanner.Err()`, dropping the rest of the log (#59). Fixed here (T2b) because folding promises the whole log.
- Out of scope, filed: watch re-fetches every completed job's log on each tick (#60); follow-up PR.
