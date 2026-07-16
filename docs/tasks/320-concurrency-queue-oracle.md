# Task 320 — Concurrency Queue Oracle

## Status

IMPLEMENTED — core, real adapter, agent surface, TUI route, queue VQA fixture, full `make check`, full `make vqa`, live adapter dogfood, and dedicated TUI QA re-audit passed. Remaining gate: final adversarial code review before PR.

## Ownership Boundary

- **Primary area:** explain GitHub Actions `pending` runs/jobs caused by official concurrency groups.
- **Allowed files:** `internal/model/`, `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/`, `.claude/automations/`, `docs/`, `skills/gh-hound/SKILL.md`, `README.md`.
- **Avoid touching:** mutation semantics for rerun/cancel/approvals except shared error mapping; default runs-list polling behavior except explicit queue route wiring.

## Design Reference

Detailed local plan: `.local/concurrency-queue-oracle-plan.md`.

Official sources checked July 16, 2026:

- GitHub REST Actions concurrency groups: <https://docs.github.com/en/rest/actions/concurrency-groups>
- GitHub Actions concurrency behavior: <https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency>
- GitHub changelog for `queue: max`: <https://github.blog/changelog/2026-05-07-github-actions-concurrency-groups-now-allow-larger-queues/>

Live endpoint existence was verified against `indrasvat/gh-hound`; both repo groups and run groups returned `200` with empty lists. July 16, 2026 dogfood also verified `queue --groups` returns `listed` with zero active groups and a nonexistent group returns `inactive`.

## Problem

Agents currently see too many distinct waits as the same state. A run may be executing, runner-queued, environment-gated, or `pending` behind a concurrency group. `watch` handles live runs, `approvals` handles gates, but hound cannot yet answer the queue-specific question:

> Is this run or job waiting behind a named GitHub concurrency group, and what is ahead of it?

Without that answer, agents rerun work that is correctly waiting, poll without proof, or report “pending” without the blocker.

## Scope

Ship `gh hound queue` as a read-only diagnostic command plus a TUI queue screen.

Target modes:

- `queue --run <run-id>`: inspect every concurrency group associated with a run, including job-level members returned by the run endpoint.
- `queue --job <job-id>`: resolve the parent run with `GetJob`, then inspect the matching job-level concurrency member.
- `queue --group <name>`: list one live group or report inactive.
- `queue --groups`: list active group summaries only. Do not reuse root `--all`.

V1 TUI scope:

- Add `:queue` diagnostics route and screen.
- Do not add ambient runs-list header chips, watch-board queue rows, or detail/failure hints in this task.
- All TUI network work must use existing async load plumbing; no keypress or render path may call GitHub.

## Public Contract

Canonical status/action/exit table:

| Mode | Evidence | `status` | `decision.action` | Exit |
| --- | --- | --- | --- | ---: |
| target | live members ahead in a concurrency group | `blocked` | `wait` | 3 |
| target | run endpoint returns no configured groups | `not_concurrency` | `inspect_gates_or_runner` | 0 |
| target | configured groups exist but no active members | `not_concurrency` | `inspect_gates_or_runner` | 0 |
| group | named group returns inactive/missing | `inactive` | `normal_watch` | 0 |
| list | `--groups` or non-target `--group` succeeds | `listed` | `list_only` | 0 |
| any | API unsupported on host | `unsupported` | `unsupported` | 0 |
| any | validation/auth/permission/rate/network/render failure | `error` | `error` | 2 |

`queue` v1 never exits `1`. A proved concurrency block is a wait state, not a failed CI state. Future dead-holder detection may introduce exit `1` only after blocker-run evidence exists.

Envelope fields:

- `repo`, `target`, `status`, `decision`, `fetched_at`, `summary`
- `configured_group_count`
- `groups[]`: `group_name`, `group_url`, `last_acquired_at`, `scope`, `state`, `configured_group_count`, `active_member_count`, `target_position`, `blocked_by_count`, `members[]`
- `blockers[]`
- `error` only on failure

Counts must remain distinct: run-group `total_count` is configured-group count; live group `total_count` is active-member count; `blocked_by_count` is derived for the selected target. Summary-only `--groups` output must not invent active-member counts; omit them unless a group detail was fetched.

## API Rules

- Use existing `X-GitHub-Api-Version: 2026-03-10`.
- Add cursor-aware client plumbing for `Link` headers; do not reuse page-number pagination for these endpoints.
- Reconstruct all `ahead_of_run` and `ahead_of_job` calls from validated repo, escaped group name, and target ID. Never send credentials to API-provided `position_url`.
- Treat `position_url` as provenance/discriminator only.
- Group-detail members do not provide positions; run-group members provide queue positions when GitHub exposes target membership. Never fabricate position from array order.
- `422` handling is contextual:
  - recognized target-not-in-group after a successful target lookup becomes `not_concurrency`
  - real argument/shape validation exits `2`
  - abuse/spam/secondary-limit language maps to rate/unknown error, exit `2`
