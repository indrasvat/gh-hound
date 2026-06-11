package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func waitingTestPending() []model.PendingDeployment {
	return []model.PendingDeployment{
		{
			EnvironmentID:         7301,
			EnvironmentName:       "production",
			WaitTimer:             0,
			CurrentUserCanApprove: true,
			Reviewers:             []model.DeploymentReviewer{{Type: "User", Name: "indrasvat"}},
		},
		{
			EnvironmentID:         7302,
			EnvironmentName:       "staging",
			WaitTimer:             1800,
			CurrentUserCanApprove: false,
			Reviewers:             []model.DeploymentReviewer{{Type: "Team", Name: "deploy-keys"}},
		},
	}
}

func waitingTestApp(t *testing.T) (App, *[]ActionRequest) {
	t.Helper()
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.runs = sampleWaitingRunsModel()
	requests := &[]ActionRequest{}
	app.approvalsResolver = func(_ context.Context, run model.Run) ([]model.PendingDeployment, error) {
		if run.Status != model.StatusWaiting {
			t.Fatalf("approvals resolver called for non-waiting run %#v", run)
		}
		return waitingTestPending(), nil
	}
	app.actionHandler = func(request ActionRequest) (usecase.ActionResult, error) {
		*requests = append(*requests, request)
		message := "gate's open."
		if request.Action == usecase.ActionRejectDeployment {
			message = "gate stays shut."
		}
		return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, Message: message}, nil
	}
	return app, requests
}

func TestRunsListShowsGateBadgeForWaitingRuns(t *testing.T) {
	view := ansi.Strip(runs.View(sampleWaitingRunsModel(), 100, time.Now()))
	if !strings.Contains(view, icons.Gate) {
		t.Fatalf("waiting run must carry the %s gate badge:\n%s", icons.Gate, view)
	}
	if !strings.Contains(view, "A review") {
		t.Fatalf("gated summary must advertise the A key:\n%s", view)
	}
}

func TestApprovalsKeyOpensOverlayThroughStartLoad(t *testing.T) {
	app, _ := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	if app.load == nil {
		t.Fatal("A must fetch the gate list through startLoad")
	}
	if app.load.kind != loadKindApprovals || app.load.label != "checking the gate" {
		t.Fatalf("load = %q %q, want approvals/checking the gate", app.load.kind, app.load.label)
	}
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("approvals load did not settle")
	}
	if app.TopOverlay() != OverlayApprovals {
		t.Fatalf("overlay = %q, want approvals", app.TopOverlay())
	}
	view := ansi.Strip(app.ViewSized(120))
	for _, want := range []string{"production", "staging", "not yours to open", "indrasvat", "deploy-keys"} {
		if !strings.Contains(view, want) {
			t.Fatalf("approvals overlay missing %q:\n%s", want, view)
		}
	}
}

func TestApprovalsKeyOnNonWaitingRunToasts(t *testing.T) {
	app, _ := waitingTestApp(t)
	app.runs = sampleRunsModel() // newest selected run is completed
	app, _ = app.Update(KeyMsg{Key: "A"})
	if app.load != nil {
		t.Fatal("non-waiting run must not fetch approvals")
	}
	if app.TopOverlay() != OverlayNone {
		t.Fatal("non-waiting run must not open the overlay")
	}
	if len(app.toasts.Toasts) == 0 || !strings.Contains(app.toasts.Toasts[0].Title, "no gate") {
		t.Fatalf("expected a no-gate toast, got %#v", app.toasts.Toasts)
	}
}

