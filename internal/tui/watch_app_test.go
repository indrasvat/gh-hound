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
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
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
				return watch.Model{}, errors.New("github log url https://api.github.com/repos/indrasvat/gh-hound/actions/jobs/7/logs returned 502: Bad Gateway")
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
	if strings.Contains(view, "#41") || !strings.Contains(view, "watch unavailable: github log url") {
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

// The detail screen follows a live run: the run and its jobs refresh
// on poll ticks while the run goes — a job listed late (a dynamic
// matrix) shows up even after every earlier job has finished — and
// once the run has finished it stops asking.
func TestDetailPollFollowsALiveRun(t *testing.T) {
	var polls atomic.Int32
	app := watchTwoRunsApp(Options{
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			n := polls.Add(1)
			lint := model.Job{ID: 1, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}
			jobs := []model.Job{lint}
			switch {
			case n >= 6:
				run.Status, run.Conclusion = model.StatusCompleted, model.ConclusionSuccess
				jobs = append(jobs, model.Job{ID: 2, Name: "shard", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess})
			case n >= 4:
				jobs = append(jobs, model.Job{ID: 2, Name: "shard", Status: model.StatusInProgress})
			}
			return detail.NewModel(run, jobs), nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	if app.Route() != RouteDetail {
		t.Fatalf("route = %s, want detail", app.Route())
	}
	for deadline := time.Now().Add(3 * time.Second); app.detail.Run.Status != model.StatusCompleted && time.Now().Before(deadline); {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	if app.detail.Run.Status != model.StatusCompleted || len(app.detail.Jobs) != 2 || app.detail.Jobs[1].Status != model.StatusCompleted {
		t.Fatalf("after %d polls run = %s, jobs = %#v; want the finished run with the late job", polls.Load(), app.detail.Run.Status, app.detail.Jobs)
	}
	settled := polls.Load()
	for range 50 {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	if polls.Load() != settled {
		t.Fatalf("detail kept polling a finished run: %d → %d", settled, polls.Load())
	}
}

// A gate approved elsewhere clears: once the run is past waiting, the
// pending-environments panel goes, even though the poll lists none.
func TestDetailPollClearsAPassedGate(t *testing.T) {
	var polls atomic.Int32
	app := watchTwoRunsApp(Options{
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			m := detail.NewModel(run, []model.Job{{ID: 1, Name: "deploy", Status: model.StatusWaiting}})
			if polls.Add(1) <= 2 {
				m.Run.Status = model.StatusWaiting
				return m.WithPendingDeployments([]model.PendingDeployment{{EnvironmentID: 1, EnvironmentName: "production"}}), nil
			}
			m.Run.Status = model.StatusInProgress
			return m, nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	for deadline := time.Now().Add(3 * time.Second); len(app.detail.PendingDeployments) > 0 && time.Now().Before(deadline); {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	if len(app.detail.PendingDeployments) != 0 || app.detail.Run.Status != model.StatusInProgress {
		t.Fatalf("after %d polls gates = %d, run = %s; want the gate gone", polls.Load(), len(app.detail.PendingDeployments), app.detail.Run.Status)
	}
}

// A poll begun for an earlier opening of the run never lands on a later
// one: esc and enter again before it returns, and its answer is dropped.
func TestDetailPollFromAnEarlierOpeningIsDropped(t *testing.T) {
	app := watchTwoRunsApp(Options{
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{{ID: 1, Name: "fresh", Status: model.StatusInProgress}}), nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	run, gen := app.detail.Run, app.detailGen
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	stale := detail.NewModel(run, []model.Job{{ID: 1, Name: "STALE", Status: model.StatusInProgress}})
	app = app.applyDetailRefresh(run, gen, stale, nil)
	if got := app.detail.Jobs[0].Name; got != "fresh" {
		t.Fatalf("a poll from the earlier opening landed: job %q", got)
	}
}

// n and l on a job still running say its log is not ready and point at
// w, instead of fetching into a "no log" error screen.
func TestDetailLogOnARunningJobSaysNotReady(t *testing.T) {
	var logs atomic.Int32
	app := watchTwoRunsApp(Options{
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{{ID: 7, Name: "bdd", Status: model.StatusInProgress, Steps: []model.Step{
				{Number: 29, Name: "Run tests", Status: model.StatusInProgress},
			}}}), nil
		},
		LogResolver: func(context.Context, model.Run, model.Job, func(read, total int64)) (logscreen.Model, error) {
			logs.Add(1)
			return logscreen.Model{}, nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	for _, key := range []string{"l", "n"} {
		app, _ = app.Update(KeyMsg{Key: key})
		app, _ = app.SettleLoads(time.Second)
		if app.Route() != RouteDetail {
			t.Fatalf("%s on a running job left detail for %s", key, app.Route())
		}
	}
	if logs.Load() != 0 {
		t.Fatalf("a running job's log was fetched %d times", logs.Load())
	}
	view := ansi.Strip(app.ViewSize(120, 30))
	if !strings.Contains(view, "log not ready") || !strings.Contains(view, "w follows its steps · bdd is still running") {
		t.Fatalf("no not-ready toast:\n%s", view)
	}
}

// A first load that failed heals on the next successful poll — for a
// finished run too — and its error goes; once answered, a finished run
// with no jobs stops asking.
func TestDetailPollRecoversAFailedLoad(t *testing.T) {
	var calls atomic.Int32
	app := watchTwoRunsApp(Options{
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			if calls.Add(1) <= 2 {
				return detail.Model{}, errors.New("boom")
			}
			run.Status, run.Conclusion = model.StatusCompleted, model.ConclusionSuccess
			return detail.NewModel(run, nil), nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	if !strings.Contains(ansi.Strip(app.ViewSize(100, 24)), "detail unavailable") {
		t.Fatalf("first load did not fail:\n%s", ansi.Strip(app.ViewSize(100, 24)))
	}
	for deadline := time.Now().Add(3 * time.Second); !app.detailLoaded && time.Now().Before(deadline); {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	app, _ = app.Refresh()
	if view := ansi.Strip(app.ViewSize(100, 24)); strings.Contains(view, "detail unavailable") || app.detail.Run.Status != model.StatusCompleted {
		t.Fatalf("after %d calls the error still shows or the answer was dropped:\n%s", calls.Load(), view)
	}
	settled := calls.Load()
	for range 50 {
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != settled {
		t.Fatalf("a finished run with no jobs kept polling: %d → %d", settled, calls.Load())
	}
}

// l and ⏎ on a live run with no job listed yet say so; nothing fetched.
func TestDetailLogOnALiveRunWithoutJobsSaysNotReady(t *testing.T) {
	var logs atomic.Int32
	app := watchTwoRunsApp(Options{
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			return detail.NewModel(run, nil), nil
		},
		LogResolver: func(context.Context, model.Run, model.Job, func(read, total int64)) (logscreen.Model, error) {
			logs.Add(1)
			return logscreen.Model{}, nil
		},
	})
	defer app.Shutdown()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.SettleLoads(time.Second)
	for _, key := range []string{"l", "enter"} {
		app, _ = app.Update(KeyMsg{Key: key})
		app, _ = app.SettleLoads(time.Second)
		if app.Route() != RouteDetail {
			t.Fatalf("%s on a run with no jobs left detail for %s", key, app.Route())
		}
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); logs.Load() != 0 || !strings.Contains(view, "no job has started yet") {
		t.Fatalf("fetches = %d; view:\n%s", logs.Load(), view)
	}
}

// While the jobs are still loading, l claims nothing: "no job has
// started yet" is only for a run GitHub answered with no jobs (seen
// live on an 88-job rust-lang run whose listing took seconds).
func TestDetailLogWhileJobsLoadClaimsNothing(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	app := watchTwoRunsApp(Options{
		DetailResolver: func(ctx context.Context, run model.Run) (detail.Model, error) {
			if calls.Add(1) > 1 { // NewApp resolves once up front
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return detail.NewModel(run, []model.Job{{ID: 1, Name: "bdd", Status: model.StatusInProgress}}), nil
		},
	})
	defer app.Shutdown()
	defer close(release)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.Update(KeyMsg{Key: "l"})
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "no job has started yet") {
		t.Fatalf("claimed no job while the jobs load:\n%s", view)
	}
}
