# gh-hound — Product Requirements Document

> **A fast, focused, beautiful TUI that hunts down your GitHub Actions CI.** `gh hound`.
> Distributed as a precompiled `gh` CLI extension. Built in Go 1.26 on Bubble Tea v2.

| | |
|---|---|
| **Version** | 1.0 — implementation-ready |
| **Status** | Approved for build (v1) |
| **Owner** | indrasvat |
| **Implementers** | Claude Code / Codex (autonomous coding agents) |
| **Companion artifact** | `gh-hound-design.html` — the **visual source of truth** for every screen. This PRD specifies *behaviour, structure, and acceptance*; the HTML specifies *exact look*. Implement to both. |
| **Repo** | `github.com/indrasvat/gh-hound` |
| **Binary** | `gh-hound` (invoked as `gh hound …`) |

---

## §0 · How to read this PRD (agent guide)

**This document is built for progressive disclosure.** You do not need to read it end to end to start work.

1. Read **§1 Executive summary** (always).
2. Read **§2 Architecture** and **§3 Tech stack & conventions** once, to internalise the seams and rules.
3. Then jump to the **one section** for the unit of work you're implementing. Every section from §7 onward is **self-contained and anchorable**: it states its inputs, dependencies, behaviour, contextual keybindings, states, and its own **Definition of Done (DoD)**.

**Per-section header convention.** Each implementable section begins with:

> **Implement if:** _what task this section covers_
> **Depends on:** _§refs that must exist first_
> **Visual ref:** _which screen(s) in `gh-hound-design.html`_
> **DoD:** _see the checklist at the end of the section_

**Three rules that are never optional, in any section:**

- **VR (Visual Rigor).** No screen is Done until its **shux snapshots pass the visual checklist at all three breakpoints** (§16). This is a hard gate. Agents reliably miss alignment, colour bleed, truncation, focus, and mock-fidelity — the shux loop exists to catch exactly that.
- **KB (Keyboard Rigor).** Keyboard navigation is non-negotiable (§7). Every binding is contextual, conflict-free, and reflected in the on-screen footer; the help modal is reachable from anywhere.
- **No emoji.** Status and chrome use text-presentation Unicode only (§6). A CI check greps the binary's string table and the rendered snapshots for emoji codepoints.

**Section index**