func TestApprovalsApproveFlowIsConfirmGated(t *testing.T) {
	app, requests := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)

	app, _ = app.Update(KeyMsg{Key: "y"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("approve must be confirm-gated, overlay = %q", app.TopOverlay())
	}
	if !strings.Contains(app.confirm.Message, "open the gate for production?") {
		t.Fatalf("confirm message = %q", app.confirm.Message)
	}
	if len(*requests) != 0 {
		t.Fatal("no mutation may fire before the confirm")
	}

	app, _ = app.Update(KeyMsg{Key: "y"})
	if len(*requests) != 1 {
		t.Fatalf("confirmed approve must call the action handler once, got %d", len(*requests))
	}
	request := (*requests)[0]
	if request.Action != usecase.ActionApproveDeployment {
		t.Fatalf("action = %q", request.Action)
	}
	if len(request.Environments) != 1 || request.Environments[0] != "production" {
		t.Fatalf("environments = %#v", request.Environments)
	}
	if app.TopOverlay() != OverlayNone {
		t.Fatalf("overlay must close after the review, got %q", app.TopOverlay())
	}
	if len(app.toasts.Toasts) == 0 || !strings.Contains(app.toasts.Toasts[len(app.toasts.Toasts)-1].Title, "gate's open.") {
		t.Fatalf("approve toast missing, got %#v", app.toasts.Toasts)
	}
}

