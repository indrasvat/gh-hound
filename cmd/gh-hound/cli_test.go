package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestHelpListsEnvVarsForFlags(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"--help"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("help exit = %d, err = %v", code, err)
	}
	for _, want := range []string{"HOUND_NO_TUI", "HOUND_FORMAT", "GH_REPO", "HOUND_REPO", "HOUND_LOG_LEVEL", "HOUND_TRACE_HTTP"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q\n%s", want, out.String())
		}
	}
}

func TestRunsNoTUIJSONUsesEnvOverrides(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env: mapEnv(map[string]string{
			"HOUND_REPO":          "indrasvat/gh-ghent",
			"HOUND_BRANCH":        "fix/parser",
			"HOUND_STATUS":        "failure",
			"HOUND_NO_TUI":        "true",
			"HOUND_FORMAT":        "json",
			"HOUND_FAKE_SCENARIO": "failure",
		}),
		IsTTY: true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs"})

	code, err := executeCommand(cmd)
	if err == nil {
		t.Fatalf("runs failure should return action-needed outcome")
	}
	if code != 1 {
		t.Fatalf("runs failure exit = %d, want 1", code)
	}
	var decoded map[string]any
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded["repo"] != "indrasvat/gh-ghent" || decoded["branch"] != "fix/parser" {
		t.Fatalf("env overrides not reflected: %#v", decoded)
	}
}

func TestPipeDetectionDefaultsToStructuredOutput(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: &cliGitHub{runs: []model.Run{cliRun(908, "CI", model.StatusCompleted, model.ConclusionSuccess)}},
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
		}},
	}, testBuildInfo())
	cmd.SetArgs([]string{})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("pipe root exit = %d, err = %v", code, err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("pipe root did not render structured output:\n%s", out.String())
	}
}

func TestTTYRootLaunchesInteractiveTUI(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Stdin:  strings.NewReader("q"),
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("tty root code=%d err=%v out=%s", code, err, out.String())
	}
	got := out.String()
	if strings.Contains(got, "TUI scaffold is ready") {
		t.Fatalf("tty root printed scaffold placeholder:\n%s", got)
	}
	for _, want := range []string{"██╗  ██╗ ██████╗", "Hunt down your GitHub Actions CI", "enter continue · ? help · q quit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tty root missing %q\n%s", want, got)
		}
	}
}

func TestTTYViewUsesCRLFWithoutTrailingScroll(t *testing.T) {
	got := ttyView("one\ntwo\n")
	if got != "one\r\ntwo" {
		t.Fatalf("ttyView = %q, want CRLF without trailing newline", got)
	}
}

func TestScreenFixtureDoesNotEmitTrailingScrollLine(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  false,
	}, testBuildInfo())
	cmd.SetArgs([]string{"__screen", "--screen", "welcome", "--width", "80", "--height", "24"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("__screen code=%d err=%v", code, err)
	}
	if strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("__screen emitted trailing newline that can scroll exact-height captures")
	}
	if lines := strings.Count(out.String(), "\n") + 1; lines != 24 {
		t.Fatalf("__screen rendered %d lines, want 24", lines)
	}
}

func TestLaunchFlagsRouteRepoAllAndWatch(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env: mapEnv(map[string]string{
			"GH_REPO": "indrasvat/env-repo",
		}),
		IsTTY:  false,
		GitHub: &cliGitHub{runs: []model.Run{cliRun(909, "CI", model.StatusCompleted, model.ConclusionSuccess)}},
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/local",
			Branch: "main",
		}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"--all", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("root --all returned code=%d err=%v", code, err)
	}
	got := out.String()
	if !strings.Contains(got, `"repo": "indrasvat/env-repo"`) || strings.Contains(got, `"branch": "main"`) {
		t.Fatalf("root --all did not route repo-wide with GH_REPO:\n%s", got)
	}

	out.Reset()
	cmd = newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: &cliGitHub{runs: []model.Run{cliRun(910, "CI", model.StatusInProgress, model.ConclusionNone)}},
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/local",
			Branch: "main",
		}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "-R", "indrasvat/other", "--json"})
	code, err = executeCommand(cmd)
	if err == nil || code != 3 {
		t.Fatalf("watch returned code=%d err=%v", code, err)
	}
	got = out.String()
	if !strings.Contains(got, `"repo": "indrasvat/other"`) || !strings.Contains(got, `"status": "in_progress"`) {
		t.Fatalf("watch did not route to requested repo and pending run:\n%s", got)
	}
}

