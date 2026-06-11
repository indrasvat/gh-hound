package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func runDiffCommand(t *testing.T, runtime commandRuntime, args ...string) (int, string, error) {
	t.Helper()
	var out bytes.Buffer
	if runtime.Stdout == nil {
		runtime.Stdout = &out
	}
	if runtime.Stderr == nil {
		runtime.Stderr = io.Discard
	}
	if runtime.Env == nil {
		runtime.Env = emptyEnv
	}
	cmd := newRootCommandWithRuntime(runtime, testBuildInfo())
	cmd.SetArgs(append([]string{"diff"}, args...))
	code, err := executeCommand(cmd)
	return code, out.String(), err
}

func TestDiffRegressionScenarioLocatesBoundaryAndExitsOne(t *testing.T) {
	var out bytes.Buffer
	code, _, _ := runDiffCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
		"--workflow", "CI", "--no-tui", "--json", "--fake-scenario", "regression")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (boundary located)\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	if decoded["status"] != "located" {
		t.Fatalf("status = %v, want located\n%s", decoded["status"], out.String())
	}
	lastGood := decoded["last_good"].(map[string]any)
	firstBad := decoded["first_bad"].(map[string]any)
	if lastGood["run_number"].(float64) != 572 || firstBad["run_number"].(float64) != 573 {
		t.Fatalf("boundary = #%v → #%v, want #572 → #573", lastGood["run_number"], firstBad["run_number"])
	}
	suspects := decoded["suspect_commits"].([]any)
	if len(suspects) != 2 {
		t.Fatalf("suspects = %d, want 2\n%s", len(suspects), out.String())
	}
	commit := suspects[0].(map[string]any)
	for _, key := range []string{"sha", "author", "message"} {
		if _, ok := commit[key]; !ok {
			t.Fatalf("suspect missing %q", key)
		}
	}
	verdict, _ := decoded["verdict"].(string)
	if !strings.Contains(verdict, "scent picked up") {
		t.Fatalf("verdict = %q, want the hound line", verdict)
	}
	if !strings.Contains(decoded["compare_url"].(string), "/compare/") {
		t.Fatalf("compare_url = %v", decoded["compare_url"])
	}
}

func TestDiffGreenScenarioExitsZero(t *testing.T) {
	var out bytes.Buffer
	code, _, err := runDiffCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
		"--workflow", "CI", "--no-tui", "--json", "--fake-scenario", "green")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want 0\n%s", code, err, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	if decoded["status"] != "green" {
		t.Fatalf("status = %v, want green\n%s", decoded["status"], out.String())
	}
	if _, ok := decoded["first_bad"]; ok {
		t.Fatalf("green verdict must omit first_bad\n%s", out.String())
	}
}

func TestDiffWithoutWorkflowRefusesWithValidationEnvelope(t *testing.T) {
	var out bytes.Buffer
	code, _, _ := runDiffCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
		"--no-tui", "--json", "--fake-scenario", "regression")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "validation" {
		t.Fatalf("error.kind = %v, want validation\n%s", errObj["kind"], out.String())
	}
}

func TestDiffAPIErrorScenarioWritesTypedEnvelopeExitTwo(t *testing.T) {
	var out bytes.Buffer
	code, _, _ := runDiffCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
		"--workflow", "CI", "--no-tui", "--json", "--fake-scenario", "api_error")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "network" {
		t.Fatalf("error.kind = %v, want network\n%s", errObj["kind"], out.String())
	}
	if decoded["status"] != "error" {
		t.Fatalf("status = %v, want error", decoded["status"])
	}
}

// diffGitHub layers the regression capabilities over the cliGitHub
// stub so the live wiring (workflow resolution, branch scoping) is
// testable without a network.
type diffGitHub struct {
	cliGitHub
	historyPages map[int][]model.Run
	gotWorkflow  string
	gotFilters   []usecase.RunFilter
	rangeInfo    model.CommitRange
	gotBase      string
	gotHead      string
}

func (g *diffGitHub) ListWorkflowRuns(_ context.Context, _ string, workflow string, filter usecase.RunFilter) ([]model.Run, error) {
	g.gotWorkflow = workflow
	g.gotFilters = append(g.gotFilters, filter)
	return g.historyPages[filter.Page], nil
}

func (g *diffGitHub) CompareCommits(_ context.Context, _ string, base, head string) (model.CommitRange, error) {
	g.gotBase = base
	g.gotHead = head
	return g.rangeInfo, nil
}

