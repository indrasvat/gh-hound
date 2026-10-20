# Task 410 — Follow A Live Run

## Status

IMPLEMENTED — the live-run report: "Can it follow a live run? It shows the Run tests step as queued but it's running." Detail called a running step `queued`, did not refresh, and did not advertise `w`; the watch of a run whose one job was still going stayed on `waiting for completed job logs from GitHub Actions` until the job ended.

## Ownership Boundary

- **Primary area:** a run still going, on the detail and watch screens.
- **Allowed files:** `internal/tui/screens/detail/`, `internal/tui/screens/watch/`, `internal/usecase/watch.go`, `cmd/gh-hound/main.go` (the watch resolver), `internal/tui/app.go` (detail poll, `l`/`⏎` on a running job), `internal/tui/keys/footers.go`, `internal/adapter/fake/` (the `live_run` scenario), tests, `.claude/automations/`, `docs/`, `README.md`.
- **Avoid touching:** the log and failure screens, the pack board, the GitHub adapter.

## Decisions

- **GitHub has no API for a running job's log** (checked 2026-10-20). `GET /actions/jobs/{id}/logs` redirects to a blob that answers `404 BlobNotFound` until the job finishes; the per-step logs endpoint now in the REST docs answered 404 on running and finished jobs alike; webhooks carry status only. The web UI's live log rides an undocumented `alive.github.com` websocket that needs a browser session, not a token (cli/cli#3484, open and `blocked` since 2021; `gh` declined to scrape it). So hound follows a running job by its steps, which the jobs endpoint does report live, and shows each job's log as soon as that job finishes.
- **The watch shows step progress.** `WatchState` carries the tick's job listing; while the run goes, each unfinished job reads as one line, its current step and that step's elapsed time (`⠹ bdd › 29 Run tests 12m40s`, `◌ report queued`), running jobs first, as many as the rows the header, notes and log tail leave (`+N more jobs not finished` after), above one dim line saying why there is no log yet. Finished jobs' logs still follow below. A finished run shows no progress block.
- **Detail reads time, not `queued`.** A running job or step reads its elapsed time, `queued`/`waiting` only before it starts, and a finished one GitHub never timed reads `—` (it read `queued`). The clock is `Model.Now`, the wall clock when zero, so tests pin it.
- **Detail follows a live run.** While the run is not finished and a job is not finished (or none is listed yet), detail re-lists jobs on the poll tick (`tickPoll`, off the event loop, dropped if the user left). The cursor stays on the same job (by ID) and step; artifacts, downloads and focus are untouched; a failed poll keeps the jobs on screen. Once every job has finished it stops asking.
- **A running job opens on its running step.** With nothing failed, detail opens on the first running job at the step it is on; selecting a running job does the same; a poll moves the cursor on with it unless the user moved it. On a real 37-step job the running step sat below the window under `+18 more`.
- **`w` is on the detail footer** (`⏎ expand · w watch · ↻ rerun job …`); it was bound but unlisted, so the reporter never found it.
- **`l` and `⏎` on a running job** toast `log not ready · w follows its steps · bdd is still running` (the hint first, so a long matrix name never cuts it) instead of fetching into `GitHub has no log for job N` under the generic route-error title.

## Task List

- [x] T1 Detail: elapsed for running jobs and steps, `queued`/`waiting` before start, `—` for an untimed finished step; `WithJobs` keeps the cursor.
- [x] T2 Detail poll on a live run; stops when every job has finished. Fails on `main` (the jobs never refresh).
- [x] T3 Watch: `WatchState.Jobs`; the step-progress block and its note at 80 and 120 columns; no blank wait while a job runs. The end-to-end test through `defaultTUIApp` fails on `main`.
- [x] T4 `l`/`⏎` on a running job: a toast, no fetch, still on detail. Fails on `main` (it opened the log screen).
- [x] T5 `w watch` on the detail footer, help, visual contract, README.
- [x] T6 Fake `live_run`: E2E #3902 with `lint` done, `bdd` on 3 Run tests, `report` queued; it moves on the clock (Upload artifacts at 45s, `bdd` done at 55s, the run done at 80s) and a running job's log answers 404, as GitHub's does. The `running` scenario's job is a finished failure, which hid this.
- [x] T7 PTY scenarios `watch-live-run`, `detail-live-run`, `detail-log-not-ready`.
- [x] T8 Verification: `make ci`, `make vqa`; shux demo on `live_run`; live read-only on rust-lang/rust; two adversarial agents, a dootsabha council (codex, agy), and two cold `tui-qa` rounds.

## Found Along The Way

- The `running` fake scenario lists an in-progress run whose only job is a finished failure, which is why no fixture ever showed a running step; `live_run` is the honest one.
- Adversarial review (detail): the poll kept the run snapshot it opened with, so polling stopped once the listed jobs had finished even though the run went on (a job listed late never appeared) and a gate approved elsewhere never cleared. The resolver now re-reads a live run (a failed re-read keeps the snapshot) and the poll applies it; past `waiting`, the gate panel goes. A queued job GitHub had stamped `started_at` on showed a running clock: the status decides now. A poll begun before esc and re-opening the same run landed on the new opening; a per-opening generation drops it. A removed selected job kept its old step index.
- Adversarial review (watch): the progress block had no height bound, so a wide matrix pushed the log tail off the screen; it is bounded to three lines (running jobs first, then `+N more jobs not finished`). A step name with a newline or tab broke the row. The fake's report step read as started in the future.
- Security review: every screen printed GitHub's run, job, step, label and runner names raw, and a job name can carry a fork PR's title (`name: ${{ github.event.pull_request.title }}`), so an ESC in one reached the terminal (OSC 52, screen clears). The GitHub adapter now drops control and bidi-override runes where it decodes them. Pre-existing; fixed here because this PR adds two more places that print a job name.
- Cold `tui-qa` round 1 (FAIL): the progress block was a fixed three lines at any height (2 of 8 running jobs over 42 blank rows at 200x60); it now takes the free rows. Also fixed: a long job name cut the toast's `w` hint, detail read `17.0s` where watch read `17s` (both whole seconds now), and a queued job with no steps showed two bare dividers.
- Live on an 88-job run, `l` pressed while the job listing was still loading toasted `no job has started yet`; that toast now waits for GitHub's answer.
- Verified live, read-only, on rust-lang/rust: CI #235463 (two jobs on `30 run the build`: detail clock, cursor on the running step, toast, watch) and CI #235461 (88 unfinished jobs: the bounded block keeps the log tail).
- Cold `tui-qa` round 2 (98f0298 smoke, 1411398 full): PASS. Every round-1 finding fixed; checked live on rust-lang/rust (9- and 86-job runs at all three sizes, a failed first load healed through a failing proxy) and across the fakes.
- Not changed (also on main): the detail jobs list never scrolls, so on an 88-job run the selection walks off the frame with no `+N more` (needs its own task). A detail load failure is titled `Runs unavailable`. Below about 16 rows the watch log tail is cut from the bottom (under the supported sizes).
- Not changed (also on main): the runs summary reads `last finished now ago` for a run under a minute old; J/K (next/previous run) on detail have no handler; a run GitHub reports finished while a job still reads in progress (its lag) shows that job nowhere until the next poll; the follow view shows six lines at any height.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; `make ci` and `make vqa` pass; evidence lives on the PR only.
