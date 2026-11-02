package fake

import (
	"fmt"
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
	run.HeadSHA = fmt.Sprintf("p%06x", number)
	run.CreatedAt = started.Add(-14 * time.Second)
	run.RunStartedAt = started
	run.UpdatedAt = started.Add(took)
	run.PullRequests = nil
	return run
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

// pacedJobs are a run's jobs: build (five steps; checkout pinned to a
// ref that Dependabot bumps every other run) and lint. #599 ran slow in
// go build and go test; #600 is in go build.
func (a *Adapter) pacedJobs(runID int64) []model.Job {
	var run model.Run
	for _, candidate := range a.pacedHistory() {
		if candidate.ID == runID {
			run = candidate
		}
	}
	if run.ID == 0 {
		return []model.Job{}
	}
	n := run.RunNumber
	ref := "34e114876b0b11c390a56381ad16ebd13914f8d5"
	if n%2 == 0 {
		ref = "11bd71901bbe5b1630ceea73d27597364c9af683"
	}
	build := []time.Duration{time.Second, 2 * time.Second, time.Duration(5+n%3) * time.Second, time.Duration(200+n%5) * time.Second, 0}
	lint := time.Duration(28+n%4) * time.Second
	if n == PacedSlowNumber {
		build[2], build[3] = 19*time.Second, 540*time.Second
	}
	names := []string{"Set up job", "Run actions/checkout@" + ref, "go build ./...", "go test ./...", "Complete job"}
	start := run.RunStartedAt
	steps := make([]model.Step, len(names))
	at := start
	for i, name := range names {
		steps[i] = model.Step{Number: i + 1, Name: name, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: at, CompletedAt: at.Add(build[i])}
		at = at.Add(build[i])
	}
	buildJob := model.Job{ID: runID*10 + 1, RunID: runID, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: start, CompletedAt: at, Labels: []string{"ubuntu-latest"}, Steps: steps}
	lintJob := model.Job{ID: runID*10 + 2, RunID: runID, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: start, CompletedAt: start.Add(lint), Labels: []string{"ubuntu-latest"}}
	if run.Status == model.StatusInProgress {
		// 90s in: build is in go build, lint has finished.
		buildJob.Status, buildJob.Conclusion, buildJob.CompletedAt = model.StatusInProgress, model.ConclusionNone, time.Time{}
		steps[2].Status, steps[2].Conclusion, steps[2].CompletedAt = model.StatusInProgress, model.ConclusionNone, time.Time{}
		for i := 3; i < len(steps); i++ {
			steps[i] = model.Step{Number: i + 1, Name: names[i], Status: model.StatusQueued}
		}
	}
	return []model.Job{buildJob, lintJob}
}
