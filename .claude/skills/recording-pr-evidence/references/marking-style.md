# Marking style

The goal is a mark a reviewer finds in under a second that still looks
like part of the frame. Both halves matter. A faint mark is missed; a
loud one makes the video look pasted together and hides the very text it
points at. `scripts/demo_compare.py` implements these rules; change the
script, not the rule, when a frame breaks them.

## The mark

- **Colour from the TUI theme only.** Fail red `#E2564B` for what main
  gets wrong, ok green `#4FD37A` for what the branch shows, run amber
  `#E0A33E` for what both do. These are the colours the screen already
  uses, so marks read as part of it. Never introduce a colour the TUI
  does not use.
- **Thin, rounded, full strength.** A 3 px outline (at 2× scale) at full
  opacity with 7 px corner radius, plus a faint tint inside (alpha about
  34/255). The outline makes it findable; the tint ties the content to
  the mark without hiding it.
- **Half a cell of breathing room** left and right of the matched text,
  2 px above and below. The outline never touches a glyph.
- **One point, one mark.** Rows that say one thing (a job, the job after
  it, and the note under both) share one mark through `through`, not
  three stacked boxes.

## Badge and legend

- A numbered badge (24 px circle, mark colour, dark numeral) identifies
  each mark. It sits in blank cells just right of the mark, centred on
  the first row. Only when those cells hold text does it fall back to
  the mark's top-left corner.
- Words never go inside the frame. The pane's title bar carries the
  legend: the pane name in its colour, then each badge and its label in
  soft foreground. Numbers restart per pane.
- Labels are short user language: `running step reads queued`,
  `opens on the running step`. Not `duration() returns queued`.

## Keycaps

- Each pane's title bar shows, right-aligned, the key that pane just
  received (`pressed [h]`, a large keycap; named keys as `⏎ ⎋ ⇥ ↑ ↓`) for two seconds of
  recording. Keys sent to one side show on that side only, so a
  main-only step reads as one.
- Drawn in the screen's own surface, line and foreground colours, never
  a mark colour: a keycap says what was pressed, not what is right.

## Captions

- One line above both panes in soft foreground, changed only at
  timeline steps: the key and what to look at (`l · full log of the
  running job`), or the fixture time (`~55s · bdd finishes`).

## Restraint

- Three or four marks per pane at once, at most. If more seem needed,
  the phase is doing too much: split it on the timeline.
- Marks appear with the screen they explain and leave with it (the rule
  window), so a mark never points at a stale screen.

## Check before publishing

Open one frame per phase at full size and confirm:

1. Every mark is found without searching; colour and badge agree with
   the legend.
2. No mark or badge covers a glyph (badge on a corner is the only
   allowed overlap, and only of a border).
3. Red only on main, green only on the branch, amber on both.
4. Labels read as plain statements a reviewer would agree with by
   looking.
5. The caption matches what the frame shows.
