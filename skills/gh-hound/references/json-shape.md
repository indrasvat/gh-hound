# JSON shape

Top level: `repo`, `branch`, `runs[]`, optional `diagnostic`. Each run: `id`, `workflow`, `run_number`, `event`, `head_branch`, `head_sha`, `status`, `conclusion`, `actor`, `triggering_actor`, `created_at`, `html_url`, `failed[]`.

`--actor @me` resolves the login your token authenticates as; if it cannot, the command exits `2` with the reason instead of listing everyone's runs — fix auth or pass `--actor <login>`. `--actor` with `--run` exits `2` (`--run` always shows that run).

`diagnostic` appears only for actionable empty listings. For `target_mismatch`, branch on `kind`, read the human `message`, and execute one of `next_commands[]` instead of silently stopping.

Each `failed[]` entry: `job`, `step`, `exit_code`, `annotations[]` (`path`, `line`, `level`, `message`), `log_excerpt`, and optional `repeats[]`. A repeating run inside the excerpt (a poll or retry loop) shows as one `⋯ <summary> · N lines hidden (La–Lb)` line, so the excerpt still ends on the failure with context before it; `repeats[]` (`start_line`, `end_line`, `cycles`, `block_lines`, `hidden_lines`, `from`, `to`, `summary`) tells you how long a job spent repeating — e.g. 720 polls over two hours before a timeout.

Artifacts: `gh hound artifacts` lists `{id, name, size_in_bytes, expired, expires_at, digest}` for a run (latest on branch when `--run` omitted); `--download <name|id>` extracts into `<dir>/<artifact-name>/` and reports `downloaded.path`. Exit `0` success, `2` any error (expired artifacts are refused before download). Add `--artifacts` to `runs` for per-run artifact metadata (opt-in: paginated artifact-list calls per run, usually one).

Caches: `gh hound caches` reports `usage` (`active_size_in_bytes`, `active_count`, `cap_bytes` — the repo's configured storage limit, 10 GB by default; usage can exceed it because eviction lags) plus `caches[]` (`id`, `key`, `ref`, `size_in_bytes`, `last_accessed_at`, `created_at`). When CI suddenly slows, check the kennel: usage near `cap_bytes` means LRU eviction is thrashing your keys. Evict with `--delete-id <id>` or `--delete-key <key> [--ref <ref>]` (reports `deleted.deleted_count`). Exit `0` deleted or listed, `2` anything else with typed `error.kind` (`not_found` when nothing matched). The default `runs` path never touches the cache API.

Triage degrades per job: when a job log has expired, `log_excerpt` is empty and `exit_code` falls back to `1`, but `job`, `step`, and `annotations` are always present for every failed job. An empty `failed[]` on a red run means job details could not be listed — fall back to `html_url`.

## Empty listings

Runs are scoped to the current git branch by default. An empty branch-scoped `runs[]` can be a target mismatch, especially for release, tag-push, or branchless workflows. If the top-level `diagnostic.kind` is `target_mismatch`, follow `diagnostic.next_commands[]`: widen with `--all`, pass `--branch <ref>`, inspect `--run <id>`, or use `watch -R owner/repo --group --run <id> --timeout 10m` for a bounded await. Do not conclude CI is absent until you have tried one of those target-correcting commands.

## Schema

The JSON schema lives at `internal/render/testdata/schema.json` in the gh-hound repo; the mutation envelope is under `$defs.mutation_result`, the approvals envelope under `$defs.approvals_result`, the queue verdict under `$defs.queue_result`, the regression verdict under `$defs.diff_result`, the caches envelope under `$defs.caches_result`, the workflows envelope under `$defs.workflows_result`, the pack stream under `$defs.watch_group_event` / `$defs.watch_group_summary`, the flake verdict under `$defs.flakes_result`, and the pace verdict under `$defs.pace_result`.
