# Task 250: deployment approvals — review waiting runs from the terminal

## Status
IN PROGRESS — implementation complete on feat/250-deployment-approvals; live POST verification + landing pass pending (orchestrator).

## Ownership Boundary
- **Primary area:** pending-deployment review: surface `waiting` runs' gated environments, approve/reject from TUI and pipe.
- **Allowed files:** `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/model/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/` (runs list badge, detail pane, new approvals overlay, keys/palette/help), `docs/`, `skill/SKILL.md`, `README.md`, `pages/`, vqa harness.
- **Avoid touching:** environment/secret administration, runner admin, required-reviewers configuration.

## Depends On
- 040 (adapter), 060 (mutations/confirm pattern), 110 (detail), 240 (mutation verb conventions).

## Parallelizable With
- 260 (after 240 lands).

## PRD / Design References
- PRD §18 v2 bucket: "Deployment approvals & queues" — this task promotes it, the same way artifacts were promoted (Task 200 precedent).
- API (verify live before coding, record in this file):
  - `GET /repos/{o}/{r}/actions/runs/{run_id}/pending_deployments` → `[{environment{id,name}, wait_timer, current_user_can_approve, reviewers[]}]`
  - `POST` same path, body `{environment_ids: [..], state: "approved"|"rejected", comment}` → 200 with deployments.
- Competitive fact: `gh` CLI has **no** approval command — this is an unfilled gap in the entire terminal ecosystem.
- **API verification record (2026-06-10):**
  - `GET .../pending_deployments` live-verified against indrasvat/gh-hound run `27319423642`: `200` with `[]` on a non-waiting run (endpoint path, auth, and empty-list envelope confirmed live).
  - No reachable repo had a `waiting` run, so the full GET payload and the POST body are pinned from the official REST reference (docs.github.com/en/rest/actions/workflow-runs, 2022-11-28 example payloads) in `internal/adapter/github/testdata/pending_deployments.json` and the adapter httptest suite. Confirmed: POST requires `environment_ids` (int64 ids), `state` (`approved`|`rejected`), AND `comment` — all three required; gh-hound always sends a comment, defaulting to `reviewed from gh-hound`.
  - **Live verification of the POST (approve AND reject through a scratch gated environment on indrasvat/gh-hound) is deferred to the PR's live-verification phase** — creating it requires pushing a workflow targeting the environment, which is not done from this branch.

## Problem
A run gated on an environment sits in `waiting` and gh-hound can see it but not act — the single remaining triage verdict that forces a browser tab. "Fetch happens" must include the deploy gate.

## Scope
- Model: `PendingDeployment {environment, wait_timer, can_approve, reviewers}`.
- Adapter: `ListPendingDeployments`, `ReviewPendingDeployments` (+ fake fixtures incl. a not-approvable environment and a multi-environment run).
- Runs list: `waiting` runs get a themed gate badge (`◫` or another strict text-presentation geometric glyph per theme contract — no emoji-presentation codepoints, the emoji check enforces) and the `A` key (approve/review) when selected — `A` is the key PRD §7.3 already reserves for "approve deploy (v2)"; footer/help/palette updated.
- Detail screen: pending environments panel when run is `waiting` (environments, reviewers, wait timer, whether you can approve).
- Approvals overlay: pick environment(s), approve or reject, optional comment, **confirmation-gated** like rerun/cancel; success/failure toasts.
- Pipe: `gh hound approvals --run <id> --no-tui --json` (list) and `--approve|--reject [--env <name>...] [--comment <text>]`; exit codes follow the global contract: `0` review accepted (or list rendered with `pending: []` — nothing to act on), `1` pending gates exist awaiting review (the actionable state, for the list form), `2` anything else (not permitted, API, validation) with typed `error.kind`. The review POST body **always includes `comment`** (the API requires the field): user-blank input sends a documented default (`reviewed from gh-hound`); pin the exact body in adapter tests and live verification. `runs` JSON: `waiting` runs gain `pending_environments: [names]` only when `--approvals` flag passed (zero-extra-calls default, Task 200 precedent).
- Resolve flow: a branch whose newest run is `waiting` should surface the gate immediately (launch context state).

