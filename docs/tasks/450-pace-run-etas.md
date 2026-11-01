# Task 450 — Pace: Run ETAs And Slow-Run Flags

## Status

IMPLEMENTED — issue #74. Job, matrix-leg and step flags and the detail-view pace line are #75, gated on these run-level ETAs proving accurate on real repositories.

## Ownership Boundary

- **Primary area:** Pace baselines (`internal/usecase/pace.go`), the `pace` verb, and the Duration cell, hunt board State cell and headline, and watch header.
- **Allowed files:** `internal/usecase/pace*.go`, `internal/render/pace*.go`, `internal/render/testdata/{schema.json,pace.golden.json}`, `cmd/gh-hound/{pace*.go,main.go}`, `internal/adapter/fake/{paced.go,fake.go}`, `internal/adapter/github/endpoints_guard_test.go`, `internal/config/config*.go`, `internal/tui/{pace*.go,app.go}`, `internal/tui/screens/{runs,watch}/`, `internal/tui/keys/`, `internal/tui/icons/`, `internal/tui/components/sparkline/` (removed), `.claude/automations/`, `docs/`, `README.md`.

## Decisions

- **The baseline matches the event and widens by branch, not to the default branch.** Samples are the last `pace_window` green first attempts of the same workflow, branch and event. Below 5 the baseline widens to the same event on any branch. The issue first said "fall back to the default branch", then "the default branch with the same event"; on this repository a `pull_request` run has 0 green `pull_request` runs on `main` and 134 across PR branches, because `pull_request` runs carry the PR's head branch. Event matching keeps `push`-only deploy legs and slow `schedule` runs out of a PR's baseline.
- **The judged run is excluded from its own baseline.** With 5 samples nearest-rank p90 is the maximum, so a slow run counted in its own sample could never exceed it.
- **ETAs are whole minutes, rounded up, and always read `left`:** `<1m left`, `3m left`, `~1h left`, and `>9h left` from ten hours on, so every form fits the 8-cell Duration column. Measured durations read `38s`, `4m05s`, `1h05m`.
- **`↑` follows the slow rule, not p90 alone.** A run is slow (or a running run overrun) when it is past p90, at least 1.5x p50 and at least 30s over p50. Between p50 and that line a running run reads `due`.
- **`not_comparable` is a status of its own** (exit 0): queued or waiting runs, re-run attempts and finished runs that did not succeed say nothing against a green-run baseline; `run.reason` says which. The issue's status list had no word for them.
- **The verb answers with `pace = false`.** The switch turns off the TUI annotations and their requests; `gh hound pace` is asked for by name (the `flakes` verb and `flake_badges` precedent).
- **The TUI fetches baselines for the rows loaded on the current screen** (runs list, hunt board, watched run), one key at a time on poll ticks; rendering only reads the session cache. A baseline refreshes after 15 minutes or when a green run of its key appears that it has not accounted for. Staleness is run membership, never a timestamp comparison: a clock ahead of the local one refetched on every tick in a first version.
- **One `pace unavailable` toast per session**, not per key: a rate-limited hour would otherwise stack a toast per workflow. A failed key is retried after 15 minutes.
- **`p` back on clears the off toast instead of adding `pace on`**, which covered the very cells it announced.
- **The CLI uses `--format md|xml`**, as every verb does; the issue's `--md`/`--xml` flags do not exist.

## Task List

- [x] T1 Usecase: `PaceService.Baseline`, `AssessPace`, `PaceMarkFor`, formatters; table tests and three fuzz targets (assessment contract, ETA width, baseline walk budget and filters).
- [x] T2 Render and CLI: `$defs.pace_result`, golden JSON, schema conformance for every status, exit codes, `paced` fake scenario, `pace` verb, `pace`/`pace_window` config, timing/billing guard test.
- [x] T3 TUI: Duration cell (the placeholder sparkline and its component removed), 80-column label-column notes, hunt board State and `all home in ~…`, watch header, `p` (`pace_toggle`, remappable, in help), async fetch lifecycle tests, layout fuzz.
- [x] T4 Fixtures `runs-pace`, `watch-pace`, `watch-board-pace` with VQA assertions; docs.

## Found Along The Way

- `drainPaceFetch` returned "unchanged" for a cancelled fetch, so the caller dropped the copy that cleared it and no later fetch could start: Pace wedged for the session after any cancel. Caught by `TestPaceCancelledFetchIsSilent`.
- Staleness first compared GitHub's `updated_at` with the local clock: 392 fetches in a second once the run's timestamp ran ahead. Now by run membership.
- `FuzzPaceRows`: a Pace note could push the Actor column right when the label column was narrower than the note. The note now needs room for a 6-cell label, and the label is clamped.
- `FuzzAssessPace`: `~10h left` is 9 cells. Now `>9h left`. `FuzzFormatPaceETA` missed it because it skipped out-of-range inputs; it folds them into range now.
- `runLabel` forced a 6-cell name plus ` · …` at any width, overflowing narrow columns (existing). The detail now goes first.
- Recorded, not changed: toasts sit over the top-right rows, including their Duration cells (the toast layer's design); the pre-commit hook runs whichever Go is first on `PATH`, and Go 1.27 rewrites files in `gofix-check` (use the pinned 1.26 toolchain).
