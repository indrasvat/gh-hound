---
name: gh-hound
description: "Use gh-hound's structured GitHub Actions CI surface for agent workflows: inspect runs, detect failures, watch fail-fast, parse JSON failure triage (job, step, exit_code, log_excerpt), and branch on gh-hound exit codes instead of screen-scraping the TUI."
---

# gh-hound

Use when inspecting GitHub Actions CI with `gh hound` — fix loops, PR checks, post-push verification. Always use the pipe surface; never screen-scrape the TUI or parse raw GitHub logs when `failed[]` is available.

## Commands

```bash
gh hound runs --no-tui --json                    # current branch's runs
gh hound runs --status failure --no-tui --json   # failure-focused loop
gh hound runs --all --no-tui --json              # all branches
gh hound runs -R owner/repo --no-tui --json      # outside a checkout
gh hound watch --json                            # active run, fail-fast
gh hound watch --group --no-tui                  # whole event group, NDJSON, blocks until the hunt settles
gh hound watch --group --no-tui --timeout 10m    # same, but bound the block (exit 3 + timed_out summary on expiry)
gh hound runs --run <id> --attempt 2 --no-tui --json   # forensics on a re-run
gh hound artifacts --no-tui --json               # latest run's artifacts
gh hound artifacts --run <id> --download <name> --dir <path> --no-tui --json
gh hound approvals --run <id> --no-tui --json    # pending deploy gates (exit 1 = awaiting review)
gh hound approvals --run <id> --approve --env production --comment "lgtm" --no-tui --json
gh hound diff --workflow CI --no-tui --json      # who broke main? last green vs first bad
gh hound flakes --workflow CI --no-tui --json    # flaky or real? scored flake verdict
gh hound caches --no-tui --json                  # cache kennel vs the eviction cap
gh hound caches --delete-key <key> --ref <ref> --no-tui --json
gh hound workflows --no-tui --json               # workflow states (why did my cron stop?)
gh hound workflows --enable <id|path> --no-tui --json   # wake a disabled workflow
```

Runs are scoped to the current git branch by default. An empty `runs[]` usually means the branch has no runs — pass `--all` or `--branch <ref>` before concluding CI is missing.

## Exit Codes

- `0`: all selected runs green; continue.
- `1`: action needed; inspect `runs[].failed[]` and fix.
- `2`: API/network/config error; retry or report infrastructure failure, not CI failure.
- `3`: pending/running; wait and re-poll, or use `watch`.

`watch --json` is fail-fast: it exits `1` the moment the watched run turns red and includes the failure payload immediately.

`watch --group --no-tui` is the blocking/await mode: it streams the selected run's whole event group (same `head_sha` + `event`) as NDJSON — one `{type:"run", ts, run_id, workflow, status, conclusion}` line per state transition, closed by a `{type:"summary", …, running, home, lost, timed_out}` object — and **blocks until the hunt settles**. Exit `1` if any run is lost, `0` when the hunt comes home. (Plain `watch --json` without `--group` only snapshots the active run and exits `3` while pending — use `--group` to block.) Events are run-level only — drill into a single run with `watch --json` for job/step detail. Rehearse with `--fake-scenario pack`.

For unattended/background agents, bound the block with `--timeout <duration>` (e.g. `--timeout 10m`) so a queued or stuck run can't hang the watcher forever. On expiry the stream closes with a `{…, "timed_out":true}` summary (still carrying live `running` members) and exits `3` — the same pending signal, distinguishable from a real settle by the `timed_out` marker. Without `--timeout` the block is unbounded (unchanged). `--timeout` requires `--group`.

## JSON Shape

Top level: `repo`, `branch`, `runs[]`. Each run: `id`, `workflow`, `run_number`, `event`, `head_branch`, `head_sha`, `status`, `conclusion`, `created_at`, `html_url`, `failed[]`.

Each `failed[]` entry: `job`, `step`, `exit_code`, `annotations[]` (`path`, `line`, `level`, `message`), `log_excerpt`.

Artifacts: `gh hound artifacts` lists `{id, name, size_in_bytes, expired, expires_at, digest}` for a run (latest on branch when `--run` omitted); `--download <name|id>` extracts into `<dir>/<artifact-name>/` and reports `downloaded.path`. Exit `0` success, `2` any error (expired artifacts are refused before download). Add `--artifacts` to `runs` for per-run artifact metadata (opt-in: paginated artifact-list calls per run, usually one).

