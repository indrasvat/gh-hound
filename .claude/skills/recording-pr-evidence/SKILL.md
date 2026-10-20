---
name: recording-pr-evidence
description: "Records and publishes before/after evidence for a gh-hound PR: main and the branch driven side by side on the same fake fixture through shux, every changed area marked in the video (theme-coloured rounded boxes, numbered badges, a legend), then an evidence page linked from the PR. This skill should be used when the user asks to \"capture evidence\", \"record a before/after PR video\", \"main vs branch demo\", \"side-by-side demo\", \"annotate the video\", \"highlight what changed\", \"evidence page\", or when a gh-hound PR that changes what the TUI shows is ready for review. Not for landing-page screenshots (capture_fixture.py)."
---

# Recording PR evidence

Every gh-hound PR that changes what the TUI shows carries one evidence
page: a short video with main on top and the branch below, on the same
fixture at the same moment, with each change marked; before/after
stills; and the test and review record. The PR body links it. #68–#70
set the bar.

Marks are part of the deliverable, not decoration. They must be
identifiable at a glance and still look like they belong to the frame;
a mark that covers text or shouts in a foreign colour is a defect.
Read `references/marking-style.md` before adding or tuning a rule.

## Prerequisites

- `shux` (0.51+) on PATH: `make tools-shux` installs the latest
  (see `docs/development.md`); `uv` (runs the script with Pillow);
  `ffmpeg` with libx264 (`ffmpeg -encoders | grep libx264`).
- JetBrainsMono Nerd Font (`brew install --cask
  font-jetbrains-mono-nerd-font`); the script stops without it.
- The pinned Go: `GO=~/sdk/go1.26.0/bin/go` for the main build.

## Workflow

1. **Make the change reproducible on a fake.** Drive a
   `__vqa-tui --scenario <name>` fixture, never a live repo, so main and
   the branch see the same data. Add or extend a fake in
   `internal/adapter/fake/` when none reaches the change. Fixtures that
   move with time should move on the wall clock from the adapter's start
   (see `live.go`), slowly enough that the first seconds are stable.

2. **Build both binaries.**

   ```bash
   make build                                   # the branch: ./bin/gh-hound
   GO=~/sdk/go1.26.0/bin/go \
   .claude/skills/recording-pr-evidence/scripts/build_main_with_fixture.sh \
     /tmp/gh-hound-main internal/adapter/fake/fake.go internal/adapter/fake/<new>.go
   ```

   Copy fixture files only; copying product code records the branch twice.
   Label the top pane "main <sha> + fixture".

3. **Write the spec** at `docs/tasks/<task>-demo.json`: scenario, size,
   length, the key timeline with captions, and the marking rules.
   Format: `references/spec-format.md`. Worked example:
   `docs/tasks/410-demo.json`.

4. **Record and mark.**

   ```bash
   uv run .claude/skills/recording-pr-evidence/scripts/demo_compare.py \
     docs/tasks/<task>-demo.json --main-bin /tmp/gh-hound-main \
     --pr-bin ./bin/gh-hound --out .shux/out/demo-<task> \
     --mp4 .shux/out/demo-<task>/demo-<task>.mp4
   ```

   It runs its own shux daemon (`/tmp/hdemo<pid>`) and stops it after.
   The run prints how many frames each rule marked and exits non-zero if
   a rule marked none.

5. **Look before publishing.** Open three or four frames from
   `.shux/out/demo-<task>/marked/` covering each phase, then the mp4.
   Check each mark against `references/marking-style.md`. To tune rules
   without re-recording, re-run with `--skip-record`. Show the user the
   final video before publishing it.

6. **Publish and link.** Start the page from `assets/evidence-page.html`
   (task 410's page) and fill it as `references/evidence-page.md`
   describes. Publish the page as an
   Artifact with the video and stills as supporting files, and put the
   link in the PR body. When the branch changes what a recorded screen
   shows, re-record: a stale video is worse than none.

## Rules that hold every time

- The same keys and the same fixture clock on both sides. When main
  needs an extra key to reach the same screen, send it to main only and
  say so in the caption.
- Mark style is fixed by `references/marking-style.md`; tune rules, not
  the look, per task.
- Evidence lives on the PR only: nothing under `.shux/out/` is
  committed. The spec in `docs/tasks/` is.

## Additional resources

- `references/spec-format.md`: the demo spec, field by field, with
  regex tips for matching TUI text.
- `references/marking-style.md`: the visual rules for marks, badges,
  legend and captions, and what to check in a frame.
- `references/evidence-page.md`: evidence page sections, stills, the
  live check, PR body conventions, publishing.
- `scripts/demo_compare.py`: records both panes (PNG plus text per
  frame), marks, composes, encodes.
- `scripts/build_main_with_fixture.sh`: the default branch plus named
  fixture files, built in a throwaway worktree.
- `assets/evidence-page.html`: task 410's evidence page, the template
  for the next one (tokens, sections, both themes).
