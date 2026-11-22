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
		HeadRepository: "new-dev/forge", Actor: "new-dev", FromFork: true, Status: model.StatusCompleted,
		Conclusion: model.ConclusionActionRequired, RunAttempt: 1, CreatedAt: time.Now().Add(-2 * time.Hour)}
}

type verdictAppOptions struct {
	runs     []model.Run
	handler  func(ActionRequest) (usecase.ActionResult, error)
	resolver func(context.Context, model.Run) (model.RunVerdict, error)
	canPush  bool
	jobs     []model.Job
	jobsErr  error
}

func verdictApp(o verdictAppOptions) App {
	cfg := config.Default()
	cfg.Welcome = false
	return NewApp(Options{
		Config:          cfg,
		Launch:          usecase.LaunchContext{Repo: "acme/forge", Branch: "main", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, Runs: o.runs},
		ActionHandler:   o.handler,
		VerdictResolver: o.resolver,
		PushAccessResolver: func(context.Context) (bool, error) {
			return o.canPush, nil
		},
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			if o.jobsErr != nil {
				return detail.Model{}, o.jobsErr
			}
			return detail.NewModel(run, o.jobs).WithRepo("acme/forge"), nil
		},
	})
}

// press sends one key and lets any load it started apply.
func press(t *testing.T, app App, key string) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: key})
	return settleApp(t, app)
}

func screenText(app App) string { return ansi.Strip(app.ViewSize(120, 40)) }

func approveRecorder(calls *[]ActionRequest) func(ActionRequest) (usecase.ActionResult, error) {
	return func(request ActionRequest) (usecase.ActionResult, error) {
		*calls = append(*calls, request)
		return usecase.ActionResult{Action: request.Action, Message: "2 runs approved · they start shortly"}, nil
	}
}

// A on a held fork run checks access, asks (default No), and approves
// every run held on that commit from that fork, off the key path.
func TestApproveHeldForkRunsBehindAConfirm(t *testing.T) {
	var calls []ActionRequest
	sameSHAOtherFork := heldRun(9410, "Docs")
	sameSHAOtherFork.HeadRepository, sameSHAOtherFork.Actor = "someone-else/forge", "someone-else"
	app := verdictApp(verdictAppOptions{runs: []model.Run{heldRun(9412, "Unit tests"), heldRun(9411, "Lint"), sameSHAOtherFork}, handler: approveRecorder(&calls), canPush: true})

	app = press(t, app, "A")
	if app.TopOverlay() != OverlayConfirm || !strings.Contains(screenText(app), "approve 2 runs from @new-dev's fork? their code will run") {
		t.Fatalf("A did not ask about the 2 runs of one fork: overlay=%s\n%s", app.TopOverlay(), screenText(app))
	}
	app = press(t, app, "enter")
	if app.TopOverlay() != OverlayNone || len(calls) != 0 {
		t.Fatalf("enter approved: overlay=%s calls=%d", app.TopOverlay(), len(calls))
	}

	app = press(t, app, "A")
	app, _ = app.Update(KeyMsg{Key: "y"})
	if app.load == nil || app.TopOverlay() != OverlayNone {
		t.Fatalf("y did not start a load: the approve calls must run off the key path")
	}
	app = settleApp(t, app)
	if len(calls) != 1 || len(calls[0].Held) != 2 || calls[0].Held[1].ID != 9411 {
		t.Fatalf("approve call = %+v, want one approve of #9412 and #9411", calls)
	}
	if v := screenText(app); app.TopOverlay() != OverlayNone || !strings.Contains(v, "2 runs approved") {
		t.Fatalf("after y: overlay=%s, want the success toast:\n%s", app.TopOverlay(), v)
	}
}

// Without write access A says a maintainer has to approve: no confirm,
// no call.
func TestApproveWithoutWriteAccessSaysWhoCan(t *testing.T) {
	var calls []ActionRequest
	app := verdictApp(verdictAppOptions{runs: []model.Run{heldRun(9412, "Unit tests")}, handler: approveRecorder(&calls), canPush: false})
	app = press(t, app, "A")
	if app.TopOverlay() != OverlayNone || len(calls) != 0 || !strings.Contains(screenText(app), "a maintainer has to approve this") {
		t.Fatalf("overlay=%s calls=%d\n%s", app.TopOverlay(), len(calls), screenText(app))
	}
}

