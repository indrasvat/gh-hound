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
gh hound approvals --run <run-id> --no-tui --json
gh hound approvals --run <run-id> --approve --env production --comment "lgtm" --no-tui --json
gh hound approvals --run <run-id> --reject --no-tui --json
gh hound runs --approvals --no-tui --json
gh hound caches --no-tui --json
gh hound caches --delete-id <cache-id> --no-tui --json
gh hound caches --delete-key <key> --ref <ref> --no-tui --json
gh hound rerun --run <run-id> --no-tui --json
gh hound rerun --run <run-id> --failed-only --debug --no-tui --json
gh hound rerun --run <run-id> --job <job-id> --no-tui --json
gh hound cancel --run <run-id> --no-tui --json
gh hound cancel --run <run-id> --force --no-tui --json
gh hound diff --workflow <name|file|id> [--branch <b>] --no-tui --json
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

## Mutations

The leash works both ways: agents that diagnose with `runs --json` can act with `rerun` and `cancel` without shelling out to `gh run`.

```bash
gh hound rerun --run <id> [--failed-only | --job <job-id>] [--debug] --no-tui --json
gh hound cancel --run <id> [--force] --no-tui --json
```

Result envelope: `{repo, run_id, job_id?, action, accepted, html_url}` where `action` is one of `rerun | rerun_failed | rerun_job | cancel | force_cancel` (`job_id` appears only for `rerun_job`). `html_url` is reconstructed from repo + run id — the verbs make exactly one API call. `--debug` enables runner diagnostic and step debug logging on every rerun form (live-verified against API v2026-03-10).

Mutation exit codes: `0` accepted, `2` anything else (validation such as `--job` with `--failed-only`, permission, conflict like cancelling a completed run, API). Exit `1` stays reserved for actionable CI state and is never returned by mutation verbs. Mutations are paced at one per second through the same serial queue as reads.

On exit `2` the envelope still writes with `accepted: false` and a typed refusal: `error: {kind, message}` where `kind` is one of `validation | permission | conflict | rate_limit | network | unknown`. Harnesses can rehearse refusals deterministically with `--fake-scenario conflict` or `--fake-scenario permission`.

## Deployment Approvals

A run gated on an environment sits in `waiting`; `approvals` is the verb that opens (or keeps shut) the gate — the `gh` CLI has no equivalent.

```bash
gh hound approvals --run <id> --no-tui --json                                  # list pending gates
gh hound approvals --run <id> --approve [--env <name>...] [--comment <text>] --no-tui --json
gh hound approvals --run <id> --reject  [--env <name>...] [--comment <text>] --no-tui --json
```

Envelope (`$defs.approvals_result`): `{repo, run_id, pending: [{environment_id, environment, wait_timer, current_user_can_approve, reviewers: [{type, name}]}]}`. Review attempts add `accepted` plus either `reviewed: {state, environments, comment}` (accepted) or the typed `error: {kind, message}` refusal.

Exit codes for the list form: `0` nothing pending (`pending: []`), `1` gates exist awaiting review (the actionable state), `2` anything else. Review form: `0` review accepted, `2` refused — `accepted: false` with `error.kind` one of `validation | permission | conflict | rate_limit | network | unknown`; exit `1` is never returned by a review.

Review semantics:

- `--approve`/`--reject` with **no** `--env` reviews ALL environments the caller can approve; if none are approvable the refusal is `permission`.
- `--env <name>` is repeatable; an unknown name refuses as `validation`, a gate you cannot review refuses as `permission` — both before any write.
- The POST body always includes a comment (the API requires the field): blank input sends the documented default `reviewed from gh-hound`.
- Reviews are paced through the same one-per-second mutation limiter as `rerun`/`cancel`.

`runs --approvals` adds `pending_environments: [names]` to `waiting` runs only — the default runs path makes zero pending-deployment calls, and even with the flag non-waiting runs cost nothing. Rehearse deterministically with `--fake-scenario waiting` (run `30433655` holds two gates, one not approvable).
## Regression Verdict (diff)

"Who broke main?" without `git bisect`: `gh hound diff` walks a workflow's run history newest-first to the most recent clean run before the current failure streak and names the suspect commit range.

```bash
gh hound diff --workflow CI --no-tui --json
gh hound diff --workflow ci.yml --branch main -R owner/repo --no-tui --json
```

Verdict envelope (`$defs.diff_result` in schema.json): `{repo, workflow, branch, status, last_good, first_bad, suspect_commits[], total_suspects, compare_url, runs_scanned, verdict}`.

