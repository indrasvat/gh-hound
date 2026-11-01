package fake

import (
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// ScenarioPaced seeds a Pace baseline. On main (push): one run in
// progress (#600, 90s in), one that ran slow (#599, 9m31s), one on
// pace (#598), eighteen green ~4m runs, and the runs a baseline must
// skip: slow schedule runs, a re-run attempt and a failure. On the PR
// branch fix/parser (pull_request): only two green runs of its own, so
// its baseline widens to pull_request runs on any branch (feat/lexer).
const ScenarioPaced Scenario = "paced"

const (
	PacedRunningNumber = 600
	PacedSlowNumber    = 599
	PacedOnPaceNumber  = 598
	PacedPRNumber      = 620
)

var pacedEpoch = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func pacedRun(number int, branch, event string, took time.Duration) model.Run {
	run := greenRun(number)
	started := pacedEpoch.Add(time.Duration(number-500) * time.Hour)
	run.HeadBranch, run.Event = branch, event
	run.HeadSHA = pacedSHA(number)
	run.CreatedAt = started.Add(-14 * time.Second)
	run.RunStartedAt = started
	run.UpdatedAt = started.Add(took)
	run.PullRequests = nil
	return run
}

func pacedSHA(number int) string {
	const hex = "0123456789abcdef"
	b := []byte("p000000")
	for i, n := len(b)-1, number; i > 0; i, n = i-1, n/16 {
		b[i] = hex[n%16]
	}
	return string(b)
}

func (a *Adapter) pacedHistory() []model.Run {
	running := pacedRun(PacedRunningNumber, "main", "push", 0)
	running.Status, running.Conclusion = model.StatusInProgress, model.ConclusionNone
	running.RunStartedAt = a.started.Add(-90 * time.Second)
	running.CreatedAt = running.RunStartedAt.Add(-14 * time.Second)
	running.UpdatedAt = a.started

	runs := []model.Run{
		pacedRun(PacedPRNumber, "fix/parser", "pull_request", 255*time.Second),
		pacedRun(619, "fix/parser", "pull_request", 250*time.Second),
	}
	for n := 618; n >= 613; n-- {
		runs = append(runs, pacedRun(n, "feat/lexer", "pull_request", time.Duration(250+n-613)*time.Second))
	}
	runs = append(runs,
		running,
		pacedRun(PacedSlowNumber, "main", "push", 571*time.Second),
		pacedRun(PacedOnPaceNumber, "main", "push", 245*time.Second),
	)
	// Eighteen green pushes between 3m50s and 4m24s.
	for n := 597; n >= 580; n-- {
		runs = append(runs, pacedRun(n, "main", "push", time.Duration(230+2*(n-580))*time.Second))
	}
	scheduled := pacedRun(579, "main", "schedule", 900*time.Second)
	rerun := pacedRun(578, "main", "push", 60*time.Second)
	rerun.RunAttempt = 2
	failed := pacedRun(577, "main", "push", 40*time.Second)
	failed.Conclusion = model.ConclusionFailure
	return append(runs, scheduled, rerun, failed)
}
