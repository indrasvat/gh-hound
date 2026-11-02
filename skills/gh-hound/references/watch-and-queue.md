# Waiting on CI: watch and queue

## Watch

`watch --json` is fail-fast: it exits `1` the moment the watched run turns red and includes the failure payload immediately.

`watch --group --no-tui` is the blocking/await mode: it streams the selected run's whole event group (same `head_sha` + `event`) as NDJSON — one `{type:"run", ts, run_id, workflow, status, conclusion}` line per state transition, closed by a `{type:"summary", …, running, home, lost, timed_out}` object — and **blocks until the hunt settles**. Exit `1` if any run is lost, `0` when the hunt comes home. (Plain `watch --json` without `--group` only snapshots the active run and exits `3` while pending — use `--group` to block.) Events are run-level only — drill into a single run with `watch --json` for job/step detail. Rehearse with `--fake-scenario pack`.

For unattended/background agents, bound the block with `--timeout <duration>` (e.g. `--timeout 10m`) so a queued or stuck run — or a hung GitHub call (the deadline bounds an in-flight poll too) — can't hang the watcher forever. On expiry the stream closes with a `{…, "timed_out":true}` summary (still carrying live `running` members). Exit follows the same worst-outcome rule as a clean settle: `1` if any member is already lost, else `3` (still in flight). The `timed_out` marker — not the exit code — is what tells a cut-short hunt from a settled one, so branch on the code as usual and read `timed_out` to know whether to re-poll. Without `--timeout` the block is unbounded (unchanged). `--timeout` requires `--group`; a negative duration is rejected.

## Why Is It Pending? (queue)

If `watch` exits `3` or a run is `pending`, ask hound whether GitHub exposes a concurrency-group blocker before rerunning:

```bash
gh hound queue --run <run-id> --no-tui --json
gh hound queue --job <job-id> --no-tui --json
gh hound queue --groups --no-tui --json
```

Branch on `decision.action`:

- `wait` (exit `3`): a named concurrency group has live members ahead; do not rerun blindly.
- `inspect_gates_or_runner` (exit `0`): this pending state is not exposed as concurrency; check approvals, runner capacity, or normal watch.
- `normal_watch` / `list_only` / `unsupported` (exit `0`): informational or fallback path.
- `error` (exit `2`): validation/API/network/auth/render failure.

Rehearse with `--fake-scenario queue --run 30433644` (blocked behind `deploy-prod`, exit `3`).

Every queue envelope includes `fetched_at`. Treat `--groups` as a summary surface: it can include `last_acquired_at`, but absent `active_member_count` means hound did not fetch group members, not that the active group is empty.