- `status` is the source of truth: `located` (boundary found), `green` (newest completed run is clean), `inconclusive` (history ran out or the page cap was hit before a clean run).
- `last_good` / `first_bad` are full run objects, present only when `status` is `located`.
- `suspect_commits[]` carries `{sha, author, message}` (subject line only), capped at 50 rendered; `total_suspects` always reports the full range. `compare_url` links the exact range.
- `verdict` is the hound's one-line reading, e.g. `scent picked up: #101 was clean, #102 wasn't.` or `trail went cold after 1,000 runs.`

Diff exit codes: `1` boundary located (a regression exists — action needed), `0` no action derivable (`green` or `inconclusive`), `2` API/validation error with a typed `error: {kind, message}` envelope (`validation | auth | permission | not_found | rate_limit | network | unknown`). Exit `3` is never used: a finished scan has nothing pending.

Scan rules agents can rely on: a run counts by its latest attempt's conclusion (a failure rerun to green is green); cancelled, skipped, neutral, stale, and still-running runs carry no signal and are stepped over. API spend is bounded by `diff_max_pages` (default 10 pages of 100 runs, env `HOUND_DIFF_MAX_PAGES`); hitting the cap yields `inconclusive`, never a hang. Rehearse deterministically with `--fake-scenario regression` (a seeded boundary: exit `1`, suspects included).

## JSON Contract

The JSON schema lives at `internal/render/testdata/schema.json` (mutation envelope under `$defs.mutation_result`, approvals envelope under `$defs.approvals_result`, regression verdict under `$defs.diff_result`); the canonical failure fixture lives at `internal/render/testdata/failure.golden.json`.

Each run includes:

- `id`, `workflow`, `run_number`, `event`, `head_branch`, `head_sha`.
- `status`, `conclusion`, `created_at`, `html_url`.
- `failed[]` entries with `job`, `step`, `exit_code`, `annotations[]`, and `log_excerpt`.

`runs --run <id>` narrows the listing to one run (any branch); add `--attempt <n>` to triage a specific attempt's jobs and logs -- the forensics path for failures that were later re-run to green. `--attempt` requires `--run` (exit `2` otherwise). Failure excerpts are timestamp-stripped and end at the terminal `##[error]` line.

Runs include `artifacts[]` (`id`, `name`, `size_in_bytes`, `expired`, `expires_at`, `created_at`, `digest`) only when `--artifacts` is passed; the default runs path makes zero artifact API calls (with the flag, expect paginated artifact-list calls per run). The `artifacts` command lists a run's artifacts (defaults to the latest run on the selected branch) and `--download <name|id>` extracts the zip into `<dir>/<artifact-name>/` (`--force` to overwrite). Expired artifacts are rejected before any network call. Download URLs are never emitted: the API's signed links are short-lived secrets. The `artifacts` command exits `0` on success and `2` on any error (including expired artifacts); it never exits `1` or `3`.

The failure object is the stable agent contract. Agents should not screen-scrape the TUI or parse raw GitHub logs when this object is available.

### Caches (the kennel)

"CI got slow" is often "the cache got evicted." `caches` lists the repo's Actions caches with the usage header agents need to do gauge math: `usage` carries `active_size_in_bytes`, `active_count`, and `cap_bytes` — the documented 10 GB eviction cap, since github.com exposes no cap endpoint (the usage-policy endpoint is GHES-only). Each `caches[]` entry: `id`, `key`, `ref`, `size_in_bytes`, `last_accessed_at`, `created_at`. Eviction lags, so `active_size_in_bytes` can legitimately exceed `cap_bytes`.

Deletion uses unambiguous flags because numeric cache keys are legal: `--delete-id <id>` evicts one cache; `--delete-key <key> [--ref <ref>]` evicts every cache with exactly that key — the API matches complete keys, so the same key cached on several refs deletes together unless `--ref` narrows it — and reports `deleted.deleted_count`. (Only the list form's key filter prefix-matches.) Exit codes follow the global contract: `0` deleted (or list rendered), `2` anything else. On exit `2` the envelope still writes with `deleted.accepted: false` and a typed `error: {kind, message}` where `kind` adds `not_found` (no cache matched) to the mutation taxonomy. Deletes are paced at one per second through the same serial queue as every other mutation. The schema lives under `$defs.caches_result` in schema.json.

The default `runs` path makes zero cache API calls; the kennel is only sniffed on the explicit verb or TUI screen.

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

Accepted aliases: `ok`, `green`, `success`; `failure`, `failed`, `failing`; `pending`, `running`, `in_progress`, `queued`; `api_error`, `network_error`, `rate_limited`; `waiting`, `gated`. The `regression` scenario seeds a deterministic last-green → first-red boundary for `diff`.

## Guardrails

- Do not emit credentials, headers, token-bearing URLs, or raw authorization metadata.
- Prefer server-side filtering flags over local post-processing.
- Use JSON for automation; Markdown/XML are presentation/export formats.
- Treat exit code `1` as an actionable CI failure, not a CLI infrastructure failure.
