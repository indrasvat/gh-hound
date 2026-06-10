# gh-hound Agent Surface

`gh hound` has two faces: a TUI for humans and a structured pipe surface for agents. Use the pipe surface whenever output will be consumed by Codex, Claude Code, scripts, CI, or another tool.

## Commands

```bash
gh hound runs --no-tui --json
gh hound runs --status failure --no-tui --json
gh hound watch --json
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

The failure object is the stable agent contract. Agents should not screen-scrape the TUI or parse raw GitHub logs when this object is available.

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
