# Task 230: dispatch ref correctness for foreign repos

## Status
PLANNED

## Ownership Boundary
- **Primary area:** dispatch ref resolution when the target repo is not the local checkout.
- **Allowed files:** `internal/usecase/` (launch/dispatch resolution), `internal/tui/screens/dispatch/`, `cmd/gh-hound/`, tests, vqa harness.
- **Avoid touching:** dispatch form layout, workflow input parsing, mutation adapter.

## Depends On
- 140 (dispatch screen), 220 (the async loading pattern — the default-branch fetch uses its shared component; do NOT build bespoke async machinery here). Closes GitHub issue #15.

## Parallelizable With
- 240.

## PRD / Design References
- PRD §9.7 (dispatch pre-fills the current ref) — the pre-fill contract implicitly assumes the local checkout IS the target repo. Issue #15: with `GH_REPO=owner/foreign`, the form pre-fills the local branch name, which likely doesn't exist on the target; submission 404s or, worse, dispatches the wrong ref.

## Problem
`gh hound dispatch -R owner/foreign` pre-fills the ref field from the local git checkout. The fix: pre-fill the local branch only when the target repo matches the local origin; otherwise pre-fill the target repo's default branch (fetched, not guessed).

## Scope
- Ref resolution: target == local origin → local branch (current behavior); target != local origin (or no git checkout) → target repo default branch via `GET /repos/{owner}/{repo}` (`default_branch`), cached for the session.
- Validation before dispatch: allowed refs are **branches and tags** (what workflow_dispatch documents; raw SHAs only if live verification proves support — pin either way). Branches validate via `GET /repos/{o}/{r}/branches/{branch}` (path-escape slash-containing names), tags via the git ref endpoint. Typed not-found maps to the form, hound-voiced: `that branch isn't in this yard`.
- Same resolution applies to the palette dispatch entry and `dispatch` subcommand.

## Out of Scope
- Ref picker/autocomplete UI (possible follow-up), multi-remote detection beyond `origin`.

## Public Contracts
- Pipe `dispatch` errors: invalid ref → exit 2 with `{error: {kind: "validation", field: "ref"}}` (existing validation error shape).

## Red / Green / Refactor Plan
- **Red:** usecase tests: foreign target pre-fills fetched default branch; matching target keeps local branch; missing-branch validation maps to form error; fake adapter gains `GetRepo`/`GetBranch`.
- **Green:** minimal resolution + validation wiring.
- **Refactor:** extract ref-resolution helper shared by TUI and pipe paths.

## Test Pyramid
- L0–L1: lint suite + race-enabled unit tests (resolution matrix: local/foreign × checkout/no-checkout × branch-exists/missing).
- L2: adapter httptest for `GetRepo` default_branch + 404 branch mapping.
- L3: `make e2e`.
- L4: `make vqa` — dispatch fixture capture showing foreign-repo pre-fill; tui-qa cold-context audit `VERDICT: PASS` (pixel check: pre-filled value, validation error styling).
- L5: shux against **real repos**: run from this checkout with `-R openclaw/openclaw`, verify the form pre-fills openclaw's default branch (not `feat/...` local); submit with a bogus ref and verify the typed error; dispatch a real `workflow_dispatch` on indrasvat/gh-hound and watch it appear.

## Performance Budget (hard gates)
- At most **one** extra API call (`GET /repos`) per dispatch-form open, only when target != local origin; cached thereafter (call-count test).
- Form opens in < 100ms after workflow metadata is loaded (no new blocking fetch on the paint path — default-branch fetch is async with a placeholder).

## Voice (MUST)
Validation and toast strings keep the hound voice. Reference: existing dispatch confirm copy. No emoji.

## Website & Docs Updates
- `docs/configuration.md` if any new config surfaces (none expected).
- README dispatch section: one line on foreign-repo behavior. Keep the hound voice.

## Definition of Done
- [ ] Red tests first; resolution matrix fully covered; race suite green.
- [ ] `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] Foreign-repo dispatch pre-fills the target's default branch; invalid ref blocked with typed error in both TUI and pipe.
- [ ] Live shux verification on openclaw/openclaw (pre-fill) + indrasvat/gh-hound (real dispatch) with screenshots in PR.
- [ ] tui-qa cold-context audit `VERDICT: PASS`; dootsabha (codex + gemini) converged.
- [ ] GitHub issue #15 closed by the PR.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound dispatch -R openclaw/openclaw            # shux: inspect pre-fill
./bin/gh-hound dispatch -R indrasvat/gh-hound           # real dispatch + watch
```

## Session Protocol
1. Reproduce issue #15 via shux before writing code; screenshot the bug.
2. Red → green → refactor; shux + tui-qa; dootsabha; push; PR; gh-ghent loop.