func TestDiffResolvesWorkflowNameThroughListWorkflows(t *testing.T) {
	github := &diffGitHub{
		cliGitHub: cliGitHub{workflows: []model.Workflow{
			{ID: 290736476, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
			{ID: 292461007, Name: "Release", Path: ".github/workflows/release.yml", State: "active"},
		}},
		historyPages: map[int][]model.Run{1: {
			{ID: 2, RunNumber: 12, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, HeadSHA: "bad"},
			{ID: 1, RunNumber: 11, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "good"},
		}},
		rangeInfo: model.CommitRange{TotalCommits: 1, Commits: []model.Commit{{SHA: "abc", Author: "x", Message: "m"}}},
	}
	var out bytes.Buffer
	code, _, _ := runDiffCommand(t, commandRuntime{
		Stdout: &out,
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, "--workflow", "ci", "--no-tui", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1\n%s", code, out.String())
	}
	if github.gotWorkflow != "ci.yml" {
		t.Fatalf("workflow selector = %q, want resolved file name ci.yml", github.gotWorkflow)
	}
	if len(github.gotFilters) == 0 || github.gotFilters[0].Branch != "main" {
		t.Fatalf("history filter = %+v, want branch main", github.gotFilters)
	}
	if github.gotBase != "good" || github.gotHead != "bad" {
		t.Fatalf("compare = %s...%s", github.gotBase, github.gotHead)
	}
}

// TestTUIDiffScansSelectedRunsBranchNotLaunchBranch pins the trail's
// anchor (ghent Codex P2): from a repo-wide or all-branches list the
// selected run's head branch must drive the scan, not the branch the
// TUI was launched from.
func TestTUIDiffScansSelectedRunsBranchNotLaunchBranch(t *testing.T) {
	foreign := model.Run{
		ID: 7, RunNumber: 70, Name: "CI",
		Path:   ".github/workflows/ci.yml",
		Status: model.StatusCompleted, Conclusion: model.ConclusionFailure,
		HeadBranch: "feat/elsewhere", HeadSHA: "bad",
	}
	github := &diffGitHub{
		cliGitHub: cliGitHub{runs: []model.Run{foreign}},
		historyPages: map[int][]model.Run{1: {
			foreign,
			{ID: 6, RunNumber: 69, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "feat/elsewhere", HeadSHA: "good"},
		}},
		rangeInfo: model.CommitRange{TotalCommits: 1, Commits: []model.Commit{{SHA: "abc", Author: "x", Message: "m"}}},
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
	for _, key := range []string{"d", "i", "f", "f"} {
		app, _ = app.Update(tui.KeyMsg{Key: key})
	}
	app, _ = app.Update(tui.KeyMsg{Key: "enter"})
	_, settled := app.SettleLoads(2 * time.Second)
	if !settled {
		t.Fatal("trail load did not settle")
	}
	if len(github.gotFilters) == 0 {
		t.Fatal("trail never hit the workflow history port")
	}
	for _, filter := range github.gotFilters {
		if filter.Branch != "feat/elsewhere" {
			t.Fatalf("history filter branch = %q, want the selected run's feat/elsewhere", filter.Branch)
		}
	}
}

func TestDiffUnknownWorkflowRefusesNotFound(t *testing.T) {
	github := &diffGitHub{
		cliGitHub: cliGitHub{workflows: []model.Workflow{
			{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
		}},
	}
	var out bytes.Buffer
	code, _, _ := runDiffCommand(t, commandRuntime{
		Stdout: &out,
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, "--workflow", "ghost", "--no-tui", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "not_found" {
		t.Fatalf("error.kind = %v, want not_found\n%s", errObj["kind"], out.String())
	}
}

func TestDiffFileAndNumericSelectorsBypassWorkflowLookup(t *testing.T) {
	if got := workflowSelectorLiteral("ci.yml"); got != "ci.yml" {
		t.Fatalf("ci.yml literal = %q", got)
	}
	if got := workflowSelectorLiteral(".github/workflows/ci.yml"); got != "ci.yml" {
		t.Fatalf("full path literal = %q, want base name", got)
	}
	if got := workflowSelectorLiteral("290736476"); got != "290736476" {
		t.Fatalf("numeric literal = %q", got)
	}
	if got := workflowSelectorLiteral("CI"); got != "" {
		t.Fatalf("name selector literal = %q, want empty (needs lookup)", got)
	}
}
