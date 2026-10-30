# Demo spec format

One JSON file per task, `docs/tasks/<task>-demo.json`. It holds
everything needed to re-record the same video: the fixture, the key
timeline and the marking rules. `docs/tasks/410-demo.json` is a complete
example.

## Top level

| Field | Meaning |
| --- | --- |
| `scenario` | The `__vqa-tui --scenario` fake both builds run. |
| `ready` | Text both panes must show before the first key; it also anchors the fixture clocks. |
| `size` | Pane size, `COLSxROWS`. `120x28` reads well stacked; use `80x24` when the change is about narrow layouts. |
| `seconds` | Recording length in wall-clock seconds. Cover every fixture transition, plus a few seconds after the last. |
| `fps` | Capture rate (default 5). |
| `playback_fps` | Encode rate (default `2 × fps`, so the video plays at 2× speed). |
| `panes.main`, `panes.pr` | Title-bar labels, e.g. `main d3b625e + fixture`, `this PR`. |
| `env` | Optional terminal env for both panes, merged over `TERM=xterm-256color COLORTERM=truecolor`; an empty value unsets (`{"COLORTERM": ""}` records a 256-colour terminal). `HOUND_THEME` here picks the starting theme. |
| `background` | Optional OSC 11 background (`#EFEDE1`), for a light theme on a light terminal. |
| `runtime_dir` | shux runtime dir (default `/tmp/hdemo<pid>`, one per run); keep it short, since Unix socket paths are limited. |
| `timeline` | Steps, below. |
| `rules` | Marks, below. |

## Timeline steps

```json
{"frame": 60, "keys": {"both": "l"}, "caption": "l · full log of the running job"}
```

- `frame` is the capture index (`seconds × fps` frames in all). At
  5 fps, frame 60 is 12 s in.
- `keys` maps `both`, `main` or `pr` to one key: a literal (`j`, `w`)
  or a named key (`enter`, `esc`, `tab`, `up`, `down`). Use one step per
  key; leave about 1 s (5 frames) between keys so each screen settles.
- `caption` replaces the caption line from this frame on. A step may
  carry only a caption, e.g. to name a fixture transition.

## Rules

```json
{"frames": [106, 217], "side": "pr", "match": "⠹ bdd › 3 Run tests",
 "through": "step progress until then", "tone": "good",
 "label": "each unfinished job: its step and clock"}
```

| Field | Meaning |
| --- | --- |
| `frames` | Inclusive `[first, last]` window. Start 2 frames after the key that reveals it, so the mark never lands on a half-drawn screen. |
| `side` | `main`, `pr` or `both`. |
| `match` | Regex searched row by row in the frame's plain text; the first matching row wins. |
| `through` | Optional regex for the last row of a multi-row mark. The mark spans from `match` to it. |
| `to_row_end` | Extend the mark to the row's last glyph, e.g. to mark an empty band. |
| `tone` | `bad` (main's wrong behaviour), `good` (the branch), `same` (both). |
| `label` | Legend text: short, user language, what to see, not how it works. |

## Matching TUI text

- A text column is a cell column: the regex match's character offsets
  are used directly. Box glyphs (`│`), `⠹`, `◌`, `›` and `…` are one
  cell each.
- gh-hound's status glyphs are static (`⠹` does not animate), so a
  literal glyph is safe to match; the per-rule frame count shows a
  flickering match as far fewer frames than its window.
- Match the stable part plus the changing part with a pattern:
  `Run tests\s+\d+m\d+s`, `bdd\s+queued`. Clocks change every frame.
- A rule should match on one pane only: keep `match` specific enough
  that the same text on the other pane (or a log line) does not catch it.
- Run with `--skip-record` after edits; the per-rule frame counts show
  a rule that never matched (the run fails) or one that matches far
  more frames than its window suggests.
