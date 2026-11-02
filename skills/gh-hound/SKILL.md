---
name: gh-hound
description: "Use gh-hound's structured GitHub Actions CI surface for agent workflows: inspect runs, detect failures, watch fail-fast, parse JSON failure triage (job, step, exit_code, log_excerpt), and branch on gh-hound exit codes instead of screen-scraping the TUI. Also answers who broke main, flaky or real, why a run is pending, and how long a run has left."
---

# gh-hound

Use when inspecting GitHub Actions CI with `gh hound` — fix loops, PR checks, post-push verification. Always use the pipe surface; never screen-scrape the TUI or parse raw GitHub logs when `failed[]` is available.

## Commands

```bash
gh hound runs --no-tui --json                    # current branch's runs
gh hound runs --status failure --no-tui --json   # failure-focused loop
gh hound runs --actor @me --no-tui --json        # only runs you started (also --actor <login>, HOUND_ACTOR)
gh hound runs --all --no-tui --json              # all branches
gh hound runs --branch <ref> --no-tui --json     # explicit branch/ref when local branch is wrong
gh hound runs -R owner/repo --no-tui --json      # outside a checkout
gh hound watch --json                            # active run, fail-fast
gh hound watch --group --no-tui                  # whole event group, NDJSON, blocks until the hunt settles
gh hound watch --group --no-tui --timeout 10m    # same, but bound the block (exit 3 + timed_out summary on expiry)
gh hound queue --run <run-id> --no-tui --json    # why is this run pending? concurrency-group blockers
gh hound queue --job <job-id> --no-tui --json    # diagnose a queued job through its parent run
gh hound queue --group <name> --no-tui --json    # inspect one active concurrency group, or report inactive
gh hound queue --groups --no-tui --json          # list active concurrency group summaries
gh hound runs --run <id> --attempt 2 --no-tui --json   # forensics on a re-run
gh hound artifacts --no-tui --json               # latest run's artifacts
gh hound artifacts --run <id> --download <name> --dir <path> --no-tui --json
gh hound approvals --run <id> --no-tui --json    # pending deploy gates (exit 1 = awaiting review)
gh hound approvals --run <id> --approve --env production --comment "lgtm" --no-tui --json
gh hound diff --workflow CI --no-tui --json      # who broke main? last green vs first bad
gh hound flakes --workflow CI --no-tui --json    # flaky or real? scored flake verdict
gh hound pace --no-tui --json                    # nearly done or stuck? ETA and slow jobs/steps
gh hound caches --no-tui --json                  # cache kennel vs the eviction cap
gh hound caches --delete-key <key> --ref <ref> --no-tui --json
gh hound workflows --no-tui --json               # workflow states (why did my cron stop?)
gh hound workflows --enable <id|path> --no-tui --json   # wake a disabled workflow
```

An empty branch-scoped `runs[]` can mean the wrong target. If `diagnostic.kind` is `target_mismatch`, run one of `diagnostic.next_commands[]` before concluding there's no CI.

## Exit Codes

- `0`: all selected runs green; continue.
- `1`: action needed; inspect `runs[].failed[]` and fix.
- `2`: API/network/config error; retry or report infrastructure failure, not CI failure.
- `3`: pending/running; wait and re-poll, or use `watch`.

`watch --json` is fail-fast: it exits `1` the moment the watched run turns red, with the failure payload.

## Fix Loop

1. `runs --json` exits `1`: read `runs[].failed[]` (`job`, `step`, `exit_code`, `annotations[]`, `log_excerpt`).
2. Flaky or real? `flakes --json`. If `flaky`, `rerun --failed-only`. Otherwise fix it.
3. Who broke it? `diff --json` gives the last green and first bad runs, with the suspect commits.
4. Pending or slow? `queue --run <id> --json` for blockers, `pace --run <id> --json` for ETA and slow steps.
5. After pushing: `watch --json` (one run) or `watch --group --no-tui --timeout 10m` (the whole push).

## References

Read only what the task needs:

- [references/json-shape.md](references/json-shape.md): `runs[]`, `failed[]`, `repeats[]`, artifacts, caches, empty-listing diagnostics, schema `$defs`.
- [references/watch-and-queue.md](references/watch-and-queue.md): `watch --group` NDJSON, `--timeout`, and why a run is pending.
- [references/diagnose.md](references/diagnose.md): `diff`, `flakes` scoring and verdicts, `pace` statuses.
- [references/act.md](references/act.md): rerun, cancel, deployment approvals, enabling workflows.

## Deterministic Scenarios

For testing agent behavior without live CI:

```bash
gh hound runs --no-tui --json --fake-scenario failure   # also: green, pending, empty, api_error, waiting, regression, flaky, paced
gh hound runs --branch release/v1 --no-tui --json --fake-scenario empty   # emits target_mismatch
```

## Guardrails

- Never expose credentials, auth headers, or token-bearing URLs.
- Prefer `--json`; use `--format md` or `--format xml` only for presentation/export.
- Treat exit code `1` as CI failure evidence, not a broken gh-hound invocation.