## Out of Scope
- Wait-timer countdown live updates; org-level environment policy display; deployment history.

## Public Contracts
- New JSON shapes in schema.json + agent-surface.md (list + review result). Signed/HTML URLs follow existing guardrails.
- Exit code semantics documented; `--no-tui` + `--approve` with no `--env` reviews ALL approvable environments (explicit in docs).

## Red / Green / Refactor Plan
- **Red:** usecase tests (list maps fields; review validates env names; not-approvable → typed permission error; fake-backed), adapter httptest (request body shape, 422 mapping), CLI flag-matrix tests, TUI tests (badge render, overlay select/confirm, toasts).
- **Green:** layer by layer: model → port → adapter → usecase → render/cmd → TUI.
- **Refactor:** share environment-picker with dispatch form patterns where sensible.

## Test Pyramid
- L0–L1: lint + race suite.
- L2: adapter httptest; schema validation; call-count (default runs path: zero approval calls).
- L3: `make e2e` — fake `waiting` scenario end-to-end (new fake scenario: `waiting`).
- L4: `make vqa` — new fixture screens: runs-with-waiting-badge, detail-pending-panel, approvals-overlay at 80x24/120x40/200x60; cold-context tui-qa audit `VERDICT: PASS` with pixel-level review (badge alignment in the runs table, overlay layering, comment field).
- L5: shux against a **real repo**: create a gated environment on indrasvat/gh-hound (or a scratch repo) with required reviewers, dispatch a workflow targeting it, drive the full approve flow live through the actual TUI binary; repeat with reject + comment; verify the run proceeds/fails accordingly. Screenshot every step.

## Performance Budget (hard gates)
- Default `runs` path: **zero** additional API calls (call-count test).
- Approvals list fetch: async, never blocks paint (Task 220 pattern mandatory).
- Overlay open keystroke-to-paint < 50ms (shux-measured).

## Voice (MUST)
The hound voice is mandatory across TUI strings, toasts, docs, and site copy. Suggested register: the gate is a door; the hound waits at it. Confirm: `open the gate for production?` Reject toast: `gate stays shut.` No emoji; theme glyphs only.

## Website & Docs Updates (required)
- Landing: new trail row in "Sit. Stay. Triage." — q: `Deploy is waiting on me?` keys: `A` — and a new screens-gallery tab (`approvals`) captured via `.claude/automations/capture_fixture.py`. Counts/copy elsewhere updated if they reference row counts. **Voice MUST hold.**
- `docs/agent-surface.md`, `skill/SKILL.md`, README (feature row + keybinding table), `docs/visual-contract.md`, `docs/configuration.md` if any config added.
- Preview-deploy via the PR workflow; pixel-check the new tab before merge.

## Definition of Done
- [ ] API shapes verified live and pinned in this spec before implementation starts.
- [ ] Red-first across all layers; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] TUI: waiting badge, pending panel, confirm-gated approve/reject with toasts; keymap/palette/help/footer truthful.
- [ ] Pipe: `approvals` verb with documented JSON + exit codes; `runs --approvals` opt-in enrichment; zero extra default calls.
- [ ] Live end-to-end on a real gated environment: approve AND reject flows driven through shux with screenshots in PR.
- [ ] Perf gates met with shux timings in PR body.
- [ ] tui-qa cold-context `VERDICT: PASS` at all breakpoints; dootsabha (codex + gemini) converged.
- [ ] Landing trail row + approvals tab live on preview, hound voice verified, then production on merge.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound approvals --run <id> --no-tui --json
./bin/gh-hound approvals --run <id> --approve --env production --comment "lgtm" --no-tui --json
HOUND_FAKE_SCENARIO=waiting ./bin/gh-hound
```

## Session Protocol
1. Live-verify the pending_deployments API against a scratch gated environment FIRST; pin payloads in adapter testdata.
2. Red → green → refactor per layer; shux + tui-qa; dootsabha; landing capture; push; PR; gh-ghent loop.
