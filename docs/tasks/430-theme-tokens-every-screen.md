# Task 430 — Theme Tokens On Every Screen, 256-Colour Fills

## Status

IMPLEMENTED — issue #56, second of three PRs. The first (#71) read config.toml, made flags beat `HOUND_*` and moved keys; the third (runs chrome at 80 columns, the stale branch-miss notice) closes the issue.

## Ownership Boundary

- **Primary area:** the screens' colours and the fills a terminal without truecolor shows.
- **Allowed files:** `internal/theme/`, every `internal/tui/screens/*/view.go` and model, `internal/tui/chrome.go`, `internal/tui/app.go` (theme wiring, palette fill), `internal/tui/loading.go`, `cmd/gh-hound/render.go` and the TUI launch in `main.go`, tests, `docs/visual-contract.md`, `.claude/automations/`.
- **Avoid touching:** layout, text and key handling (PR 3 owns the runs chrome).

## Decisions

- **Screens colour through the theme only.** Ten views wrote Bramble truecolor constants, so Bone left them dark and `T` did not recolour them. Each model now carries the app's theme (set in `screenBody`, as the runs list already was) and builds its styles once per frame through `theme.Fg`/`theme.Bg`; a zero theme is Bramble. With Bramble every view draws byte for byte what it drew before, except the queue's stray grey (#969180, now `subtle`) and the log header, whose combined bold-and-colour SGR is now two sequences.
- **Four tints joined the tokens.** Search hits, the current hit, a failed step's row and a folded repeat had colours of their own: `run-tint`, `run-tint-strong`, `fail-tint`, `info-tint`, with Bone values about 12-30% of the semantic colour over the Bone background.
- **Terminals without truecolor get their own fills.** Measured through the writer: below #303030 every grey downsamples to 232, the background's index, so every Bramble fill vanished, and the Bone fills shared 253. `Theme.For(palette)` picks the fills: at 256 colours xterm indices the writer passes through (Bramble on the dark grey ramp, since the cube's darkest hues are as bright as dim text; dim and subtle text a step lighter, so every fill keeps 2:1 against it); at 16 colours none, as a fill shares a colour with the text on it, and the cursor bar marks the selection. The TUI reads the palette once, from the check that installs the writer, and `T` keeps it.
- **The palette's selected row takes the surface-2 fill** behind a green bar, as visual contract ⑧ says. Approvals and the time-jump picker also mark a row with `▌`; their contracts do not ask for a fill, so they keep theirs.

## Task List

- [x] T1 Theme: tint tokens (contract table and test), `Fg`/`Bg` (hex or a 256 index), `Limited()`.
- [x] T2 Screens: log, failure, watch and board, detail, diff, flakes, queue, caches, workflows themed; byte-identity on Bramble checked against 2,000+ rendered frames before and after; a Bone test per package.
- [x] T3 App: theme on every screen model; `TestThemeToggleRecoloursEveryScreen` (fails when one screen is not wired); limited fills across `T`; the palette fill.
- [x] T4 256: `TestLimitedThemeFillsStayDistinctAt256` and the 16-colour selection test through the real writer; shux captures main vs branch, Bramble and Bone, three screens, 80x24/120x40/200x60.
- [x] T5 Verification: `make ci` (1,226 tests), `make vqa`, takshaka, Codex twice, cold `tui-qa` (FAIL, FAIL, then PASS on a0ed3a3), evidence.

## Found Along The Way

- Chrome kept its own hex-to-SGR helper beside the runs list's; both now use the theme's.
- Cold `tui-qa` round 1 (05f0ea6, FAIL; Bramble truecolor byte-identical to main, Bone recolours every screen): at 256 colours the cube tints (58, 94, 23) were as bright as dim text, so line numbers, timestamps and a search inside a timestamp could not be read on hit and fold rows; at 16 colours fills landed on the text's own colour (the selected run's detail, a failed step red on red, a fold label cyan on cyan). Fixed as above, with a contrast test per fill and a no-fills test at 16. Also: Bone QA needs OSC 10 and 11 (a foreground as well as a background), as a real light terminal sets both.
- Cold `tui-qa` round 2 (0be06b7, FAIL on one new P2; both round-1 findings fixed): with no fills at 16 colours, the runs filter's cursor set a green background and "restored" it with the now empty shading, so the green ran to the row's end. An empty fill now resets to the terminal's background (`theme.Bg("")`), which every fill-then-restore path relies on. Recorded, not changed: at 256 colours a plain search hit is one grey step from a group header; at 16 colours Bone's fail red downsamples to grey (the runs list on main does too) and only the current hit is marked.
- Cold `tui-qa` round 3 (a0ed3a3, PASS): the filter cursor is one cell at 16 colours in both themes and sizes; Bramble truecolor still byte-identical to main but for the palette and queue; 256 and Bone unchanged from round 2.
- Codex follow-up on the palette commits (1 major, recorded): at 16 colours only the current search hit is marked (by its line number); plain hits have no fill to show them. main draws the same at 16 colours (its hit fill folds into the background), so this is not a regression; a non-fill marker for hits on 16-colour terminals is its own change. Codex found no other defect: decimal tokens never reach a hex parser, the overlay dimming handles `38;5`, `48;5` and `49`, and the runs cursor stays visible.
- takshaka (no defects, ~85 lines): ten copies of the zero-theme fallback became `Theme.OrBramble`; six "the views differ" guards the next assertion already implied are gone; `parseHex` folded into `hexRGB`, whose comment now says only BG and FG may reach it (Limited's fills are indices); two doubled doc comments; the colour profile is read once in `runTUI` and handed to the startup frame and the app.
- Codex (1 major, 3 minor): the welcome screen and the banner still drew Bramble (the banner's six-step gradient had two colours of its own); the gradient is the `mark` token now and both follow the theme, Bramble byte-identical. The failure panel rebuilt the evidence styles per row; `flakes.EvidenceRows` builds them once. The toggle test could pass on the frame's chrome; it judges the body only, welcome included, and fails when one screen is not wired. Not changed: the `__screen` fixtures render Bramble with truecolor fills (they are VQA's fixed reference frames); Bone and 256-colour behaviour is verified on the real launch path (`__vqa-tui`).
