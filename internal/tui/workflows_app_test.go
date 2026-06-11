package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func kennelWorkflows() []model.Workflow {
	return []model.Workflow{
		{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/ci.yml"},
		{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
		{ID: 126, Name: "Fork Gate", Path: ".github/workflows/fork-gate.yml", State: model.WorkflowStateDisabledFork},
	}
}

func kennelReadyApp(t *testing.T, fetches *atomic.Int32) App {
	t.Helper()
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.workflowsResolver = func(context.Context) ([]model.Workflow, error) {
		if fetches != nil {
			fetches.Add(1)
		}
		return kennelWorkflows(), nil
	}
	return app
}

func paletteJumpToWorkflows(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"p", "a", "c", "k"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	return app
}

func TestPaletteWorkflowsOpensKennelAsync(t *testing.T) {
	app := paletteJumpToWorkflows(t, kennelReadyApp(t, nil))
	if app.Route() != RouteWorkflows {
		t.Fatalf("route = %q, want workflows", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindWorkflows {
		t.Fatal("palette workflows must start the shared async load")
	}
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("workflows load never settled")
	}
	view := ansi.Strip(app.ViewSize(100, 30))
	for _, want := range []string{"the pack", "✔ active", "◌ asleep", "⊘ fork-disabled"} {
		if !strings.Contains(view, want) {
			t.Fatalf("pack screen missing %q:\n%s", want, view)
		}
	}
}

// The Task 220 invariant: the keystroke that opens the pack must not
// block on the resolver.
func TestWorkflowsKeystrokeNeverBlocks(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	release := make(chan struct{})
	app.workflowsResolver = func(context.Context) ([]model.Workflow, error) {
		<-release
		return kennelWorkflows(), nil
	}
	started := time.Now()
	app = paletteJumpToWorkflows(t, app)
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("workflows open blocked the keystroke for %v", elapsed)
	}
	if app.load == nil {
		t.Fatal("no pending load registered")
	}
	close(release)
	if _, ok := app.SettleLoads(time.Second); !ok {
		t.Fatal("workflows load never settled after release")
	}
}

func TestWorkflowsToggleIsConfirmGatedFlipsStateLocally(t *testing.T) {
	var fetches atomic.Int32
	var requests []ActionRequest
	app := kennelReadyApp(t, &fetches)
	app.actionHandler = func(request ActionRequest) (usecase.ActionResult, error) {
		requests = append(requests, request)
		return usecase.ActionResult{Action: request.Action, WorkflowID: request.Workflow.ID, Message: "muzzled."}, nil
	}
	app = paletteJumpToWorkflows(t, app)
	app, _ = app.SettleLoads(time.Second)

	// e on the active CI: confirm gate first, nothing fired yet.
	app, _ = app.Update(KeyMsg{Key: "e"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("overlay = %q, want confirm", app.TopOverlay())
	}
	if len(requests) != 0 {
		t.Fatal("toggle fired before confirmation")
	}
	view := ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "muzzle workflow CI") {
		t.Fatalf("confirm prompt missing singular muzzle target:\n%s", view)
	}

	app, _ = app.Update(KeyMsg{Key: "y"})
	if len(requests) != 1 || requests[0].Action != usecase.ActionDisableWorkflow {
		t.Fatalf("requests = %#v, want one disable_workflow", requests)
	}
	if requests[0].Workflow.ID != ".github/workflows/ci.yml" {
		t.Fatalf("toggle identifier = %q, want the workflow path", requests[0].Workflow.ID)
	}
	view = ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "muzzled.") {
		t.Fatalf("toast missing hound voice:\n%s", view)
	}
	// State flips locally — derived, never re-fetched.
	if !strings.Contains(view, "⊘ muzzled") {
		t.Fatalf("CI badge did not flip to muzzled:\n%s", view)
	}
	if fetches.Load() != 1 {
		t.Fatalf("workflow fetches = %d, want 1 (no refetch after toggle)", fetches.Load())
	}
}

func TestWorkflowsWakePromptAndToast(t *testing.T) {
	app := kennelReadyApp(t, nil)
	app.actionHandler = func(request ActionRequest) (usecase.ActionResult, error) {
		return usecase.ActionResult{Action: request.Action, WorkflowID: request.Workflow.ID, Message: "back on duty."}, nil
	}
	app = paletteJumpToWorkflows(t, app)
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, _ = app.Update(KeyMsg{Key: "e"})
	view := ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "wake workflow Nightly Sweep") {
		t.Fatalf("confirm prompt missing wake target:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	view = ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "back on duty.") {
		t.Fatalf("toast missing hound voice:\n%s", view)
	}
	if !strings.Contains(view, "✔ active") {
		t.Fatalf("Nightly Sweep badge did not flip to active:\n%s", view)
	}
}

func TestWorkflowsRefusedToggleKeepsStateAndOffersRetry(t *testing.T) {
	app := kennelReadyApp(t, nil)
	app.actionHandler = func(ActionRequest) (usecase.ActionResult, error) {
		return usecase.ActionResult{}, usecase.ActionError{Kind: usecase.ActionErrorPermission, Message: "not your leash"}
	}
	app = paletteJumpToWorkflows(t, app)
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "e"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	view := ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "✔ active") {
		t.Fatalf("refused toggle must not flip the badge:\n%s", view)
	}
}

