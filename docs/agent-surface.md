# gh-hound Agent Surface

`gh hound` has two faces: a TUI for humans and a structured pipe surface for agents. Use the pipe surface whenever output will be consumed by Codex, Claude Code, scripts, CI, or another tool.

## Commands

```bash
gh hound runs --no-tui --json
gh hound runs --status failure --no-tui --json
gh hound watch --json
gh hound runs --run <run-id> --no-tui --json
gh hound runs --run <run-id> --attempt <n> --no-tui --json
gh hound artifacts --run <run-id> --no-tui --json
gh hound artifacts --run <run-id> --download <name> --dir <path> --no-tui --json
gh hound runs --artifacts --no-tui --json
gh hound runs --no-tui --format md
gh hound runs --no-tui --format xml
```

`--json` always forces JSON and disables the TUI. Piped stdout also defaults to JSON. Every CLI flag has an environment-variable equivalent where documented in `--help`.

## Exit Codes

| Code | Meaning | Agent action |
| ---: | --- | --- |
| 0 | all selected runs are green or no action is needed | continue |
| 1 | at least one selected run failed or needs action | inspect `runs[].failed[]` and fix |
| 2 | API, network, config, or render error | retry or report infrastructure error |
| 3 | a selected run is still pending/running | wait, poll, or call `watch` |

`watch --json` is fail-fast: if the watched run becomes red, it exits `1` and includes the failure object immediately.

## JSON Contract

The JSON schema lives at `internal/render/testdata/schema.json`; the canonical failure fixture lives at `internal/render/testdata/failure.golden.json`.

Each run includes:

- `id`, `workflow`, `run_number`, `event`, `head_branch`, `head_sha`.
- `status`, `conclusion`, `created_at`, `html_url`.
- `failed[]` entries with `job`, `step`, `exit_code`, `annotations[]`, and `log_excerpt`.

`runs --run <id>` narrows the listing to one run (any branch); add `--attempt <n>` to triage a specific attempt's jobs and logs -- the forensics path for failures that were later re-run to green. `--attempt` requires `--run` (exit `2` otherwise). Failure excerpts are timestamp-stripped and end at the terminal `##[error]` line.

Runs include `artifacts[]` (`id`, `name`, `size_in_bytes`, `expired`, `expires_at`, `created_at`, `digest`) only when `--artifacts` is passed; the default runs path makes zero artifact API calls (with the flag, expect paginated artifact-list calls per run). The `artifacts` command lists a run's artifacts (defaults to the latest run on the selected branch) and `--download <name|id>` extracts the zip into `<dir>/<artifact-name>/` (`--force` to overwrite). Expired artifacts are rejected before any network call. Download URLs are never emitted: the API's signed links are short-lived secrets. The `artifacts` command exits `0` on success and `2` on any error (including expired artifacts); it never exits `1` or `3`.

The failure object is the stable agent contract. Agents should not screen-scrape the TUI or parse raw GitHub logs when this object is available.

Triage degrades per job instead of failing the listing: when a job log has expired or cannot be fetched, `log_excerpt` is empty and `exit_code` falls back to `1`, but the failure entry itself — `job`, `step`, `annotations[]` — is always present for every failed job.

## Deterministic Verification

For local tests and docs, use fake scenarios:

```bash
./bin/gh-hound runs --no-tui --json --fake-scenario green
./bin/gh-hound runs --no-tui --json --fake-scenario failure
./bin/gh-hound runs --no-tui --json --fake-scenario pending
./bin/gh-hound runs --no-tui --json --fake-scenario api_error
./bin/gh-hound watch --json --fake-scenario failure
```

Accepted aliases: `ok`, `green`, `success`; `failure`, `failed`, `failing`; `pending`, `running`, `in_progress`, `queued`; `api_error`, `network_error`, `rate_limited`.

## Guardrails

- Do not emit credentials, headers, token-bearing URLs, or raw authorization metadata.
- Prefer server-side filtering flags over local post-processing.
- Use JSON for automation; Markdown/XML are presentation/export formats.
- Treat exit code `1` as an actionable CI failure, not a CLI infrastructure failure.
