---
name: gh-hound
description: Use gh-hound's structured GitHub Actions CI surface for agent workflows: inspect runs, detect failures, watch fail-fast, parse JSON failure triage (job, step, exit_code, log_excerpt), and branch on gh-hound exit codes instead of screen-scraping the TUI.
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
gh hound runs --run <id> --attempt 2 --no-tui --json   # forensics on a re-run
gh hound artifacts --no-tui --json               # latest run's artifacts
gh hound artifacts --run <id> --download <name> --dir <path> --no-tui --json
```

Runs are scoped to the current git branch by default. An empty `runs[]` usually means the branch has no runs — pass `--all` or `--branch <ref>` before concluding CI is missing.

## Exit Codes

- `0`: all selected runs green; continue.
- `1`: action needed; inspect `runs[].failed[]` and fix.
- `2`: API/network/config error; retry or report infrastructure failure, not CI failure.
- `3`: pending/running; wait and re-poll, or use `watch`.

`watch --json` is fail-fast: it exits `1` the moment the watched run turns red and includes the failure payload immediately.

## JSON Shape

Top level: `repo`, `branch`, `runs[]`. Each run: `id`, `workflow`, `run_number`, `event`, `head_branch`, `head_sha`, `status`, `conclusion`, `created_at`, `html_url`, `failed[]`.

Each `failed[]` entry: `job`, `step`, `exit_code`, `annotations[]` (`path`, `line`, `level`, `message`), `log_excerpt`.

Artifacts: `gh hound artifacts` lists `{id, name, size_in_bytes, expired, expires_at, digest}` for a run (latest on branch when `--run` omitted); `--download <name|id>` extracts into `<dir>/<artifact-name>/` and reports `downloaded.path`. Exit `0` success, `2` any error (expired artifacts are refused before download). Add `--artifacts` to `runs` for per-run artifact metadata (opt-in: paginated artifact-list calls per run, usually one).

Triage degrades per job: when a job log has expired, `log_excerpt` is empty and `exit_code` falls back to `1`, but `job`, `step`, and `annotations` are always present for every failed job. An empty `failed[]` on a red run means job details could not be listed — fall back to `html_url`.

## Mutations

After diagnosing with `runs --json`, act without leaving the surface:

```bash
gh hound rerun --run <id> --failed-only --debug --no-tui --json   # rerun failures with debug logs
gh hound cancel --run <id> --no-tui --json                        # call a run off
```

`action` in the result is one of `rerun | rerun_failed | rerun_job | cancel | force_cancel`. Exit `0` means accepted; `2` means it did not happen (read `error`). A sound agent loop: exit 1 from `runs` -> inspect `failed[]` -> if transient, `rerun --failed-only --debug` -> `watch --json`.

## Deterministic Scenarios

For testing agent behavior without live CI:

```bash
gh hound runs --no-tui --json --fake-scenario failure   # also: green, pending, empty, api_error
```

The JSON schema lives at `internal/render/testdata/schema.json` in the gh-hound repo; the mutation envelope is under `$defs.mutation_result`.

## Guardrails

- Never expose credentials, auth headers, or token-bearing URLs.
- Prefer `--json`; use `--format md` or `--format xml` only for presentation/export.
- Treat exit code `1` as CI failure evidence, not a broken gh-hound invocation.
