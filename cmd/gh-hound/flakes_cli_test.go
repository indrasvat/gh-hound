package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func runFlakesCommand(t *testing.T, runtime commandRuntime, args ...string) (int, string, error) {
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
	cmd.SetArgs(append([]string{"flakes"}, args...))
	code, err := executeCommand(cmd)
	return code, out.String(), err
}

func TestFlakesFlakyScenarioScoresTheSquirrelAndExitsOne(t *testing.T) {
	var out bytes.Buffer
	code, _, _ := runFlakesCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
		"--workflow", "CI", "--no-tui", "--json", "--fake-scenario", "flaky")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (action needed)\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	if decoded["status"] != "flaky" {
		t.Fatalf("status = %v, want flaky\n%s", decoded["status"], out.String())
	}
	jobs := decoded["jobs"].([]any)
	if len(jobs) == 0 {
		t.Fatalf("jobs empty\n%s", out.String())
	}
	job := jobs[0].(map[string]any)
	if job["job"] != "build" {
		t.Fatalf("worst job = %v, want build\n%s", job["job"], out.String())
	}
	if job["attempt_flips"].(float64) < 2 {
		t.Fatalf("attempt_flips = %v, want >= 2", job["attempt_flips"])
	}
	kinds := map[string]bool{}
	for _, item := range job["evidence"].([]any) {
		kinds[item.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["attempt_flip"] || !kinds["retry_mask"] {
		t.Fatalf("evidence kinds = %v, want attempt_flip and retry_mask", kinds)
	}
	verdict, _ := decoded["verdict"].(string)
	if !strings.Contains(verdict, "squirrel") {
		t.Fatalf("verdict = %q, want the squirrel call", verdict)
	}
	signals := decoded["signals_evaluated"].([]any)
	if len(signals) != 3 {
		t.Fatalf("signals_evaluated = %v, want all three", signals)
	}
}

// The green fixture only has three runs on record: too thin for a
// clean verdict, so the honest answer is insufficient_data — exit 0.
func TestFlakesGreenScenarioIsInsufficientDataExitZero(t *testing.T) {
	var out bytes.Buffer
	code, _, err := runFlakesCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
		"--workflow", "CI", "--no-tui", "--json", "--fake-scenario", "green")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want 0\n%s", code, err, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	if decoded["status"] != "insufficient_data" {
		t.Fatalf("status = %v, want insufficient_data\n%s", decoded["status"], out.String())
	}
}

func TestFlakesAPIErrorScenarioWritesTypedEnvelopeExitTwo(t *testing.T) {
	var out bytes.Buffer
	code, _, _ := runFlakesCommand(t, commandRuntime{Stdout: &out, IsTTY: true},
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

// flakesGitHub layers history + attempt forensics over the cliGitHub
// stub so live wiring is testable without a network.
type flakesGitHub struct {
	cliGitHub
	historyPages    map[int][]model.Run
	gotWorkflow     string
	gotFilters      []usecase.RunFilter
	attemptJobsByID map[string][]model.Job
	attemptCalls    int
}

func (g *flakesGitHub) ListWorkflowRuns(_ context.Context, _ string, workflow string, filter usecase.RunFilter) ([]model.Run, error) {
	g.gotWorkflow = workflow
	g.gotFilters = append(g.gotFilters, filter)
	return g.historyPages[filter.Page], nil
}

func (g *flakesGitHub) ListJobsForAttempt(_ context.Context, _ string, runID int64, attempt int) ([]model.Job, error) {
	g.attemptCalls++
	return g.attemptJobsByID[fmt.Sprintf("%d/%d", runID, attempt)], nil
}

func TestFlakesWithoutWorkflowFollowsTheLatestRun(t *testing.T) {
	github := &flakesGitHub{
		cliGitHub: cliGitHub{runs: []model.Run{{
			ID: 9, RunNumber: 90, Name: "CI",
			Path:   ".github/workflows/ci.yml",
			Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess,
			HeadBranch: "main", HeadSHA: "headsha",
		}}},
		historyPages: map[int][]model.Run{1: {
			{ID: 9, RunNumber: 90, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "s9", RunAttempt: 1},
			{ID: 8, RunNumber: 89, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "s8", RunAttempt: 1},
			{ID: 7, RunNumber: 88, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "s7", RunAttempt: 1},
			{ID: 6, RunNumber: 87, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "s6", RunAttempt: 1},
			{ID: 5, RunNumber: 86, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "s5", RunAttempt: 1},
		}},
	}
	var out bytes.Buffer
	code, _, err := runFlakesCommand(t, commandRuntime{
		Stdout: &out,
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, "--no-tui", "--json")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want 0\n%s", code, err, out.String())
	}
	if github.gotWorkflow != "ci.yml" {
		t.Fatalf("workflow selector = %q, want ci.yml derived from the latest run", github.gotWorkflow)
	}
	decoded := decodeJSON(t, out.Bytes())
	if decoded["status"] != "clean" {
		t.Fatalf("status = %v, want clean\n%s", decoded["status"], out.String())
	}
	verdict, _ := decoded["verdict"].(string)
	if !strings.Contains(verdict, "fresh scent") {
		t.Fatalf("verdict = %q, want fresh scent", verdict)
	}
}

func TestFlakesUnknownWorkflowRefusesNotFound(t *testing.T) {
	github := &flakesGitHub{
		cliGitHub: cliGitHub{workflows: []model.Workflow{
			{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
		}},
	}
	var out bytes.Buffer
	code, _, _ := runFlakesCommand(t, commandRuntime{
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

func TestFlakesWithoutAnyRunsRefusesValidation(t *testing.T) {
	github := &flakesGitHub{historyPages: map[int][]model.Run{}}
	var out bytes.Buffer
	code, _, _ := runFlakesCommand(t, commandRuntime{
		Stdout: &out,
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, "--no-tui", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "validation" {
		t.Fatalf("error.kind = %v, want validation\n%s", errObj["kind"], out.String())
	}
}

// HOUND_FLAKE_WINDOW drives the scan window from the environment, same
// as the config file knob.
func TestFlakesHonorsFlakeWindowEnv(t *testing.T) {
	github := &flakesGitHub{
		historyPages: map[int][]model.Run{1: {
			{ID: 9, RunNumber: 90, Name: "CI", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadSHA: "s9", RunAttempt: 1},
		}},
	}
	var out bytes.Buffer
	code, _, _ := runFlakesCommand(t, commandRuntime{
		Stdout: &out,
		IsTTY:  true,
		GitHub: github,
		Env:    mapEnv(map[string]string{"HOUND_FLAKE_WINDOW": "7"}),
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, "--workflow", "ci.yml", "--no-tui", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	if decoded["window"].(float64) != 7 {
		t.Fatalf("window = %v, want 7 from HOUND_FLAKE_WINDOW", decoded["window"])
	}
}
