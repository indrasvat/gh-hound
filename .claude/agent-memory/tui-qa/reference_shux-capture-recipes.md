---
name: shux-capture-recipes
description: Working shux capture recipes and gotchas specific to gh-hound audits
metadata:
  type: reference
---

- `shux pane wait-for` flag is `--timeout-ms` (NOT `--timeout`); a failed
  wait still lets a later snapshot succeed, but snapshots taken right
  after a flag error are blank frames — recapture.
- wait-for needles are CASE-SENSITIVE, and a timed-out wait
  short-circuits `&&` chains — later send-keys in the chain never fire.
  A seemingly "stuck overlay" may just be an esc that was never sent;
  re-probe interactively before filing.
- gh-hound toasts TTL out in ~5s: snapshot in the SAME chained command
  right after the wait-for; a follow-up text capture usually misses
  them.
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
- Diff-vs-full-repaint pixel parity: snapshot the diffed state, then
  `pane set-size` to cols±1 and back (two SIGWINCH invalidates → full
  repaints), snapshot again, `cmp` the PNGs — shux snapshots are
  byte-deterministic for identical screens. Launch inside a plain
  `bash --noprofile --norc` pane so exit hygiene (prompt back, cursor
  restored, alt screen left) is observable after `q`.
- Fake-lens depth limits for scroll tests: failing scenario runs list
  has 1 row on branch scope; no fake log overflows a 24-row pane even
  unfolded — force real viewport scroll with an 80x12 pane, or use
  flaky scenario (6 rows) for selection movement.

**How to apply:** reuse these in every gh-hound audit capture loop.
Related: [[gh-hound-qa-ledger]].
