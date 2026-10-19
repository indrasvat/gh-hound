# Task 400 — Finished Watch And Missing Logs

## Status

IMPLEMENTED — the two items task 390 left open. `make ci` and `make vqa` pass; verified with shux 0.51 on fakes and live (read-only) on this repo.

## Ownership Boundary

- **Primary area:** a job log GitHub no longer has (or does not have yet), from the adapter to the watch; the watch of a finished run.
- **Allowed files:** `internal/adapter/github/logs.go`, `internal/usecase/errors.go`, `internal/usecase/watch.go`, `cmd/gh-hound/main.go` (the watch resolver), `internal/tui/screens/watch/`, `internal/tui/app.go` (watch chrome and footer), `internal/tui/keys/footers.go`, `internal/adapter/fake/` (the `skipped_job` scenario), tests, `.claude/automations/`, `docs/`.
- **Avoid touching:** the failure and log screens beyond the error they print, the pack board.

## Decisions

- **A missing log is a typed answer.** The adapter returns `usecase.LogUnavailableError{JobID, Status}`: 410 when the log is past the repository's retention, 404 when there is none. From the API itself either is final (no re-request); a stale redirect is still re-requested once, as before. The message is plain (`the log for job N has expired (past the repository's log retention)`), never the URL and JSON body, so the failure and log screens read clean too.
- **The watch shows what it can.** A 410 is held as expired and never asked for again; a 404 is asked for again next tick (a log can land just after its job completes). Neither fails the tick: the other jobs' logs show, and dim lines under the header name the jobs without a log, the status first so a cut line keeps it (`log expired · build, publish`, `log not available yet · notify`). Other errors still fail the tick, keeping the logs fetched so far.
- **A finished run is not streaming.** The header reads its conclusion (`✔ success 4m45s`) where it read `streaming ⠹`, the chrome reads `success · follow ●`, and a run with no lines reads `no job logs to show` instead of waiting. The footer is the new `watch_done` (`f follow · d debug · ⎋ detach`): `x` has nothing to cancel.

## Task List

- [x] T1 Adapter: 410 and 404 from the logs endpoint are one request each and a typed error without the response body; a stale redirect that stays 404 ends in the same error.
- [x] T2 Usecase: expired logs fetched once and named; not-yet logs named and fetched each tick until they land; the tick never fails on either. `FuzzWatchSession` now scripts each job's answer (log, 410, 404, failure) and checks fetches, lines, missing logs and the held cache every tick; it catches an expired log left uncached in under a second.
- [x] T3 Screen and app: a finished run's header, empty state and `x`; the jobs-without-a-log lines at 80 and 120 columns (long, multi-line and empty names); the `watch_done` footer and chrome.
- [x] T4 End to end through `defaultTUIApp`: a run with one expired log opens, shows the other job's log and the note, and asks for each log once over 4 polls.
- [x] T5 PTY scenarios on `skipped_job` (run #569 added, whose log download fails): `watch-skipped-job` (a finished run: conclusion, no cancel) and `watch-expired-log` each fail on main; `watch-failed-open` moves to #569 and still guards a failed open.
- [x] T6 Verification: `make ci`, `make vqa`; shux before/after live on this repo; adversarial review and a cold `tui-qa` round.

## Found Along The Way

- The watch-cancel confirmation test cancelled a finished run, which GitHub refuses (409); it now uses a running one.
- Adversarial review: a finished run with no conclusion read `◇ ` with a blank word; it reads `completed`. One no-log line lost its status words when long matrix names filled 80 columns; each status now leads its own line, and names are trimmed to one line.
- Not changed (also on main): `x` pressed while a run is still going opens the confirm; if the run finishes before `y`, the cancel is sent and GitHub's 409 shows as a rejected mutation (polls do not apply under a confirm).

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; `make ci` and `make vqa` pass; evidence lives on the PR only.
