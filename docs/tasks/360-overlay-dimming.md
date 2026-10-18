# Task 360 — Overlays Dim The Base View

## Status

IMPLEMENTED — issue #62, second of two PRs (PR #64), which closes it. The first (task 350, PR #63) shipped the search tint, truthful headers, esc layers, time-jump echo and modal hint, and queue XML. `make ci`, `make vqa`, interaction audit and render hygiene pass; verified with shux on fakes, live read-only, Bone on a light terminal and a 256-colour terminal; an adversarial review and a cold `tui-qa` round (PASS on cb19581), each finding fixed with a test that failed first or recorded below.

## Ownership Boundary

- **Primary area:** how modal overlays are composed over the screen behind them: help, palette, confirm, time-jump and approvals.
- **Allowed files:** `internal/tui/app.go`, `internal/tui/chrome.go`, `internal/tui/*_test.go`, `.claude/automations/`, `docs/`.
- **Avoid touching:** overlay contents and their footers, toasts (non-blocking, not a modal), theme tokens, screen renderers, the render path in `cmd/gh-hound`.

## Decisions

- **The base view stays visible behind every modal, dimmed after the mock's `.dimbg` (`brightness(.46) saturate(.65)`).** Every colour an SGR sets — truecolor, 256-colour, the 16 basic colours, `;` or `:` separated, as raw CI log colours arrive — becomes a truecolor taken 46% of the way from the theme background (the mock's brightness, measured from the background rather than black, so cells painted the background colour do not turn into darker patches), then keeps 65% of its saturation (the CSS saturate matrix, exactly). Text without a colour takes the theme foreground dimmed to a neutral grey, also after every reset. On Bone the base fades toward the light background, which suits a light terminal (the mock's terminal is dark).
- **Colours are rewritten before the output writer downsamples.** Plain text is a neutral grey so it stays grey at 16 colours (`90`) and 256 colours (a grey index) instead of turning yellow or olive. At 256 colours dimmed hues collapse to greys; at 16 colours theme colours dim but a log's own vivid basic colours quantise back to themselves (a 16-colour palette has nothing dimmer).
- **The box keeps its place**, from the top of the body, now centred in the body itself (one column left of main, which centred it in the frame), and is spliced into the dimmed rows: the base keeps its columns left and right of the box, and the right side carries the colour state from before the box, so tinted rows do not change colour across it. Box rows are never dimmed.
- **Toasts are unchanged**: they are non-blocking, and dimming the screen under them would read as disabled. This departs from the mock, whose toast screen is its only `.dimbg`; visual contract ⑦ records it.

## Task List

- [x] T1 Dimming: unit tests for the blend (Bramble and Bone), every colour form (basic, bright, 256, colon, `58`, truncated), resets and default colour, attributes passed through; downsampled plain text stays grey. App test, both themes: every colour beside and below the box is truecolor and the dimmed image of a colour the undimmed frame draws, including a log full of raw ANSI colours under the time-jump modal.
- [x] T2 Layering: splice tests (box intact, base columns kept, colour carried across the box, wide characters at the edges, base shorter than the box); a fuzz test against a plain-text oracle.
- [x] T3 App: help, palette, confirm, time-jump and approvals each show the dimmed base around the box at 80x24/120x40/200x60, on Bramble and Bone. A regression guard (it passes on main too) checks esc restores the exact frame from before the overlay.
- [x] T4 Visual contract, fixtures and VQA assertions for the five overlays.
- [x] T5 Verification: `make ci`, `make vqa`, interaction audit, render hygiene; shux before/after on fakes and live (read-only), Bramble and Bone; cold `tui-qa` and adversarial review.
- [x] T6 PR #64 closing #62 with an evidence comment: screenshots and a demo video.

## Found Along The Way

- Adversarial review (Chāyābhedī, 5d49d5b): only truecolor was dimmed, so a log's raw colours (`\x1b[36;1m` command echoes in real runner logs, 256-colour, colon forms) stayed bright behind the time-jump modal, and the "no undimmed colour" check only knew bright truecolor. Every colour form now dims, and the app test compares against the dimmed image of the colours the undimmed frame draws. The warm dimmed text downsampled to yellow (16) and olive (256); it is a neutral grey now.
- Cold `tui-qa` (Tamodarśī): 5d49d5b would have failed on the two items above; cb19581 passes (cell-level column checks, pixel-identical esc restore, loading state, live resize, 256 and 16 colours, Bone on a light terminal).
- Not fixed here: Bone keeps the log and detail screens' hard-coded Bramble bands (#56), which dim to mid-grey slabs; VQA assertions are text only, so dimming is pinned by the unit and app tests; the fuzz column check is limited to ordinary text (a lone combining mark just before the box joins its border).
- Pre-existing, seen during QA: non-ASCII typed into inputs is decoded byte by byte (mojibake); the live root shows a blank screen for ~1.8 s on start and echoes keys typed meanwhile; `?` inside help opens a second help layer.

## Measurements

- A help frame at 200x60 renders in 1.22 ms (main: 0.80 ms, which blanks the base; the plain runs frame is 0.63 ms). Frames are drawn on input, so the extra 0.4 ms is not felt.
- `FuzzLayerOver`: 13M+ execs clean. It found one real crash (a negative pad when text that clusters sits under the box's right edge, fixed with a floor; the seed is kept) and several oracle artifacts; exact columns are checked for ordinary text, every input for crashes and for rows below the box.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; every modal leaves the base readable but clearly behind, with no colour bleed or misalignment at the box edges; `make ci` and `make vqa` pass; evidence lives on the PR only.
