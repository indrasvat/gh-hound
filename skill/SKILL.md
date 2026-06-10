---
name: gh-hound
description: Use gh-hound's structured GitHub Actions CI surface for agent workflows: inspect runs, detect failures, watch fail-fast, parse JSON failure excerpts, and respect gh-hound exit codes instead of screen-scraping the TUI.
---

# gh-hound

Use this skill when an agent needs to inspect GitHub Actions CI with `gh hound`, especially in fix loops, PR checks, or local repo verification.

## Default Workflow

1. Prefer structured output:

```bash
gh hound runs --no-tui --json
```

2. For failure-focused loops:

```bash
gh hound runs --status failure --no-tui --json
```

3. For active runs:

```bash
gh hound watch --json
```

4. Parse `runs[].failed[]` for `job`, `step`, `exit_code`, `annotations`, and `log_excerpt`. Do not screen-scrape the TUI.

## Exit Codes

- `0`: all good.
- `1`: action needed; inspect failure objects and fix.
- `2`: API/network/config/render error; retry or report infrastructure failure.
- `3`: pending/running; wait or keep watching.

`watch --json` is fail-fast: it exits `1` as soon as a watched run turns red and includes failure details.

## JSON Shape

The top-level object has `repo`, `branch`, and `runs`. Each run has `id`, `workflow`, `run_number`, `event`, `head_branch`, `head_sha`, `status`, `conclusion`, `created_at`, `html_url`, and `failed`.

Failure entries include `job`, `step`, `exit_code`, `annotations`, and `log_excerpt`.

## Local Verification

Use deterministic scenarios when testing agent behavior:

```bash
./bin/gh-hound runs --no-tui --json --fake-scenario green
./bin/gh-hound runs --no-tui --json --fake-scenario failure
./bin/gh-hound runs --no-tui --json --fake-scenario pending
./bin/gh-hound watch --json --fake-scenario failure
```

Schema and golden fixture:

- `internal/render/testdata/schema.json`
- `internal/render/testdata/failure.golden.json`

## Guardrails

- Never expose credentials, auth headers, or token-bearing URLs.
- Prefer `--json`; use `--format md` or `--format xml` only for presentation/export.
- Treat exit code `1` as CI failure evidence, not a broken gh-hound invocation.
