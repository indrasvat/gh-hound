package icons

import "github.com/indrasvat/gh-hound/internal/model"

const (
	Success        = "✔"
	Failure        = "✗"
	InProgress     = "⠹"
	Queued         = "◌"
	Cancelled      = "⊘"
	Skipped        = "⊝"
	ActionRequired = "▲"
	TimedOut       = "⧗"
	Neutral        = "◇"
	Cursor         = "▌"
	Branch         = "⎇"
	Breadcrumb     = "›"
	FoldOpen       = "▾"
	FoldClosed     = "▸"
	Sparkline      = "▁▂▃▅▇"
	Prompt         = "❯"
	Rerun          = "↻"
	Dispatch       = "▶"
	Enter          = "⏎"
	Escape         = "⎋"
	Artifact       = "▣"
	// Gate marks a run waiting on a deployment review. Text
	// presentation only (U+25EB), per the theme glyph contract.
	Gate = "◫"
)

// SpinnerFrames is the one loading-spinner glyph cycle for the whole
// app (braille, the in-progress glyph family). Every loading indicator
// renders from this set; a bespoke per-screen spinner is a
// review-blocking defect per docs/visual-contract.md.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func ForStatus(status model.Status) string {
	switch status {
	case model.StatusInProgress:
		return InProgress
	case model.StatusQueued, model.StatusPending, model.StatusWaiting, model.StatusRequested:
		return Queued
	case model.StatusCompleted:
		return Neutral
	default:
		return Neutral
	}
}

func ForConclusion(conclusion model.Conclusion) string {
	switch conclusion {
	case model.ConclusionSuccess:
		return Success
	case model.ConclusionFailure:
		return Failure
	case model.ConclusionCancelled, model.ConclusionStale:
		return Cancelled
	case model.ConclusionSkipped:
		return Skipped
	case model.ConclusionActionRequired:
		return ActionRequired
	case model.ConclusionTimedOut:
		return TimedOut
	case model.ConclusionNeutral, model.ConclusionNone:
		return Neutral
	default:
		return Neutral
	}
}
