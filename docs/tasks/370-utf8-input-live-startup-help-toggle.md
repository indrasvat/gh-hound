# Task 370 — UTF-8 Input, Live Startup Paint, Help Toggle

## Status

IMPLEMENTED — issue #65, first of two PRs. The second (the cut failure view's keys, log search inside a time range, grouped match and fold counters, the time-jump modal at small sizes) closes it. `make ci`, `make vqa`, interaction audit and render hygiene pass; verified with shux 0.51 on fakes and live (read-only) on this repo.

## Ownership Boundary

- **Primary area:** the key decoder and the live launch in `cmd/gh-hound`, text-input backspace, and the overlay stack.
- **Allowed files:** `cmd/gh-hound/main.go`, `internal/tui/app.go`, `internal/tui/loading.go`, the text-input models, `internal/tui/keys/footers.go`, tests, `.claude/automations/`, `docs/`.
- **Avoid touching:** screen renderers, overlay contents, the second PR's areas.

## Decisions

- **One key per rune.** The decoder holds a rune split across reads until it completes and drops invalid bytes; named keys are unchanged. Text inputs already accepted any single-rune key; their backspace now removes a whole rune (the runs filter's idiom), or the first non-ASCII character typed and deleted would leave half a rune behind.
- **The live launch paints before it resolves.** Raw input and the SIGWINCH watch come first, then a startup frame: the shared loading indicator (`picking up the scent`) in an empty frame, footer `ctrl+c quit`. The app (which lists runs over the network) is built meanwhile. Keys typed before it exists are replayed into it in order, each rendered as if typed live; `ctrl+c` quits at once (raw mode no longer lets the terminal turn it into SIGINT); EOF waits for the app, replays, then exits.
- **An overlay opens at most once.** Opening one already in the stack returns to it, closing those above (`pushOverlay`); `?` in help closes it, as its footer says. Help over the palette and the palette over help still stack, as before.

## Task List

- [x] T1 Decoder: unit tests (multi-byte runes, splits across reads, invalid bytes) and `FuzzKeyDecoder` (any bytes, any read size: every key valid; text without control bytes decodes back exactly).
- [x] T2 Inputs: a backspace test per text input (palette, approvals comment, log search, dispatch field, caches filter).
- [x] T3 Startup: a launch whose runs listing is held paints the frame, replays a key typed meanwhile, and quits on `ctrl+c` without waiting.
- [x] T4 Overlays: `?` in help closes it; table of key sequences; `FuzzOverlayStack` (no duplicate, one esc per overlay gets out).
- [x] T5 PTY scenarios `input-utf8`, `palette-utf8-backspace`, `help-toggle`, `startup-frame` (each fails on main); visual contract rows for the startup footer and loading state.
- [x] T6 Verification: `make ci`, `make vqa`; shux before/after on fakes and live; adversarial review and a cold `tui-qa` round.

## Found Along The Way

- The issue's lost resize does not reproduce on main: main reads the size after the launch resolves, so a resize during the blank wait is picked up. The real window was between that read and `signal.Notify`, which main registered after the first paint. The new order (watch, then read) closes it, and it matters now that the frame paints immediately.
- Main echoed keys typed during the blank wait into the shell, where they stayed after quitting (`jjk[gh-hound exited]`); the keys themselves were applied once the app started, so nothing was lost.

## Measurements

- Live `gh hound -R indrasvat/gh-hound -A` (unauthenticated, 100x30, three runs each): first frame 62–80 ms (main 1,309–1,480 ms); the runs list lands at the same time on both (1.41–1.43 s vs 1.31–1.48 s).
- Fuzz: `FuzzKeyDecoder` 450k+ execs, `FuzzOverlayStack` 2.4M+, and the existing `FuzzLayerOver` and `FuzzViewport`, clean.

## Definition Of Done

Every T item checked; each fix has a test that fails on `main`; `make ci` and `make vqa` pass; evidence lives on the PR only.
