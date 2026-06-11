package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func cliWorkflows() []model.Workflow {
	return []model.Workflow{
		{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive},
		{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
		{ID: 125, Name: "Future Hound", Path: ".github/workflows/future.yml", State: "disabled_by_future_rule"},
	}
}

func TestWorkflowsListEmitsStateVerbatimAndExitsZero(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{workflows: cliWorkflows()}
	cmd := newRootCommandWithRuntime(approvalsRuntime(&out, github), testBuildInfo())
	cmd.SetArgs([]string{"workflows", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("workflows list exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		Repo      string `json:"repo"`
		Workflows []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Path  string `json:"path"`
			State string `json:"state"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if decoded.Repo != "indrasvat/gh-hound" || len(decoded.Workflows) != 3 {
		t.Fatalf("workflows envelope = %s", out.String())
	}
	if decoded.Workflows[1].State != "disabled_inactivity" {
		t.Fatalf("state = %q", decoded.Workflows[1].State)
	}
	// Unknown future states pass through verbatim — never rejected.
	if decoded.Workflows[2].State != "disabled_by_future_rule" {
		t.Fatalf("unknown state = %q", decoded.Workflows[2].State)
	}
	// The performance gate: state rides the one workflows fetch.
	if github.listWorkflows != 1 {
		t.Fatalf("list calls = %d, want exactly 1", github.listWorkflows)
	}
}

func TestWorkflowsToggleIsExactlyOneCallAndReportsLandingState(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{workflows: cliWorkflows()}
	cmd := newRootCommandWithRuntime(approvalsRuntime(&out, github), testBuildInfo())
	cmd.SetArgs([]string{"workflows", "--enable", ".github/workflows/nightly.yml", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("enable exit = %d, err = %v\n%s", code, err, out.String())
	}
	if github.listWorkflows != 0 {
		t.Fatalf("toggle made %d list calls, want 0 (one-call budget)", github.listWorkflows)
	}
	if len(github.enableTargets) != 1 || github.enableTargets[0] != ".github/workflows/nightly.yml" {
		t.Fatalf("enable targets = %#v", github.enableTargets)
	}
	var decoded struct {
		Accepted *bool `json:"accepted"`
		Toggled  *struct {
			Target string `json:"target"`
			Action string `json:"action"`
			State  string `json:"state"`
		} `json:"toggled"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if decoded.Accepted == nil || !*decoded.Accepted || decoded.Toggled == nil {
		t.Fatalf("toggle envelope = %s", out.String())
	}
	if decoded.Toggled.Action != "enable" || decoded.Toggled.State != "active" {
		t.Fatalf("toggled = %#v", decoded.Toggled)
	}

	out.Reset()
	github = &cliGitHub{workflows: cliWorkflows()}
	cmd = newRootCommandWithRuntime(approvalsRuntime(&out, github), testBuildInfo())
	cmd.SetArgs([]string{"workflows", "--disable", "123", "--no-tui", "--json"})
	code, err = executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("disable exit = %d, err = %v\n%s", code, err, out.String())
	}
	if len(github.disableTargets) != 1 || github.disableTargets[0] != "123" {
		t.Fatalf("disable targets = %#v", github.disableTargets)
	}
	if !strings.Contains(out.String(), `"state": "disabled_manually"`) {
		t.Fatalf("disable must report the landing state:\n%s", out.String())
	}
}

func TestWorkflowsRefusalsWriteTypedEnvelopes(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		toggleErr error
		kind      string
		field     string
	}{
		{"display name selector", []string{"workflows", "--enable", "CI", "--no-tui", "--json"}, nil, "validation", "workflow"},
		{"both toggles", []string{"workflows", "--enable", "ci.yml", "--disable", "ci.yml", "--no-tui", "--json"}, nil, "validation", "workflow"},
		{"permission refusal", []string{"workflows", "--disable", "ci.yml", "--no-tui", "--json"}, usecase.ActionError{Kind: usecase.ActionErrorPermission, Status: http.StatusForbidden, Message: "Resource not accessible"}, "permission", ""},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		github := &cliGitHub{workflows: cliWorkflows(), toggleErr: tt.toggleErr}
		cmd := newRootCommandWithRuntime(approvalsRuntime(&out, github), testBuildInfo())
		cmd.SetArgs(tt.args)
		code, _ := executeCommand(cmd)
		if code != 2 {
			t.Fatalf("%s exit = %d, want 2\n%s", tt.name, code, out.String())
		}
		var decoded struct {
			Accepted *bool `json:"accepted"`
			Error    *struct {
				Kind  string `json:"kind"`
				Field string `json:"field"`
			} `json:"error"`
		}
		if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
			t.Fatalf("%s: invalid JSON: %v\n%s", tt.name, err, out.String())
		}
		if decoded.Accepted == nil || *decoded.Accepted {
			t.Fatalf("%s: refusal must carry accepted:false\n%s", tt.name, out.String())
		}
		if decoded.Error == nil || decoded.Error.Kind != tt.kind {
			t.Fatalf("%s: error = %v, want kind %s\n%s", tt.name, decoded.Error, tt.kind, out.String())
		}
		if tt.field != "" && decoded.Error.Field != tt.field {
			t.Fatalf("%s: field = %q, want %q", tt.name, decoded.Error.Field, tt.field)
		}
		if len(github.enableTargets)+len(github.disableTargets) != 0 && tt.toggleErr == nil {
			t.Fatalf("%s: validation refusal must not reach the adapter", tt.name)
		}
	}
}

// GET failures stay typed: a list that cannot load still writes the
// envelope with error.kind and exits 2 — never a bare stderr message.
func TestWorkflowsListFailureStaysTyped(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{err: usecase.APIError{Kind: usecase.APIErrorRateLimit, Status: http.StatusForbidden, Message: "API rate limit exceeded"}}
	cmd := newRootCommandWithRuntime(approvalsRuntime(&out, github), testBuildInfo())
	cmd.SetArgs([]string{"workflows", "--no-tui", "--json"})

	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("list failure exit = %d, want 2\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"kind": "rate_limit"`) {
		t.Fatalf("list failure must keep the typed kind:\n%s", out.String())
	}
}

func TestWorkflowsFakeScenarioCoversAllStates(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{Stdout: &out, Stderr: &bytes.Buffer{}, Env: emptyEnv, IsTTY: false}, testBuildInfo())
	cmd.SetArgs([]string{"workflows", "--no-tui", "--json", "--fake-scenario", "green"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("fake workflows exit = %d, err = %v\n%s", code, err, out.String())
	}
	for _, state := range []string{"active", "disabled_manually", "disabled_inactivity", "disabled_fork", "deleted"} {
		if !strings.Contains(out.String(), `"state": "`+state+`"`) {
			t.Fatalf("fake workflows missing state %q:\n%s", state, out.String())
		}
	}

	out.Reset()
	cmd = newRootCommandWithRuntime(commandRuntime{Stdout: &out, Stderr: &bytes.Buffer{}, Env: emptyEnv, IsTTY: false}, testBuildInfo())
	cmd.SetArgs([]string{"workflows", "--no-tui", "--json", "--fake-scenario", "api_error"})
	code, _ = executeCommand(cmd)
	if code != 2 || !strings.Contains(out.String(), `"kind": "network"`) {
		t.Fatalf("api_error scenario: exit = %d\n%s", code, out.String())
	}
}

// The dispatch chooser keeps toggleable disabled workflows visible
// (badged in the picker) but drops fork-disabled/deleted/unknown
// states — they can be neither dispatched nor woken from here.
func TestChooseDispatchWorkflowsKeepsToggleableDisabledStates(t *testing.T) {
	dispatchYAML := "on:\n  workflow_dispatch:\n"
	github := &cliGitHub{
		workflowFiles: map[string]string{
			".github/workflows/ci.yml":      dispatchYAML,
			".github/workflows/nightly.yml": dispatchYAML,
			".github/workflows/stale.yml":   dispatchYAML,
			".github/workflows/fork.yml":    dispatchYAML,
			".github/workflows/old.yml":     dispatchYAML,
			".github/workflows/future.yml":  dispatchYAML,
		},
	}
	workflows := []model.Workflow{
		{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive},
		{ID: 2, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
		{ID: 3, Name: "Stale Patrol", Path: ".github/workflows/stale.yml", State: model.WorkflowStateDisabledManually},
		{ID: 4, Name: "Fork Gate", Path: ".github/workflows/fork.yml", State: model.WorkflowStateDisabledFork},
		{ID: 5, Name: "Old Patrol", Path: ".github/workflows/old.yml", State: model.WorkflowStateDeleted},
		{ID: 6, Name: "Future Hound", Path: ".github/workflows/future.yml", State: "disabled_by_future_rule"},
	}
	chosen, err := chooseDispatchWorkflows(context.Background(), github, "indrasvat/gh-hound", workflows)
	if err != nil {
		t.Fatalf("chooseDispatchWorkflows: %v", err)
	}
	names := make([]string, 0, len(chosen))
	for _, workflow := range chosen {
		names = append(names, workflow.Name)
	}
	want := []string{"CI", "Nightly Sweep", "Stale Patrol"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("chosen = %v, want %v", names, want)
	}
}

// dispatchWorkflowModels must carry State through so the picker can
// badge non-active workflows.
func TestDispatchWorkflowModelsCarryState(t *testing.T) {
	dispatchYAML := "on:\n  workflow_dispatch:\n"
	github := &cliGitHub{
		workflowFiles: map[string]string{
			".github/workflows/ci.yml":      dispatchYAML,
			".github/workflows/nightly.yml": dispatchYAML,
		},
	}
	launch := usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Workflows: []model.Workflow{
			{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive},
			{ID: 2, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
		},
	}
	models, err := dispatchWorkflowModels(context.Background(), github, launch)
	if err != nil {
		t.Fatalf("dispatchWorkflowModels: %v", err)
	}
	if len(models) != 2 || models[1].State != model.WorkflowStateDisabledInactivity {
		t.Fatalf("models = %#v, want Nightly Sweep carrying its state", models)
	}
}

// End-to-end TUI wiring: the live app must resolve the kennel through
// the configured GitHub client and route workflow toggles through the
// shared ActionHandler.
func TestDefaultTUIAppWiresWorkflowsSurface(t *testing.T) {
	github := &cliGitHub{
		runs: []model.Run{{ID: 9001, RunNumber: 571, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, HeadBranch: "main", Event: "push"}},
		workflows: []model.Workflow{
			{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive},
			{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
		},
	}
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
		IsTTY:  true,
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat",
		}},
	}, tui.BuildInfo{Version: "v0.1.0"}, cliOptions{})
	if err != nil {
		t.Fatalf("defaultTUIApp: %v", err)
	}
	app, _ = app.Update(tui.KeyMsg{Key: ":"})
	for _, key := range []string{"k", "e", "n", "n", "e", "l"} {
		app, _ = app.Update(tui.KeyMsg{Key: key})
	}
	app, _ = app.Update(tui.KeyMsg{Key: "enter"})
	app, settled := app.SettleLoads(2 * time.Second)
	if !settled {
		t.Fatal("workflows load did not settle")
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	for _, want := range []string{"the kennel", "◌ asleep", "✔ active"} {
		if !strings.Contains(view, want) {
			t.Fatalf("kennel view missing %q:\n%s", want, view)
		}
	}

	// j to the asleep workflow, e → confirm, y → exactly one PUT.
	app, _ = app.Update(tui.KeyMsg{Key: "j"})
	app, _ = app.Update(tui.KeyMsg{Key: "e"})
	app, _ = app.Update(tui.KeyMsg{Key: "y"})
	if len(github.enableTargets) != 1 || github.enableTargets[0] != ".github/workflows/nightly.yml" {
		t.Fatalf("enable targets = %#v, want the nightly path", github.enableTargets)
	}
	view = ansi.Strip(app.ViewSize(120, 32))
	if !strings.Contains(view, "back on duty.") {
		t.Fatalf("toast missing hound voice:\n%s", view)
	}
}