- `404` for named group inspection means `inactive`; missing run/job remains an error.

## Visual Contract

User-facing name: `GitHub queue`.

Required state colors and glyphs:

- `blocked`: `◌` in `info`
- holder/running member: `⠹` in `run`
- `not_concurrency`: `✔` in `ok`
- `inactive` or `unsupported`: `◇` in `neutral`
- API error: `✗` in `fail`
- backoff/loading: shared spinner only

Queue screen:

- `80x24`: single column; no forced secondary metadata.
- `120x40`: group/member list plus selected detail.
- `200x60`: add useful diagnosis columns only; no decorative empty panel.
- Footer generated from keymap data: `j/k move · ⏎ open run · y copy JSON · ⎋ back · ?`.
- 100-member overflow must scroll or summarize without layout corruption.

## Red / Green / Refactor Plan

1. **Red — model/usecase.**
   Add failing tests for statuses, decisions, blocker counts, target selection, job-level groups, case-insensitive group names, nullable positions, unsupported adapter, inactive group, and target-not-in-group mapping.

2. **Green — model/usecase.**
   Add `internal/model/concurrency.go`, optional `GitHubConcurrency` port, `QueueService`, and typed queue result model.

3. **Red — adapter/fake.**
   Add failing httptest cases for all three endpoints, cursor accumulation, escaped group names with `/`, unpaginated 100-member group detail, `404`, contextual `422`, `403`, `429`, and `410`.

4. **Green — adapter/fake.**
   Add `internal/adapter/github/concurrency.go`, cursor helper, fake scenarios `queue`, `queue_empty`, `queue_job`, `queue_inactive`, and `queue_target_not_member`.

5. **Red — render/CLI.**
   Add failing schema, exit-code, target-mode validation, schema-valid error-envelope, JSON/Markdown/XML, and fake-scenario tests.

6. **Green — render/CLI.**
   Add `internal/render/queue.go`, `$defs.queue_result`, `cmd/gh-hound/queue.go`, and root wiring.

7. **Red — TUI/VQA.**
   Add failing route, palette, footer, copy JSON, escape, overflow, input-leakage, and fixture-registration tests.

8. **Green — TUI/VQA.**
   Add queue screen package, route wiring, async resolver, `docs/gh-hound-design.html` mock, `.claude/automations/assertions/queue.json`, VQA screen registration, and interaction fixture.

9. **Docs and skill.**
   Update `docs/agent-surface.md`, `docs/visual-contract.md`, `docs/development.md`, `README.md`, `skills/gh-hound/SKILL.md`, and any docs-check allowlists. Keep progressive disclosure: teach the pending-run recipe, not raw REST endpoints.

## Verification Gates

Focused tests:

```bash
go test ./internal/model ./internal/usecase ./internal/adapter/github ./internal/adapter/fake ./internal/render ./cmd/gh-hound
```

Full gates:

```bash
make check
make docs-check
make build
make vqa-screen SCREEN=queue
make vqa
```

Required shux proof:

```bash
shux lens run --size 80x24 -- ./bin/gh-hound __screen --screen queue --width 80 --height 24
shux lens run --size 120x40 -- ./bin/gh-hound __screen --screen queue --width 120 --height 40
shux lens run --size 200x60 -- ./bin/gh-hound __screen --screen queue --width 200 --height 60
```

Artifacts:

- `.shux/out/cqo/queue-80x24.png`
- `.shux/out/cqo/queue-120x40.png`
- `.shux/out/cqo/queue-200x60.png`
- `.shux/out/cqo/queue-interaction.txt`
- `.shux/out/cqo/contact-sheet.png`

A dedicated TUI QA agent must review the contact sheet and return `VERDICT: PASS`.

Dogfood:

- `gh hound queue --groups --no-tui --json -R indrasvat/gh-hound`
- `gh hound queue --run <live-or-fake-run> --no-tui --json`
- Use a `.local/` dogfood workflow plan to create real workflow-level and job-level concurrency waits before final adapter fixtures are frozen.

## Documentation Done Criteria

- `docs/agent-surface.md` teaches `decision.action` as canonical and preserves exit-code doctrine.
- `skills/gh-hound/SKILL.md` adds a short “Why is it pending?” recipe below `watch`.
- `docs/visual-contract.md` names the queue screen, footer, glyphs, and fixture list.
- `docs/development.md` lists the queue VQA fixture after it exists.
- README gets one feature bullet and quick-start line after the command works.
- Landing/site update is deferred until shux captures exist; then add a small “Why is this pending?” feature row, not a hero rewrite.

## Review Record

- Dootsabha round 1: no-ship; incorporated exit semantics, endpoint precision, job lookup, cursor plumbing, safe `position_url`, and shux scope fixes.
- Dootsabha round 2: not converged; incorporated `--all` collision fix, unified status/action/exit table, live endpoint grounding, v1 TUI scope, and executable shux commands.
- Focused dootsabha consult after patches: PASS on prior blockers.
