package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

const packTestSHA = "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432"

func packGroupRun(id int64, name string, status model.Status, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:           id,
		Name:         name,
		Event:        "push",
		HeadSHA:      packTestSHA,
		HeadBranch:   "main",
		Status:       status,
		Conclusion:   conclusion,
		RunNumber:    int(id % 1000),
		RunStartedAt: time.Now().Add(-time.Minute),
		UpdatedAt:    time.Now(),
	}
}

func packTestRuns() []model.Run {
	return []model.Run{
		packGroupRun(101, "CI", model.StatusInProgress, model.ConclusionNone),
		packGroupRun(102, "Release", model.StatusInProgress, model.ConclusionNone),
		packGroupRun(103, "Docs", model.StatusQueued, model.ConclusionNone),
		// Same sha, foreign event: outside the pack.
		{ID: 900, Name: "Deploy Pages", Event: "workflow_run", HeadSHA: packTestSHA, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 24},
		// Different sha entirely.
		{ID: 99, Name: "CI", Event: "push", HeadSHA: "f6e5d4c", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 99},
	}
}

func packTestApp(t *testing.T, options Options) App {
	t.Helper()
	cfg := config.Default()
	cfg.Welcome = false
	options.Config = cfg
	if !hasLaunchContext(options.Launch) {
		options.Launch = usecase.LaunchContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs:   packTestRuns(),
		}
	}
	if options.WatchResolver == nil {
		options.WatchResolver = func(_ context.Context, run model.Run) (watch.Model, error) {
			return watch.NewModel(watch.State{Repo: "indrasvat/gh-hound", Branch: "main", Run: run}), nil
		}
	}
	return NewApp(options)
}

func TestWatchKeyOpensThePackBoardForAnEventGroup(t *testing.T) {
	var packCalls atomic.Int32
	app := packTestApp(t, Options{
		PackResolver: func(_ context.Context, state usecase.PackState) (usecase.PackState, error) {
			packCalls.Add(1)
			if state.HeadSHA != packTestSHA || state.Event != "push" || state.Max != 10 {
				t.Errorf("pack state = %#v", state)
			}
			return state, nil
		},
	})

	app, handled := app.Update(KeyMsg{Key: "w"})
	if !handled {
		t.Fatal("w was not handled")
	}
	if app.Route() != RouteWatchBoard {
		t.Fatalf("route = %s, want watch_board", app.Route())
	}
	if len(app.board.Runs) != 3 {
		t.Fatalf("board rows = %d, want the 3-run pack (no foreign events, no other shas)", len(app.board.Runs))
	}
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("pack load did not settle")
	}
	if packCalls.Load() != 1 {
		t.Fatalf("pack resolver calls = %d, want 1 open tick", packCalls.Load())
	}
	view := ansi.Strip(app.ViewSized(100))
	if !strings.Contains(view, "the pack: 3 running · 0 home · 0 lost") {
		t.Fatalf("board view missing aggregate header:\n%s", view)
	}
}

func TestWatchKeySingleRunGroupDegradesToSingleWatch(t *testing.T) {
	var watched atomic.Int64
	app := packTestApp(t, Options{
		Launch: usecase.LaunchContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs:   []model.Run{packGroupRun(101, "CI", model.StatusInProgress, model.ConclusionNone)},
		},
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			watched.Store(run.ID)
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})

	app, _ = app.Update(KeyMsg{Key: "w"})
	if app.Route() != RouteWatch {
		t.Fatalf("route = %s, want the classic single-run watch", app.Route())
	}
	app, _ = app.SettleLoads(time.Second)
	if watched.Load() != 101 {
		t.Fatalf("watched run = %d, want 101", watched.Load())
	}
	_ = app
}

