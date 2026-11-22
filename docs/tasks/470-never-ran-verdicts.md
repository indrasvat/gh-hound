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
- **Approve.** `A` on a held fork run (list or detail) asks `Approve N runs from @login's fork? Their code will run. y/N`, default No, then `POST …/actions/runs/{id}/approve` for each held run of that head commit, through the one-per-second mutation queue. `A` already opens deploy gates on waiting runs; it means "approve" in both places. The endpoint covers public-fork runs only (docs: first-time contributors, `repo` scope); a held bot or Copilot run gets an info toast "approve this one on GitHub" instead. Write access is checked first (`GET /repos/{o}/{r}` `permissions.push`), and both the check and the approvals run behind a load, never on the key path.
- **flakes and diff** take no evidence from a `never_ran` run. diff keeps `could_not_start` red (the commit's workflow file did not parse) but no other `never_ran` reason.

## Task List

- [x] T1 Domain: `model.RunVerdict`, reasons, `Job.Ran`, `NoJobRan`, `Run.FromFork`; pure `ClassifyRun` with table tests.
- [x] T2 Use case: `VerdictService.Classify`; triage returns the verdict; flakes/diff skip `never_ran`; port `RunApprover`, `ActionService.ApproveRun`.
- [x] T3 Adapters: `head_repository`/`repository` decode, `ApproveRun`; fake scenario `never_ran` with every reason.
- [x] T4 Pipe: `verdict`, `reason`, exit 3 for `awaiting_approval`; schema (enums pinned to the model), golden, docs, skill.
- [x] T5 TUI runs list: ∅ / ↯ glyphs, Duration/label words, summary counts with a width budget, async verdict fetch.
- [x] T6 TUI detail: "Why this run never ran" pane, `detail_never_ran` footer, `A` approve confirm.
- [x] T7 VQA fixtures `runs-never-ran`, `approve-confirm`, `detail-never-ran`, `detail-blocked` at 80x24, 120x40, 200x60; visual contract, PRD and mock.

## Decisions Made While Building

- **Glyphs.** The mock's `○` is East Asian ambiguous-width (the class the contract warns mis-renders under capture), so never ran is `∅` (U+2205, narrow). The proposed `⌁` for infra was indistinguishable from the branch glyph `⎇` in the header at real size, so infra is `↯` (U+21AF).
- **A job that failed without a runner is real.** "No job ran" ignores only a listed check run that neither ran nor failed (the next.js phantom). A job that failed before any runner took it (no matching label) still makes the run `code`: its annotations explain it.
- **A held run exits 3 only when nothing else needs action.** In a repo-wide listing, held fork runs at the top must not hide a code failure further down.
- **Approving from detail returns to the list.** The detail of a just-approved run is stale; the next poll shows it queued.
- **The board's lost count is unchanged.** A hunt that settles with only held runs exits 3, but `watch --group`'s summary still counts them as GitHub's red.

## Found Along The Way

- `A` on a waiting run's detail did nothing, though the gate pane said "A review". Fixed here.
- #84 counted a `startup_failure` as failing in the summary; under this issue it counts as never ran (`could_not_start`), and diff still treats it red.
- Both golangci-lint binaries on the dev machine are v2.15 built with Go 1.27 and fail the hook on files nobody touched; the repo pins v2.8.0 (`make tools-ci`).

## Review Round 1 (takshaka, codex via dootsabha, cold tui-qa)

Fixed:

- Approving ran paced POSTs on the keystroke (frozen ~1s per extra run): now behind a load; a partial approval says how many went through.
- `A` offered approval with no access check: write access is read first.
- On a never-ran detail, `r R x X w l ⏎` still acted (a rerun of the phantom check run): they toast why instead.
- A failed detail load read as "never ran" (empty skeleton): only a loaded empty listing counts.
- Two forks can push the same commit: one approval batches only the selected fork's runs.
- Jobs of a re-run's attempt could classify the attempt shown: verdicts read the run's own attempt, and jobs of another attempt make the verdict unknown.
- A same-repo PR run with no job read `approval_missed`: only a fork's can; others are `no_job_ran`.
- A warning that mentions "No space left on device" made a code failure `infra`: only failure-level annotations count.
- Annotations were read from page one only: every page is read.
- A known never-ran run could pair with a populated `failed[]`: triage returns before listing jobs.
- Fork PR runs listed under the scanned branch name fed `diff` and `flakes`: they are no evidence.
- `watch --group --timeout` exited `1` for a hunt whose only lost runs are held: `3`, as at settle.
- At 80 columns the summary cut `G load more`: `A approve` gives way first.
- The filter could not find the verdict words a row shows: it can, and the key path sees the same rows the screen paints.
- The fake gains `no_job_ran`, `disk_full` and a held bot run; the help legend names ∅ and ↯; the infra detail names the runner cause.
- Takshaka tidy-ups: dead cancel plumbing, a hand-rolled sort, a one-caller forwarder, repeated comments, a special-cased toast, unused scenario aliases, a const that realigned 23 lines.

Kept, with reasons: `blocked_by_policy` stays an inference from `pull_request_target` + `startup_failure` (the issue asks for it before 2 Dec; every spike case had that cause); the detail and docs say "likely". `runs` keeps its newest-first exit order (a newer queued run reads pending before an older failure), as before this change.
