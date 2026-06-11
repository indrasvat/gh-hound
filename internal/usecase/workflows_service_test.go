package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type workflowsGitHub struct {
	*launchGitHub
	listCalls    int
	enableCalls  []string
	disableCalls []string
}

func (g *workflowsGitHub) ListWorkflows(ctx context.Context, repo string) ([]model.Workflow, error) {
	g.listCalls++
	return g.launchGitHub.ListWorkflows(ctx, repo)
}

func (g *workflowsGitHub) EnableWorkflow(_ context.Context, _ string, workflowID string) (usecase.ActionResult, error) {
	g.enableCalls = append(g.enableCalls, workflowID)
	return usecase.ActionResult{Action: usecase.ActionEnableWorkflow, Repo: "indrasvat/gh-hound", WorkflowID: workflowID, Message: "Workflow enabled"}, nil
}

func (g *workflowsGitHub) DisableWorkflow(_ context.Context, _ string, workflowID string) (usecase.ActionResult, error) {
	g.disableCalls = append(g.disableCalls, workflowID)
	return usecase.ActionResult{Action: usecase.ActionDisableWorkflow, Repo: "indrasvat/gh-hound", WorkflowID: workflowID, Message: "Workflow disabled"}, nil
}

func TestWorkflowsServiceListsStateUnchanged(t *testing.T) {
	github := &workflowsGitHub{launchGitHub: &launchGitHub{workflows: []model.Workflow{
		{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive},
		{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
	}}}
	service := usecase.WorkflowsService{GitHub: github}
	workflows, err := service.List(context.Background(), "indrasvat/gh-hound")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(workflows) != 2 || workflows[1].State != model.WorkflowStateDisabledInactivity {
		t.Fatalf("workflows = %#v", workflows)
	}
	if github.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1 (state rides the existing fetch)", github.listCalls)
	}
}

// The hard gate: a toggle is exactly one API call — no list lookup, no
// state re-fetch. The selector goes to the API as given.
func TestWorkflowsServiceToggleIsExactlyOneCallWithHoundVoice(t *testing.T) {
	github := &workflowsGitHub{launchGitHub: &launchGitHub{}}
	clock := &fakeClock{now: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)}
	service := usecase.WorkflowsService{
		GitHub:  github,
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second, Clock: clock},
	}

	enabled, err := service.Enable(context.Background(), "indrasvat/gh-hound", "ci.yml")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if enabled.Message != "back on duty." {
		t.Fatalf("enable message = %q, want %q", enabled.Message, "back on duty.")
	}
	disabled, err := service.Disable(context.Background(), "indrasvat/gh-hound", "290736476")
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if disabled.Message != "muzzled." {
		t.Fatalf("disable message = %q, want %q", disabled.Message, "muzzled.")
	}

	if github.listCalls != 0 {
		t.Fatalf("toggle made %d list calls, want 0", github.listCalls)
	}
	if len(github.enableCalls) != 1 || github.enableCalls[0] != "ci.yml" {
		t.Fatalf("enable calls = %#v", github.enableCalls)
	}
	if len(github.disableCalls) != 1 || github.disableCalls[0] != "290736476" {
		t.Fatalf("disable calls = %#v", github.disableCalls)
	}
	if clock.slept <= 0 {
		t.Fatalf("second toggle skipped the shared mutation limiter")
	}
}

// Pipe contract: the toggle selector is an id or workflow file path
// ONLY — what the API accepts. Display names refuse as validation
// before any API call.
func TestWorkflowsServiceRefusesNonAPISelectors(t *testing.T) {
	github := &workflowsGitHub{launchGitHub: &launchGitHub{}}
	service := usecase.WorkflowsService{GitHub: github}

	accepted := []string{"ci.yml", "290736476", ".github/workflows/ci.yml", "deploy.yaml"}
	for _, target := range accepted {
		if _, err := service.Enable(context.Background(), "indrasvat/gh-hound", target); err != nil {
			t.Fatalf("Enable(%q) refused: %v", target, err)
		}
	}
	// Zero and negative IDs refuse too (codex review: they parsed as
	// numeric and burned a real mutation call).
	refused := []string{"", "  ", "CI", "Nightly Sweep", "ci.txt", "0", "-1"}
	for _, target := range refused {
		_, err := service.Enable(context.Background(), "indrasvat/gh-hound", target)
		actionErr, ok := usecase.AsActionError(err)
		if !ok {
			t.Fatalf("Enable(%q) error = %v, want typed validation", target, err)
		}
		if actionErr.Kind != usecase.ActionErrorValidation || actionErr.Field != "workflow" {
			t.Fatalf("Enable(%q) = %#v, want validation on field workflow", target, actionErr)
		}
	}
	if got, want := len(github.enableCalls), len(accepted); got != want {
		t.Fatalf("enable calls = %d, want %d (refusals must not burn a write)", got, want)
	}
}
