# Diagnosing a red or slow run: diff, flakes, pace

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

## Nearly Done or Stuck? (pace)

Before cancelling a long run, ask how it compares with recent green runs:

```bash
gh hound pace --run <run-id> --no-tui --json
```

Branch on `status`: `running` (exit `3`) is within its usual time, so wait (`run.eta_seconds` is the time left). `overrun` or `slow` (exit `1`) means `slow[]` names the jobs and steps that took the time. Anything else exiting `0` needs no action. `--no-steps` skips the job and step checks and their API calls.
