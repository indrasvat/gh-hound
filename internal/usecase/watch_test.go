package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestWatchServicePollsAndAppendsCompletedJobLogs(t *testing.T) {
	gh := &watchGitHub{
		runs: []model.Run{
			{ID: 570, Status: model.StatusInProgress, Conclusion: model.ConclusionNone},
			{ID: 570, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure},
		},
		jobs: []model.Job{{
			ID:         100,
			Name:       "build",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
			Steps: []model.Step{{
				Number:     6,
				Name:       "go test ./...",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}},
		}},
		logs: "17:43:02Z go test ./...\nok internal/api 0.214s\n",
	}
	service := usecase.WatchService{GitHub: gh, MinPoll: time.Second, MaxPoll: 30 * time.Second}
	state := usecase.WatchState{Repo: "indrasvat/gh-hound", RunID: 570}

	next, err := service.Tick(context.Background(), state)
	if err != nil {
		t.Fatalf("first tick returned error: %v", err)
	}
	if next.NextPoll != time.Second || next.Terminal || len(next.Appended) != 2 {
		t.Fatalf("first tick = %#v", next)
	}

	next, err = service.Tick(context.Background(), next)
	if err != nil {
		t.Fatalf("second tick returned error: %v", err)
	}
	if !next.Terminal || next.NextPoll != 30*time.Second || len(next.Appended) != 2 {
		t.Fatalf("second tick = %#v", next)
	}
	if gh.fetches != 1 {
		t.Fatalf("fetches = %d, want completed job log once", gh.fetches)
	}
}

type watchGitHub struct {
	runs    []model.Run
	jobs    []model.Job
	logs    string
	fetches int
}

func (g *watchGitHub) GetRun(context.Context, string, int64) (model.Run, error) {
	if len(g.runs) == 0 {
		return model.Run{}, errors.New("no runs")
	}
	run := g.runs[0]
	if len(g.runs) > 1 {
		g.runs = g.runs[1:]
	}
	return run, nil
}

func (g *watchGitHub) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	return g.jobs, nil
}

func (g *watchGitHub) GetJob(context.Context, string, int64) (model.Job, error) {
	return model.Job{}, errors.New("not implemented")
}

func (g *watchGitHub) FetchJobLog(context.Context, string, int64) (string, error) {
	g.fetches++
	return g.logs, nil
}

func (g *watchGitHub) ListRuns(context.Context, usecase.RunFilter) ([]model.Run, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) RerunRun(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) RerunFailedJobs(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) RerunJob(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) CancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) ForceCancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) DispatchWorkflow(context.Context, string, string, usecase.DispatchRequest) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}
