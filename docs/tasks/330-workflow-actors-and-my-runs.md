# Task 330 — Workflow Actors and My-Runs Filtering

## Status

IN PROGRESS — issue #55. Out-of-scope defects found while scoping are filed as #56.

## Ownership Boundary

- **Primary area:** who started a run (Actor column, detail attribution), `actor:` / `actor:@me` filtering, and the `/` filter input.
- **Allowed files:** `internal/tui/`, `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/render/`, `cmd/gh-hound/`, `.claude/automations/`, `docs/`, `skills/gh-hound/SKILL.md`, `README.md`, `pages/`.
- **Avoid touching:** theme tokens themselves, screens other than runs/detail attribution (their hard-coded colours are #56), mutation flows.

## Decisions

- `@me` resolves through `GET /user` on the same HTTP client as run requests (`GH_TOKEN` → `GITHUB_TOKEN` → `gh auth token`), never `git config`. One lookup per session, cached on success; a failure is cached until the user re-applies the query, so polls never repeat it.
- The header shows `@login` only once that lookup lands (async, after first paint); until then it shows no identity.
- `--run` always shows that run: `--actor` with `--run` is a validation error (exit 2); `HOUND_ACTOR` is ignored for `--run`.
- Theme tokens and the 256-colour fallback cover the surfaces this task touches. The fallback is a colour-profile writer on the TTY path: truecolor terminals get the bytes unchanged.
- Bone is a light theme for light terminals (as in `docs/gh-hound-design.html`); it is verified on a light terminal background.

## Filter Grammar

Tokens split on whitespace.

| Token | Meaning | Where |
| --- | --- | --- |
| `actor:<login>` / `actor:@me` | original run actor (`@` optional, case-insensitive, bots allowed) | server |
| `status:<s>` / `conclusion:<s>` | status or conclusion (`failing`, `running`, `passed` aliases) | server |
| `branch:<ref>` | head branch | server |
| `event:<e>` | trigger event | server |
| `sha:<sha>` | head SHA | server |
| a lone bare status/event word (`failing`, `push`) | as before: status / event | server |
| anything else (`fix`, `#571`, `feat: bump`) | text over loaded rows | client |

Empty values, repeated keys, invalid logins and unknown statuses are input errors shown inline; the applied query never changes on an error. Unknown `key:` text is searched as text (commit titles start with `feat:`).

## Task List

- [ ] T1 Task doc, visual contract, design reference (input, hints, applied summary, empty/error states, Actor column, attribution).
- [ ] T2 Query parser (`internal/tui/screens/runs/query.go`) with table tests.
- [ ] T3 Identity port (`usecase.ViewerProvider`), GitHub `GET /user`, fake identity + `actors` scenario (actor variety, filters, paging).
- [ ] T4 Render: `actor` / `triggering_actor` in JSON/MD/XML, schema, golden.
- [ ] T5 CLI: `--actor`, `HOUND_ACTOR`, `@me`, `--run` guard, actor-aware diagnostics, fake path.
- [ ] T6 Launch: actor-aware branch fallback, notice and empty state.
- [ ] T7 Runs table: shared column spec (header == rows), Actor column, narrow-width yield (Duration, then Event), theme tokens.
- [ ] T8 Filter input: draft/applied split, shaded input + cursor + placeholder, hint row, Enter/Esc semantics, inline errors, applied summary with accurate counts, empty/error states, footer + help syntax.
- [ ] T9 App: identity state, query generation guard, scope caches keyed by server filter, refresh/pagination/back keep the filter, `f`/`s` keep actor, palette My runs / All actors, header identity.
- [ ] T10 Detail attribution: `Started by @alice · Attempt 2 · Re-run by @bob`.
- [ ] T11 256-colour fallback on the TTY writer.
- [ ] T12 Fixtures, assertions, interaction scenarios; README, configuration, agent surface, skill.
- [ ] T13 Verification: `make ci`, `make vqa`, interaction audit, render hygiene; shux sessions on fakes and on `cli/cli`, `indrasvat/gh-hound`, a foreign repo; both themes, 256-colour, 80x24/120x40/200x60; cold-context QA and review agents.
- [ ] T14 Website: copy + runs/My-runs screenshots (deploy only after approval).
- [ ] T15 PR closing #55 with evidence comment (screenshots, video) and the review loop.
