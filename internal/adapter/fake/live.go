package fake

import (
	"net/http"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// ScenarioLiveRun is a run still going, as GitHub reports one: a
// finished lint job with its log, a bdd job part way through its
// steps, and a report job waiting on it. GitHub serves a job's log
// only once the job finishes, so the running jobs answer 404.
//
// It moves on the wall clock from the adapter's start, slowly enough
// that a capture taken in the first seconds is stable:
//
//	0–45s:  lint done · bdd on 3 Run tests · report queued
//	45–55s: bdd on 4 Upload artifacts
//	55–80s: bdd done · report running
//	80s+:   the run is done
const ScenarioLiveRun Scenario = "live_run"

const (
	liveRunNumber   = 3902
	liveLintJobID   = 399444510
	liveBDDJobID    = 399444511
	liveReportJobID = 399444512
)

func (a *Adapter) liveAge() time.Duration {
	return time.Since(a.started)
}

func (a *Adapter) liveRun() model.Run {
	run := greenRun(liveRunNumber)
	run.Name, run.DisplayTitle, run.Event = "E2E", "bdd suite", "workflow_dispatch"
	run.RunStartedAt = a.started.Add(-15 * time.Minute).UTC().Truncate(time.Second)
	run.CreatedAt = run.RunStartedAt
	run.UpdatedAt = a.started.Add(80 * time.Second).UTC().Truncate(time.Second)
	if a.liveAge() < 80*time.Second {
		run.Status, run.Conclusion = model.StatusInProgress, model.ConclusionNone
		run.UpdatedAt = time.Now().UTC()
	}
	return run
}

func (a *Adapter) liveJobs() []model.Job {
	age := a.liveAge()
	t0 := a.started.UTC().Truncate(time.Second)
	at := func(offset time.Duration) time.Time { return t0.Add(offset) }
	done := func(number int, name string, start, end time.Duration) model.Step {
		return model.Step{Number: number, Name: name, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: at(start), CompletedAt: at(end)}
	}
	queued := func(number int, name string) model.Step {
		return model.Step{Number: number, Name: name, Status: model.StatusQueued}
	}
	running := func(number int, name string, start time.Duration) model.Step {
		return model.Step{Number: number, Name: name, Status: model.StatusInProgress, StartedAt: at(start)}
	}
	base := func(id int64, name string) model.Job {
		j := job()
		j.ID, j.RunID, j.Name = id, greenRun(liveRunNumber).ID, name
		j.Status, j.Conclusion, j.StartedAt, j.CompletedAt, j.Steps = model.StatusQueued, model.ConclusionNone, time.Time{}, time.Time{}, nil
		return j
	}

	lint := base(liveLintJobID, "lint")
	lint.Status, lint.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	lint.StartedAt, lint.CompletedAt = at(-15*time.Minute), at(-14*time.Minute)
	lint.Steps = []model.Step{
		done(1, "Set up job", -15*time.Minute, -15*time.Minute+2*time.Second),
		done(2, "golangci-lint", -15*time.Minute+2*time.Second, -14*time.Minute),
	}

	bdd := base(liveBDDJobID, "bdd")
	bdd.Status, bdd.StartedAt = model.StatusInProgress, at(-14*time.Minute)
	setup := done(1, "Set up job", -14*time.Minute, -14*time.Minute+15*time.Second)
	checkout := done(2, "Checkout", -14*time.Minute+15*time.Second, -12*time.Minute-40*time.Second)
	switch {
	case age < 45*time.Second:
		bdd.Steps = []model.Step{setup, checkout, running(3, "Run tests", -12*time.Minute-40*time.Second), queued(4, "Upload artifacts"), queued(5, "Post Checkout")}
	case age < 55*time.Second:
		bdd.Steps = []model.Step{setup, checkout, done(3, "Run tests", -12*time.Minute-40*time.Second, 45*time.Second), running(4, "Upload artifacts", 45*time.Second), queued(5, "Post Checkout")}
	default:
		bdd.Status, bdd.Conclusion, bdd.CompletedAt = model.StatusCompleted, model.ConclusionSuccess, at(55*time.Second)
		bdd.Steps = []model.Step{setup, checkout, done(3, "Run tests", -12*time.Minute-40*time.Second, 45*time.Second), done(4, "Upload artifacts", 45*time.Second, 54*time.Second), done(5, "Post Checkout", 54*time.Second, 55*time.Second)}
	}

	report := base(liveReportJobID, "report")
	switch {
	case age >= 80*time.Second:
		report.Status, report.Conclusion, report.StartedAt, report.CompletedAt = model.StatusCompleted, model.ConclusionSuccess, at(55*time.Second), at(80*time.Second)
		report.Steps = []model.Step{done(1, "Set up job", 55*time.Second, 57*time.Second), done(2, "Publish report", 57*time.Second, 80*time.Second)}
	case age >= 57*time.Second:
		report.Status, report.StartedAt = model.StatusInProgress, at(55*time.Second)
		report.Steps = []model.Step{done(1, "Set up job", 55*time.Second, 57*time.Second), running(2, "Publish report", 57*time.Second)}
	case age >= 55*time.Second:
		report.Status, report.StartedAt = model.StatusInProgress, at(55*time.Second)
		report.Steps = []model.Step{running(1, "Set up job", 55*time.Second), queued(2, "Publish report")}
	}
	return []model.Job{lint, bdd, report}
}

// liveJobLog serves a finished job's log; a running one answers 404,
// as GitHub's log download does until the job finishes.
func (a *Adapter) liveJobLog(jobID int64) (string, error) {
	for _, j := range a.liveJobs() {
		if j.ID != jobID {
			continue
		}
		if j.Status != model.StatusCompleted {
			return "", usecase.LogUnavailableError{JobID: jobID, Status: http.StatusNotFound}
		}
		stamp := j.CompletedAt.Format("15:04:05.000Z")
		return strings.Join([]string{
			stamp + " ##[group]" + j.Name,
			stamp + " " + j.Name + " finished",
			stamp + " ok    " + j.Name + " 0 issues",
			"##[endgroup]",
		}, "\n"), nil
	}
	return "", usecase.LogUnavailableError{JobID: jobID, Status: http.StatusNotFound}
}
