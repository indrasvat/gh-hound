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
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// A watch that fails to open must not leave the previous run's watch
// on screen: on main the old run kept polling, its next tick cleared
// the error, and the user watched the wrong run with no sign of it.
func TestFailedWatchLoadDoesNotKeepThePreviousRun(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	cfg.PollMin, cfg.PollMax = time.Millisecond, time.Millisecond
	first := model.Run{ID: 101, Name: "CI", RunNumber: 41, Event: "push", HeadSHA: "aaa1111", HeadBranch: "main", Status: model.StatusInProgress}
	second := model.Run{ID: 102, Name: "CI", RunNumber: 42, Event: "push", HeadSHA: "bbb2222", HeadBranch: "main", Status: model.StatusInProgress}
	var firstCalls atomic.Int32
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo: "indrasvat/gh-hound", Branch: "main",
			Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns,
			Runs: []model.Run{first, second},
		},
		WatchResolver: func(_ context.Context, run model.Run) (watch.Model, error) {
			if run.ID == second.ID {
				return watch.Model{}, errors.New("github log download failed for job 7: status 404")
			}
			firstCalls.Add(1)
			return watch.NewModel(watch.State{Repo: "indrasvat/gh-hound", Branch: "main", Run: run}), nil
		},
	})
	defer app.Shutdown()

	app, _ = app.Update(KeyMsg{Key: "w"})
	app, _ = app.SettleLoads(time.Second)
	if view := ansi.Strip(app.ViewSize(100, 24)); !strings.Contains(view, "watch · CI #41") {
		t.Fatalf("first watch did not open:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, _ = app.Update(KeyMsg{Key: "w"})
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