Caches: `gh hound caches` reports `usage` (`active_size_in_bytes`, `active_count`, `cap_bytes` — the repo's configured storage limit, 10 GB by default; usage can exceed it because eviction lags) plus `caches[]` (`id`, `key`, `ref`, `size_in_bytes`, `last_accessed_at`, `created_at`). When CI suddenly slows, check the kennel: usage near `cap_bytes` means LRU eviction is thrashing your keys. Evict with `--delete-id <id>` or `--delete-key <key> [--ref <ref>]` (reports `deleted.deleted_count`). Exit `0` deleted or listed, `2` anything else with typed `error.kind` (`not_found` when nothing matched). The default `runs` path never touches the cache API.

Triage degrades per job: when a job log has expired, `log_excerpt` is empty and `exit_code` falls back to `1`, but `job`, `step`, and `annotations` are always present for every failed job. An empty `failed[]` on a red run means job details could not be listed — fall back to `html_url`.

## Regression Boundary (diff)

When a workflow is red and the question is which commits turned it, do not bisect — the answer is in run history:

```bash
gh hound diff --workflow CI --no-tui --json
```

Branch on `status`: `located` (exit `1`) means a regression exists — `last_good`/`first_bad` are full run objects and `suspect_commits[]` (`{sha, author, message}`, capped at 50; `total_suspects` is the full count) is the blame range, with `compare_url` for humans. `green` and `inconclusive` both exit `0` — read `verdict` for the hound's one-liner (`trail went cold after 1,000 runs.` means the page budget ran out; raise `HOUND_DIFF_MAX_PAGES`). Exit `2` carries `error: {kind, message}`. Runs count by their latest attempt: a failure rerun to green is green. A sound loop: `diff --json` -> if `located`, inspect `first_bad` with `runs --run <id> --no-tui --json` -> fix or rerun.

Rehearse with `--fake-scenario regression` (deterministic boundary, exit `1`).

## Flaky or Real? (flakes)

Before burning time on a red run, ask whether the failure is a known squirrel:

```bash
gh hound flakes --workflow CI --no-tui --json    # --workflow omitted follows the latest run
```

Branch on `status` (always the worst job verdict): `flaky` (exit `1`) — the failure pattern is established, `rerun --failed-only` is the rational first move; `suspect` (exit `1`) — one wobble on record, look at `jobs[].evidence[]` before deciding; `clean` (exit `0`) — fresh scent, this failure is worth chasing as real; `insufficient_data` (exit `0`) — under 5 signal runs and no evidence, treat as real. The decision recipe:

1. `runs --json` exits `1` -> `flakes --workflow <w> --json`.
2. `status == "flaky"` -> `rerun --run <id> --failed-only --no-tui --json` -> `watch --json`.
3. `status == "clean"` (or `insufficient_data`) -> investigate `failed[]` for real; do not rerun blindly.
4. `status == "suspect"` -> read `evidence[]` (`attempt_flip` beats `cross_run_flap` beats `retry_mask`) and decide.

Scoring is documented and stable: 0.45 per attempt flip + 0.30 per cross-run flap + 0.20 per retry mask, capped at 1.0; >= 0.6 is `flaky`, any evidence is `suspect`. `sample_size`/`window`/`runs_scanned` size the claim; `signals_evaluated` lists what was checked (retry masking is only detectable in runs that had a failed attempt — the call budget never blanket-downloads logs). Evidence comes from attempt job conclusions and logs, never annotations (the API only serves annotations for the latest attempt). An underfilled-but-flaky window still exits `1` — trust `status`, not `sample_size`.

Rehearse with `--fake-scenario flaky` (seeded flips + retry mask, exit `1`).

## Mutations

After diagnosing with `runs --json`, act without leaving the surface:

```bash
gh hound rerun --run <id> --failed-only --debug --no-tui --json   # rerun failures with debug logs
gh hound cancel --run <id> --no-tui --json                        # call a run off
```

`action` in the result is one of `rerun | rerun_failed | rerun_job | cancel | force_cancel`. Exit `0` means accepted; `2` means it did not happen (read `error`). A sound agent loop: exit 1 from `runs` -> inspect `failed[]` -> if transient, `rerun --failed-only --debug` -> `watch --json`.

Deployment approvals: a `waiting` run is gated on environment review. `approvals --run <id>` lists the gates (exit `1` while any await review, `0` when none); `--approve`/`--reject` with no `--env` reviews everything you can approve, `--env <name>` (repeatable) targets gates, `--comment` is optional (a blank one sends `reviewed from gh-hound` — the API requires the field). Refusals are typed: unknown environment -> `validation`, not a required reviewer -> `permission`. Add `--approvals` to `runs` for `pending_environments` on waiting runs (opt-in, one call per waiting run).

## Workflow State (workflows)

When a scheduled workflow silently stops, check its state before debugging YAML: GitHub disables crons after 60 days of repo inactivity (`disabled_inactivity`).

```bash
gh hound workflows --no-tui --json     # [{id, name, path, state}]
gh hound workflows --enable ci.yml --no-tui --json
```

`state` is an open string; documented values are `active`, `disabled_manually`, `disabled_inactivity`, `disabled_fork`, `deleted` — branch on the ones you know, pass the rest through. Toggle by **numeric id or workflow file path only** (display names refuse as `validation`); only `active` ↔ `disabled_manually`/`disabled_inactivity` flips are valid. A toggle is exactly one API call and reports the landing state in `toggled.state`. Exit `0` ok, `2` refused with `error: {kind, field?, message}` — this verb never exits `1` or `3`. A sound loop: empty `runs[]` on a branch with a schedule -> `workflows --json` -> if `disabled_inactivity`, `--enable <path>` -> re-poll `runs`.

## Deterministic Scenarios

For testing agent behavior without live CI:

```bash
gh hound runs --no-tui --json --fake-scenario failure   # also: green, pending, empty, api_error, waiting, regression, flaky
```

The JSON schema lives at `internal/render/testdata/schema.json` in the gh-hound repo; the mutation envelope is under `$defs.mutation_result`, the approvals envelope under `$defs.approvals_result`, the regression verdict under `$defs.diff_result`, the caches envelope under `$defs.caches_result`, the workflows envelope under `$defs.workflows_result`, the pack stream under `$defs.watch_group_event` / `$defs.watch_group_summary`, and the flake verdict under `$defs.flakes_result`.

## Guardrails

- Never expose credentials, auth headers, or token-bearing URLs.
- Prefer `--json`; use `--format md` or `--format xml` only for presentation/export.
- Treat exit code `1` as CI failure evidence, not a broken gh-hound invocation.
