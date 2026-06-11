package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func runsPollApp(t *testing.T, resolver func(context.Context, usecase.RunFilter) ([]model.Run, error)) App {
	t.Helper()
	cfg := config.Default()
	cfg.Welcome = false
	return NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:    "openclaw/openclaw",
			Branch:  "main",
			Scope:   usecase.LaunchScopeBranch,
			State:   usecase.LaunchStateRuns,
			PerPage: 30,
			Runs: []model.Run{{
				ID: 9001, Name: "CI", RunNumber: 44,
				Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "main",
			}},
		},
		RunsResolver: resolver,
	})
}

// The bug this fixes (Task 28): the poll resolver used to run inline in
// Refresh, so a slow poll stalled the next keystroke. Refresh must now
// return immediately even when the resolver blocks indefinitely, and a
// keystroke arriving while that poll is in flight must not block either.
func TestTickPollNeverBlocksTheLoop(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	app := runsPollApp(t, func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		<-release // hang the resolver for the whole test
		return nil, nil
	})

	start := time.Now()
	app, _ = app.Refresh()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("Refresh blocked on the resolver for %v — the poll is not async", elapsed)
	}
	if app.tickPoll == nil {
		t.Fatal("Refresh started no background poll")
	}

	start = time.Now()
	app, handled := app.Update(KeyMsg{Key: "j"})
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("keystroke blocked for %v while a poll was in flight", elapsed)
	}
	if !handled {
		t.Fatal("keystroke was not handled during an in-flight poll")
	}
}

// One poll at a time: a tick that lands while a poll is still in flight
// must not start a second fetch.
func TestTickPollIsOneAtATime(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	app := runsPollApp(t, func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		calls.Add(1)
		<-release
		return nil, nil
	})
	app, _ = app.Refresh()
	first := app.tickPoll
	if first == nil {
		t.Fatal("first Refresh started no poll")
	}
	app, _ = app.Refresh()
	app, _ = app.Refresh()
	if app.tickPoll != first {
		t.Fatal("a second poll started while one was already in flight")
	}
}

// A poll started on the runs route must not fold into a different
// screen if the user navigates away before it lands — the result is
// dropped and the slot freed.
func TestTickPollResultDroppedAfterRouteChange(t *testing.T) {
	release := make(chan struct{})
	app := runsPollApp(t, func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		<-release
		return []model.Run{{
			ID: 7777, Name: "CI", RunNumber: 99,
			Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "main",
		}}, nil
	})
	app, _ = app.Refresh()
	// Navigate away while the poll is in flight.
	app.routes = append(app.routes, RouteDetail)
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for !app.tickPoll.ready() {
		if time.Now().After(deadline) {
			t.Fatal("poll never completed")
		}
		time.Sleep(time.Millisecond)
	}
	app, changed := app.drainTickPoll()
	if changed {
		t.Fatal("a runs poll must not apply after the route changed")
	}
	if app.tickPoll != nil {
		t.Fatal("a dropped poll must free the slot")
	}
	for _, run := range app.runs.Context.Runs {
		if run.ID == 7777 {
			t.Fatal("the dropped poll leaked its data into the runs list")
		}
	}
}

// A completed poll applies on the keypress path too, so an active user
// acts on the freshest list the background fetch produced.
func TestKeypressDrainsCompletedTickPoll(t *testing.T) {
	app := runsPollApp(t, func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		return []model.Run{
			{ID: 9001, Name: "CI", RunNumber: 44, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "main"},
			{ID: 9002, Name: "CI", RunNumber: 45, Status: model.StatusInProgress, Conclusion: model.ConclusionNone, HeadBranch: "main"},
		}, nil
	})
	app, _ = app.Refresh()
	deadline := time.Now().Add(2 * time.Second)
	for !app.tickPoll.ready() {
		if time.Now().After(deadline) {
			t.Fatal("poll never completed")
		}
		time.Sleep(time.Millisecond)
	}
	app, _ = app.Update(KeyMsg{Key: "j"})
	if app.tickPoll != nil {
		t.Fatal("the keypress did not drain the completed poll")
	}
	view := ansi.Strip(app.ViewSize(120, 20))
	if !strings.Contains(view, "#45") {
		t.Fatalf("the freshly polled run never reached the view:\n%s", view)
	}
}
