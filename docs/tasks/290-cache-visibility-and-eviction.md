# Task 290: cache visibility and eviction

## Status
PLANNED

## Ownership Boundary
- **Primary area:** Actions cache listing, usage-vs-cap, and deletion.
- **Allowed files:** `internal/usecase/`, `internal/adapter/github/`, `internal/adapter/fake/`, `internal/model/`, `internal/render/`, `cmd/gh-hound/`, `internal/tui/` (caches screen via palette, keys/help), `docs/`, `skill/SKILL.md`, `README.md`, vqa harness.
- **Avoid touching:** org-level cache policy, cache content download (not in the API).

## Depends On
- 200 (artifacts pattern — caches follow it closely), 240 (mutation conventions).

## Parallelizable With
- 270, 280.

## PRD / Design References
- PRD §18 v2 bucket: "Caches administration".
- API: `GET /repos/{o}/{r}/actions/caches` (sort/filter by key/ref, paginated; `key`, `ref`, `size_in_bytes`, `last_accessed_at`, `created_at`), `GET /repos/{o}/{r}/actions/cache/usage` (`active_caches_size_in_bytes`, count), `DELETE /repos/{o}/{r}/actions/caches?key=` and by id. Repo cap is 10 GB with LRU eviction — thrash is invisible from the terminal today.

## Problem
"CI got slow" is often "the cache got evicted" — and there is no terminal view of cache pressure. List caches, show usage against the 10 GB cap, delete stale keys, all on the artifacts interaction pattern users already know.

## Scope
- Model: `Cache {id, key, ref, size, last_accessed_at, created_at}` + `CacheUsage {total_size, count}`.
- Pipe: `gh hound caches --no-tui --json` (list + usage header), deletion via **unambiguous flags** (numeric keys are legal, so a shared operand is a foot-gun): `--delete-id <id>` and `--delete-key <key> [--ref <ref>]`; key deletes can match multiple caches — the JSON result reports `deleted_count` and the TUI confirm shows the match count first. Exit codes follow the global contract: `0` deleted (or list rendered), `2` anything else (no match → typed `not_found`, API, validation).
- TUI: palette `caches` entry → caches screen: usage gauge vs the repo's actual cap — fetched from the API where exposed (verify the storage-limit endpoint live in session protocol; **10 GB is the documented fallback only**) — themed bar, sortable list (size / last-used), `d` delete (confirm-gated with match count, toast), `/` filter by key substring.
- Eviction-pressure hint: when usage > 90% of the effective cap, the gauge warns (`kennel's almost full`).

## Out of Scope
- Cross-run cache-miss attribution (heuristics deferred; revisit after Task 300 lands a history walker), org rollups, cache restore.

## Public Contracts
- `caches` JSON schema (list + usage + delete result) in schema.json + agent-surface.md.

## Red / Green / Refactor Plan
- **Red:** adapter httptest (pagination, sort params, delete by key vs id, usage), usecase tests (filtering, gauge thresholds), CLI flag/exit-code tests, TUI screen tests (gauge states, delete confirm, empty state).
- **Green:** artifacts-pattern implementation per layer.
- **Refactor:** share human-size formatting + list/confirm plumbing with artifacts.

## Test Pyramid
- L0–L1: lint + race suite.
- L2: adapter httptest; schema validation; call-count (caches calls only on explicit verb/screen — zero on default paths).
- L3: `make e2e` — fake scenario with caches near cap.
- L4: `make vqa` — caches screen fixtures at 80x24/120x40/200x60 (gauge at <50%, >90%, empty); cold-context tui-qa `VERDICT: PASS`, pixel-level (gauge fill vs theme, long-key truncation).
- L5: shux against **real repos**: indrasvat/gh-hound (real Go module caches exist from CI) — list, filter, delete a stale key live and verify on GitHub; openclaw/openclaw read-only list for scale. Screenshots in PR.

## Performance Budget (hard gates)
- Zero cache API calls on default `runs` path (call-count test).
- List of 200+ caches paginates within 2 pages-per-second budget; screen paint async (Task 220 pattern).

## Voice (MUST)
The cache is the `kennel`. Usage header: `kennel: 7.2/10 GB`. Delete toast: `dug that one up.` Warning: `kennel's almost full — GitHub starts evicting at 10 GB.` Hound voice across TUI/docs. No emoji.

## Website & Docs Updates
- Landing: engineering section gains nothing (it's about API manners) but README feature table + agent-surface/SKILL get the verb. If a caches gallery tab is added, capture via `.claude/automations/capture_fixture.py`. **Voice MUST hold.**
- `docs/visual-contract.md`: caches screen states.

## Definition of Done
- [ ] Red-first; race suite green; `make ci && make e2e && make vqa && make docs-check` pass.
- [ ] Pipe verb (list/usage/delete) + TUI screen with gauge, filter, confirm-gated delete shipped.
- [ ] Live delete round-trip verified on a real cache key (evidence in PR).
- [ ] Call-count gates green; async paint verified.
- [ ] tui-qa cold-context `VERDICT: PASS` at all breakpoints; dootsabha (codex + gemini) converged.
- [ ] Docs updated in hound voice.

## Verification Commands
```bash
make ci && make e2e && make vqa
./bin/gh-hound caches --no-tui --json
./bin/gh-hound caches --delete-key "go-mod-<stale>" --ref refs/heads/main --no-tui --json; echo $?
```

## Session Protocol
1. Pin live cache payloads in adapter testdata first (this repo's CI produces real caches).
2. Red → green → refactor on the artifacts pattern; shux + tui-qa; dootsabha; push; PR; gh-ghent loop.
