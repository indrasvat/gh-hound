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
	Branch         = "⌥"
	Breadcrumb     = "›"
	FoldOpen       = "▾"
	FoldClosed     = "▸"
	Sparkline      = "▁▂▃▅▇"
	Prompt         = "❯"
	Rerun          = "↻"
	Dispatch       = "▶"
	Enter          = "⏎"
	Escape         = "⎋"
)

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
