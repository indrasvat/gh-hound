# Task 470 — Runs That Never Ran: Verdicts And Reasons

## Status

IN PROGRESS — issue #85. Builds on the `startup_failure` work in #83/#84.

## Ownership Boundary

- **Primary area:** the red-run verdict (`internal/model/verdict.go`), its use case (`internal/usecase/verdict.go`), the runs list, the detail view and the `runs` pipe.
- **Allowed files:** `internal/model/`, `internal/usecase/{verdict,triage,flakes,diff,ports}*.go`, `internal/adapter/github/{dto,client,mutations}*.go`, `internal/adapter/fake/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/{app.go,*_test.go}`, `internal/tui/screens/{runs,detail}/`, `internal/tui/icons/`, `internal/tui/keys/`, `internal/theme/`, `.claude/automations/`, `docs/`, `skills/gh-hound/`, `README.md`.

## Spike (22 Nov 2026, live API)

- **cli/cli:** 29 of the newest 30 `status=failure` runs are fork `pull_request` runs with 0 jobs and 0 check runs on their check suite.
- **Approval missed is its own state.** A fork run waiting on approval is `action_required` (created = updated). When the PR closes, GitHub moves it to `failure`: the run's `updated_at` matches the PR's `closed_at` to the second (PRs 15128, 15127, 15107, 15089). Some runs of a closed PR stay `action_required`.
- **A blocked `pull_request_target` run is `startup_failure`.** Every brownout run found (vercel/next.js, facebook/react; 9 and 16 Nov 16:00–18:00 UTC) reads `startup_failure`, event `pull_request_target`.
- **The block's text is not in the API.** REST check runs, GraphQL `CheckSuite.checkRuns.annotations` and `gh run view` carry nothing; only the web page says "blocked due to a scheduled brownout of the default `pull_request_target` protection". Hound names the reason from the event and conclusion and points at the run page. It does not scrape HTML.
- **`/jobs` can list a check run that is no job.** The next.js blocked run lists one "job" with 0 steps and no runner (another workflow's API-created check run). "No job ran" means no job has steps or a runner, not `total_count == 0`.

## Decisions

- **`conclusion` and `status` stay GitHub's.** The mock put `never_ran` in `status`; that would break every `status == "completed"` check and the watch stream. A red run gains `verdict` (`code` | `never_ran` | `infra`) and `reason` instead. They are absent on runs that are not red.
- **Reasons** (enum, documented in `docs/agent-surface.md`):
  - `never_ran`: `awaiting_approval` (`action_required`, waiting on a write collaborator), `approval_missed` (a PR run ended `failure` before any job ran: the approval never came), `blocked_by_policy` (`startup_failure` on `pull_request_target`, the 2 Dec default rule or an event policy), `could_not_start` (any other `startup_failure`: invalid workflow file or a blocked action), `no_job_ran` (any other red run with no job that ran).
  - `infra`: every failing job carries a platform signal in its annotations or exit code: `runner_lost` (lost communication, shutdown signal), `killed` (exit 137, usually out of memory), `disk_full` (no space left on device). One failing job without a signal makes the run `code`.
  - A timeout stays `code`: a hang is usually the code's.
- **Exit codes.** `awaiting_approval` exits 3 (pending: a person has to act, a watch loop waits). Every other red verdict exits 1, now with a `reason`.
- **Cost.** Run fields alone decide `action_required` and `startup_failure` (zero calls). A red `failure`/`timed_out` run needs its jobs and its failing jobs' annotations; the pipe already fetches both for `failed[]`, so it pays nothing extra. The TUI fetches them off the key path, one run at a time per poll tick, cached per run and attempt (a finished attempt never changes), only for red rows that are loaded.
- **Approve.** `A` on a held fork run (list or detail) asks `Approve N runs from @login's fork? Their code will run. y/N`, default No, then `POST …/actions/runs/{id}/approve` for each held run of that head commit, through the one-per-second mutation queue. `A` already opens deploy gates on waiting runs; it means "approve" in both places. The endpoint covers public-fork runs only (docs: first-time contributors, `repo` scope); a held bot or Copilot run shows `o approve in the browser` instead.
- **flakes and diff** take no evidence from a `never_ran` run. diff keeps `could_not_start` red (the commit's workflow file did not parse) but no other `never_ran` reason.

## Task List

- [ ] T1 Domain: `model.RunVerdict`, reasons, `Job.Ran`, `Run.FromFork`; pure `ClassifyRun` with table tests.
- [ ] T2 Use case: `VerdictService.Classify`; triage returns the verdict; flakes/diff skip `never_ran`; port `RunApprover`.
- [ ] T3 Adapters: `head_repository`/`repository` decode, `ApproveRun`; fake scenario `never_ran` with every reason.
- [ ] T4 Pipe: `verdict`, `reason`, exit 3 for `awaiting_approval`; schema, goldens, docs, skill.
- [ ] T5 TUI runs list: glyphs, Duration/label words, summary counts, async verdict fetch.
- [ ] T6 TUI detail: "why this run never ran" pane; `A` approve confirm; hint line.
- [ ] T7 VQA fixtures and assertions at 80x24, 120x40, 200x60; visual contract and mock.

## Found Along The Way