func TestNormalJSONPathUsesGitHubAdapter(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{runs: []model.Run{cliRun(777, "Release", model.StatusCompleted, model.ConclusionSuccess)}}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  true,
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/real-repo",
			Branch: "feature/real",
		}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--json"})

	code, err := executeCommand(cmd)
	if code != 0 || err != nil {
		t.Fatalf("runs --json code=%d err=%v out=%s", code, err, out.String())
	}
	if len(github.filters) != 1 {
		t.Fatalf("ListRuns calls = %d, want 1", len(github.filters))
	}
	if got := github.filters[0]; got.Repo != "indrasvat/real-repo" || got.Branch != "feature/real" {
		t.Fatalf("filter = %#v", got)
	}
	decoded := decodeJSON(t, out.Bytes())
	runs := decoded["runs"].([]any)
	run := runs[0].(map[string]any)
	if run["id"] != float64(777) || run["workflow"] != "Release" {
		t.Fatalf("normal path rendered fixture-looking data instead of adapter data:\n%s", out.String())
	}
}

func TestNormalStatusFailureFiltersCompletedRunsByConclusion(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{runs: []model.Run{
		cliRun(777, "Release", model.StatusCompleted, model.ConclusionSuccess),
		cliRun(778, "CI", model.StatusCompleted, model.ConclusionFailure),
	}}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  true,
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/real-repo",
			Branch: "feature/real",
		}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--json", "--status", "failure"})

	code, err := executeCommand(cmd)
	if code != 1 || err == nil {
		t.Fatalf("runs --status failure code=%d err=%v out=%s", code, err, out.String())
	}
	if got := github.filters[0].Status; got != model.StatusCompleted {
		t.Fatalf("GitHub status filter = %q, want completed", got)
	}
	decoded := decodeJSON(t, out.Bytes())
	runs := decoded["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("rendered runs = %d, want one failure\n%s", len(runs), out.String())
	}
	run := runs[0].(map[string]any)
	if run["id"] != float64(778) || run["conclusion"] != "failure" {
		t.Fatalf("wrong run rendered after failure filter:\n%s", out.String())
	}
}

func TestAgentSurfaceFakeScenariosExitCodesAndSchema(t *testing.T) {
	tests := []struct {
		name       string
		scenario   string
		wantCode   int
		wantStatus string
		wantFailed bool
	}{
		{name: "green", scenario: "green", wantCode: 0, wantStatus: "completed"},
		{name: "failure", scenario: "failure", wantCode: 1, wantStatus: "completed", wantFailed: true},
		{name: "pending", scenario: "pending", wantCode: 3, wantStatus: "in_progress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommandWithRuntime(commandRuntime{
				Stdout: &out,
				Stderr: io.Discard,
				Env:    emptyEnv,
				IsTTY:  true,
			}, testBuildInfo())
			cmd.SetArgs([]string{"runs", "--no-tui", "--json", "--fake-scenario", tt.scenario})

			code, err := executeCommand(cmd)
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d, err=%v, out=%s", code, tt.wantCode, err, out.String())
			}
			if tt.wantCode == 0 && err != nil {
				t.Fatalf("green scenario err = %v", err)
			}
			if tt.wantCode != 0 && err == nil {
				t.Fatalf("scenario %s should return outcome error", tt.scenario)
			}
			decoded := decodeJSON(t, out.Bytes())
			runs := decoded["runs"].([]any)
			run := runs[0].(map[string]any)
			if run["status"] != tt.wantStatus {
				t.Fatalf("status = %v, want %s\n%s", run["status"], tt.wantStatus, out.String())
			}
			failed := run["failed"].([]any)
			if tt.wantFailed {
				if len(failed) != 1 {
					t.Fatalf("failed entries = %d, want 1\n%s", len(failed), out.String())
				}
				failure := failed[0].(map[string]any)
				for _, key := range []string{"job", "step", "exit_code", "annotations", "log_excerpt"} {
					if _, ok := failure[key]; !ok {
						t.Fatalf("failure missing %q in %#v", key, failure)
					}
				}
				return
			}
			if len(failed) != 0 {
				t.Fatalf("failed entries = %d, want 0\n%s", len(failed), out.String())
			}
		})
	}
}

