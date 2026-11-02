# Task 460 — Pace, Part 2: Slow Jobs, Matrix Legs And Steps

## Status

IN PROGRESS — issue #75. Builds on run-level Pace (#74, task 450).

## Ownership Boundary

- **Primary area:** job and step baselines (`internal/usecase/pace_jobs.go`), the detail view's Pace cells and pace line, the `pace` verb's `jobs[]` / `slow[]`.
- **Allowed files:** `internal/usecase/pace*.go`, `internal/render/pace*.go`, `internal/render/testdata/{schema.json,pace*.golden.json}`, `cmd/gh-hound/{pace*.go,main.go}`, `internal/adapter/fake/{paced.go,fake.go}`, `internal/tui/{pace*.go,app.go}`, `internal/tui/screens/detail/`, `internal/tui/keys/`, `.claude/automations/`, `docs/`, `README.md`.

## Decisions

- **Jobs and steps sample the run baseline's runs.** No second walk and no separate fallback (the issue's open question): a job baseline reads the jobs of the runs the run-level baseline already chose, widened or not, so the run row and its jobs are judged against the same history. A job that appears in fewer than 5 of them is thin.
- **Exactly `window` job listings per key.** The run baseline keeps one spare sample; the job walk leaves the judged run out and reads at most `window` of the rest. A listing is one call per 100 jobs (GitHub's page size), so a matrix past 100 legs costs more pages, not more runs.
- **Jobs of a finished sample run never change**, so they are cached per run ID for the session; a refreshed run baseline lists only its new runs.
- **Keys.** Jobs by full name (`test (1.25)`, `caller / callee`). Steps by job key plus the step name, with `Run owner/action@<ref>` (and `Post Run …`) normalised to drop the ref, so a Dependabot bump of a pinned action keeps its baseline. Explicit `name:` steps are verbatim. Never by step number.
- **Samples are green work.** A sample job counts only when it concluded `success`; a step only when it concluded `success` (a skipped step inside a green run is not a sample).
- **Durations are whole seconds** (`completed_at − started_at`; GitHub stamps steps to the second). The detail view drops `4.0s` for `4s` everywhere, so measured and judged cells agree.
- **Slow rule** as for runs: past p90, at least 1.5× p50, and at least 30s (jobs) or 5s (steps) over p50. A running job or step past the line is an overrun and reads `↑` too.
- **Fetched lazily, on poll ticks only, one at a time,** while the detail view is open: the run baseline first, then the job baseline built on it; rendering only reads the cache. `p` hides it on the detail screen as everywhere.
- **A failed run's green work is judged.** The run-level key refuses failed runs (a failed run has no run verdict), but the detail view is mostly opened on failures and their green steps are what this is for, so it keys by workflow, branch and event whatever the outcome; for green first attempts the key is the same, so the cache is shared.
- **`slow[]` never changes `status` or the exit code**: those stay the run's, as agents already branch on them. `slow` is `[]` when nothing ran slow and absent with `--no-steps`.
- **The re-run ETA in the issue is not here.** It needs job baselines for runs-list rows (window listings per re-run row), which the runs list never pays; it belongs with a design for that cost.

## Task List

- [x] T1 Usecase: `stepKey`, `PaceService.JobBaseline`, `AssessJobs`; table tests, a counting fake pinning `window` listings, fuzz.
- [x] T2 Detail view: whole-second durations, `↑` on job and step cells, `p50 …` in the steps pane header, the focused-step pace line inside the `stepWindow` budget, `· p pace` in the hint.
- [x] T3 App: lazy job-baseline fetch, per-run jobs cache, `pace_toggle` on `ScreenDetail`.
- [x] T4 CLI: `jobs[]` and `slow[]` in `$defs.pace_result`, `--no-steps`.
- [x] T5 Fixtures `detail-pace`, `detail-pace-thin`; VQA assertions; docs.

## Found Along The Way

- The detail view printed `0.4s`, `18.7s` for steps GitHub stamps to the second. Durations are whole seconds now (existing).
