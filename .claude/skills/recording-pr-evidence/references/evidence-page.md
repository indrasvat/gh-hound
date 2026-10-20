# Evidence page and PR conventions

The evidence page is a private Artifact linked from the PR body. Start
from `assets/evidence-page.html` (task 410's page): keep its tokens and
layout (IBM Plex Sans and Mono, a single reading column, main/branch
pairs side by side that stack on phones, light and dark themes) and
replace the content. Without the Artifact tool, attach the mp4 and
stills to a PR comment instead and put the page's text in it.

## Sections, in order

1. **Header.** An eyebrow (`gh-hound · task <n> · evidence`), the title
   (the PR's change in plain words), one paragraph naming the report or
   problem and what the two panes are, and a meta line: branch, base
   sha, shux version and size.
2. **Demo.** The annotated video (`controls muted playsinline`), one line
   on what red, green and amber mean, and one line listing the key
   sequence.
3. **Result.** A table: behaviour, main, this PR. One row per change a
   user can see, values in mono, red/green text.
4. **One section per change.** Two sentences on why main behaves that
   way, then a main/PR pair of stills cut from the same frame index of
   the raw recording (2× nearest-neighbour, no marks), each with a tag
   (`main` / `this PR`) and a mono caption of what to see.
5. **Live check** when the change concerns real GitHub data: read-only
   captures of the branch on a public repo (name the runs), since a fake
   can only show what someone thought to fake.
6. **Fixtures.** The PTY interaction scenarios added, as 80x24 stills.
7. **Tests.** A table: test, on main, this PR. Red-first results and
   which guards were mutation-checked.
8. **Review.** One bullet per review pass and what it found; then
   "Recorded, not changed" for findings deferred (also on main).
9. **Reproduce.** The commands: build, run the fixture, the key
   sequence, focused tests.

## Stills

Cut stills from `.shux/out/demo-<task>/raw/{main,pr}/<frame>.png` at a
frame where the change is on screen, scale 2× nearest-neighbour, and
keep them unmarked: the video carries the marks, the stills carry the
pixels.

## Publishing

- Publish with the Artifact tool: the page as the file, the video and
  stills through `files` (relative paths such as `demo-<task>.mp4`,
  `img/detail-main.png`). Pass `icon` on the first publish only.
- Re-publish from the same file path to keep the URL when the video or
  stills change.
- The page is private: say so in the PR body ("private to the repo
  owner; shareable from its Share menu").

## PR body

Match recent PRs (#68–#70): a short opening paragraph on the problem,
**What changes** as bullets, **Worth knowing when reviewing**,
**Verified** (gates, red first, live, reviews), **Evidence** with the
link, then the session link line. Plain words, no headings beyond
those, nothing an image says better.
