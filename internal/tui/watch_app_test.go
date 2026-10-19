package tui

import (
	"context"
	"errors"
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

var (
	watchRunA = model.Run{ID: 101, Name: "CI", RunNumber: 41, Event: "push", HeadSHA: "aaa1111", HeadBranch: "main", Status: model.StatusInProgress}
	watchRunB = model.Run{ID: 102, Name: "CI", RunNumber: 42, Event: "push", HeadSHA: "bbb2222", HeadBranch: "main", Status: model.StatusInProgress}
)

// watchTwoRunsApp lists #41 and #42 (one-run packs, so w opens the
// single-run watch) and polls as fast as the config allows.
func watchTwoRunsApp(options Options) App {
	cfg := config.Default()
	cfg.Welcome = false
	cfg.AutoWatch = true
	cfg.PollMin, cfg.PollMax = time.Millisecond, time.Millisecond
	options.Config = cfg
	options.Launch = usecase.LaunchContext{
		Repo: "indrasvat/gh-hound", Branch: "main",
		Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns,
		Runs: []model.Run{watchRunA, watchRunB},
	}
	return NewApp(options)
}

// watchAThenB watches #41, goes back, and opens #42.
func watchAThenB(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)
	if view := ansi.Strip(app.ViewSize(100, 24)); !strings.Contains(view, "watch · CI #41") {
		t.Fatalf("first watch did not open:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, _ = app.Update(KeyMsg{Key: "w"})
	return app
}

// A watch that fails to open must not leave the previous run's watch
// on screen: on main the old run kept polling, its next tick cleared
// the error, and the user watched the wrong run with no sign of it.
func TestFailedWatchLoadDoesNotKeepThePreviousRun(t *testing.T) {
	var firstCalls atomic.Int32
	app := watchTwoRunsApp(Options{
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			if run.ID == watchRunB.ID {
				return watch.Model{}, errors.New("github log download failed for job 7: status 404")
			}
			firstCalls.Add(1)
			return watch.NewModel(watch.State{Repo: "indrasvat/gh-hound", Branch: "main", Run: run}), nil
		},
	})
	defer app.Shutdown()

	app = watchAThenB(t, app)
	app, _ = app.SettleLoads(time.Second)
	opened := firstCalls.Load()

	for range 20 {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	view := ansi.Strip(app.ViewSize(100, 24))
	if strings.Contains(view, "#41") || !strings.Contains(view, "watch unavailable: github log download failed") {
		t.Fatalf("failed watch of #42 must show its error, not #41:\n%s", view)
	}
	if calls := firstCalls.Load(); calls != opened {
		t.Fatalf("run #41 polled %d more times after the user left it", calls-opened)
	}
	// On main, x here asked to cancel #41, the run the user had left.
	if app, _ = app.Update(KeyMsg{Key: "x"}); app.TopOverlay() != OverlayNone {
		t.Fatalf("x on a failed watch opened %s:\n%s", app.TopOverlay(), ansi.Strip(app.ViewSize(100, 24)))
	}
}

// While #42 loads, the screen still holds #41's watch: x must not
// offer to cancel #41 (also on main).
func TestCancelWaitsForTheWatchToLoad(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	app := watchTwoRunsApp(Options{
		WatchResolver: func(ctx context.Context, run model.Run) (watch.Model, error) {
			if run.ID == watchRunB.ID {
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})
	defer app.Shutdown()

	app = watchAThenB(t, app)
	if app, _ = app.Update(KeyMsg{Key: "x"}); app.TopOverlay() != OverlayNone {
		t.Fatalf("x while #42 loads opened %s:\n%s", app.TopOverlay(), ansi.Strip(app.ViewSize(100, 24)))
	}
}

// An auto-watch after a rerun or a 204 dispatch must not show the
// previous watch's failure (also on main: only w cleared it).
func TestAutoWatchClearsAPreviousFailure(t *testing.T) {
	for _, attach := range []string{"rerun", "dispatch-204"} {
		t.Run(attach, func(t *testing.T) {
			app := watchTwoRunsApp(Options{
				ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
					return usecase.ActionResult{Action: request.Action}, nil
				},
				RerunAttachResolver: func(_ context.Context, run model.Run) (model.Run, error) { return run, nil },
				DispatchAttachResolver: func(context.Context, string, string, time.Time) (model.Run, error) {
					return model.Run{ID: 555, Name: "Release", RunNumber: 7}, nil
				},
				WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
					if run.ID == watchRunB.ID {
						return watch.Model{}, errors.New("boom-42")
					}
					return watch.NewModel(watch.State{Run: run}), nil
				},
			})
			defer app.Shutdown()
			app = watchAThenB(t, app)
			app, _ = app.SettleLoads(time.Second)
			app, _ = app.Update(KeyMsg{Key: "esc"})

			if attach == "rerun" {
				app, _ = app.executeAction(RouteRuns, ActionRequest{Action: usecase.ActionRerunRun, Run: watchRunA})
			} else {
				app.PushRoute(RouteDispatch)
				app.dispatch = dispatch.NewModel(dispatch.Workflow{Name: "Release", ID: "release.yml", Ref: "main"})
				app, _ = app.executeAction(RouteDispatch, ActionRequest{Action: usecase.ActionDispatch, Workflow: app.dispatch.Workflow, Dispatch: usecase.DispatchRequest{Ref: "main"}})
			}
			if view := ansi.Strip(app.ViewSize(100, 24)); strings.Contains(view, "boom-42") {
				t.Fatalf("%s auto-watch shows #42's failure while it loads:\n%s", attach, view)
			}
			app, _ = app.SettleLoads(time.Second)
			if view := ansi.Strip(app.ViewSize(100, 24)); strings.Contains(view, "boom-42") {
				t.Fatalf("%s auto-watch shows #42's failure after it loads:\n%s", attach, view)
			}
		})
	}
}

// With no run on screen (a failed open, or another run's watch still
// loading) the footer offers only what works, and the chrome does not
// claim a stream.
func TestWatchWithoutARunOffersOnlyBack(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	app := watchTwoRunsApp(Options{
		WatchResolver: func(ctx context.Context, run model.Run) (watch.Model, error) {
			if run.ID == watchRunB.ID {
				select {
				case <-release:
				case <-ctx.Done():
				}
				return watch.Model{}, errors.New("boom-42")
			}
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})
	defer app.Shutdown()

	app = watchAThenB(t, app)
	loading := ansi.Strip(app.ViewSize(100, 24))
	release <- struct{}{}
	app, _ = app.SettleLoads(time.Second)
	failed := ansi.Strip(app.ViewSize(100, 24))
	for name, view := range map[string]string{"loading": loading, "failed": failed} {
		if strings.Contains(view, "cancel") || strings.Contains(view, "follow") || strings.Contains(view, "streaming") || !strings.Contains(view, "⎋ back") {
			t.Fatalf("%s watch without a run:\n%s", name, view)
		}
	}
}

// f and d are the user's: a poll must not reset them (also on main).
func TestWatchPollKeepsFollowAndDebug(t *testing.T) {
	var polls atomic.Int32
	app := watchTwoRunsApp(Options{
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			polls.Add(1)
			return watch.NewModel(watch.State{Run: run}), nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "f"})
	app, _ = app.Update(KeyMsg{Key: "d"})
	for deadline := time.Now().Add(2 * time.Second); polls.Load() < 4 && time.Now().Before(deadline); {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	app, _ = app.Refresh()
	if app.watch.Follow || !app.watch.Debug {
		t.Fatalf("after %d polls follow=%v debug=%v, want the user's off/on", polls.Load(), app.watch.Follow, app.watch.Debug)
	}
	if view := ansi.Strip(app.ViewSize(100, 24)); !strings.Contains(view, "follow ○") || strings.Contains(view, "follow ●") {
		t.Fatalf("view does not show follow off:\n%s", view)
	}
}

// A finished run's watch offers no cancel and claims no stream.
func TestFinishedRunWatchOffersNoCancel(t *testing.T) {
	done := watchRunA
	done.Status, done.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	app := watchTwoRunsApp(Options{
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			return watch.NewModel(watch.State{Run: done}), nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)
	view := ansi.Strip(app.ViewSize(100, 24))
	if strings.Contains(view, "cancel") || strings.Contains(view, "streaming") ||
		!strings.Contains(view, "f follow · d debug · ⎋ detach") || !strings.Contains(view, "success · follow ●") {
		t.Fatalf("finished run's watch:\n%s", view)
	}
	if app, _ = app.Update(KeyMsg{Key: "x"}); app.TopOverlay() != OverlayNone {
		t.Fatalf("x on a finished run opened %s", app.TopOverlay())
	}
}
