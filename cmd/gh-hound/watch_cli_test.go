package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// Issue #60, end to end: the TUI's watch polls keep their state, so a
// completed job's log is downloaded once however many ticks follow,
// and a job that never ran (no steps, no log) is not fetched at all.
func TestTUIWatchFetchesEachCompletedJobLogOnce(t *testing.T) {
	run := cliRun(9100, "CI", model.StatusInProgress, model.ConclusionNone)
	step := []model.Step{{Number: 1, Name: "go test ./...", Status: model.StatusCompleted}}
	github := &cliGitHub{
		runs:    []model.Run{run},
		runByID: map[int64]model.Run{run.ID: run},
		jobs: []model.Job{
			{ID: 1, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Steps: step},
			{ID: 2, Name: "test", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Steps: step},
			{ID: 3, Name: "build", Status: model.StatusInProgress, Steps: step},
			{ID: 4, Name: "deploy", Status: model.StatusCompleted, Conclusion: model.ConclusionSkipped},
		},
		jobLog: "17:43:02Z go test ./...\nok internal/api 0.214s\n",
	}
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false", "HOUND_POLL_MIN_MS": "1", "HOUND_POLL_MAX_MS": "1"}),
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo: "indrasvat/gh-hound", Branch: "feature/real", Actor: "indrasvat",
		}},
	}, tui.BuildInfo{Version: "v0.1.0"}, cliOptions{})
	if err != nil {
		t.Fatalf("defaultTUIApp: %v", err)
	}
	defer app.Shutdown()

	app, _ = app.Update(tui.KeyMsg{Key: "w"})
	app, settled := app.SettleLoads(2 * time.Second)
	if !settled || app.Route() != tui.RouteWatch {
		t.Fatalf("watch did not open: settled=%v route=%s", settled, app.Route())
	}
	const ticks = 6
	deadline := time.Now().Add(5 * time.Second)
	for github.jobListings() < ticks+1 && time.Now().Before(deadline) {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	if listings := github.jobListings(); listings < ticks+1 {
		t.Fatalf("watch polled %d times in 5s, want %d", listings, ticks+1)
	}
	if fetches := github.jobLogFetches(); fetches != 2 {
		t.Fatalf("job logs fetched %d times over %d ticks, want 2 (lint and test, once each)", fetches, github.jobListings())
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	if !strings.Contains(view, "watch · CI #100") || !strings.Contains(view, "ok internal/api") {
		t.Fatalf("watch view lost the completed logs:\n%s", view)
	}
}

func (g *cliGitHub) jobListings() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.listJobs
}

func (g *cliGitHub) jobLogFetches() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.fetchJobLog
}