func TestAgentSurfaceAPIErrorExitsTwo(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--no-tui", "--json", "--fake-scenario", "api_error"})

	code, err := executeCommand(cmd)
	if code != 2 || err == nil {
		t.Fatalf("api error code=%d err=%v out=%s", code, err, out.String())
	}
	if strings.Contains(out.String(), "token") || strings.Contains(out.String(), "Authorization") {
		t.Fatalf("error output leaked credential-shaped data:\n%s", out.String())
	}
}

func TestWatchFailFastFailureScenario(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "--json", "--fake-scenario", "failure"})

	code, err := executeCommand(cmd)
	if code != 1 || err == nil {
		t.Fatalf("watch failure code=%d err=%v out=%s", code, err, out.String())
	}
	decoded := decodeJSON(t, out.Bytes())
	run := decoded["runs"].([]any)[0].(map[string]any)
	if run["conclusion"] != "failure" || len(run["failed"].([]any)) != 1 {
		t.Fatalf("watch did not fail fast with failure details:\n%s", out.String())
	}
}

func TestJSONFlagOverridesFormat(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--json", "--format", "md", "--fake-scenario", "green"})

	code, err := executeCommand(cmd)
	if code != 0 || err != nil {
		t.Fatalf("json override code=%d err=%v out=%s", code, err, out.String())
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("--json did not force JSON output:\n%s", out.String())
	}
}

func decodeJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(raw))
	}
	return decoded
}

func testBuildInfo() buildInfo {
	return buildInfo{Version: "v0.1.0", Commit: "a1b2c3d", Date: "2026-06-07T00:00:00Z"}
}

func emptyEnv(string) (string, bool) {
	return "", false
}

func mapEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

type cliRepo struct {
	context usecase.RepositoryContext
	err     error
}

func (r *cliRepo) Current(context.Context) (usecase.RepositoryContext, error) {
	return r.context, r.err
}

type cliGitHub struct {
	runs    []model.Run
	filters []usecase.RunFilter
	err     error
}

func (g *cliGitHub) ListRuns(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	g.filters = append(g.filters, filter)
	return g.runs, g.err
}

func (g *cliGitHub) GetRun(context.Context, string, int64) (model.Run, error) {
	return model.Run{}, nil
}

func (g *cliGitHub) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	return nil, nil
}

func (g *cliGitHub) GetJob(context.Context, string, int64) (model.Job, error) {
	return model.Job{}, nil
}

func (g *cliGitHub) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return nil, nil
}

func (g *cliGitHub) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	return nil, nil
}

func (g *cliGitHub) FetchJobLog(context.Context, string, int64) (string, error) {
	return "", nil
}

func (g *cliGitHub) RerunRun(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) RerunFailedJobs(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) RerunJob(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) CancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) ForceCancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) DispatchWorkflow(context.Context, string, string, usecase.DispatchRequest) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func cliRun(id int64, workflow string, status model.Status, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:         id,
		Name:       workflow,
		Status:     status,
		Conclusion: conclusion,
		Event:      "pull_request",
		HeadBranch: "feature/real",
		HeadSHA:    "abcdef0",
		RunNumber:  int(id % 1000),
		CreatedAt:  time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC),
		HTMLURL:    "https://github.com/indrasvat/gh-hound/actions/runs/777",
	}
}