func TestApprovalsRejectFlowCarriesCommentAndVoice(t *testing.T) {
	app, requests := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)

	// Type an optional comment first.
	app, _ = app.Update(KeyMsg{Key: "c"})
	for _, key := range []string{"n", "o", "p", "e"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})

	app, _ = app.Update(KeyMsg{Key: "n"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("reject must be confirm-gated, overlay = %q", app.TopOverlay())
	}
	if !strings.Contains(app.confirm.Message, "keep the gate shut for production?") {
		t.Fatalf("confirm message = %q", app.confirm.Message)
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	if len(*requests) != 1 {
		t.Fatalf("confirmed reject must call the handler once, got %d", len(*requests))
	}
	request := (*requests)[0]
	if request.Action != usecase.ActionRejectDeployment || request.Comment != "nope" {
		t.Fatalf("reject request = %#v", request)
	}
	if len(app.toasts.Toasts) == 0 || !strings.Contains(app.toasts.Toasts[len(app.toasts.Toasts)-1].Title, "gate stays shut.") {
		t.Fatalf("reject toast missing, got %#v", app.toasts.Toasts)
	}
}

func TestApprovalsRefusedReviewKeepsOverlayForRetry(t *testing.T) {
	app, _ := waitingTestApp(t)
	calls := 0
	app.actionHandler = func(request ActionRequest) (usecase.ActionResult, error) {
		calls++
		if calls == 1 {
			return usecase.ActionResult{}, usecase.ActionError{Kind: usecase.ActionErrorConflict, Message: "the gate moved"}
		}
		return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, Message: "gate's open."}, nil
	}
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)

	// Refused review: overlay must survive with picks and comment.
	app, _ = app.Update(KeyMsg{Key: "c"})
	app, _ = app.Update(KeyMsg{Key: "g"})
	app, _ = app.Update(KeyMsg{Key: "o"})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	if app.TopOverlay() != OverlayApprovals {
		t.Fatalf("refused review must keep the approvals overlay, got %q", app.TopOverlay())
	}
	if got := app.approvals.Comment; got != "go" {
		t.Fatalf("refused review lost the comment, got %q", got)
	}
	if picked := app.approvals.PickedEnvironments(); len(picked) == 0 {
		t.Fatal("refused review lost the picked environments")
	}

	// The retry succeeds and the overlay closes.
	app, _ = app.Update(KeyMsg{Key: "y"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	if app.TopOverlay() != OverlayNone {
		t.Fatalf("accepted review must close the overlay, got %q", app.TopOverlay())
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}
}

func TestApprovalsCommentEscDiscardsEdit(t *testing.T) {
	app, requests := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)

	// Type a comment but abandon it with esc — the advertised cancel.
	app, _ = app.Update(KeyMsg{Key: "c"})
	for _, key := range []string{"o", "o", "p", "s"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.TopOverlay() != OverlayApprovals {
		t.Fatalf("esc in comment mode must stay on the overlay, got %q", app.TopOverlay())
	}

	// A committed comment survives a later esc-cancelled edit session.
	app, _ = app.Update(KeyMsg{Key: "c"})
	for _, key := range []string{"k", "e", "p", "t"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.Update(KeyMsg{Key: "c"})
	app, _ = app.Update(KeyMsg{Key: "z"})
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if got := app.approvals.Comment; got != "kept" {
		t.Fatalf("esc must restore the last committed comment, got %q", got)
	}

	app, _ = app.Update(KeyMsg{Key: "y"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	if len(*requests) != 1 {
		t.Fatalf("confirmed approve must call the handler once, got %d", len(*requests))
	}
	if comment := (*requests)[0].Comment; comment != "kept" {
		t.Fatalf("review carried %q, want the committed comment with discarded drafts dropped", comment)
	}
}

func TestApprovalsOverlaySpaceRefusesUnapprovableEnvironment(t *testing.T) {
	app, requests := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)

	// Move to the locked environment and try to pick it, then unpick
	// production so nothing approvable is selected.
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, _ = app.Update(KeyMsg{Key: "space"})
	app, _ = app.Update(KeyMsg{Key: "k"})
	app, _ = app.Update(KeyMsg{Key: "space"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	if app.TopOverlay() == OverlayConfirm {
		t.Fatal("empty selection must not reach the confirm")
	}
	if len(*requests) != 0 {
		t.Fatal("no mutation may fire with nothing picked")
	}
}

func TestApprovalsOverlayEscCloses(t *testing.T) {
	app, _ := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.TopOverlay() != OverlayNone {
		t.Fatalf("esc must close the approvals overlay, got %q", app.TopOverlay())
	}
}

func TestApprovalsConfirmEscReturnsToOverlay(t *testing.T) {
	app, requests := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "y"})
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.TopOverlay() != OverlayApprovals {
		t.Fatalf("cancelling the confirm must return to the overlay, got %q", app.TopOverlay())
	}
	if len(*requests) != 0 {
		t.Fatal("cancelled confirm must not mutate")
	}
}

func TestApprovalsPaletteEntryOpensGate(t *testing.T) {
	app, _ := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"a", "p", "p", "r"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("palette approvals load did not settle")
	}
	if app.TopOverlay() != OverlayApprovals {
		t.Fatalf("palette approvals must open the overlay, got %q", app.TopOverlay())
	}
}

func TestDetailRendersPendingEnvironmentsPanelForWaitingRun(t *testing.T) {
	run := sampleWaitingRunsModel().Context.Runs[0]
	m := DetailModelForRun(run).WithRepo("indrasvat/gh-hound").WithPendingDeployments(waitingTestPending())
	view := ansi.Strip(detail.ViewSize(m, 120, 40))
	for _, want := range []string{"Deploy gate (2)", "production", "staging", "not yours to open"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail pending panel missing %q:\n%s", want, view)
		}
	}
}

func TestApprovalsFixturesRender(t *testing.T) {
	for _, screen := range []string{"runs-waiting", "approvals", "detail-pending"} {
		view := ansi.Strip(RenderFixtureSize(screen, 120, 40))
		if strings.TrimSpace(view) == "" {
			t.Fatalf("fixture %s rendered empty", screen)
		}
		if !strings.Contains(view, "production") {
			t.Fatalf("fixture %s missing gate content:\n%s", screen, view)
		}
	}
	badge := ansi.Strip(RenderFixtureSize("runs-waiting", 120, 40))
	if !strings.Contains(badge, icons.Gate) {
		t.Fatalf("runs-waiting fixture missing gate badge:\n%s", badge)
	}
}

func TestApprovalsOverlayFooterIsTruthful(t *testing.T) {
	app, _ := waitingTestApp(t)
	app, _ = app.Update(KeyMsg{Key: "A"})
	app, _ = app.SettleLoads(time.Second)
	view := ansi.Strip(app.ViewSize(120, 40))
	for _, want := range []string{"space pick", "y open gate", "n keep shut", "c comment"} {
		if !strings.Contains(view, want) {
			t.Fatalf("approvals footer missing %q:\n%s", want, view)
		}
	}
}
