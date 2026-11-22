package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func heldRun(id int64, name string) model.Run {
	return model.Run{ID: id, RunNumber: int(id), Name: name, Event: "pull_request", HeadSHA: "a1f93c0", HeadBranch: "patch-1",
		Actor: "new-dev", FromFork: true, Status: model.StatusCompleted, Conclusion: model.ConclusionActionRequired, RunAttempt: 1,
		CreatedAt: time.Now().Add(-2 * time.Hour)}
}

func verdictApp(runs []model.Run, handler func(ActionRequest) (usecase.ActionResult, error), resolver func(context.Context, model.Run) (model.RunVerdict, error)) App {
	cfg := config.Default()
	cfg.Welcome = false
	return NewApp(Options{
		Config:          cfg,
		Launch:          usecase.LaunchContext{Repo: "acme/forge", Branch: "main", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, Runs: runs},
		ActionHandler:   handler,
		VerdictResolver: resolver,
	})
}

// A on a held fork run asks first, defaults to No, and approves every
// run held on that commit once confirmed.
func TestApproveHeldForkRunsBehindAConfirm(t *testing.T) {
	var called []ActionRequest
	handler := func(request ActionRequest) (usecase.ActionResult, error) {
		called = append(called, request)
		return usecase.ActionResult{Action: request.Action, Message: "2 runs approved · they start shortly"}, nil
	}
	other := heldRun(9409, "Docs")
	other.HeadSHA = "ffffff0"
	app := verdictApp([]model.Run{heldRun(9412, "Unit tests"), heldRun(9411, "Lint"), other}, handler, nil)

	app, _ = app.Update(KeyMsg{Key: "A"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("A did not ask: overlay=%s", app.TopOverlay())
	}
	if view := ansi.Strip(app.ViewSize(120, 40)); !strings.Contains(view, "approve 2 runs from @new-dev's fork? their code will run") {
		t.Fatalf("confirm does not name the runs and the fork:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.TopOverlay() != OverlayNone || len(called) != 0 {
		t.Fatalf("enter approved: overlay=%s calls=%d", app.TopOverlay(), len(called))
	}

	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	if len(called) != 1 || called[0].Action != usecase.ActionApproveRun || len(called[0].Held) != 2 {
		t.Fatalf("approve call = %+v, want one approve of the 2 runs held on a1f93c0", called)
	}
	if view := ansi.Strip(app.ViewSize(120, 40)); app.TopOverlay() != OverlayNone || !strings.Contains(view, "2 runs approved") {
		t.Fatalf("after y: overlay=%s, want the success toast:\n%s", app.TopOverlay(), view)
	}
}

// The same flow from the detail view: A, then y, approves.
func TestApproveHeldForkRunFromDetail(t *testing.T) {
	var called []ActionRequest
	cfg := config.Default()
	cfg.Welcome = false
	runs := []model.Run{heldRun(9412, "Unit tests"), heldRun(9411, "Lint")}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{Repo: "acme/forge", Branch: "main", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, Runs: runs},
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			return detail.NewModel(run, nil).WithRepo("acme/forge"), nil
		},
		ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
			called = append(called, request)
			return usecase.ActionResult{Action: request.Action, Message: "2 runs approved · they start shortly"}, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	view := ansi.Strip(app.ViewSize(120, 40))
	if app.Route() != RouteDetail || !strings.Contains(view, "A approve and run") {
		t.Fatalf("detail does not offer A: route=%s\n%s", app.Route(), view)
	}
	// Keys that cannot help a run in which no job ran leave the footer.
	if !strings.Contains(view, "o browser · y copy URL · ⎋ back · ?") || strings.Contains(view, "rerun job") {
		t.Fatalf("detail footer still offers keys that cannot help:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "A"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("A did not ask: overlay=%s", app.TopOverlay())
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	if len(called) != 1 || len(called[0].Held) != 2 || app.TopOverlay() != OverlayNone {
		t.Fatalf("y from detail: calls=%+v overlay=%s", called, app.TopOverlay())
	}
	// The held run's detail is stale now: back on the list.
	if app.Route() != RouteRuns {
		t.Fatalf("route after approve = %s, want runs", app.Route())
	}
	if view := ansi.Strip(app.ViewSize(120, 40)); !strings.Contains(view, "2 runs approved") {
		t.Fatalf("no success toast:\n%s", view)
	}
}

// A held run that is not from a fork (a bot or agent PR) is approved on
// the web: no confirm, no API call, a toast that says so.
func TestApproveHeldBotRunPointsToTheWeb(t *testing.T) {
	bot := heldRun(9412, "Unit tests")
	bot.FromFork = false
	var calls int
	app := verdictApp([]model.Run{bot}, func(ActionRequest) (usecase.ActionResult, error) { calls++; return usecase.ActionResult{}, nil }, nil)
	app, _ = app.Update(KeyMsg{Key: "A"})
	if app.TopOverlay() != OverlayNone || calls != 0 {
		t.Fatalf("overlay=%s calls=%d, want a toast only", app.TopOverlay(), calls)
	}
	if view := ansi.Strip(app.ViewSize(120, 40)); !strings.Contains(view, "approve this one on GitHub") {
		t.Fatalf("no toast pointing at the web:\n%s", view)
	}
}

// Rows the run fields cannot settle are classified off the key path;
// a failed read keeps the row failing and warns once.
func TestRedRowsGetTheirVerdictInTheBackground(t *testing.T) {
	missed := model.Run{ID: 7, RunNumber: 7, Name: "Unit tests", Event: "pull_request", HeadBranch: "main", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunAttempt: 1}
	broken := missed
	broken.ID, broken.RunNumber = 8, 8
	release := make(chan struct{})
	resolver := func(_ context.Context, run model.Run) (model.RunVerdict, error) {
		<-release
		if run.ID == broken.ID {
			return model.RunVerdict{}, errors.New("502")
		}
		return model.RunVerdict{Verdict: model.VerdictNeverRan, Reason: model.ReasonApprovalMissed}, nil
	}
	app := verdictApp([]model.Run{missed, broken}, nil, resolver)
	app, _ = app.Refresh()
	if view := ansi.Strip(app.ViewSize(120, 40)); !strings.Contains(view, "2 failing") {
		t.Fatalf("unread red rows do not count as failing:\n%s", view)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for app.verdictFetch != nil && time.Now().Before(deadline) {
		app, _ = app.Refresh()
		time.Sleep(10 * time.Millisecond)
	}
	view := ansi.Strip(app.ViewSize(120, 40))
	for _, want := range []string{"1 failing", "1 never ran", "missed", "could not read why a run is red"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view lacks %q:\n%s", want, view)
		}
	}
}
