# Task 390 — Watch Fetches Each Job Log Once

## Status

IMPLEMENTED — issue #60. `make ci` and `make vqa` pass; verified with shux 0.51 on fakes and live (read-only) on this repo.

## Ownership Boundary

- **Primary area:** the single-run watch's polling: `usecase.WatchService.Tick`, the resolver's state between ticks, and the watch screen's failed-open path.
- **Allowed files:** `internal/usecase/watch.go`, `cmd/gh-hound/main.go` (the watch resolver), `internal/tui/app.go` (watch load and keys), tests, `docs/`.
- **Avoid touching:** the pack board (one list call per tick already), the log adapter, the watch screen's layout.

## Decisions

- **A session carries the logs.** `usecase.WatchSession` holds the last tick's state for one repo and run; the resolver ticks through it, so a completed job's log is downloaded once. Another run or repo starts empty. The TUI and `WatchResolver` are unchanged.
- **Tick is a function of the cache and the listing.** It keeps a log only for a completed job still listed and returns `Lines` in job order: what an uncached tick returns, without the downloads. A rerun's new job ids drop the old attempt's lines. It never mutates its input, so a load and a poll that overlap can share the cache (both may fetch a log neither had; the lines stay right). A failed tick keeps the logs it held and the ones it fetched before the failure. `Appended`/`CompletedJobLogs` become `Lines`/`JobLogs`; the unread `Jobs` field goes.
- **A job that never ran is not fetched.** A completed job with no steps (skipped, cancelled before it started, or stopped at an environment gate) has no log; GitHub answers 404 and the whole tick failed. It is skipped and not cached, so it is fetched if it ever lists steps.
- **A failed open drops the old watch.** The error stays on screen, no poll runs for the run the user left, and `x` needs a run to cancel.

## Task List

- [x] T1 Usecase: each completed log fetched once over ticks; a rerun's new jobs replace the old; a run or repo switch starts empty; zero-step jobs never fetched; a failed tick keeps the cache, and a log that keeps failing costs the jobs before it no download; 16 concurrent ticks under `-race`, then a settled tick fetches nothing.
- [x] T2 `FuzzWatchSession`: random listings (status, steps, order, membership), run switches and failing fetches, checked against a model of the cache on every tick (fetches, lines, held jobs). With the session's carry-over removed it fails in under a second.
- [x] T3 End to end through `defaultTUIApp`: 7 polls of a run with two finished jobs, one running and one skipped fetch 2 logs (18 on main).
- [x] T4 App: a failed open after another watch shows its error, polls nothing, and `x` opens no confirm.
- [x] T5 Verification: `make ci`, `make vqa`; shux before/after live on this repo; adversarial review and a cold `tui-qa` round.

## Found Along The Way

- Live on main: CI #164 (5 finished jobs) downloaded 5 logs on every 3 s tick; CI #162, whose matrix build was cancelled before it started, could not be watched (`status 404`). From the runs list, `w` on #162 after watching #164 showed #164 and kept polling it.
- `FuzzWatchSession` found that caching a step-less job as "no log" hid its log if it later listed steps; such jobs are now skipped, not cached (seed committed).
- Not changed: the watch header reads `streaming ⠹` and the footer offers `✗ cancel` on a run that has finished; the error view's title is `Runs unavailable` on every route.
- Adversarial review: a failed tick dropped the logs it had fetched, so a log that kept failing re-downloaded every job before it on every tick; fixed with a test that failed first. The returned state shares the session's logs and is read-only (the resolver only reads `Lines`).
- Live probe (282 jobs on this repo): every zero-step job within retention 404s, every job with steps returned its log; a zero-step job can also be a `failure` stopped at an environment gate.
- Not changed: a job with steps whose log fails (briefly 404 right after it completes, or 410 past retention) still fails the tick, so a run older than log retention cannot be watched, as on main; skipping it needs a typed log error from the adapter, which the failure and log screens share.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; `make ci` and `make vqa` pass; evidence lives on the PR only.
