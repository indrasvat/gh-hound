# Task 310 — Artifacts download UX: location, lifecycle, and post-download actions

## Problem

The artifacts download interaction (task 200) is functional but blind:

1. **Destination opacity.** Downloads extract to `./<name>/` relative to the
   TUI process's CWD. The confirm modal and completion toast show only the
   relative path, and the toast — the single place the path ever appears —
   expires. A user who missed it has no way to recover the location short of
   re-downloading.
2. **No post-download affordances.** Nothing opens the extracted folder,
   nothing copies its path. The artifact row looks identical before and
   after a download.
3. **No progress.** The "downloading artifact" toast is static. A multi-MB
   artifact gives zero feedback between keypress and completion toast, and
   extraction adds more silent seconds.
4. **`DestinationExistsError` dead-ends.** The CLI has `--force`; the TUI
   surfaces the error toast and offers nothing.
5. **Discoverability.** Because none of the above exists, the hint line and
   help can't teach it.

## Design

### Download lifecycle on the rows (detail screen)

`detail.Model` gains `Downloads map[int64]DownloadStatus`:

```go
type DownloadState string // "", downloading, extracting, done, failed
type DownloadStatus struct {
    State     DownloadState
    Bytes     int64  // bytes received so far (downloading)
    Path      string // absolute extraction path (done)
    FileCount int    // extracted files (done)
    Reason    string // short failure reason (failed)
}
```

Artifact row right-side annotation by state:

- downloading: `↓ 4.2 MB…` — the live byte counter is the animation; no
  fake percentages against an uncertain total (the API's `size_in_bytes`
  is not the archive transfer size).
- extracting: `extracting…`
- done: green `✓` + dim `N files`
- failed: red `✗ failed`
- none/expired: unchanged (size / `expired`).

The **selected** downloaded artifact grows a dim subline —
`↳ /abs/path · o open · y copy path` — making both the location and the
actions discoverable exactly when they apply. The pane header becomes
`Artifacts (3 · 1 ✓)` once any download landed. All annotations are
session-scoped UI state, deliberately not persisted.

### Contextual `o` / `y`

On `FocusArtifacts` with the selected artifact in state `done`, `o` opens
the extracted folder (system opener: `open`/`xdg-open` — the existing
browser-opener closure handles directories) and `y` copies its absolute
path via the existing clipboard closure. In every other context both keys
keep their current meanings (browser / copy URL). New intents:
`IntentOpenArtifactDir`, `IntentCopyArtifactPath`.

### Absolute destination, everywhere

`main.go` resolves the download root once at startup — `$GH_HOUND_ARTIFACT_DIR`
if set, else the CWD — to an absolute path, passes it to the downloader
closure **and** to the TUI (`Options.ArtifactDir`) for display. The confirm
modal shows the resolved absolute target on its own line; the completion
toast repeats it along with the action hints.

### Overwrite recovery

`ArtifactDownloader` gains `force bool`. When a download fails with
`DestinationExistsError`, the app opens a second confirm —
"destination exists: <abs path> — overwrite and re-download?" — and retries
with `force=true` on confirm. `d` on an already-downloaded artifact flows
into the same path naturally. The confirm overlay learns multi-line
messages (split on `\n`) so paths don't truncate the question.

### Progress plumbing

`usecase.Download` gains an optional `onProgress func(DownloadProgress)`:

```go
type DownloadProgress struct {
    Phase string // "download" | "extract"
    Bytes int64  // cumulative bytes received
}
```

A counting writer wraps the archive copy; the extract phase fires once
before extraction begins. The TUI's downloader goroutine stores progress
in atomics on `artifactDownloadState`; `Refresh()` drains them into
`detail.Downloads` and reports `changed` so the loop repaints;
`PollInterval()` returns the load frame interval while a download is live
(same pattern the loading spinner uses).

### Out of scope

- Persisting download history across sessions.
- CLI envelope changes (the JSON path stays as-is: golden-pinned and
  machine-dependent if made absolute).
- Parallel downloads (the one-at-a-time invariant from task 200 stands).

## Verification

- Unit: model intent routing (contextual o/y), view annotations per state,
  subline/header/hint variants, app drains (progress advances mark changed),
  abs-path confirm wording, overwrite confirm + force retry, opener/copier
  invocation and toasts, usecase progress callback monotonicity + phases,
  multi-line confirm view.
- vqa: detail scenario screenshots pick up the new row/hint chrome;
  interaction audit drives d → confirm → done-row ✓ → o/y toasts on the
  fake lens.
- Live: real download on indrasvat/gh-hound CI artifacts, open + copy
  verified, overwrite flow exercised against an existing destination.
