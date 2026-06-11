---
name: shux-capture-recipes
description: Working shux capture recipes and gotchas specific to gh-hound audits
metadata:
  type: reference
---

- `shux pane wait-for` flag is `--timeout-ms` (NOT `--timeout`); a failed
  wait still lets a later snapshot succeed, but snapshots taken right
  after a flag error are blank frames — recapture.
- Fixture captures: wrap in
  `sh -c 'sleep 2; ./bin/gh-hound __screen --screen X --width W --height H; sleep 300'`,
  `pane set-size` during the initial sleep. The frame is exactly
  pane-sized, so the SHELL CURSOR lands on the bottom-right cell and
  shows as a small pink/red rounded mark in every `__screen` PNG. It is
  a capture artifact, NOT an app defect — interactive sessions
  (`__vqa-tui`) show no cursor (alt screen hides it). Do not file it.
- `__vqa-tui --scenario failing` fake adapter has no latency knob; runs
  reload resolves under the 100ms spinner grace so the dimmed loading
  state is fixture-only-observable. Detail open has enough latency to
  catch "fetching jobs…" live with an immediate post-keystroke snapshot.
- `__screen --screen <bogus-name>` silently falls back to the empty
  screen instead of erroring.

**How to apply:** reuse these in every gh-hound audit capture loop.
Related: [[gh-hound-qa-ledger]].
