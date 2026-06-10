# Task 210: status-cycle filter, attempt history, excerpt quality

## Status
IN PROGRESS

## Ownership Boundary
- **Primary area:** runs-screen status filter, pipe-surface attempt targeting, failure-excerpt windowing.
- **Allowed files:** `internal/logs/` (window terminus), `internal/usecase/` (triage attempt + excerpt), `internal/adapter/github/` (+fake), `cmd/gh-hound/` (flags), `internal/tui/screens/runs/` + keymap/help/palette, docs, vqa harness.
- **Avoid touching:** detail/failure/log/watch/dispatch internals beyond keymap registration.

## Issues
- #17 excerpt tail noise + timestamps on the pipe surface.
- #18 attempt history (`--attempt N`).
- #19 cyclic CI-status filter keybind.

## Scope & Contracts
1. **#17 excerpt quality** (`internal/logs`, `internal/usecase/triage`):
   - The failure window stops at the terminal `##[error]`/`::error` line when one falls inside the window (no post-job cleanup tail).
   - Pipe-surface excerpts strip the leading ISO timestamp from each line (parity with the TUI).
2. **#18 attempt history** (pipe surface only; TUI deferred):
   - `gh hound runs --run <id> [--attempt <n>] --no-tui --json`: `--run` narrows the listing to that run (fetched directly, any branch); `--attempt` targets that attempt's jobs/logs for `failed[]` enrichment. `--attempt` without `--run` is a usage error (exit 2).
   - Adapter: `GetRunAttempt(ctx, repo, runID, attempt)` + `ListJobsForAttempt(ctx, repo, runID, attempt)`.
   - Docs: agent-surface.md + SKILL.md gain the forensics recipe.
3. **#19 status cycle** (TUI):
   - `f` on the runs screen cycles all → failing → running → passed → all, reusing the server-filter vocabulary (`failing`/`running`/`passed`) and the existing reload machinery (ServerFiltered set, filter line shows `/failing` etc.).
   - `f` and `/` compose by replacement (the cycle overwrites any text filter; `/` typing overwrites the cycle; esc clears either and restores).
   - Footer gains `f status`; help and visual-contract updated; vqa interaction scenario added.

## Red / Green
- logs: window-terminus test on a synthetic post-job-noise log + the real attempt-2 fixture shape; timestamp-strip test.
- usecase: triage attempt plumb-through test (fake counts attempt-scoped calls).
- cmd: `--run` narrows + `--attempt` enriches from attempt jobs; `--attempt` without `--run` exits 2.
- runs model: `f` cycles vocabulary + emits IntentFilter; esc resets cycle; footer truth test.

## Definition of Done
- [ ] All red tests written first, then green; race-enabled suite, `make check`/`e2e`/`vqa` pass.
- [ ] Real-log proof: attempt-2 release log excerpt ends at the `##[error]` terminus, timestamp-free, via `--run <id> --attempt 2` against the live repo.
- [ ] tui-qa cold-context audit PASS (status cycle at 3 breakpoints, footer/help truth, compose-with-`/` behavior, regression sweep).
- [ ] Dootsabha (codex+gemini) converged.
- [ ] Docs updated; ship as v0.3.0 (new surface).

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound runs --run <id> --attempt 2 -R indrasvat/gh-hound --no-tui --json | jq '.runs[0].failed[0].log_excerpt'
```