// The same flow from the detail view: A, then y, approves and returns
// to the list. Keys that act on a job toast instead of acting.
func TestApproveHeldForkRunFromDetail(t *testing.T) {
	var calls []ActionRequest
	app := verdictApp(verdictAppOptions{runs: []model.Run{heldRun(9412, "Unit tests"), heldRun(9411, "Lint")}, handler: approveRecorder(&calls), canPush: true})
	app = press(t, app, "enter")
	v := screenText(app)
	if app.Route() != RouteDetail || !strings.Contains(v, "A approve and run") {
		t.Fatalf("detail does not offer A: route=%s\n%s", app.Route(), v)
	}
	if !strings.Contains(v, "o browser · y copy URL · ⎋ back · ?") || strings.Contains(v, "rerun job") {
		t.Fatalf("detail footer still offers keys that cannot help:\n%s", v)
	}
	for _, key := range []string{"r", "R", "x", "X", "w", "l", "enter"} {
		app = press(t, app, key)
		if app.Route() != RouteDetail || app.TopOverlay() != OverlayNone || len(calls) != 0 {
			t.Fatalf("%s on a held run's detail acted: route=%s overlay=%s calls=%d", key, app.Route(), app.TopOverlay(), len(calls))
		}
	}
	if !strings.Contains(screenText(app), "no job to open, rerun, cancel or watch") {
		t.Fatalf("hidden keys do not say why:\n%s", screenText(app))
	}
	app = press(t, app, "A")
	app = press(t, app, "y")
	if len(calls) != 1 || len(calls[0].Held) != 2 || app.Route() != RouteRuns {
		t.Fatalf("y from detail: calls=%+v route=%s, want one approve and the list", calls, app.Route())
	}
}

// A failed detail load leaves no jobs on screen; that proves nothing
// about whether a job ran, so the footer keeps the run's keys.
func TestFailedDetailLoadIsNoNeverRanProof(t *testing.T) {
	red := model.Run{ID: 7, RunNumber: 7, Name: "Unit tests", Event: "pull_request", FromFork: true, HeadBranch: "main",
		Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunAttempt: 1}
	app := verdictApp(verdictAppOptions{runs: []model.Run{red}, jobsErr: errors.New("502 bad gateway")})
	app = press(t, app, "enter")
	if _, never := app.detailNeverRan(); never || app.footerScreen() == "detail_never_ran" {
		t.Fatalf("a failed load reads as never ran (footer %s)", app.footerScreen())
	}
	// Loaded and empty, the same run never ran.
	app = verdictApp(verdictAppOptions{runs: []model.Run{red}})
	app = press(t, app, "enter")
	if verdict, never := app.detailNeverRan(); !never || verdict.Reason != model.ReasonApprovalMissed {
		t.Fatalf("a loaded empty listing: verdict=%+v never=%v", verdict, never)
	}
}

// A held run that is not from a fork (a bot or agent PR) is approved on
// the web: no confirm, no API call, a toast that says so.
func TestApproveHeldBotRunPointsToTheWeb(t *testing.T) {
	bot := heldRun(9412, "Unit tests")
	bot.FromFork, bot.HeadRepository = false, ""
	var calls []ActionRequest
	app := verdictApp(verdictAppOptions{runs: []model.Run{bot}, handler: approveRecorder(&calls), canPush: true})
	app = press(t, app, "A")
	if app.TopOverlay() != OverlayNone || len(calls) != 0 || !strings.Contains(screenText(app), "approve this one on GitHub") {
		t.Fatalf("overlay=%s calls=%d\n%s", app.TopOverlay(), len(calls), screenText(app))
	}
}

// Rows the run fields cannot settle are classified off the key path;
// a failed read keeps the row failing and warns once. The filter finds
// a row by the verdict words it shows.
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
	app := verdictApp(verdictAppOptions{runs: []model.Run{missed, broken}, resolver: resolver})
	app, _ = app.Refresh()
	if !strings.Contains(screenText(app), "2 failing") {
		t.Fatalf("unread red rows do not count as failing:\n%s", screenText(app))
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for app.verdictFetch != nil && time.Now().Before(deadline) {
		app, _ = app.Refresh()
		time.Sleep(10 * time.Millisecond)
	}
	for _, want := range []string{"1 failing", "1 never ran", "missed", "could not read why a run is red"} {
		if !strings.Contains(screenText(app), want) {
			t.Fatalf("view lacks %q:\n%s", want, screenText(app))
		}
	}
	for _, key := range []string{"/", "m", "i", "s", "s", "e", "d", "enter"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	if rows := app.runs.FilteredRuns(); len(rows) != 1 || rows[0].ID != missed.ID {
		t.Fatalf("/missed matched %d rows, want #7 only", len(rows))
	}
}
