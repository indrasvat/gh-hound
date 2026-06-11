package watch

import (
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// Board is the pack watch: one row per run in the event group, the
// aggregate header above, drill-in below. It deliberately knows
// nothing about jobs or steps — the runs-list poll budget carries the
// whole board.
type Board struct {
	Repo    string
	HeadSHA string
	Event   string
	Branch  string

	Runs     []model.Run
	Selected int
	// Follow keeps the cursor on the worst-status run as states move.
	// Manual movement takes the leash back.
	Follow bool

	Intent BoardIntent

	// Loading state is transient render input set by the app from its
	// pending load each frame, mirroring the runs screen.
	Loading     bool
	LoadingLine string
}

type BoardIntentKind string

const (
	BoardIntentNone   BoardIntentKind = ""
	BoardIntentDrill  BoardIntentKind = "drill"
	BoardIntentCancel BoardIntentKind = "cancel"
	BoardIntentBack   BoardIntentKind = "back"
)

type BoardIntent struct {
	Kind  BoardIntentKind
	RunID int64
}

// NewBoard seats the pack: rows come pre-grouped (usecase.PackForRun),
// the cursor starts on the anchor the user pressed w on.
func NewBoard(repo, branch string, anchor model.Run, runs []model.Run) Board {
	board := Board{
		Repo:    repo,
		Branch:  branch,
		HeadSHA: anchor.HeadSHA,
		Event:   anchor.Event,
		Runs:    runs,
	}
	for index, run := range runs {
		if run.ID == anchor.ID {
			board.Selected = index
			break
		}
	}
	return board
}

func (b Board) Update(msg KeyMsg) Board {
	b.Intent = BoardIntent{}
	switch msg.Key {
	case "j", "down":
		if b.Selected < len(b.Runs)-1 {
			b.Selected++
		}
		b.Follow = false
	case "k", "up":
		if b.Selected > 0 {
			b.Selected--
		}
		b.Follow = false
	case "f":
		b.Follow = !b.Follow
		if b.Follow {
			b.Selected = usecase.WorstRunIndex(b.Runs)
		}
	case "enter":
		if run, ok := b.SelectedRun(); ok {
			b.Intent = BoardIntent{Kind: BoardIntentDrill, RunID: run.ID}
		}
	case "x":
		if run, ok := b.SelectedRun(); ok {
			b.Intent = BoardIntent{Kind: BoardIntentCancel, RunID: run.ID}
		}
	case "esc":
		b.Intent = BoardIntent{Kind: BoardIntentBack}
	}
	return b
}

// WithRuns folds a fresh poll into the board: selection sticks to the
// same run ID (rows are ID-sorted and stable), and follow mode
// retargets the worst run.
func (b Board) WithRuns(runs []model.Run) Board {
	selectedID := int64(0)
	if run, ok := b.SelectedRun(); ok {
		selectedID = run.ID
	}
	b.Runs = runs
	b.Selected = 0
	for index, run := range runs {
		if run.ID == selectedID {
			b.Selected = index
			break
		}
	}
	if b.Follow {
		b.Selected = usecase.WorstRunIndex(runs)
	}
	return b
}

func (b Board) SelectedRun() (model.Run, bool) {
	if len(b.Runs) == 0 {
		return model.Run{}, false
	}
	selected := max(b.Selected, 0)
	if selected >= len(b.Runs) {
		selected = len(b.Runs) - 1
	}
	return b.Runs[selected], true
}

// Summary aggregates the header counts.
func (b Board) Summary() usecase.PackSummary {
	return usecase.SummarizePack(b.Runs)
}