func TestWorkflowsNonToggleableStatesNeverConfirm(t *testing.T) {
	app := kennelReadyApp(t, nil)
	app = paletteJumpToWorkflows(t, app)
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "G"})
	app, _ = app.Update(KeyMsg{Key: "e"})
	if app.TopOverlay() == OverlayConfirm {
		t.Fatal("fork-disabled workflow must show the why-line, not the confirm")
	}
	view := ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "the fork holds this leash") {
		t.Fatalf("why-line missing for fork-disabled:\n%s", view)
	}
}

func TestWorkflowsEscReturnsToRuns(t *testing.T) {
	app := paletteJumpToWorkflows(t, kennelReadyApp(t, nil))
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteRuns {
		t.Fatalf("route after esc = %q, want runs", app.Route())
	}
}

func TestWorkflowsResolverErrorBecomesRouteError(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.workflowsResolver = func(context.Context) ([]model.Workflow, error) {
		return nil, usecase.APIError{Kind: usecase.APIErrorRateLimit, Message: "API rate limit exceeded"}
	}
	app = paletteJumpToWorkflows(t, app)
	app, _ = app.SettleLoads(time.Second)
	view := ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "workflows unavailable") {
		t.Fatalf("route error not surfaced:\n%s", view)
	}
}

// The dispatch picker badges non-active workflows and refuses to open
// the form for them: dispatching a muzzled workflow is a doomed 422.
func TestDispatchPickerBadgesAndRefusesNonActiveWorkflows(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.dispatchWorkflowsResolver = func(context.Context) ([]dispatch.Workflow, error) {
		return []dispatch.Workflow{
			{Name: "CI", ID: "ci.yml", Ref: "main", State: model.WorkflowStateActive},
			{Name: "Nightly Sweep", ID: "nightly.yml", Ref: "main", State: model.WorkflowStateDisabledInactivity},
		}, nil
	}
	app, _ = app.Update(KeyMsg{Key: "D"})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("dispatch workflows load never settled")
	}
	if app.TopOverlay() != OverlayPalette {
		t.Fatalf("overlay = %q, want the picker palette", app.TopOverlay())
	}
	view := ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "◌ asleep") {
		t.Fatalf("picker missing the asleep badge:\n%s", view)
	}

	// Pick the asleep workflow: refusal toast, no dispatch form.
	app, _ = app.Update(KeyMsg{Key: "down"})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.Route() == RouteDispatch {
		t.Fatal("asleep workflow must not open the dispatch form")
	}
	view = ansi.Strip(app.ViewSize(100, 30))
	if !strings.Contains(view, "asleep") || !strings.Contains(view, "workflows") {
		t.Fatalf("refusal toast must point at the pack roster:\n%s", view)
	}
}

func TestWorkflowsFixturesRender(t *testing.T) {
	roster := RenderFixtureSize("workflows", 80, 24)
	for _, want := range []string{"the pack", "✔ active", "◌ asleep", "⊘ muzzled", "⊘ fork-disabled", "✗ deleted", "e wake/muzzle"} {
		if !strings.Contains(roster, want) {
			t.Fatalf("workflows fixture missing %q:\n%s", want, roster)
		}
	}
	picker := RenderFixtureSize("dispatch-picker", 80, 24)
	for _, want := range []string{"jump to…", "dispatch: CI", "dispatch: Nightly Sweep", "◌ asleep", "⊘ muzzled"} {
		if !strings.Contains(picker, want) {
			t.Fatalf("dispatch-picker fixture missing %q:\n%s", want, picker)
		}
	}
}
