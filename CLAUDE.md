# gh-hound

A `gh` extension: a Go TUI for GitHub Actions and a structured pipe surface (`--json`) for agents. Each rule below points to the doc that holds the detail. Read that doc only when the task touches its area.

## Toolchain

- Go 1.26, as pinned in `go.mod`. IMPORTANT: Go 1.27's `go fix` rewrites files and fails `make gofix-check` in the pre-commit and pre-push hooks. Build, commit and push with 1.26.
- `make hooks` installs the hooks. Pre-push runs `make ci`. `make help` lists every target.

## TUI changes

The bar is high: a TUI change is not done when tests pass.

- Follow `docs/visual-contract.md` and the mock `docs/gh-hound-design.html`. Colours go through theme tokens only; never write a colour into a screen.
- Every change must hold at 80x24, 120x40 and 200x60 (the contract's Breakpoints and Visual QA Gate).
- No keystroke or poll tick may block on the network, and frames never clear the whole screen (contract: Loading States, Render Hygiene).
- A new screen or state gets a `__vqa-tui` fixture (`internal/tui/app.go`), an assertion in `.claude/automations/assertions/`, and an entry in `.claude/automations/vqa.sh`. `make vqa` must pass.
- Before the PR, run a cold `tui-qa` agent review (`.claude/agents/tui-qa.md`). The PR carries before/after evidence from the `recording-pr-evidence` skill.

## Agent surface (`--json` verbs)

- A field or verb change updates all of these in the same PR: `internal/render/testdata/schema.json`, the golden files (`UPDATE_GOLDEN=1`), `docs/agent-surface.md`, and `skills/gh-hound/`.
- Keep `skills/gh-hound/SKILL.md` lean. Put details in `skills/gh-hound/references/`.
- Exit codes are a contract: 0 ok, 1 action needed, 2 error, 3 pending. Branch-worthy states live in `status`.
- Drive behaviour through a fake (`internal/adapter/fake/`, `--fake-scenario`), never a live repo.

## Boundaries

- `internal/usecase` and `internal/adapter` never import `internal/tui` (`make arch-check`).
- Config keys and keybindings are documented in `docs/configuration.md`. A new key goes there too.
- Feature work has a task doc in `docs/tasks/NNN-*.md` (decisions, task list, Found Along The Way).

## Release

IMPORTANT: a tag is public and irreversible. In order:

1. `make release-prep` passes on the exact commit to tag (`docs/development.md`, Release Prep).
2. Annotated tag `vX.Y.Z` on main's merge commit (`git tag -a vX.Y.Z <sha> -m "gh-hound X.Y.Z"`), then push the tag. The version lives only in the tag; there is no file to bump.
3. When the release workflow finishes, the published files verify against `checksums.txt`. `gh extension upgrade hound` installs it, and the installed `gh hound` passes a smoke check.
4. Release notes: highlights, then fixes, above GitHub's generated list.
