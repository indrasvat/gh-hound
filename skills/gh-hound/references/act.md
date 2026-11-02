# Acting: rerun, cancel, approvals, workflows

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
