package fake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// The live run moves like a real one: bdd on Run tests, then Upload
// artifacts, then done with report running, then the run is done. A
// running job's log answers 404, as GitHub's does until it finishes.
func TestLiveRunAdvancesAndServesOnlyFinishedLogs(t *testing.T) {
	ctx := context.Background()
	runID := greenRun(liveRunNumber).ID
	stages := []struct {
		age        time.Duration
		bdd        model.Status
		bddStep    string
		report     model.Status
		runStatus  model.Status
		bddLogDone bool
	}{
		{0, model.StatusInProgress, "Run tests", model.StatusQueued, model.StatusInProgress, false},
		{50 * time.Second, model.StatusInProgress, "Upload artifacts", model.StatusQueued, model.StatusInProgress, false},
		{60 * time.Second, model.StatusCompleted, "", model.StatusInProgress, model.StatusInProgress, true},
		{90 * time.Second, model.StatusCompleted, "", model.StatusCompleted, model.StatusCompleted, true},
	}
	for _, stage := range stages {
		a := New(ScenarioLiveRun)
		a.started = time.Now().Add(-stage.age)
		run, err := a.GetRun(ctx, "indrasvat/gh-hound", runID)
		if err != nil || run.Status != stage.runStatus {
			t.Fatalf("at %s run = %s, %v; want %s", stage.age, run.Status, err, stage.runStatus)
		}
		jobs, _ := a.ListJobs(ctx, "indrasvat/gh-hound", runID)
		if len(jobs) != 3 || jobs[0].Status != model.StatusCompleted || jobs[1].Status != stage.bdd || jobs[2].Status != stage.report {
			t.Fatalf("at %s jobs = %#v", stage.age, jobs)
		}
		if stage.bddStep != "" {
			var running string
			for _, step := range jobs[1].Steps {
				if step.Status == model.StatusInProgress {
					running = step.Name
				}
			}
			if running != stage.bddStep {
				t.Fatalf("at %s bdd runs %q, want %q", stage.age, running, stage.bddStep)
			}
		}
		if _, err := a.FetchJobLog(ctx, "indrasvat/gh-hound", liveLintJobID); err != nil {
			t.Fatalf("at %s lint log: %v", stage.age, err)
		}
		_, err = a.FetchJobLog(ctx, "indrasvat/gh-hound", liveBDDJobID)
		missing, notYet := errors.AsType[usecase.LogUnavailableError](err)
		if stage.bddLogDone != (err == nil) || (!stage.bddLogDone && (!notYet || missing.Expired())) {
			t.Fatalf("at %s bdd log err = %v, want served=%v else a 404", stage.age, err, stage.bddLogDone)
		}
	}
}