func TestBoardDrillInAndEscReturnsToBoardThenRuns(t *testing.T) {
	app := packTestApp(t, Options{
		PackResolver: func(_ context.Context, state usecase.PackState) (usecase.PackState, error) {
			return state, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)

	app, _ = app.Update(KeyMsg{Key: "j"})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.Route() != RouteWatch {
		t.Fatalf("drill route = %s, want watch", app.Route())
	}
	app, _ = app.SettleLoads(time.Second)
	if app.watch.State.Run.ID != 102 {
		t.Fatalf("drilled run = %d, want 102", app.watch.State.Run.ID)
	}

	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteWatchBoard {
		t.Fatalf("esc from drill = %s, want back on the board", app.Route())
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteRuns {
		t.Fatalf("esc from board = %s, want runs", app.Route())
	}
}

func TestBoardCancelIsConfirmGated(t *testing.T) {
	var cancelled atomic.Int64
	app := packTestApp(t, Options{
		PackResolver: func(_ context.Context, state usecase.PackState) (usecase.PackState, error) {
			return state, nil
		},
		ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
			if request.Action == usecase.ActionCancelRun {
				cancelled.Store(request.Run.ID)
			}
			return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, Message: "accepted"}, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)

	app, _ = app.Update(KeyMsg{Key: "x"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("x must confirm before cancelling, overlay = %q", app.TopOverlay())
	}
	if cancelled.Load() != 0 {
		t.Fatal("cancel fired before confirmation")
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	if cancelled.Load() != 101 {
		t.Fatalf("cancelled run = %d, want the selected 101", cancelled.Load())
	}
	if app.Route() != RouteWatchBoard {
		t.Fatalf("route after cancel = %s, want the board kept", app.Route())
	}
}

func TestRefreshPackPushesTheSettledToastOnce(t *testing.T) {
	settled := []model.Run{
		packGroupRun(101, "CI", model.StatusCompleted, model.ConclusionSuccess),
		packGroupRun(102, "Release", model.StatusCompleted, model.ConclusionSuccess),
		packGroupRun(103, "Docs", model.StatusCompleted, model.ConclusionSuccess),
	}
	app := packTestApp(t, Options{
		PackResolver: func(_ context.Context, state usecase.PackState) (usecase.PackState, error) {
			state.Runs = settled
			state.Terminal = true
			return state, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)
	// The open tick already settled the board; force a live row so the
	// refresh transition is observable.
	app.board = app.board.WithRuns(packTestRuns()[:3])

	app, changed := app.Refresh()
	if !changed {
		t.Fatal("refresh reported no change")
	}
	view := ansi.Strip(app.ViewSized(100))
	if !strings.Contains(view, "pack's home.") {
		t.Fatalf("settled toast missing:\n%s", view)
	}

	// A second refresh of the already-settled pack must not re-toast.
	app.toasts.Toasts = nil
	app, _ = app.Refresh()
	view = ansi.Strip(app.ViewSized(100))
	if strings.Contains(view, "pack's home.") {
		t.Fatal("settled toast repeated after settle")
	}
}

func TestDispatchHandoffAttachesDirectlyFromThe200Body(t *testing.T) {
	var watched atomic.Int64
	app := packTestApp(t, Options{
		ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
			return usecase.ActionResult{
				Action:        usecase.ActionDispatch,
				Message:       "Workflow dispatch queued",
				WorkflowRunID: 27318354797,
				HTMLURL:       "https://github.com/indrasvat/gh-hound/actions/runs/27318354797",
			}, nil
		},
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			watched.Store(run.ID)
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})
	app.config.AutoWatch = true
	app.routes = []Route{RouteRuns, RouteDispatch}
	app.dispatch = dispatch.NewModel(dispatch.Workflow{Name: "Release", ID: "release.yml", Ref: "main"})

	app, accepted := app.executeAction(RouteDispatch, ActionRequest{
		Action:   usecase.ActionDispatch,
		Workflow: dispatch.Workflow{Name: "Release", ID: "release.yml"},
		Dispatch: usecase.DispatchRequest{Ref: "main"},
	})
	if !accepted {
		t.Fatal("dispatch was refused")
	}
	if app.Route() != RouteWatch {
		t.Fatalf("route = %s, want watch (direct 200-body attach)", app.Route())
	}
	app, _ = app.SettleLoads(time.Second)
	if watched.Load() != 27318354797 {
		t.Fatalf("watched run = %d, want the dispatched 27318354797", watched.Load())
	}
}

func TestDispatchHandoff204FallsBackToBoundedDiscovery(t *testing.T) {
	var sawWorkflow, sawRef string
	var sawSince time.Time
	discovered := model.Run{ID: 4242, Name: "Release", Event: "workflow_dispatch", CreatedAt: time.Now()}
	var watched atomic.Int64
	app := packTestApp(t, Options{
		ActionHandler: func(ActionRequest) (usecase.ActionResult, error) {
			// A 204 host: no run identity in the result.
			return usecase.ActionResult{Action: usecase.ActionDispatch, Message: "Workflow dispatch queued"}, nil
		},
		DispatchAttachResolver: func(_ context.Context, workflowID, ref string, since time.Time) (model.Run, error) {
			sawWorkflow, sawRef, sawSince = workflowID, ref, since
			return discovered, nil
		},
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			watched.Store(run.ID)
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})
	app.config.AutoWatch = true
	app.routes = []Route{RouteRuns, RouteDispatch}

	before := time.Now()
	app, _ = app.executeAction(RouteDispatch, ActionRequest{
		Action:   usecase.ActionDispatch,
		Workflow: dispatch.Workflow{Name: "Release", ID: "release.yml"},
		Dispatch: usecase.DispatchRequest{Ref: "main"},
	})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("discovery load did not settle")
	}
	if sawWorkflow != "release.yml" || sawRef != "main" {
		t.Fatalf("discovery args = %q %q", sawWorkflow, sawRef)
	}
	if !sawSince.Before(before) {
		t.Fatalf("discovery fence %s must predate the dispatch (skew allowance)", sawSince)
	}
	if watched.Load() != 4242 {
		t.Fatalf("watched run = %d, want the discovered 4242", watched.Load())
	}
	if app.Route() != RouteWatch {
		t.Fatalf("route = %s, want watch", app.Route())
	}
}

func TestDispatchHandoffScentLostReturnsGracefully(t *testing.T) {
	app := packTestApp(t, Options{
		ActionHandler: func(ActionRequest) (usecase.ActionResult, error) {
			return usecase.ActionResult{Action: usecase.ActionDispatch, Message: "Workflow dispatch queued"}, nil
		},
		DispatchAttachResolver: func(context.Context, string, string, time.Time) (model.Run, error) {
			return model.Run{}, usecase.ErrScentLost
		},
	})
	app.config.AutoWatch = true
	app.routes = []Route{RouteRuns, RouteDispatch}

	app, _ = app.executeAction(RouteDispatch, ActionRequest{
		Action:   usecase.ActionDispatch,
		Workflow: dispatch.Workflow{Name: "Release", ID: "release.yml"},
		Dispatch: usecase.DispatchRequest{Ref: "main"},
	})
	app, _ = app.SettleLoads(time.Second)
	if app.Route() != RouteRuns {
		t.Fatalf("route after lost scent = %s, want a graceful return to runs", app.Route())
	}
	view := ansi.Strip(app.ViewSized(100))
	if !strings.Contains(view, "couldn't pick up the scent.") {
		t.Fatalf("scent-lost toast missing:\n%s", view)
	}
}

func TestDispatchHandoffHonorsAutoWatchOff(t *testing.T) {
	app := packTestApp(t, Options{
		ActionHandler: func(ActionRequest) (usecase.ActionResult, error) {
			return usecase.ActionResult{Action: usecase.ActionDispatch, Message: "Workflow dispatch queued", WorkflowRunID: 1}, nil
		},
	})
	app.routes = []Route{RouteRuns, RouteDispatch}

	app, _ = app.executeAction(RouteDispatch, ActionRequest{
		Action:   usecase.ActionDispatch,
		Workflow: dispatch.Workflow{Name: "Release", ID: "release.yml"},
		Dispatch: usecase.DispatchRequest{Ref: "main"},
	})
	if app.Route() != RouteDispatch {
		t.Fatalf("route = %s, want dispatch unchanged with auto_watch off", app.Route())
	}
	if app.load != nil {
		t.Fatal("auto_watch off must not start a handoff load")
	}
}

func TestRerunHandoffAttachesToTheExistingRun(t *testing.T) {
	stale := packGroupRun(101, "CI", model.StatusCompleted, model.ConclusionFailure)
	stale.RunAttempt = 1
	advanced := stale
	advanced.RunAttempt = 2
	advanced.Status = model.StatusQueued
	advanced.Conclusion = model.ConclusionNone

	var attachCalls atomic.Int32
	var watchedAttempt atomic.Int32
	app := packTestApp(t, Options{
		ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
			return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, Message: "Re-run queued"}, nil
		},
		RerunAttachResolver: func(_ context.Context, run model.Run) (model.Run, error) {
			attachCalls.Add(1)
			if run.ID != stale.ID {
				t.Errorf("rerun attach polled run %d, want the existing %d (no discovery)", run.ID, stale.ID)
			}
			return advanced, nil
		},
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			watchedAttempt.Store(int32(run.RunAttempt))
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})
	app.config.AutoWatch = true

	app, _ = app.executeAction(RouteRuns, ActionRequest{Action: usecase.ActionRerunRun, Run: stale})
	if app.Route() != RouteWatch {
		t.Fatalf("route = %s, want watch", app.Route())
	}
	app, _ = app.SettleLoads(time.Second)
	if attachCalls.Load() != 1 {
		t.Fatalf("attach calls = %d, want 1", attachCalls.Load())
	}
	if watchedAttempt.Load() != 2 {
		t.Fatalf("watched attempt = %d, want the advanced attempt 2", watchedAttempt.Load())
	}
}

func TestPaletteWatchItemOpensThePack(t *testing.T) {
	app := packTestApp(t, Options{
		PackResolver: func(_ context.Context, state usecase.PackState) (usecase.PackState, error) {
			return state, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"w", "a", "t", "c", "h"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.Route() != RouteWatchBoard {
		t.Fatalf("palette watch route = %s, want watch_board", app.Route())
	}
}
