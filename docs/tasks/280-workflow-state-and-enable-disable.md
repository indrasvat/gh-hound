# Task 280: workflow state — surface and toggle disabled workflows

## Status
PLANNED

## Ownership Boundary
- **Primary area:** show workflow `state` — **all** API values: `active`, `disabled_manually`, `disabled_inactivity`, `disabled_fork`, `deleted` — and enable/disable workflows where toggling is valid.
- **Allowed files:** `internal/usecase/workflow.go`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/model/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/` (workflows surface in palette/dispatch picker, badge), `docs/`, `skill/SKILL.md`, `README.md`, vqa harness.
- **Avoid touching:** scheduling logic, runner admin.

## Depends On
- 140 (workflow listing exists for dispatch), 240 (mutation conventions).

## Parallelizable With
- 270, 290.

## PRD / Design References
- API: list-workflows payload includes `state`, and it is **already plumbed**: `model.Workflow.State` exists (`internal/model/actions.go`) and the adapter maps it (`internal/adapter/github/client.go`). The work is rendering/exposing it, not plumbing it. Mutations: `PUT /repos/{o}/{r}/actions/workflows/{id}/enable` and `/disable`.
- The classic mystery this solves: scheduled workflows silently stop after 60 days of repo inactivity (`disabled_inactivity`) and the answer is buried in the web UI.

## Problem
"My cron workflow stopped running" has a one-field answer gh-hound already downloads and discards. Surface it, badge it, and let the user flip it back on without a browser.

## Scope
- Model: `Workflow.State` (already present) treated as an open string (unknown future states render verbatim with a neutral badge, never rejected); fake fixtures for **all five** documented states: `active`, `disabled_manually`, `disabled_inactivity`, `disabled_fork`, `deleted`.
- TUI: wherever workflows are listed (dispatch picker, palette workflows entry), non-active workflows get a themed badge (`◌ asleep` for disabled_inactivity, `⊘ muzzled` for disabled_manually, `⊘ fork-disabled` for disabled_fork, `✗ deleted` for deleted — final glyphs per theme contract); `e` toggles enable/disable, confirm-gated, with toast — offered **only** for toggleable states (`active` ↔ `disabled_manually`/`disabled_inactivity`); `disabled_fork` and `deleted` show the badge with a why-line instead of the toggle.
- Launch context: if the branch's relevant workflow is disabled, the empty/all-green screens say so (this is the "why are there no runs" answer).
- Pipe: `gh hound workflows --no-tui --json` (new verb: id, name, path, state) and `--enable|--disable <id|path>` — id or workflow file path ONLY (what the API accepts; keeps the one-call budget). Display names resolve only in the TUI, from the workflows list already in hand. Exit `0` ok, `2` API/validation error.

## Out of Scope
- Editing workflow YAML, schedule introspection, org-wide listings.

## Public Contracts
- `workflows` JSON schema + agent-surface.md entry; mutation result shape follows Task 240 conventions.

## Red / Green / Refactor Plan
- **Red:** model/usecase tests (state plumbed; toggle calls right endpoint), CLI tests (verb + flags + exit codes), TUI tests (badges, toggle confirm, empty-screen explanation), launch-context test (disabled workflow surfaces notice).
- **Green:** smallest wiring per layer.
- **Refactor:** none expected; keep diff small.

## Test Pyramid
- L0–L1: lint + race suite.
- L2: adapter httptest (enable/disable PUT, state parsing); schema validation.
- L3: `make e2e` with new fake states.
- L4: `make vqa` — dispatch-picker-with-badges fixture at three breakpoints; cold-context tui-qa `VERDICT: PASS` (pixel: badge color vs theme, alignment in picker rows).
- L5: shux against **real repo**: disable a low-stakes workflow on indrasvat/gh-hound via the TUI, verify on GitHub, re-enable via pipe verb, verify state round-trip. Screenshots in PR.

## Performance Budget (hard gates)
- Zero additional API calls on default paths (state rides the existing workflows fetch — call-count test).
- Toggle = exactly one API call.

## Voice (MUST)
Hound voice on badges and toasts: disabled_inactivity = `asleep` (fell asleep after 60 quiet days), enable toast = `back on duty.` Docs same register. No emoji.

## Website & Docs Updates
- README: workflows verb + badge meanings (hound voice).
- `docs/agent-surface.md`, `skill/SKILL.md`. Landing only if a screens recapture happens to include badges — no dedicated section.

## Definition of Done
- [ ] Red-first; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] State visible in TUI pickers + empty-screen notice; toggle confirm-gated with toasts.
- [ ] `workflows` verb + enable/disable shipped, schema documented.
- [ ] Live round-trip verified on a real workflow (evidence in PR).
- [ ] tui-qa cold-context `VERDICT: PASS`; dootsabha (codex + gemini) converged.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound workflows --no-tui --json
./bin/gh-hound workflows --disable .github/workflows/ci.yml --no-tui --json && ./bin/gh-hound workflows --enable .github/workflows/ci.yml --no-tui --json
```

## Session Protocol
1. Confirm `state` values present in the live list-workflows payload; pin in adapter testdata.
2. Red → green; shux + tui-qa; dootsabha; push; PR; gh-ghent loop.