- [§1 Executive summary](#1--executive-summary)
- [§2 Architecture](#2--architecture)
- [§3 Tech stack & conventions](#3--tech-stack--conventions)
- [§4 GitHub API integration (data layer)](#4--github-api-integration-data-layer)
- [§5 Theme & design tokens — "Bramble"](#5--theme--design-tokens--bramble)
- [§6 Icon system](#6--icon-system)
- [§7 Keyboard navigation & input model](#7--keyboard-navigation--input-model)
- [§8 Launch & context resolution](#8--launch--context-resolution)
- [§9 Screens](#9--screens)
- [§10 Progressive disclosure & responsive layout](#10--progressive-disclosure--responsive-layout)
- [§11 Performance](#11--performance)
- [§12 Errors, toasts & resilience](#12--errors-toasts--resilience)
- [§13 Dual surface & agent integration](#13--dual-surface--agent-integration)
- [§14 Configuration](#14--configuration)
- [§15 Distribution](#15--distribution)
- [§16 Visual verification with shux (hard gate)](#16--visual-verification-with-shux-hard-gate)
- [§17 Definition of Done](#17--definition-of-done)
- [§18 Build phases & implementation order](#18--build-phases--implementation-order)
- [§19 Repo lineage & learnings](#19--repo-lineage--learnings)
- [§20 Caveats & gotchas](#20--caveats--gotchas)
- [§21 References (verified)](#21--references-verified)
- [Appendix A — Data model](#appendix-a--data-model)
- [Appendix B — JSON output schema](#appendix-b--json-output-schema)
- [Appendix C — Full keymap table](#appendix-c--full-keymap-table)

---

## §1 · Executive summary

**What.** `gh hound` is a terminal UI for GitHub Actions and nothing else. It answers the five questions a CI tool must answer instantly: **is it green? what's running? why did it fail? re-run it. watch it.** It opens to your current branch's runs, lets you drill run → job → step, surfaces the de-noised failure with annotations, streams logs as a job runs, and re-runs / cancels / dispatches — without leaving the terminal or clicking through the web UI.

**Why.** The Actions web UI is the most-cited friction in CI debugging: three-to-four page loads to reach an error ("the DMV of CI"), logs that reveal slowly and can crash a browser tab, the failure buried in nested collapsible groups, re-running blind. `gh hound` collapses that to one launch, zero clicks to the error, and one key to act. (Pain-point sources in §21.)

**Who.** Terminal-first platform/infra engineers, and the coding agents (Claude Code, Codex, दूतसभा) that drive CI fix-loops.

**Wedge & moat.** No mature "k9s for Actions" exists; `gh-dash` is PR-first, dedicated TUIs are hobby-grade. The moat is the de-noised failure view, the colour log viewer, and a dual human/agent surface — the parts the web UI and raw MCP servers won't casually build.

**v1 scope (the daily loop), each backed by a real endpoint:**

1. Runs list, branch-scoped, server-side filtered, live.
2. Run detail — jobs + step timeline (master-detail).
3. De-noised failure view (error window + annotations).
4. Watch — live step status + streamed-per-step logs.
5. Colour log viewer — folding, search, ANSI + syntax.
6. Re-run (run / failed jobs / single job) & cancel / force-cancel.
7. Dispatch with typed inputs.
8. Dual surface (`--no-tui --json`), exit codes `0/1/2/3`, Agent Skill.

**v1 Definition of Done:** you stop opening the Actions web tab for your own repos; every screen passes its shux visual gate at three breakpoints; keymap has zero conflicts; help modal works from every context; `make check` is green.

**Non-negotiables:** keyboard navigation (§7), visual fidelity verified via shux (§16), grounding in the real API (§4), the Bramble theme (§5), no emoji (§6).

**Non-goals (v1):** PR review, fleet/multi-repo, billing/usage charts, runner administration, secret/variable management. (See roadmap, §18.)

---

## §2 · Architecture

> **Implement if:** scaffolding the project or any cross-cutting concern.
> **Depends on:** nothing.
> **Visual ref:** n/a.

### §2.1 Hexagonal core — `tui → usecase → adapter`

Inherited verbatim from **vivecaka**. Three layers, one-directional dependencies:

```
          ┌─────────────────────────────────────────────┐
 input →  │  tui/        Bubble Tea models, views,        │  → terminal
          │              key handling, layout, overlays   │
          └───────────────────────┬─────────────────────┘
                                   │ calls (never imports tui)
          ┌───────────────────────▼─────────────────────┐
          │  usecase/    pure orchestration:              │
          │              "load branch runs", "rerun       │
          │              failed", "stream job log".        │
          │              No Bubble Tea, no HTTP.            │
          └───────────────────────┬─────────────────────┘
                                   │ depends on a port (interface)
          ┌───────────────────────▼─────────────────────┐
          │  adapter/    GitHubAdapter (port):            │
          │              go-gh REST/GQL, ETag cache,       │
          │              serial queue, poller, log parse.  │  → GitHub API
          └─────────────────────────────────────────────┘
```

- `usecase` depends on a **port interface** (`adapter.GitHub`), never the concrete client → unit tests inject a fake; agent verification (§16) and offline dev work without the network.
- The **same `usecase` layer powers both faces**: the TUI renders its results, and the `--no-tui` JSON path serialises them. The render surface is the only thing that differs (gh-ghent's "two faces, one core").

### §2.2 Package layout

```
gh-hound/
├── main.go                      # cobra root, gh-extension entrypoint
├── go.mod                       # go 1.26
├── cmd/
│   └── hound/                   # cobra commands: root, watch, runs, dispatch, version
├── internal/
│   ├── tui/                     # Bubble Tea: app model, screen models, overlays, keymap
│   │   ├── app.go               # root model, modal/focus stack, routing
│   │   ├── keys/                # key.Map definitions per context (§7)
│   │   ├── screens/             # one model per screen (§9)
│   │   ├── components/          # reusable: statuspill, sparkline, toast, breadcrumb
│   │   └── overlay/             # help modal, command palette (lipgloss Canvas/Layer)
│   ├── usecase/                 # orchestration (no Bubble Tea, no HTTP)
│   ├── adapter/                 # GitHub port + go-gh implementation + cache + poller + log parse
│   ├── model/                   # Run/Job/Step/Annotation structs (Appendix A)
│   ├── theme/                   # Bramble + Bone tokens, lipgloss styles (§5)
│   ├── layout/                  # responsive breakpoints, split-pane engine (from nidhi)
│   ├── render/                  # JSON/md/xml serialisers for the pipe face (§13)
│   └── config/                  # TOML load/merge, keybinding overrides (§14)
├── .claude/automations/         # shux visual-verification harness (§16)
├── .github/workflows/           # ci.yml, release.yml (gh-extension-precompile)
└── Makefile                     # check / test / e2e / vqa / smoke-test
```

### §2.3 Concurrency model

- **One owner of the network:** the adapter runs a **single serial request queue** (a goroutine consuming a channel). The TUI never calls the network directly; it dispatches `tea.Cmd`s that enqueue work and return a `tea.Msg` on completion. This dodges GitHub's secondary rate limits (§4, §11).
- **Cache-first render.** The poller writes to an in-memory store (keyed by run/job IDs, with ETags). The TUI renders off the store; network latency never blocks a keystroke (vicaya's daemon-decoupled paint, in-process).
- **Adaptive poller.** One ticker whose interval is a function of state: fast (~2–3s) while any visible run/job is `in_progress`, slow/paused when idle. Every poll is a conditional request (§4, §11).

**§2 DoD:** packages compile and respect the dependency direction (a lint rule or `go list` check forbids `usecase`/`adapter` importing `tui`); a fake `adapter.GitHub` exists and the app runs against it offline.

---

## §3 · Tech stack & conventions

> **Implement if:** scaffolding, choosing a dependency, or wiring CLI/flags/logging.
> **Depends on:** §2.

### §3.1 Versions (use the latest; pin in `go.mod`)

| Concern | Choice | Module |
|---|---|---|
| Language | **Go 1.26** | `go 1.26` in `go.mod` |
| TUI runtime | Bubble Tea **v2** | `charm.land/bubbletea/v2` |
| Styling | Lip Gloss **v2** | `charm.land/lipgloss/v2` |
| Components | Bubbles **v2** | `charm.land/bubbles/v2` |
| Forms | huh (latest) | `github.com/charmbracelet/huh` |
| GitHub API + auth | go-gh **v2** | `github.com/cli/go-gh/v2` |
| Commands/flags | cobra | `github.com/spf13/cobra` (optionally wrap help with `charmbracelet/fang`) |
| ANSI parsing | x/ansi | `github.com/charmbracelet/x/ansi` |
| Syntax accents | chroma | `github.com/alecthomas/chroma/v2` |
| Logging | stdlib `log/slog` → JSON | (no dep) |
| Config | TOML | `github.com/BurntSushi/toml` |

> **Gotcha (Lip Gloss v2):** `AdaptiveColor` was **removed**. Light/dark is explicit — query once via `tea.RequestBackgroundColor` → `tea.BackgroundColorMsg.IsDark()`, then select the Bramble/Bone token set. Do **not** reach for `AdaptiveColor`. (§5, §20)

> **Sparklines:** hand-roll (map a normalised value to one of `▁▂▃▄▅▆▇█`). No charting dependency required for v1.

> **Dependency baseline = gh-ghent (the approved extension), upgraded to the v2 Charm stack.** gh-ghent's `go.mod` is `module github.com/indrasvat/gh-ghent`, `go 1.26.0`, with `cli/go-gh/v2`, `spf13/cobra`, `golang.org/x/sync` (serial queue / errgroup), `muesli/termenv`, `google/go-cmp` (tests). Mirror those exact choices; the **only** deliberate divergence is using **Bubble Tea / Lip Gloss / Bubbles v2** (as nidhi does) instead of ghent's v1.

### §3.2 CLI conventions (Robin's standing prefs)

- Every flag supports **arg + env var**: `--flag`, and `HOUND_<FLAG>` (e.g. `HOUND_DEFAULT_SCOPE`). Auth/repo env (`GH_TOKEN`, `GH_HOST`, `GH_REPO`) are inherited via go-gh — do not re-implement.
- Clean subcommands with helpful `--help`. Root with no subcommand = launch the TUI (§8).
- Structured **JSON logs** via `slog` to the XDG state dir `~/.local/state/gh-hound/gh-hound.log` (pattern from nidhi). `--log-level debug`; `--trace-http` logs every API call: method, path, status, duration, `etag`, `x-ratelimit-remaining`.
- Conventional Commits. `gofmt`/`goimports` clean. `golangci-lint` clean. No AI/Claude attribution in commits or files (Robin's standing rule).
- Output build artifacts and generated files under `out/` where applicable; never leave generated junk in the repo root.

### §3.3 Makefile targets (mirror nidhi)

| Target | Does |
|---|---|
| `make ci` | **what CI runs** (gh-ghent's entrypoint): `make check` + `go vet ./...` + `go build ./...` |
| `make check` | `golangci-lint run` (v2) + `make test` (the local green-bar gate) |
| `make test` | race + coverage via `gotestsum` |
| `make e2e` | end-to-end against a fixture repo (real API, recorded or live) |
| `make vqa` | **shux visual-quality audit** (§16) — the visual gate |
| `make demo` | render README gif via **VHS** (from a `.tape`) |
| `make smoke-test` | release smoke test (install → `gh hound --version` → launch → quit) |

**§3 DoD:** `go build ./...` on Go 1.26; `make check` green; `--version` prints semver + commit; `slog` writes JSON to the XDG state path; every flag has a matching env var.

---

## §4 · GitHub API integration (data layer)

> **Implement if:** building the adapter, models, poller, or log fetch/parse.
> **Depends on:** §2, §3.
> **Visual ref:** n/a (feeds every screen).

**Auth & client.** Use `go-gh/v2`: `gh.RESTClient(opts)` and `gh.GQLClient(opts)` resolve host + token from the `gh` environment (`GH_TOKEN`/`GH_HOST`/stored OAuth). Resolve the active repo with `repository.Current()` (respects `GH_REPO`, falls back to the git remote). **Never prompt for or store a token** — inheriting `gh` auth is the whole point and unlocks the **5,000 req/hr** authenticated ceiling (vs 60 unauthenticated).

**REST is the backbone. GraphQL is optional.** The entire Actions domain — runs, jobs, steps, logs, rerun, cancel, dispatch — is **REST only**. GraphQL has no `WorkflowRun`/`Job`/`Step` types; it only exposes the **Checks rollup** (`CheckSuite`/`CheckRun`/`StatusContext`) on a ref. Use GraphQL *only* for an optional one-query "is this ref green" rollup (incl. third-party checks). Everything else is REST. API version header: **`2026-03-10`**.

### §4.1 Read & logs

| Method | Endpoint | Returns / use |
|---|---|---|
| `GET` | `/repos/{o}/{r}/actions/runs` | Runs list. Server-side filters: `status`, `branch`, `event`, `actor`, `head_sha`, `created`, `per_page≤100`, `page`. **Push filtering to the server.** Returns `total_count` + `workflow_runs[]`. |
| `GET` | `…/actions/workflows/{id}/runs` | Runs for one workflow. |
| `GET` | `…/actions/runs/{run_id}` | One run. |
| `GET` | `…/runs/{run_id}/jobs?filter=latest` | Jobs + embedded `steps[]`. Use `filter=latest` by default; `filter=all` only when inspecting prior attempts. |
| `GET` | `…/actions/jobs/{job_id}` | One job. |
| `GET` | `…/actions/workflows` | Workflow definitions + `state` (for the dispatch picker; only `workflow_dispatch`-enabled ones are dispatchable). |
| `GET` | `…/actions/jobs/{job_id}/logs` | **Plain text**, 302 redirect, link **expires in 60s**. Single-job drill-down uses this — skips the zip. |
| `GET` | `…/runs/{run_id}/logs` | **Zip** archive, 302. Only when a whole-run export is needed. |
| `GET` | `…/check-runs/{check_run_id}/annotations` | Failure annotations (`path`, `start_line`, `annotation_level`, `message`). `check_run_id` comes from `job.check_run_url`. |

### §4.2 Write (`Actions: write`, space mutations ≥1s apart)

| Method | Endpoint | Action |
|---|---|---|
| `POST` | `…/runs/{id}/rerun` | Re-run whole workflow (body: `enable_debug_logging`). |
| `POST` | `…/runs/{id}/rerun-failed-jobs` | Re-run only failed jobs. |
| `POST` | `…/jobs/{id}/rerun` | Re-run one job + dependents. |
| `POST` | `…/runs/{id}/cancel` | Cancel. |
| `POST` | `…/runs/{id}/force-cancel` | Force-cancel a stuck run. |
| `POST` | `…/workflows/{id}/dispatches` | Trigger `workflow_dispatch` (body: `ref`, `inputs`). |
| `POST` | `…/runs/{id}/pending_deployments` | Approve/reject a gated environment **(v2)**. |
| `POST` | `…/runs/{id}/approve` | Approve a fork-PR run awaiting review **(v2)**. |

### §4.3 Enums (exact, do not invent)

- **`status`**: `queued`, `in_progress`, `completed`, `requested`, `waiting`, `pending`.
- **`conclusion`**: `success`, `failure`, `cancelled`, `skipped`, `neutral`, `timed_out`, `action_required`, `stale`, `null` (when not completed).

Model fields are enumerated in **Appendix A**. Each enum value maps to exactly one glyph + colour (§5, §6) — nothing decorative is invented.

### §4.4 Log parsing (de-noise)

1. Fetch the **plain-text job log** (§4.1). Follow the 302; do **not** cache or reuse the redirect URL (60s expiry).
2. Split on `##[group]` / `##[endgroup]` into foldable sections; treat `##[error]`, `##[warning]` as level markers.
3. **Error window:** find the failing step (the step whose `conclusion=failure`) by correlating step `started_at`/`completed_at`/`number` with the log's step markers; extract ~12 lines around the first `##[error]` / first `FAIL`/`error:` token.
4. Merge in **annotations** (§4.1) keyed by `path:line`.
5. Preserve ANSI; parse with `x/ansi` into styled spans for the colour viewer (§9.6). Apply light syntax accents via `chroma` for common patterns (test output, stack traces).

### §4.5 Caching, polling, rate limits

- **Conditional requests.** Store the `ETag` per resource; send `If-None-Match`. A **304** costs **zero** against the primary limit → idle polling is effectively free.
- **Serial queue + spacing.** All requests serial (secondary-limit safe); mutations spaced ≥1s.
- **Backoff.** On `403`/`429`: honour `retry-after`; else if `x-ratelimit-remaining: 0`, wait until `x-ratelimit-reset`; else exponential backoff. Surface as a toast (§12), keep showing cached data.

**§4 DoD:** adapter returns typed models for all read paths; ETag/304 verified (a `--trace-http` run shows 304s on unchanged polls); mutations succeed and are spaced ≥1s; log de-noise produces the correct error window + annotations for a known failing run; the port interface is fake-able and unit-tested.

---

## §5 · Theme & design tokens — "Bramble"

> **Implement if:** building `internal/theme`, or styling any view.
> **Depends on:** §3.1 (no `AdaptiveColor`).
> **Visual ref:** §07 "Theme & design tokens" in `gh-hound-design.html`.

Default **Bramble** (dark): warm ink-and-bone neutrals, one electric "on-the-scent" green, earthy signal hues. `T` cycles to **Bone** (light). Token names map 1:1 between the HTML and the Lip Gloss styles so the mock and the build cannot drift.

### §5.1 Tokens (Bramble dark / Bone light)

| Token | Bramble | Bone | Role → Lip Gloss |
|---|---|---|---|
| `bg` | `#0E0F0C` | `#EFEDE1` | app canvas — base `Background` |
| `bg-elev` | `#141512` | `#E7E4D5` | elevated panels |
| `surface` | `#1B1D17` | `#DEDACB` | cards / inactive panes |
| `surface-2` | `#24271E` | `#D2CDBA` | **selected-row fill** |
| `line` / `line-2` | `#2E3227` / `#3D4233` | `#CBC6B2` / `#B5B09A` | borders / focused `BorderForeground` |
| `fg` → `dim` | `#EAE8D9` → `#6B7060` | `#23241C` → `#8A8773` | text ramp (`fg`,`fg-soft`,`muted`,`subtle`,`dim`) |
| `ok` | `#4FD37A` | `#1F9E55` | success · live · cursor bar · brand |
| `fail` | `#E2564B` | `#C24033` | failure · error |
| `run` | `#E0A33E` | `#B57A1E` | in_progress |
| `info` | `#6E9CB5` | `#3E7491` | queued/pending · keys · sparkline |
| `warn` | `#E8895A` | `#C2632F` | action_required · timed_out |
| `neutral` | `#6B7060` | `#8A8773` | cancelled · stale · skipped |

### §5.2 Style rules

- **Border:** Lip Gloss `RoundedBorder()`, 1-cell padding. **Focused** pane uses a gradient border via `BorderForegroundBlend(ok, info)`.
- **Selection:** `surface-2` background fill **plus** a green (`ok`) left bar (1 cell, positional). The bar is **never** a status glyph.
- **Title bars are composed, not native.** Lip Gloss borders are plain rectangles; render the panel title as a styled header row (or overlay it onto the top border). Do not expect `Border()` to embed text. (§20)
- **Degradation:** truecolor → 256 → 16 → ASCII handled by the Lip Gloss colour profile; verify the 256-colour fallback looks intentional.
- **Type (HTML mock only):** display Bricolage Grotesque, body Hanken Grotesk, mono JetBrains Mono. In the terminal the font is the user's; rely on weight/colour/spacing, not typeface.

### §5.3 Semantic map (status → token)

`success→ok` · `failure→fail` · `in_progress→run` · `queued|pending|waiting→info` · `cancelled|stale→neutral` · `skipped|neutral→neutral` · `action_required|timed_out→warn`.

**§5 DoD:** a single `theme.Theme` struct holds all tokens; `T` swaps Bramble↔Bone live and **every** view recolours (no hard-coded colours anywhere outside `internal/theme`); 256-colour fallback verified via shux on a `TERM` without truecolor.

---

## §6 · Icon system

> **Implement if:** rendering any status, chrome, or hint glyph.
> **Depends on:** none.
> **Visual ref:** §08 "Icon system" in the HTML.

**No emoji, ever.** Text-presentation Unicode only — colour carries meaning, rendering is identical across terminals, no Nerd Font required.

| Meaning | Glyph | Code | Meaning | Glyph | Code |
|---|---|---|---|---|---|
| success | `✔` | U+2714 | cursor | `▌` | U+258C |
| failure | `✗` | U+2717 | branch | `⎇` | U+2387 |
| in_progress | `⠹` (braille spinner) | — | breadcrumb | `›` | U+203A |
| queued | `◌` | U+25CC | fold open/closed | `▾`/`▸` | U+25BE/B8 |
| cancelled | `⊘` | U+2298 | sparkline | `▁▂▃▅▇` | blocks |
| skipped | `⊝` | U+229D | prompt | `❯` | U+276F |
| action_required | `▲` | U+25B2 | rerun | `↻` | U+21BB |
| timed_out | `⧗` | U+29D7 | dispatch | `▶` | U+25B6 |
| neutral | `◇` | U+25C7 | enter/back/esc | `⏎`/`⎋` | U+23CE/238B |

> **Gotcha:** `✔` (U+2714), `✗`-adjacent dingbats, and `▶` (U+25B6) are *emoji-capable* but render as **text** as long as no Variation-Selector-16 (`U+FE0F`) is appended. Never emit VS16. The emoji CI check fails the build on any `U+FE0F` or astral-plane codepoint.

**§6 DoD:** all glyphs centralised in one `icons` package; the emoji scanner (string table + shux snapshots) reports zero emoji.

---

## §7 · Keyboard navigation & input model

> **Implement if:** building input handling, the keymap, focus, or any screen's bindings. **This is the most-scrutinised section. Build it rock-solid.**
> **Depends on:** §2.
> **Visual ref:** §⑩ Help in the HTML (the legend + grouping).

Keyboard navigation is the product. Patterns inherited from nidhi: `Tab` toggles focus; `j/k` + `Ctrl+d/Ctrl+u` act on the **focused** pane; destructive actions double-confirm.

### §7.1 Layered keymap

Implement with `bubbles/v2/key` as a layered `key.Map`. Resolution order per keypress:

1. **Input mode** (a text field is focused) — see §7.4. Captures all printable keys.
2. **Active overlay** (help modal / command palette / confirm prompt) — see §7.5.
3. **Active screen context** (runs / detail / log / watch / dispatch).
4. **Global** (always available when not in input mode).

A key bound at a higher layer **shadows** lower layers; document every shadow. A unit test asserts **no unintended collisions** within a layer (§7.6).

### §7.2 Global keys (any context, not in input mode)

| Key | Action |
|---|---|
| `?` | Toggle the **help modal** (contextual; §7.7) |
| `:` | Open the **command palette** |
| `T` | Cycle theme (Bramble ↔ Bone) |
| `Esc` | Pop the top layer (close modal → close palette → exit detail → … → quit prompt) |
| `q` | Same as `Esc` at top level → quit prompt; closes overlays otherwise |
| `Ctrl+C` | Immediate quit (always) |
| `R`* | Refresh / force-poll now (*not in input mode) |

### §7.3 Context bindings (mnemonic, conflict-free)

Convention: **lowercase = narrowest sensible action; Shift = broader/stronger.** Destructive actions (cancel, force-cancel, rerun-all) **double-confirm** (§7.5).

| Key | Runs list | Run detail | Log / Failure | Watch |
|---|---|---|---|---|
| `j`/`k`, `↓`/`↑` | move row | move (focused pane) | scroll line | — |
| `Ctrl+d`/`Ctrl+u` | half-page | half-page | half-page | — |
| `g`/`G` | top / bottom | top / bottom | top / bottom | — |
| `⏎` | open run → detail | expand step / open log | — | — |
| `Tab`/`Shift+Tab` | — | cycle jobs↔steps focus | — | — |
| `J`/`K` | — | next / prev run | — | — |
| `l` | logs of selected run | full log of selected job | (in failure) full log | — |
| `w` | watch selected run | watch this run | — | — |
| `o` | open in browser | open in browser | open in browser | — |
| `y` | copy run URL | copy URL / `Y` copy SHA | `y` copy excerpt | — |
| `r` | rerun **failed jobs** | rerun **this job** | rerun job | — |
| `R` | rerun **entire run** | rerun **failed jobs** | rerun failed | — |
| `x` / `X` | cancel / force-cancel | cancel / force-cancel | — | `x` cancel run |
| `D` | dispatch workflow | — | — | — |
| `n`/`N` | — | jump to failed step | next / prev search match | — |
| `z`/`Z` | — | — | fold / unfold all | — |
| `/` | filter list | — | search | — |
| `f` | — | — | follow toggle | follow toggle |
| `d` | — | — | — | toggle debug logging |
| `A` | approve deploy (v2) | — | — | — |

### §7.4 Input mode (rock-solid rule)

When a text input is focused (list filter, log search, dispatch fields):

- **All printable keys insert text.** Single-letter commands are **suppressed** — typing `q` types `q`, it does not quit. This is the #1 source of broken TUI keynav; it must be impossible here.
- Special keys: `Esc` blur/cancel (restores prior state), `Enter` submit/apply, `Tab`/`Shift+Tab` next/prev field (forms). `Ctrl+C` still quits.
- The footer switches to input affordances (`⏎ apply · ⎋ cancel`).

### §7.5 Overlays & confirms

- **Modal stack.** Overlays push onto a stack; `Esc` pops exactly one. Only the top overlay receives keys; the base is dimmed and inert.
- **Confirm prompt** (destructive: cancel, force-cancel, rerun-all): a small centred modal, default **No**, `y`/`Enter` confirm, `n`/`Esc` abort. Force-cancel requires explicit `y` (no Enter default).

### §7.6 Contextual footer (auto-generated, never drift)

Each screen renders a footer from its **own** `key.Map` via `bubbles/v2/help` ShortHelp — so the hints shown **are** the bindings that work. `?` expands to the full grouped help (FullHelp). The footer must show the 5–7 most relevant bindings for that exact context (see each screen in §9).

### §7.7 Help modal (callable from anywhere)

- Reachable with `?` from any non-input context (in input mode, `Esc` first). Rendered as a centred **Lip Gloss Canvas/Layer** over a dimmed base.
- Content is **contextual**: the active screen's full keymap **+** global keys **+** the status legend. Three columns (Navigate / Actions / View) as in the HTML.
- Dismiss with `?`, `Esc`, or `q`.

**§7 DoD:** (1) a table-driven test proves no unintended collisions per layer; (2) automated shux interaction tests (§16) drive every binding on every screen and assert the resulting state; (3) input mode swallows single-letter commands (tested); (4) `?` opens the correct contextual help from **every** screen and `Esc` always pops exactly one layer; (5) the footer hints equal the active keymap (generated, not hard-coded).

---

## §8 · Launch & context resolution

> **Implement if:** building the entrypoint / no-arg behaviour.
> **Depends on:** §4 (repo resolution), §9.1/§9.2 (landing screens).
> **Visual ref:** §③ default-behaviour section + §① all-green + §② runs list in the HTML.

`gh hound` with no args opens to a **predictable home** that is context-scoped and live, so you effectively see your current CI immediately (model: `lazygit`/`k9s`).

### §8.1 Resolution order

1. Resolve repo: `repository.Current()` (`GH_REPO` → git remote). `-R owner/repo` overrides.
2. Resolve branch: current `HEAD`. 
3. Land on **runs list scoped to the branch** (`GET …/runs?branch=<branch>&per_page=30`), newest first, **auto-select the newest run**, detail pane showing its step timeline. If that run is `in_progress`, it's already live-updating — no keypress.

### §8.2 Fallbacks (no blank screens — every one handled)

| Situation | Behaviour |
|---|---|
| Branch has zero runs | Auto-widen to repo-wide; one-line notice `no runs on <branch> — showing all branches`. |
| Repo has no workflows / Actions disabled | Explicit empty state with next steps. |
| Not a git repo / no remote | Friendly error: suggest `gh hound -R owner/repo`. |
| Detached HEAD / amended-unpushed SHA | Fall back to branch, then repo-wide. Never silent error. |
| All runs green | **All-green screen** (§9.2) — calm summary, de-emphasised list. |

### §8.3 Overrides & subcommands

| Invocation | Effect |
|---|---|
| `gh hound` | current branch, newest in focus, live (default) |
| `gh hound -A` / `--all` | repo-wide |
| `gh hound watch` | full-screen auto-attach to the current/latest in-progress run (§9.5) |
| `gh hound runs` / `dispatch` | jump straight to that surface |
| `gh hound -R owner/repo` | another repo |
| `gh hound … --no-tui --json` | pipe face (§13) |

Config: `default_scope = branch|repo`, `auto_watch = bool`, `per_page = N` (§14).

**§8 DoD:** each fallback row reproduced and verified; `gh hound` in a fresh clone with a running CI shows the live run in focus within one poll cycle; `-R`, `-A`, `watch`, `--no-tui` all route correctly.

---

## §9 · Screens

> **Implement if:** building any screen. Each subsection is self-contained.
> **Depends on:** §4 (data), §5 (theme), §6 (icons), §7 (keys), §10 (layout).
> **Visual ref:** the numbered screen in `gh-hound-design.html` cited per subsection. **The HTML is the pixel truth; match it.**

### §9.0 Screen flow

```
launch (§8)
  └─► resolve repo+branch
        ├─ all green ───────► [§9.2 ALL-GREEN] ──┐
        └─ mixed/failing ───► [§9.1 RUNS LIST] ◄─┘  (default home)
                                  │
        ┌──────────────┬──────────┼───────────┬──────────────┐
        ▼              ▼          ▼           ▼              ▼
  [§9.3 DETAIL]   [§9.5 WATCH] [§9.7 DISPATCH] [§9.8 PALETTE] [§9.9 HELP]
        │  (⏎ on run)  (w)         (D)            (:)            (?)
        ▼
  [§9.4 FAILURE] ──(l)──► [§9.6 LOG VIEWER]
        ▲ (n jump)
```

Transitions: `⏎` descends, `Esc` ascends, `:`/`?` overlay from anywhere, `J/K` moves to the sibling run while keeping depth. Toasts (§12) float over any screen.

### §9.0.1 Welcome & version banner — visual ref ⓪

- **Banner asset.** The `HOUND` wordmark is **figlet "ANSI Shadow"**, embedded as a Go **constant** (do not add a runtime figlet dependency). Colour it with a Lip Gloss green→steel→bone blend (`ok → info → fg`), per-line or per-rune. Box-drawing/block glyphs only — no emoji. This is the same family of banner gh-ghent/nidhi/vivecaka render on launch.
- **`gh hound --version`** prints: the gradient banner, then `vX.Y.Z · commit <sha> · built <date>`, then the tagline `Hunt down your GitHub Actions CI` — the exact shape of `gh ghent --version`. Version/commit/date are injected at build time via `-ldflags` in `scripts/build-release.sh` (§15).
- **First-run welcome splash** (optional, dismissible — nidhi/yukti pattern): banner + tagline + three cards (**WATCH** / **DIAGNOSE** / **RERUN**) + `⏎ Press Enter to continue` + version line. Shown **only** on first run; disabled by `welcome = false` (§14) or a "don't show again" action. `⏎` → runs list (§9.1); `?` help; `q` quit. Never shown with `--no-tui` or when piped.
- **DoD:** `--version` renders the gradient banner + build metadata; the splash shows once then honours the dismissal flag; both emoji-free; the gradient degrades cleanly on a 256-colour terminal (shux check, §16). **VR gate.**

Per screen below: **purpose · when shown · layout/components · data · contextual keys · states · DoD**.

---

### §9.1 Runs list (landing) — visual ref §②

- **Purpose / when:** the home screen; the no-arg default mid-flight (some runs failing/running).
- **Layout:** focused panel (gradient border). Header: `hound` mark · breadcrumb (`⎇ branch · @actor`) · right: rate budget, live pulse, `304` cache indicator. Body: a `bubbles/v2/table` — columns `status-glyph · Workflow · Event · # · Duration(sparkline) · Age`. Selected row = `surface-2` fill + green bar. Summary line: `N failing · N running · N passed`. Footer (contextual).
- **Components:** `table`, `spinner` (live cell), `sparkline` (hand-rolled bars), status pills, `breadcrumb`.
- **Data:** `GET …/runs?branch&per_page` (§4.1); adaptive poll; ETag.
- **Keys:** `j/k g/G ⏎ l w o y r R x X D / : ? T` (§7.3). Footer: `⏎ open · ↻ rerun · ✗ cancel · l logs · w watch · / filter · ? help`.
- **States:** loading (skeleton rows + spinner), empty (→ widen, §8.2), error (toast + cached rows, §12), live (spinner cells update in place).
- **DoD:** columns align at all breakpoints (§10); selection bar + fill render; live cell animates without tearing (synchronized output); filter (`/`) narrows server-side; footer == keymap; matches §② mock. **VR + KB gates.**

### §9.2 All-green state — visual ref §①

- **Purpose / when:** every visible run is green. Answers "is it green?" before reading rows; calm, gets out of the way.
- **Layout:** a green-tinted summary band — large `✔`, `All checks passing on <branch>`, `N recent runs · 0 failing · last <age>`, right-aligned latest-run meta. Below: de-emphasised recent (all-green) rows. Footer biases to `w watch next push · D dispatch · / filter · ?`.
- **Data:** same as §9.1; this is a render variant when `failing == 0 && running == 0`.
- **DoD:** triggers exactly when nothing is failing/running; band + dimmed list render; matches §① mock. **VR gate.**

### §9.3 Run detail (master-detail) — visual ref §③

- **Purpose / when:** `⏎` on a run.
- **Layout:** `lipgloss.JoinHorizontal` of two bordered panes. **Left:** jobs (`table`/`list`) — glyph · name · duration. **Right:** the selected job's step timeline in a `viewport` — glyph · # · step name · duration; the **failed step is highlighted** (red-tinted fill + bar) and pre-targeted by `n`. Right pane header: job name · status pill · runner chip · duration. Breadcrumb: `repo › CI #571 › branch · @sha`.
- **Keys:** `Tab` cycles pane focus; `j/k` on focused pane; `J/K` sibling run; `n` jump to failure; `l` full log; `r` rerun job; `R` rerun failed; `x/X` cancel; `⎋` back. Footer: `⏎ expand · ↻ rerun job · R rerun failed · ✗ cancel · ⎋ back · ?`.
- **States:** loading (jobs shimmer), partial (some jobs still queued), error (toast).
- **DoD:** two panes align and share height; focus visibly moves with `Tab`; failed step highlighted and `n` lands on it; on narrow widths the right pane becomes a full-screen push (§10); matches §③. **VR + KB gates.**

### §9.4 Failure view — visual ref §④

- **Purpose / when:** the killer screen; reached by `n` (jump to failure) or opening a failed step.
- **Layout:** header `… › build › ✗ go test ./... · step 6 · exit 1` + `exit N` pill. **Annotations** block (each `✗ path:line — message`, path underlined). Then the **de-noised, coloured error window** (`12 of 1,284 lines`, `⤓ expand`) rendered with the §9.6 colour engine. Footer: `↻ rerun failed · r rerun job · l full log · o browser · y copy excerpt`.
- **Data:** job log (de-noise, §4.4) + annotations (§4.1).
- **DoD:** error window contains the actual failure (correct correlation, §4.4); annotations resolve from `check_run_url`; colours match the log token map; `l` opens the full viewer at the same offset; matches §④. **VR gate.**

### §9.5 Watch / live stream — visual ref §⑤

- **Purpose / when:** `w`, or `gh hound watch`; a run is in progress.
- **Layout:** focused panel, header with a **pulsing `streaming` badge** + elapsed + `follow ●`. Body = a follow-mode `viewport`: completed steps already coloured, the **active step tails** with a blinking block cursor (`tea.Cursor`); an "incoming" bottom fade. Footer: `✗ cancel · f follow · d debug · ⎋ detach`.
- **Honest mechanism (do not over-promise):** GitHub exposes **no line-stream socket**. Implement "streaming" as: poll job status (adaptive); when a step **concludes**, fetch and **append its log chunk**; for the active step, show live status (spinner) + the latest available lines. Paint atomically via synchronized output. This is what `gh run watch` does and is the truthful ceiling. (§20)
- **DoD:** step bars/timeline update on the poll without tearing; new step chunks append with autoscroll while `follow` is on; `f` toggles follow; cancel works; matches §⑤. **VR gate.**

### §9.6 Colour log viewer — visual ref §⑥

- **Purpose / when:** `l` from anywhere a log exists. **Beautiful, blazing-fast, fully coloured — never plain text.**
- **Layout:** `viewport` with a **line-number gutter** (dim), foldable `##[group]` sections (`▾`/`▸` + collapsed line count), a decorative scrollbar, header with the search query + `match m/n`. Search hit line highlighted (`warn` tint).
- **Colour engine:** parse ANSI with `x/ansi` (preserve 24-bit); layer syntax accents via `chroma` for common log patterns — timestamps dim, shell commands `info`, `ok`/PASS `ok`, `FAIL`/`##[error]` `fail`, warnings `run`, file paths underlined `info`, quoted got/want strings (`str`/`want`). Degrades to 256/16/ASCII.
- **Keys:** `j/k g/G Ctrl+d/u` scroll; `/` search, `n/N` match; `z/Z` fold/unfold all; `w` wrap toggle; `⎋` back. Footer mirrors these.
- **Performance:** must stay smooth on large logs — render only the viewport's visible window; lazily style off-screen lines; fold-aware. (§11)
- **DoD:** ANSI colours preserved; folds collapse/expand and counts are correct; search highlights + counts correct; scrolls smoothly on a 10k-line log; no colour bleed across lines; matches §⑥. **VR gate (color + alignment specifically).**

### §9.7 Dispatch — visual ref §⑦

- **Purpose / when:** `D`; only for `workflow_dispatch`-enabled workflows.
- **Layout:** a **huh** form — `ref` select (`▾`), then one field per declared input (text with live `tea.Cursor`, radio with `●`/`○`, select). A faint `POST …/workflows/<file>/dispatches` line. Footer: `⏎ run · ⇥ next · ⎋ cancel`.
- **Data:** workflow inputs schema from the workflow file / `GET …/workflows`. `POST …/dispatches` with `ref` + `inputs` (§4.2).
- **States:** validation errors inline; submit → success toast + return to list.
- **DoD:** fields reflect the workflow's declared inputs/types; `⇥`/`Shift+⇥` navigate; input mode (§7.4) holds; submit dispatches and toasts; matches §⑦. **VR + KB gates.**

### §9.8 Command palette — visual ref §⑧

- **Purpose / when:** `:` from anywhere; k9s-style jump.
- **Layout:** a centred **Canvas/Layer** overlay over a dimmed base. Prompt `❯` + cursor; a `list` filtered as you type; each row `name · description · tag`; selected row green bar + fill. Footer shows other surfaces (`workflows · watch · diff (v2) · theme`).
- **Keys:** type to filter (input mode), `j/k` move, `⏎` go, `Esc` close.
- **DoD:** opens over any screen; fuzzy filter works; selecting routes correctly; `Esc` pops only the palette; matches §⑧. **KB gate.**

### §9.9 Help modal — visual ref §⑩

- See §7.7. Three-column contextual keymap + legend, Canvas overlay, callable from anywhere.
- **DoD:** content is the active screen's keymap + globals + legend; opens/closes from every screen; matches §⑩. **KB + VR gates.**

### §9.10 Toasts — visual ref §⑦(toasts) — see §12.

---

## §10 · Progressive disclosure & responsive layout

> **Implement if:** building `internal/layout` or any multi-pane screen.
> **Depends on:** §2, §9.
> **Visual ref:** disclosure ladder in the HTML.

### §10.1 Depth ladder

| Level | Shows | Reach |
|---|---|---|
| **L0 Glance** | header summary line (`1 failing · 1 running`) | always |
| **L1 List** | runs table | default / `⏎` from L0 |
| **L2 Detail** | jobs + step timeline | `⏎` on a run · `⎋` back |
| **L3 Deep** | error window + annotations → full folded log | `n` / `l` · `⤓` expand |

The most likely **next action** is always the most prominent affordance: a failed run surfaces "rerun failed"; a running one "watch"; a green one gets out of the way (§9.2).

### §10.2 Responsive breakpoints (from nidhi)

Three target geometries; define column-collapse rules for each:

| Breakpoint | Layout rule |
|---|---|
| **80×24** (min) | single column; master-detail becomes a full-screen push (detail replaces list, `⎋` returns); drop the Event and sparkline columns from the runs table; keep glyph · Workflow · # · Age. |
| **120×40** (default) | master-detail side-by-side; full runs columns. |
| **200×60** (wide) | side-by-side with extra padding; show actor + SHA in the runs table. |

Never truncate a glyph or break a column; collapse columns instead. Account for East-Asian/wide runes when measuring (use display width, not byte/rune count). (§20)

**§10 DoD:** all screens verified at **80×24, 120×40, 200×60** via shux (§16); no truncation/overlap at any; master-detail collapses correctly at 80×24.

---

## §11 · Performance

> **Implement if:** building the poller, cache, queue, or log viewer.
> **Depends on:** §4.
> **Visual ref:** §05 performance in the HTML.

- **Authenticated 5,000 req/hr** via inherited `gh` auth.
- **ETag/304** on every poll → idle polling is free.
- **Adaptive poll:** ~2–3s while anything `in_progress`; back off when idle.
- **Serial queue + ≥1s between mutations** → secondary-limit safe.
- **Cache-first paint:** render off the in-memory store; never block a keystroke on the network.
- **Synchronized output** (Bubble Tea v2, terminal mode 2026): atomic repaints, no tearing on live updates.
- **Log viewer:** style/render only the visible viewport window; large logs stay smooth.

**Targets (DoD):** input → first paint < 150ms on a warm cache; keystroke → visible response < 16ms (render off cache); a `--trace-http` session shows 304s dominating idle polls; smooth scroll on a 10k-line log (no jank).

---

## §12 · Errors, toasts & resilience

> **Implement if:** building the notification system or any error path.
> **Depends on:** §7.5 (overlay stack), §9.
> **Visual ref:** §⑦ toasts in the HTML.

### §12.1 Toast system

Non-blocking notifications stacked top-right as a **Canvas/Layer** over the current screen. Each: severity left-accent, glyph, bold title, dim message, a timed auto-dismiss progress bar. `Esc` dismiss top, `g` dismiss all. Severities: `err`(fail) · `warn`(run) · `info` · `ok`. Pattern + "clean error toasts with cache invalidation" inherited from nidhi.

### §12.2 Error taxonomy → behaviour (runbook)

| Class | Example | Detection | Toast | Recovery |
|---|---|---|---|---|
| **API rate limit** | `403`/`429` | status + `x-ratelimit-remaining:0` / `retry-after` | `err` "GitHub API · 403 — backing off, retry in 42s" | honour `retry-after`/reset; keep cached view; auto-resume |
| **Network / CI render** | dial timeout, DNS | request error | `warn` "Runs unavailable — showing cached (3m old). r retry" | exponential backoff; `r` retries now |
| **Log render** | zip/text decode, **60s link expiry** | parse error / 404 on expired redirect | `err` "Log render failed — re-requesting job log" | refetch log (new redirect); fold the bad section |
| **Mutation rejected** | rerun/cancel `403`/`409` | status | `err` with the API message | no state change; allow retry |
| **Success** | rerun queued | `201`/`202` | `ok` "Re-run queued · CI #572" | refresh the run |

**Principle:** never crash to a blank screen; always show cached data + a toast + a retry path.

**§12 DoD:** each taxonomy row reproducible (inject via the fake adapter) and renders the correct toast; auto-dismiss timing works; `Esc`/`g` dismiss; toasts never steal focus or block keys; matches §⑦.

---

## §13 · Dual surface & agent integration

> **Implement if:** building the pipe face, exit codes, or agent skill.
> **Depends on:** §2 (shared usecase core).
> **Visual ref:** n/a.

- **TTY → TUI; piped or `--no-tui` → structured output** (default JSON; `--format json|md|xml`). Same `usecase` core; the renderer differs only (gh-ghent's two faces).
- **Exit codes** (gh-ghent): `0` all good · `1` action needed (a failing run/check) · `2` error (API/network) · `3` pending (still running).
- **JSON schema:** Appendix B. The failure object includes `{job, step, exit_code, annotations[], log_excerpt}` so an agent fixes without screen-scraping.
- **Agent Skill:** ship a skill in `indrasvat/claude-code-skills` describing the JSON surface, flags, and exit codes so Claude Code / Codex discover it.
- **`--watch` fail-fast** (gh-ghent): poll until complete; exit non-zero the instant something goes red, with the failure attached.
- **v3 `serve`:** JSON-RPC + MCP emitting run lifecycle events for दूतसभा fix-loops (roadmap, §18).

**§13 DoD:** `gh hound runs --status failure --no-tui --json | jq` returns schema-valid JSON; exit codes correct for each scenario; `--watch` exits fail-fast; the skill is present and discoverable.

---

## §14 · Configuration

> **Implement if:** building `internal/config`.
> **Depends on:** §5 (theme), §7 (keys), §8 (scope).

TOML at `~/.config/gh-hound/config.toml` (XDG). Precedence: built-in defaults → config file → env (`HOUND_*`, `GH_*`) → flags.

```toml
default_scope = "branch"   # branch | repo
auto_watch    = false      # auto-attach watch on launch if a run is in progress
per_page      = 30
theme         = "bramble"  # bramble | bone
poll_min_ms   = 2000
poll_max_ms   = 30000

[keybindings]              # override any binding; merged over defaults (vivecaka pattern)
rerun_failed  = "r"
rerun_run     = "R"
cancel        = "x"
force_cancel  = "X"
```

**§14 DoD:** missing config → sane defaults; invalid TOML → clear error, not a crash; keybinding overrides merge and a conflict in overrides is rejected with a message; theme/scope honoured.

---

## §15 · Distribution

> **Implement if:** setting up release/CI.
> **Depends on:** §3.

> **gh-ghent is the canonical extension template — copy its mechanics, not just its UX patterns.**

- **The repo MUST be named `gh-hound`** and the module **MUST** be `github.com/indrasvat/gh-hound`. This is non-negotiable: `gh` discovers and runs extensions as `gh-<name>` executables, so the repo, the binary, and the install spec must all be `gh-hound` (`gh extension install indrasvat/gh-hound` → runs as `gh hound`). gh-ghent encodes exactly this (`module github.com/indrasvat/gh-ghent`, binary `gh-ghent`).
- **Release** (`.github/workflows/release.yml`) — byte-for-byte the gh-ghent shape: trigger on tag `v*`; `permissions: contents: write`; `cli/gh-extension-precompile@v2` with `go_version_file: go.mod` **and** `build_script_override: scripts/build-release.sh`. The build script injects version/commit/date via `-ldflags` so `--version` prints the banner + build metadata (§9.0.1). Pushing a `v*` tag publishes per-platform binaries → `gh extension install indrasvat/gh-hound`.
- **CI** (`.github/workflows/ci.yml`) — also from gh-ghent: `actions/setup-go@v5` with `go-version-file: go.mod`; install `golangci-lint/v2` (pinned); run **`make ci`**.
- Tag the repo with the **`gh-extension`** topic (`gh repo edit --add-topic gh-extension`) so it surfaces in `gh ext search/browse`. Binary assets are named `<os>-<arch>`; `--pin` targets a version.
- **Also** `go install github.com/indrasvat/gh-hound/cmd/hound@latest` for Go users (nidhi pattern).
- **Install script** (nidhi pattern): detect platform, download latest SemVer release, verify SHA-256, install to `~/.local/share/extensions/gh-hound` (or `~/.local/bin`), clear macOS Gatekeeper quarantine, verify `gh hound --version`, warn if not on `PATH`.
- **VHS** `.tape` → README demo gif (`make demo`).

**§15 DoD:** a `v0.x.y-rc.1` tag produces installable prerelease binaries; `gh extension install indrasvat/gh-hound` works clean on macOS + Linux (x86_64/arm64); `make smoke-test` passes.

---

## §16 · Visual verification with shux (hard gate)

> **Implement if:** every screen — this gate runs continuously, not once.
> **Depends on:** the screen under test; `indrasvat/shux` installed.
> **Visual ref:** all screens.

**Why this section is mandatory.** Coding agents (including Claude) reliably ship TUIs that *look* wrong: misaligned columns, colour bleed across lines, truncation at small sizes, broken focus, footers that don't match the real keymap, and output that simply isn't faithful to the mock. Text-only assertions do **not** catch these. So every screen is verified **visually** by rendering it in a real terminal and **looking at the pixels**, driven by **shux** (Robin's typed-RPC multiplexer built for exactly this). This supersedes the older iTerm2 harness used in nidhi.

### §16.1 The loop (per screen, per breakpoint)

`shux` mirrors every CLI verb to a JSON-RPC method 1:1; the harness lives in `.claude/automations/` (nidhi layout).

```bash
# 1. deterministic session/pane at a target geometry (see shux docs/agents.md for the
#    exact size method; pin the pane so layout is reproducible)
shux session create hound-vqa --title "hound vqa"

# 2. launch hound against a fixture repo and drive keystrokes
shux pane send-keys -s hound-vqa --text 'gh hound -R indrasvat/gh-ghent\n'
shux pane send-keys -s hound-vqa --text 'j'      # move
shux pane send-keys -s hound-vqa --text '\n'     # ⏎ open detail
shux pane send-keys -s hound-vqa --text 'n'      # jump to failure

# 3. capture a PNG of the exact frame
shux pane snapshot -s hound-vqa   # → .claude/automations/screenshots/detail_failure.png

# 4. (optional) capture pane text/state for exact-string assertions (column headers,
#    footer hints) via the queryable state RPC — see shux docs/agents.md
shux rpc call <state-query-method> --params @assert.json
```

The agent then **opens each PNG** and compares it against the matching screen in `gh-hound-design.html`.

### §16.2 Per-screen visual checklist (the gate)

For **every** screen, at **80×24, 120×40, 200×60**, assert:

- [ ] **Alignment** — table columns line up; no ragged borders; gutters consistent.
- [ ] **Colour mapping** — each status uses its exact token (§5); no colour **bleed** across lines/cells.
- [ ] **No truncation/overflow** — content fits or collapses per §10; no clipped glyphs.
- [ ] **Selection** — `surface-2` fill + green bar present and on the right row.
- [ ] **Focus** — the focused pane is visibly distinct (gradient border); `Tab` moves it.
- [ ] **Footer == keymap** — every hint shown actually works (cross-check with the §7 interaction test).
- [ ] **Overlays** — help/palette/toasts render over a dimmed base, not garbled into it.
- [ ] **No tearing** — live updates (spinner/stream) repaint cleanly (synchronized output).
- [ ] **Mock fidelity** — the frame is recognisably the same design as the HTML screen.

### §16.3 Interaction audit (KB gate)

A second harness drives **every keybinding on every screen** via `shux pane send-keys` and asserts the resulting frame/state (nidhi's `comprehensive_tui_test.py` did 34 interaction checks; match or exceed that coverage). Includes: input-mode swallowing single letters (§7.4), `Esc` popping exactly one layer, `?` opening contextual help from each screen, destructive double-confirm.

**§16 DoD (the hard gate):** `make vqa` runs the visual + interaction audits, captures PNGs at all three breakpoints, and **no screen is marked Done until its checklist passes and the agent has visually confirmed fidelity to the mock.** Target ≥ nidhi's bar: ~70 layout/alignment/tearing checks + ~34 interaction checks.

---

## §17 · Definition of Done

> **Implement if:** closing out any unit of work.

### §17.1 Per-unit DoD (every screen/feature)

1. Compiles on **Go 1.26**; `make check` (lint + race + coverage) green.
2. Its **section DoD** (in §9/§12/etc.) satisfied.
3. **VR gate** (§16.2) passes at all three breakpoints with agent-confirmed mock fidelity.
4. **KB gate** (§16.3): all contextual bindings work; footer == keymap; `?` help contextual; input mode safe.
5. **No emoji** (§6 scanner green).
6. Theme: no hard-coded colours outside `internal/theme`; `T` recolours it.
7. If it has a pipe path: JSON schema-valid + correct exit code (§13).
8. Unit tests against the **fake adapter**; no network needed to test.

### §17.2 Product/v1 DoD

- All §1 v1 features shipped, each meeting §17.1.
- `gh extension install indrasvat/gh-hound` works on macOS + Linux (×2 arch); `make smoke-test` green.
- **The honest bar:** Robin stops opening the Actions web tab for his own repos.

---

## §18 · Build phases & implementation order

> Dependency-ordered. Each step gated by §17.1.

| # | Phase | Builds | Done when |
|---|---|---|---|
| 1 | **Scaffold** | repo **named `gh-hound`** (module `github.com/indrasvat/gh-hound`) mirroring gh-ghent's `go.mod`/`release.yml`/`ci.yml`/`scripts/build-release.sh`; cobra root + gh-extension entrypoint; go-gh client; config; slog; theme tokens; keymap skeleton; hexagonal seams; **banner asset + `--version`** (§9.0.1); **shux harness skeleton** | `gh hound --version` prints the gradient banner + build metadata; empty TUI launches; `gh extension install` works from a prerelease tag; `make ci` green; `make vqa` runs |
| 2 | **Data layer** | adapter (REST + ETag + serial queue + adaptive poller), models, fake adapter, JSON serialisers | §4 DoD; app runs offline against the fake |
| 3 | **Runs list + launch** | §8 resolution, §9.1, §9.2 all-green, empty/error states, toasts (§12) | §9.1/§9.2/§8/§12 DoD + VR/KB |
| 4 | **Run detail** | §9.3 master-detail + step timeline, §10 responsive collapse | §9.3 + §10 DoD |
| 5 | **Failure + log fetch** | §4.4 de-noise, §9.4 | §9.4 DoD |
| 6 | **Colour log viewer** | §9.6 (ANSI + chroma + folding + search) | §9.6 DoD |
| 7 | **Watch / stream** | §9.5 | §9.5 DoD |
| 8 | **Dispatch** | §9.7 (huh) | §9.7 DoD |
| 9 | **Palette + help** | §9.8, §9.9 (Canvas overlays) | §9.8/§9.9 DoD |
| 10 | **Dual surface + dist** | §13 JSON/exit/skill, §15 precompile + VHS | §13/§15 DoD; v1 DoD (§17.2) |

**Roadmap beyond v1:** **v2 moat** — run-diff vs last green, flake detection, multi-repo pulse, deployment approvals, caches/artifacts/runners. **v3 agent** — `serve` (JSON-RPC + MCP), lifecycle events, दूतसभा fix-loops. (Usage/billing intentionally excluded: the `timing`/usage endpoints are closing down — §20.)

---

## §19 · Repo lineage & learnings

> Carry these forward explicitly; cite them in code comments where a pattern originates.

| Repo | What it is | Learnings carried into gh-hound |
|---|---|---|
| **nidhi** | git-stash TUI, **Go 1.26 + Bubble Tea/Lip Gloss/Bubbles v2** | The template. Progressive-disclosure **modes** with a "reach" column; **responsive breakpoints 80×24/120×40/200×60** with column-collapse; clean **error toasts**; XDG **structured slog** logs + `--trace-*`; `make check/test/e2e/smoke-test`; `.claude/automations/` **visual + interaction audits** (replace iTerm2 driver with **shux**). |
| **vivecaka** | gh PR-triage TUI (Catppuccin) | `tui → usecase → adapter` hexagonal core; `ghcli.Adapter` **plugin port**; `T` **theme cycling**; `[keybindings]` config block; **VHS** demos. |
| **gh-ghent** | agentic PR-check monitor | **The canonical `gh`-extension scaffold:** `gh-`-prefixed repo/module name, `release.yml` (`gh-extension-precompile@v2` + `build_script_override: scripts/build-release.sh`), `ci.yml` (setup-go from go.mod + golangci-lint v2 + `make ci`), `gh-extension` topic, `--version` gradient banner. Plus **two faces, one core** (TTY→TUI / pipe→JSON); **exit codes 0/1/2/3**; `--watch` **fail-fast**; `--logs` **failing-step excerpts + annotations**; ships an **Agent Skill**. |
| **vicaya** | Rust fast file finder (daemon+TUI) | **Decouple data fetch from paint** (cache-first render) for instant feel; speed obsession. |
| **dorikin** | K8s drift TUI | Dual **CLI/TUI** surface; **filter-by-status**; drift/divergence framing (→ v2 run-diff). |
| **shux** | Rust multiplexer, typed JSON-RPC | **The visual/interaction verification harness** (§16): `pane send-keys`, `pane snapshot` (PNG), queryable state. |
| **दूतसभा** | multi-agent orchestrator | **v3 target** consumer of `serve` lifecycle events. |
| **claude-code-skills** | skills repo | Home of the gh-hound **Agent Skill** (§13). |

---

## §20 · Caveats & gotchas

> Read before implementing the matching section. Each has a detection + workaround.

1. **No live-log socket.** The public API cannot stream log lines. "Watch" = live step status + per-step chunk append (§9.5). Do not promise line streaming.
2. **Job-log link expires in 60s.** `…/jobs/{id}/logs` returns a 302 to a short-lived URL — follow immediately, never cache/reuse it. On expiry → refetch (§12).
3. **Run-logs are a zip; job-logs are plain text.** Prefer the per-job text endpoint for single-job drill-down; only unpack the run zip for a full export.
4. **Secondary rate limits.** Requests serial; mutations ≥1s apart. Concurrency triggers bans (§4.5).
5. **ETag/304 is free** against the primary limit — use it on every poll, or idle polling burns the budget.
6. **Lip Gloss border titles aren't native.** Borders are plain rectangles; compose the title onto the top edge (§5.2).
7. **`AdaptiveColor` removed in Lip Gloss v2.** Detect background explicitly (`tea.RequestBackgroundColor`) and select tokens (§3.1, §5).
8. **Wide-rune alignment.** Measure **display width**, not bytes/runes, or columns drift with CJK/emoji-width glyphs (§10).
9. **`filter=latest` vs `all`.** Default `latest` on `…/runs/{id}/jobs`; rerun history needs `all`.
10. **Dispatch constraints.** `workflow_dispatch` must be defined and the workflow on the default branch; only such workflows are dispatchable (§9.7).
11. **Pagination cap.** Filtered runs list returns up to 1,000 results across pages — design the list around recency, not exhaustiveness.
12. **Follow 302s; don't reconstruct URLs.** Use the `Location` header; never hand-build API URLs (go-gh handles base URLs).
13. **Usage/`timing` endpoints are closing down** — do not build billing on them (excluded from roadmap).
14. **Input-mode key capture** is the classic TUI bug — single letters must never trigger commands while typing (§7.4).

---

## §21 · References (verified)

**GitHub REST API (version 2026-03-10)**
- Workflow runs — https://docs.github.com/en/rest/actions/workflow-runs
- Workflow jobs — https://docs.github.com/en/rest/actions/workflow-jobs
- Actions REST index — https://docs.github.com/en/rest/actions
- REST best practices — https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api
- Manually running a workflow (`workflow_dispatch`) — https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow

**Charm v2 stack**
- Bubble Tea — https://github.com/charmbracelet/bubbletea · v2 "what's new" — https://github.com/charmbracelet/bubbletea/discussions/1374
- Lip Gloss — https://github.com/charmbracelet/lipgloss · v2 pkg — https://pkg.go.dev/charm.land/lipgloss/v2 · v2 beta notes — https://charm.land/blog/lipgloss-v2-beta-2/
- Bubbles — https://github.com/charmbracelet/bubbles · v2 release — https://github.com/charmbracelet/bubbles/releases/tag/v2.0.0 · v2 upgrade guide — https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md
- huh — https://github.com/charmbracelet/huh
- x/ansi — https://github.com/charmbracelet/x · chroma — https://github.com/alecthomas/chroma

**GitHub CLI extension tooling**
- go-gh v2 — https://pkg.go.dev/github.com/cli/go-gh/v2
- gh-extension-precompile — https://github.com/cli/gh-extension-precompile
- New CLI extension tools (blog) — https://github.blog/developer-skills/github/new-github-cli-extension-tools/

**Robin's repos**
- shux — https://github.com/indrasvat/shux (verification harness)
- nidhi — https://github.com/indrasvat/nidhi (template)
- vivecaka — https://github.com/indrasvat/vivecaka · gh-ghent — https://github.com/indrasvat/gh-ghent · vicaya — https://github.com/indrasvat/vicaya · dorikin — https://github.com/indrasvat/dorikin · claude-code-skills — https://github.com/indrasvat/claude-code-skills

**Web-UI pain-point sources**
- "GitHub Actions Is Slowly Killing Your Engineering Team" — https://www.iankduncan.com/engineering/2026-02-05-github-actions-killing-your-team/
- Gradle/Develocity on Actions debugging — https://gradle.com/blog/determine-the-root-cause-of-github-actions-failures-faster-with-gradle-enterprise/
- github-commando (failed-log pain) — https://github.com/cescoffier/github-commando

---

## Appendix A — Data model

```go
// internal/model
type Status string     // queued|in_progress|completed|requested|waiting|pending
type Conclusion string // success|failure|cancelled|skipped|neutral|timed_out|action_required|stale|"" (null)

type Run struct {
    ID              int64
    Name            string
    DisplayTitle    string
    Status          Status
    Conclusion      Conclusion
    Event           string
    HeadBranch      string
    HeadSHA         string
    Path            string // ".github/workflows/ci.yml@main"
    RunNumber       int
    RunAttempt      int
    WorkflowID      int64
    Actor           string // actor.login
    TriggeringActor string
    CreatedAt       time.Time
    UpdatedAt       time.Time
    RunStartedAt    time.Time
    HTMLURL         string
    JobsURL         string
    LogsURL         string
    PullRequests    []int
}

type Job struct {
    ID              int64
    RunID           int64
    Status          Status
    Conclusion      Conclusion
    StartedAt       time.Time
    CompletedAt     time.Time
    Name            string
    Steps           []Step
    Labels          []string
    RunnerName      string
    RunnerGroupName string
    WorkflowName    string
    HeadBranch      string
    HTMLURL         string
    CheckRunURL     string // → annotations
}

type Step struct {
    Name        string
    Status      Status
    Conclusion  Conclusion
    Number      int
    StartedAt   time.Time
    CompletedAt time.Time
}

type Annotation struct {
    Path      string
    StartLine int
    EndLine   int
    Level     string // failure|warning|notice
    Message   string
    Title     string
}
```

## Appendix B — JSON output schema

Emitted by the pipe face (`--no-tui --json`):

```jsonc
{
  "repo": "indrasvat/gh-ghent",
  "branch": "fix/parser",
  "runs": [{
    "id": 30433642,
    "workflow": "CI",
    "run_number": 571,
    "event": "pull_request",
    "head_branch": "fix/parser",
    "head_sha": "a1b2c3d",
    "status": "completed",
    "conclusion": "failure",
    "created_at": "2026-06-07T17:42:00Z",
    "html_url": "https://github.com/indrasvat/gh-ghent/actions/runs/30433642",
    "failed": [{
      "job": "build",
      "step": "go test ./...",
      "exit_code": 1,
      "annotations": [{ "path": "internal/parser/lexer.go", "line": 142, "level": "failure", "message": "…" }],
      "log_excerpt": "--- FAIL: TestLexIdent/trailing_underscore …"
    }]
  }]
}
// process exit code: 0 ok · 1 action needed · 2 error · 3 pending
```

## Appendix C — Full keymap table

The canonical keymap is §7.2/§7.3 plus per-screen footers in §9. The implementation defines it once in `internal/tui/keys` as a layered `key.Map`; the help modal (§7.7) and footers (§7.6) are generated from it — there is **no** second copy. Any change to a binding changes the help and footer automatically. The conflict test (§7 DoD) is the guard.

---

*End of PRD. Hand off bundle: this file + `gh-hound-design.html`. Build top-to-bottom by §18; gate every screen on §16 + §17.*
