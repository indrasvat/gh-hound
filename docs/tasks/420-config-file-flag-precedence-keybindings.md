# Task 420 — Config File, Flag Precedence, Keybindings

## Status

IN PROGRESS — issue #56, first of three PRs. The second (theme tokens on every screen, the 256-colour fallback, palette selection fill) and the third (runs footer and header at 80 columns, the stale branch-miss notice) close it. UTF-8 input and `?` closing help already shipped in #66.

## Ownership Boundary

- **Primary area:** config loading and env/flag layering in `cmd/gh-hound`, `internal/config`, and the key translation layer in `internal/tui/keys`.
- **Allowed files:** `cmd/gh-hound/main.go` and the subcommand files' `applyEnv` calls, `internal/config/`, `internal/tui/keys/`, `internal/tui/app.go` (key entry, footers, help, two toasts), `internal/tui/overlay/help/`, tests, `.claude/automations/`, `docs/`.
- **Avoid touching:** screen renderers and colours (PR 2), the runs chrome (PR 3).

## Decisions

- **The file is read.** `$XDG_CONFIG_HOME/gh-hound/config.toml`, else `~/.config/gh-hound/config.toml` (the documented path on every OS, so not `os.UserConfigDir`). A missing file is defaults. Invalid TOML, an unknown key or an out-of-range value stops the launch with the file's path; the file layer is validated on its own so a bad env value is not blamed on the file.
- **A flag beats its variable.** Each `HOUND_*` variable applies only when its flag was not given (cobra's `Changed`). An explicit `--format` also beats `HOUND_JSON`, since it names the output; `--json` still beats `--format`.
- **Keybindings move actions.** `[keybindings]` maps an action to one printable character. One translation at `App.Update` turns the press into the default key the handlers already compare, so no handler changed; a moved action's old key stops acting on that screen. Text inputs (filters, search, dispatch, the palette, comments, time-jump) are never translated. Footers, the toast footer, help (including the filter syntax line) and the detail screen's "log not ready" toasts show the moved key.
- **Only keys no in-screen hint names are remappable.** Runs, detail, caches and workflows bodies and usecase notices name `s`, `A`, `/`, `a`, `d`, `o`, `y`, `n`, `l`, `e`, `G` and `:` in place; moving those would leave the hints wrong. They stay fixed, with navigation (`j/k`, `g/G`), Enter, Esc and Tab. The catalog and rules are in `docs/configuration.md#keybindings`.
- **Conflicts are per screen.** `keys.NewKeymap` checks every screen: its fixed keys, `:` and the toast layer's `g`, its remappable actions and the global ones (`?`, `q`, `T`). One key may serve actions that never share a screen (`rerun` and `rerun_job`). `config.Validate`'s flat duplicate check was wrong under that rule and is gone. Only the TUI reads keybindings, so only a TUI launch fails on a bad one.
- **Fixtures stay hermetic.** `__vqa-tui` ignores the user's config unless `--config-home` hands it one, so the PTY gates do not depend on the machine.

## Task List

- [x] T1 Config path: missing, XDG, HOME fallback, env over file, invalid TOML / unknown key / out of range naming the file; a TTY launch paints the file's theme; `FuzzLoadFile`.
- [x] T2 Precedence: a table test per variable (flag wins; without the flag the variable still beats the default) and the issue's repro through cobra.
- [x] T3 Keymap: default tables conflict-free, translation per screen, display everywhere keys are listed, rejection messages, `FuzzKeymap`; app tests (moved key acts, old key dead, inputs type); CLI tests through the real decoder.
- [x] T4 PTY scenarios `keymap-help` and `keymap-watch` (each fails on main).
- [x] T5 Docs: configuration, README, PRD §14 sample.
- [ ] T6 Verification: `make ci`, `make vqa`, shux on the fakes, takshaka, adversarial review, Codex, cold `tui-qa`.

## Found Along The Way

- `--log-level` / `HOUND_LOG_LEVEL` are parsed and never used: logging is configured only by `--trace-http`, at debug. The docs' sample config also listed a `log_level` key the file never had. The key is gone from the docs; wiring the level is a separate issue.
- Adversarial review (config surface, 2 major, 6 minor, all fixed with a test): a `keybindings = "x"` that is not a table was dropped silently by the decoder; `HOUND_JSON=1 --format md` under a TTY launched the TUI (main printed JSON), so HOUND_JSON now yields only its format; `runs --no-tui` never read the file and hard-coded 30 per page; `poll_*_ms` overflowed into a sub-millisecond poll (now 1-3600000, checked before the multiply); a dangling symlink read as no file; a FIFO hung the launch and `/dev/zero` grew without bound (regular files up to 1 MiB only); a relative `XDG_CONFIG_HOME` followed the cwd; validating the file alone rejected a file the environment repairs (file `poll_min_ms` above the default max with `HOUND_POLL_MAX_MS` raised), and `theme = ""` read as unset. Kept: keybindings are checked by the TUI only, and `__vqa-tui` passes config env through (the harness sets `HOUND_WELCOME`). `default_scope` still does not apply to `--no-tui` listings, which keep the branch scope agents rely on.
- Adversarial review (keymap surface, driven in a PTY; 2 major, 2 minor fixed with a test): the approvals overlay compared raw keys, so after `quit = "Q"` its old `q` still quit, and with `help = "q"`, `quit = "?"` the help key quit the app; approvals now translates outside comment mode and its `j k c y n` are fixed. Detail's gate pane names `A`, which was not a fixed detail key, so `rerun_job = "A"` made the pane and the footer disagree; `A` is fixed there. The palette still opened help on `?` after help moved; it now types it. The log footer measured runes, not cells, so wide keys overflowed it. Pre-existing and kept: the toast footer's `r retry` (no handler), now with the moved key showing nowhere else.
- Codex (dootsabha, 2 major, 2 minor, fixed with a test): keys were translated before the background drains, so a fetch that finished between keys (the dispatch workflows list) opened its form and the form got the runs list's translation (`h` arrived as `?`); translation now runs after the drains. `rerun`, `cancel`, `workflows`, `caches`, `queue` and the other commands that use no config value ignored a broken file; the root's pre-run now validates it for every public command (not `version`, `-v` or the hidden fixtures). A file `poll_min_ms = 0` was rejected before `HOUND_POLL_MIN_MS` could repair it; out-of-range poll values now fail only in the merged check. An env error was blamed on the file whenever the file had a different, repaired error; the file is blamed only for its own.
- takshaka: about 40 lines cut (a duplicated test helper, two assertions that could not fail, the dead `FooterLayer`, an unreachable error branch); `config.Validate`'s empty-key loop dropped so only the TUI judges keybindings; the string-flag precedence rows now set an explicit empty flag, the case the new guards exist for.
- From the key inventory, pre-existing and not changed here: the toast footer advertises `r retry` but no toast has a retry handler, so `r` falls through to the screen's rerun; detail's `J`/`K` set intents nothing handles; detail's deploy-gate pane shows `A review` though detail does not handle `A`; the runs footer omits `D`, `o`, `y`, `g/G` (now `R` and `X` are listed in help); ctrl+c does not quit inside inputs, dispatch, confirm or the time-jump.
