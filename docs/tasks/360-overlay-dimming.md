# Task 360 — Overlays Dim The Base View

## Status

IN PROGRESS — issue #62, second of two PRs; this one closes #62. The first (task 350, PR #63) shipped the search tint, truthful headers, esc layers, time-jump echo and modal hint, and queue XML.

## Ownership Boundary

- **Primary area:** how modal overlays are composed over the screen behind them: help, palette, confirm, time-jump and approvals.
- **Allowed files:** `internal/tui/app.go`, `internal/tui/chrome.go`, `internal/tui/*_test.go`, `.claude/automations/`, `docs/`.
- **Avoid touching:** overlay contents and their footers, toasts (non-blocking, not a modal), theme tokens, screen renderers, the render path in `cmd/gh-hound`.

## Decisions

- **The base view stays visible behind every modal, dimmed as the mock's `.dimbg` (`brightness(.46) saturate(.65)`).** Each truecolor (`38;2`/`48;2`) colour is blended toward the theme background by 0.46, then desaturated to 0.65 (CSS saturate weights). Text without a colour takes the dimmed theme foreground, also after every reset. On Bramble this is the mock's filter; on Bone it fades toward the light background instead of smearing dark.
- **Colours are rewritten before the output writer downsamples**, so 256- and 16-colour terminals dim too; no profile logic here.
- **The box keeps today's place** (centred horizontally, from the top of the body) and is spliced into the dimmed rows: the base keeps its columns left and right of the box, and the right side carries the colour state from before the box, so tinted rows do not change colour across it. Box rows are never dimmed.
- **Toasts are unchanged**: they are non-blocking, and dimming the screen under them would read as disabled.

## Task List

- [x] T1 Dimming: unit tests for the blend (Bramble and Bone), resets and default colour, background colours, non-colour SGR passed through; a full frame's dimmed rows contain no undimmed colour.
- [x] T2 Layering: splice tests (box intact, base columns kept, colour carried across the box, wide characters at the edges, base shorter than the box); a fuzz test against a plain-text oracle.
- [x] T3 App: help, palette, confirm, time-jump and approvals each show the dimmed base around the box at 80x24/120x40/200x60; esc restores the exact frame from before the overlay.
- [x] T4 Visual contract, fixtures and VQA assertions for the five overlays.
- [ ] T5 Verification: `make ci`, `make vqa`, interaction audit, render hygiene; shux before/after on fakes and live (read-only), Bramble and Bone; cold `tui-qa` and adversarial review.
- [ ] T6 PR closing #62 with an evidence comment: screenshots and a demo video.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; every modal leaves the base readable but clearly behind, with no colour bleed or misalignment at the box edges; `make ci` and `make vqa` pass; evidence lives on the PR only.
