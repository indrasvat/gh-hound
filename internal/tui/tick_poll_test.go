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

// codex r1 #1: a runs poll started under one scope must not fold its
// result into the list after the user toggles scope mid-flight.
func TestStaleRunsPollDroppedAfterScopeChange(t *testing.T) {
	release := make(chan struct{})
	app := runsPollApp(t, func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		<-release
		return []model.Run{{
			ID: 5555, Name: "CI", RunNumber: 55,
			Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "feature",
		}}, nil
	})
	app = app.startRunsPoll() // branch-scope poll in flight
	app.runs.Context.Scope = usecase.LaunchScopeRepo
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
		t.Fatal("a branch-scope poll must not apply after the scope toggled to repo")
	}
	for _, run := range app.runs.Context.Runs {
		if run.ID == 5555 {
			t.Fatal("the stale-scope poll leaked its run into the list")
		}
	}
}

// codex r1 #1 (filter variant): a runs poll started under one filter
// must not apply after the filter changes.
func TestStaleRunsPollDroppedAfterFilterChange(t *testing.T) {
	release := make(chan struct{})
	app := runsPollApp(t, func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		<-release
		return []model.Run{{ID: 4444, Name: "CI", RunNumber: 44, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "main"}}, nil
	})
	app = app.startRunsPoll()
	app.runs.Filter = "failure"
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for !app.tickPoll.ready() {
		if time.Now().After(deadline) {
			t.Fatal("poll never completed")
		}
		time.Sleep(time.Millisecond)
	}
	if _, changed := app.drainTickPoll(); changed {
		t.Fatal("a poll started before a filter change must not apply after it")
	}
}

// codex r1 #2: a hunt-board poll started for one board must not fold
// into a different board the user switched to mid-flight.
func TestStalePackPollDroppedAfterBoardSwitch(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	app := packTestApp(t, Options{
		PackResolver: func(_ context.Context, state usecase.PackState) (usecase.PackState, error) {
			if calls.Add(1) > 1 {
				<-release // the poll hangs; the foreground open does not
			}
			state.Runs = packTestRuns()
			return state, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "w"}) // opens the board (foreground load)
	app = settleApp(t, app)
	app = app.startPackPoll() // board-A poll in flight
	// User switches to a different board (different head SHA + event).
	app.board.HeadSHA = "deadbeef"
	app.board.Event = "schedule"
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for !app.tickPoll.ready() {
		if time.Now().After(deadline) {
			t.Fatal("poll never completed")
		}
		time.Sleep(time.Millisecond)
	}
	if _, changed := app.drainTickPoll(); changed {
		t.Fatal("a board-A poll must not apply after switching to board B")
	}
}

// codex r1 #3: a stuck poll for one surface must not wedge polling for
// the surface the user moves to — the stale poll is cancelled and the
// slot freed for the new one.
func TestStuckPollSupersededByNewSurface(t *testing.T) {
	cancelled := make(chan struct{})
	app := runsPollApp(t, nil)
	// A runs poll whose fetch hangs until its context is cancelled.
	app = app.startTickPoll(RouteRuns, "runs|stuck|", func(ctx context.Context) func(App) App {
		<-ctx.Done()
		close(cancelled)
		return func(a App) App { return a }
	})
	stuck := app.tickPoll
	if stuck == nil {
		t.Fatal("the stuck poll never started")
	}
	// Moving to a different surface supersedes it.
	app = app.startTickPoll(RouteWatchBoard, "board|x", func(context.Context) func(App) App {
		return func(a App) App { return a }
	})
	if app.tickPoll == stuck {
		t.Fatal("the stuck poll wedged the slot — new surface could not poll")
	}
	if app.tickPoll == nil || app.tickPoll.route != RouteWatchBoard {
		t.Fatal("the new surface's poll did not take the slot")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the superseded poll's context was never cancelled")
	}
}
