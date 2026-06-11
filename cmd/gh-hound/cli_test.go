package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui"
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

func TestDispatchRefRequiresLaunchBranch(t *testing.T) {
	ref, err := dispatchRef(usecase.LaunchContext{Branch: "release/v1"})
	if err != nil {
		t.Fatalf("dispatchRef returned error for launch branch: %v", err)
	}
	if ref != "release/v1" {
		t.Fatalf("dispatchRef = %q, want release/v1", ref)
	}

	_, err = dispatchRef(usecase.LaunchContext{Repo: "openclaw/openclaw"})
	if err == nil {
		t.Fatal("dispatchRef without a branch should fail instead of guessing a ref")
	}
	if !strings.Contains(err.Error(), "dispatch ref is unavailable") {
		t.Fatalf("dispatchRef error = %v", err)
	}
}

func TestWorkflowDispatchMetadataUsesGitHubPathOrID(t *testing.T) {
	pathWorkflow := model.Workflow{Name: "Release", Path: ".github/workflows/release.yml"}
	if got := workflowDisplayName(pathWorkflow); got != "Release" {
		t.Fatalf("workflowDisplayName(path) = %q", got)
	}
	if got := workflowIdentifier(pathWorkflow); got != ".github/workflows/release.yml" {
		t.Fatalf("workflowIdentifier(path) = %q", got)
	}

	idWorkflow := model.Workflow{ID: 123456}
	if got := workflowDisplayName(idWorkflow); got != "123456" {
		t.Fatalf("workflowDisplayName(id) = %q", got)
	}
	if got := workflowIdentifier(idWorkflow); got != "123456" {
		t.Fatalf("workflowIdentifier(id) = %q", got)
	}

	nameOnly := model.Workflow{Name: "Release"}
	if got := workflowIdentifier(nameOnly); got != "" {
		t.Fatalf("workflowIdentifier(name only) = %q, want empty so dispatch fails instead of guessing", got)
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
	cmd.SetArgs([]string{})

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
		Stdin:  strings.NewReader("\nq"),
		Env:    emptyEnv,
		IsTTY:  true,
		GitHub: &cliGitHub{runs: []model.Run{
			cliRun(901, "Release", model.StatusCompleted, model.ConclusionSuccess),
			cliRun(902, "CodeQL", model.StatusCompleted, model.ConclusionSuccess),
		}},
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
			Actor:  "indrasvat",
		}},
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
	for _, want := range []string{"\x1b[?25l", "\x1b[?25h", "██╗  ██╗ ██████╗", "Hunt down your GitHub Actions CI", "⏎ continue · ? help · q quit", "Release"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tty root missing %q\n%s", want, got)
		}
	}
}

func TestInteractiveTUIRefreshesWithoutKeypress(t *testing.T) {
	var out bytes.Buffer
	stdin, stdinWriter := io.Pipe()
	defer func() { _ = stdinWriter.Close() }()
	defer func() { _ = stdin.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()
	github := &cliGitHub{runBatches: [][]model.Run{
		{cliRun(910, "CI", model.StatusInProgress, model.ConclusionNone)},
		{cliRun(910, "CI", model.StatusCompleted, model.ConclusionSuccess)},
	}}

	err := runTUI(ctx, commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Stdin:  stdin,
		Env: mapEnv(map[string]string{
			"HOUND_WELCOME":     "false",
			"HOUND_POLL_MIN_MS": "20",
			"HOUND_POLL_MAX_MS": "20",
		}),
		IsTTY:  false,
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "feature/real",
			Actor:  "indrasvat",
		}},
	}, buildInfo{Version: "test"}, cliOptions{})
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("runTUI err = %v", err)
	}
	if len(github.filters) < 2 {
		t.Fatalf("ListRuns calls = %d, want initial load plus refresh", len(github.filters))
	}
	visible := ansi.Strip(out.String())
	if !strings.Contains(visible, "⠹") || !strings.Contains(visible, "✔") || !strings.Contains(visible, "live") {
		t.Fatalf("TUI did not repaint from running to success without keypress:\n%s", visible)
	}
}

func TestDefaultTUIAppDeepRoutesUseGitHubPortData(t *testing.T) {
	run := cliRun(777, "Real CI", model.StatusCompleted, model.ConclusionFailure)
	job := model.Job{
		ID:         444,
		RunID:      run.ID,
		Name:       "real build",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
		Steps: []model.Step{{
			Number:     3,
			Name:       "real integration test",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
		}},
	}
	github := &cliGitHub{
		runs:   []model.Run{run},
		jobs:   []model.Job{job},
		jobLog: "17:42:53Z real production log\n##[error]real failure from GitHub",
	}
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env: mapEnv(map[string]string{
			"HOUND_WELCOME": "false",
		}),
		IsTTY:  true,
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Actor:  "indrasvat",
		}},
	}, tui.BuildInfo{Version: "v0.1.0"}, cliOptions{})
	if err != nil {
		t.Fatalf("defaultTUIApp returned error: %v", err)
	}
	if github.listJobs == 0 {
		t.Fatal("initial detail did not load jobs through the GitHub port")
	}

	app, handled := app.Update(tui.KeyMsg{Key: "l"})
	if !handled || app.Route() != tui.RouteLog {
		t.Fatalf("l did not open live log route: handled=%v route=%s", handled, app.Route())
	}
	app, settled := app.SettleLoads(2 * time.Second)
	if !settled {
		t.Fatal("log load did not settle")
	}
	view := app.ViewSize(120, 32)
	if github.fetchJobLog != 1 {
		t.Fatalf("FetchJobLog calls = %d, want 1", github.fetchJobLog)
	}
	for _, want := range []string{"real production log", "real failure from GitHub"} {
		if !strings.Contains(view, want) {
			t.Fatalf("live log view missing %q\n%s", want, view)
		}
	}
	for _, banned := range []string{"TestLexIdent", "parser fix validation", "internal/parser/lexer.go"} {
		if strings.Contains(view, banned) {
			t.Fatalf("live TUI route rendered sample data %q\n%s", banned, view)
		}
	}
}

func TestDefaultTUIAppLoadsDispatchInputsFromWorkflowFile(t *testing.T) {
	github := &cliGitHub{
		runs: []model.Run{cliRun(778, "CI", model.StatusCompleted, model.ConclusionSuccess)},
		workflows: []model.Workflow{
			{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
			{ID: 2, Name: "Release", Path: ".github/workflows/release.yml", State: "active"},
		},
		workflowFiles: map[string]string{
			".github/workflows/release.yml": "on:\n  workflow_dispatch:\n    inputs:\n      version:\n        required: true\n        type: string\n      channel:\n        type: choice\n        options: [stable, beta, nightly]\n",
		},
		workflowErrors: map[string]error{
			".github/workflows/ci.yml": usecase.APIError{Kind: usecase.APIErrorNotFound, Status: 404, Message: "Not Found"},
		},
	}
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "openclaw/openclaw",
			Branch: "release/v1",
			Actor:  "indrasvat",
		}},
	}, tui.BuildInfo{Version: "v0.1.0"}, cliOptions{})
	if err != nil {
		t.Fatalf("defaultTUIApp returned error: %v", err)
	}

	app, handled := app.Update(tui.KeyMsg{Key: "D"})
	if !handled {
		t.Fatal("D was not handled")
	}
	app, settled := app.SettleLoads(2 * time.Second)
	if !settled {
		t.Fatal("dispatch load did not settle")
	}
	if app.Route() != tui.RouteDispatch {
		t.Fatalf("D did not open dispatch: route=%s", app.Route())
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	for _, want := range []string{"dispatch · Release", "version", "channel", "● stable  ○ beta  ○ nightly", "POST …/workflows/.github/workflows/release.yml/dispatches"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dispatch view missing %q\n%s", want, view)
		}
	}
}

func TestExternalCommandSelection(t *testing.T) {
	name, args, err := browserCommand("darwin", "https://github.com/openclaw/openclaw/actions/runs/1")
	if err != nil || name != "open" || len(args) != 1 {
		t.Fatalf("darwin browser command = %s %#v %v", name, args, err)
	}
	name, args, err = browserCommand("linux", "https://github.com/openclaw/openclaw/actions/runs/1")
	if err != nil || name != "xdg-open" || len(args) != 1 {
		t.Fatalf("linux browser command = %s %#v %v", name, args, err)
	}
	if _, _, err = browserCommand("linux", " "); err == nil {
		t.Fatal("empty browser URL should fail")
	}
	name, args, err = clipboardCommand("darwin")
	if err != nil || name != "pbcopy" || len(args) != 0 {
		t.Fatalf("darwin clipboard command = %s %#v %v", name, args, err)
	}
	name, args, err = clipboardCommand("windows")
	if err != nil || name != "clip" || len(args) != 0 {
		t.Fatalf("windows clipboard command = %s %#v %v", name, args, err)
	}
}

func TestKeyNameDecodesANSIArrowsAndShiftTab(t *testing.T) {
	tests := map[string]string{
		"\x1b[A": "up",
		"\x1b[B": "down",
		"\x1b[C": "right",
		"\x1b[D": "left",
		"\x1b[Z": "shift+tab",
		"\x1b":   "esc",
		"\x04":   "ctrl+d",
		"\x15":   "ctrl+u",
		"\r":     "enter",
		"\x7f":   "backspace",
	}
	for input, want := range tests {
		if got := keyName([]byte(input)); got != want {
			t.Fatalf("keyName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestKeyDecoderDoesNotDropBatchedInput(t *testing.T) {
	reader := strings.NewReader("j\x1b[Bq")
	decoder := keyDecoder{}
	scratch := make([]byte, 8)
	for _, want := range []string{"j", "down", "q"} {
		got, err := decoder.Next(reader, scratch)
		if err != nil {
			t.Fatalf("Next returned error: %v", err)
		}
		if got != want {
			t.Fatalf("Next = %q, want %q", got, want)
		}
	}
}

func TestInteractiveTUIRejectsFakeScenario(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env: mapEnv(map[string]string{
			"HOUND_FAKE_SCENARIO": "failure",
		}),
		IsTTY: true,
		Repo:  &cliRepo{context: usecase.RepositoryContext{Repo: "openclaw/openclaw", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{})

	code, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("interactive TUI accepted --fake-scenario")
	}
	if code == 0 {
		t.Fatalf("interactive fake exit = %d, want non-zero", code)
	}
	if !strings.Contains(err.Error(), "--fake-scenario is not available for the interactive TUI") {
		t.Fatalf("interactive fake error = %v", err)
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
	if got := github.filters[0].Status; got != "failure" {
		t.Fatalf("GitHub status filter = %q, want failure", got)
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

func TestAuthenticatedHTTPClientFallsBackToGhKeyring(t *testing.T) {
	client := authenticatedHTTPClient(emptyEnv, func() string {
		return "gh-keyring-token"
	})
	transport, ok := client.Transport.(authTransport)
	if !ok {
		t.Fatalf("transport = %T, want authTransport", client.Transport)
	}
	req := requestThroughAuthTransport(t, transport, "https://api.github.com/repos/openclaw/openclaw/actions/runs")
	if got := req.Header.Get("Authorization"); got != "Bearer gh-keyring-token" {
		t.Fatalf("Authorization = %q, want gh keyring bearer token", got)
	}
}

func TestAuthenticatedHTTPClientEnvTokenWinsOverGhKeyring(t *testing.T) {
	client := authenticatedHTTPClient(mapEnv(map[string]string{"GH_TOKEN": "env-token"}), func() string {
		return "gh-keyring-token"
	})
	transport, ok := client.Transport.(authTransport)
	if !ok {
		t.Fatalf("transport = %T, want authTransport", client.Transport)
	}
	req := requestThroughAuthTransport(t, transport, "https://api.github.com/repos/openclaw/openclaw/actions/runs")
	if got := req.Header.Get("Authorization"); got != "Bearer env-token" {
		t.Fatalf("Authorization = %q, want env bearer token", got)
	}
}

func TestAuthenticatedHTTPClientDoesNotAttachTokenToRedirectedLogHosts(t *testing.T) {
	transport := authTransport{token: "github-token"}
	githubReq := requestThroughAuthTransport(t, transport, "https://api.github.com/repos/openclaw/openclaw/actions/jobs/1/logs")
	if got := githubReq.Header.Get("Authorization"); got != "Bearer github-token" {
		t.Fatalf("github Authorization = %q", got)
	}

	blobReq := requestThroughAuthTransport(t, transport, "https://productionresultssa18.blob.core.windows.net/actions-results/log.txt")
	if got := blobReq.Header.Get("Authorization"); got != "" {
		t.Fatalf("redirected log Authorization leaked to blob host: %q", got)
	}
}

func requestThroughAuthTransport(t *testing.T, transport authTransport, rawURL string) *http.Request {
	t.Helper()
	var seen *http.Request
	transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     http.Header{},
			Request:    req,
		}, nil
	})
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if seen == nil {
		t.Fatal("base transport was not called")
	}
	return seen
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDefaultTUIAppHonorsWelcomeEnvOverride(t *testing.T) {
	gh := &cliGitHub{runs: []model.Run{cliRun(101, "CI", model.StatusCompleted, model.ConclusionSuccess)}}
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env: mapEnv(map[string]string{
			"HOUND_WELCOME": "false",
		}),
		GitHub: gh,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Actor:  "indrasvat",
		}},
	}, tui.BuildInfo{Version: "test"}, cliOptions{})
	if err != nil {
		t.Fatalf("defaultTUIApp returned error: %v", err)
	}
	if app.Route() != tui.RouteRuns {
		t.Fatalf("HOUND_WELCOME=false route = %s, want runs", app.Route())
	}
	if len(gh.filters) != 2 ||
		gh.filters[0].Repo != "openclaw/openclaw" || gh.filters[0].Branch != "main" ||
		gh.filters[1].Repo != "openclaw/openclaw" || gh.filters[1].Branch != "" {
		t.Fatalf("unexpected launch filters: %#v", gh.filters)
	}
}

func TestDefaultTUIAppRejectsInvalidConfigEnv(t *testing.T) {
	_, err := defaultTUIApp(context.Background(), commandRuntime{
		Env: mapEnv(map[string]string{
			"HOUND_PER_PAGE": "1000",
		}),
		GitHub: &cliGitHub{},
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "openclaw/openclaw", Branch: "main"}},
	}, tui.BuildInfo{Version: "test"}, cliOptions{})
	if err == nil || !strings.Contains(err.Error(), "per_page") {
		t.Fatalf("defaultTUIApp error = %v, want per_page validation", err)
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

func TestRunsJSONIncludesFailureTriage(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{
		runs: []model.Run{cliRun(911, "CI", model.StatusCompleted, model.ConclusionFailure)},
		jobs: []model.Job{{
			ID:         77,
			Name:       "Lint, Test, Build",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
			Steps: []model.Step{
				{Name: "Set up job", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Number: 1},
				{Name: "Run CI", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, Number: 7},
			},
		}},
		jobLog: "--- FAIL: TestLexIdent (0.00s)\n##[error]Process completed with exit code 1",
		annotations: []model.Annotation{{
			Path:      "internal/parser/lexer.go",
			StartLine: 142,
			Level:     "failure",
			Message:   "identifier mismatch",
		}},
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("runs with a failed run should return an action-needed outcome")
	}
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var decoded struct {
		Runs []struct {
			Failed []struct {
				Job         string `json:"job"`
				Step        string `json:"step"`
				ExitCode    int    `json:"exit_code"`
				LogExcerpt  string `json:"log_excerpt"`
				Annotations []struct {
					Path    string `json:"path"`
					Line    int    `json:"line"`
					Level   string `json:"level"`
					Message string `json:"message"`
				} `json:"annotations"`
			} `json:"failed"`
		} `json:"runs"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if len(decoded.Runs) != 1 || len(decoded.Runs[0].Failed) != 1 {
		t.Fatalf("failed[] not populated for a real failed run:\n%s", out.String())
	}
	failure := decoded.Runs[0].Failed[0]
	if failure.Job != "Lint, Test, Build" || failure.Step != "Run CI" {
		t.Fatalf("failure identity = %q / %q", failure.Job, failure.Step)
	}
	if failure.ExitCode != 1 {
		t.Fatalf("exit_code = %d, want 1", failure.ExitCode)
	}
	if !strings.Contains(failure.LogExcerpt, "--- FAIL: TestLexIdent") {
		t.Fatalf("log_excerpt missing failure anchor: %q", failure.LogExcerpt)
	}
	if len(failure.Annotations) != 1 || failure.Annotations[0].Path != "internal/parser/lexer.go" || failure.Annotations[0].Line != 142 {
		t.Fatalf("annotations = %#v", failure.Annotations)
	}
}

func TestRunsJSONGreenRunsSkipTriageCalls(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{
		runs: []model.Run{
			cliRun(912, "CI", model.StatusCompleted, model.ConclusionSuccess),
			cliRun(913, "Release", model.StatusCompleted, model.ConclusionSuccess),
		},
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("green runs exit = %d, err = %v", code, err)
	}
	if github.listJobs != 0 || github.fetchJobLog != 0 {
		t.Fatalf("green runs must not trigger triage API calls: listJobs=%d fetchJobLog=%d", github.listJobs, github.fetchJobLog)
	}
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
	mu               sync.Mutex
	dispatchCalls    int
	runs             []model.Run
	runBatches       [][]model.Run
	jobs             []model.Job
	jobLog           string
	annotations      []model.Annotation
	artifactList     []model.Artifact
	artifactZip      string
	attemptRun       model.Run
	attemptJobs      []model.Job
	attemptJobCalls  int
	listArtifacts    int
	downloadArtifact int
	filters          []usecase.RunFilter
	listJobs         int
	fetchJobLog      int
	workflows        []model.Workflow
	workflowFiles    map[string]string
	workflowErrors   map[string]error
	err              error
}

func (g *cliGitHub) ListRuns(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	g.filters = append(g.filters, filter)
	if len(g.runBatches) > 0 {
		batch := g.runBatches[0]
		if len(g.runBatches) > 1 {
			g.runBatches = g.runBatches[1:]
		}
		return batch, g.err
	}
	return g.runs, g.err
}

func (g *cliGitHub) GetRun(context.Context, string, int64) (model.Run, error) {
	return model.Run{}, nil
}

func (g *cliGitHub) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.listJobs++
	return g.jobs, nil
}

func (g *cliGitHub) GetJob(context.Context, string, int64) (model.Job, error) {
	return model.Job{}, nil
}

func (g *cliGitHub) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return g.workflows, g.err
}

func (g *cliGitHub) FetchWorkflowFile(_ context.Context, _ string, path string) (string, error) {
	if g.workflowErrors != nil && g.workflowErrors[path] != nil {
		return "", g.workflowErrors[path]
	}
	if g.workflowFiles != nil {
		return g.workflowFiles[path], g.err
	}
	return "on:\n  workflow_dispatch:\n", g.err
}

func (g *cliGitHub) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.annotations, nil
}

func (g *cliGitHub) FetchJobLog(context.Context, string, int64) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fetchJobLog++
	return g.jobLog, nil
}

func (g *cliGitHub) RerunRun(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) RerunFailedJobs(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) RerunJob(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) CancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) ForceCancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) DispatchWorkflow(context.Context, string, string, usecase.DispatchRequest) (usecase.ActionResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dispatchCalls++
	return usecase.ActionResult{}, nil
}

func (g *cliGitHub) dispatches() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.dispatchCalls
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

func (g *cliGitHub) ListArtifacts(context.Context, string, int64) ([]model.Artifact, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.listArtifacts++
	return g.artifactList, nil
}

func (g *cliGitHub) DownloadArtifact(context.Context, string, int64) (io.ReadCloser, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.downloadArtifact++
	return io.NopCloser(strings.NewReader(g.artifactZip)), nil
}

func artifactZipBytes(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("report.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func cliArtifact(id int64, name string, expired bool) model.Artifact {
	return model.Artifact{
		ID:          id,
		Name:        name,
		SizeInBytes: 2048,
		Expired:     expired,
		CreatedAt:   time.Date(2026, 6, 7, 17, 44, 0, 0, time.UTC),
		ExpiresAt:   time.Date(2026, 6, 14, 17, 44, 0, 0, time.UTC),
		Digest:      "sha256:abc",
	}
}

func TestArtifactsJSONListsRunArtifacts(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{
		runs:         []model.Run{cliRun(908, "CI", model.StatusCompleted, model.ConclusionSuccess)},
		artifactList: []model.Artifact{cliArtifact(901, "coverage", false), cliArtifact(902, "old-report", true)},
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"artifacts", "--run", "42", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("artifacts exit = %d, err = %v", code, err)
	}
	var decoded struct {
		Repo      string `json:"repo"`
		RunID     int64  `json:"run_id"`
		Artifacts []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			SizeInBytes int64  `json:"size_in_bytes"`
			Expired     bool   `json:"expired"`
		} `json:"artifacts"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded.Repo != "indrasvat/gh-hound" || decoded.RunID != 42 {
		t.Fatalf("envelope wrong: %#v", decoded)
	}
	if len(decoded.Artifacts) != 2 || decoded.Artifacts[0].Name != "coverage" || !decoded.Artifacts[1].Expired {
		t.Fatalf("artifacts wrong: %#v", decoded.Artifacts)
	}
	if github.listArtifacts != 1 {
		t.Fatalf("listArtifacts calls = %d, want 1", github.listArtifacts)
	}
}

func TestArtifactsDownloadExtractsAndReportsPath(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	github := &cliGitHub{
		artifactList: []model.Artifact{cliArtifact(901, "coverage", false)},
		artifactZip:  artifactZipBytes(t),
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"artifacts", "--run", "42", "--download", "coverage", "--dir", dir, "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("download exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		Downloaded struct {
			Path      string `json:"path"`
			FileCount int    `json:"file_count"`
		} `json:"downloaded"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded.Downloaded.FileCount != 1 {
		t.Fatalf("file_count = %d, want 1", decoded.Downloaded.FileCount)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "coverage", "report.txt")); statErr != nil {
		t.Fatalf("extracted file missing: %v", statErr)
	}
}

func TestArtifactsDownloadExpiredExitsTwo(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{artifactList: []model.Artifact{cliArtifact(902, "old-report", true)}}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"artifacts", "--run", "42", "--download", "old-report", "--no-tui", "--json"})

	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("expired download exit = %d, want 2\n%s", code, out.String())
	}
	if github.downloadArtifact != 0 {
		t.Fatalf("expired artifact must not be downloaded, got %d calls", github.downloadArtifact)
	}
}

func TestArtifactsDefaultsToLatestRunOnBranch(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{
		runs:         []model.Run{cliRun(908, "CI", model.StatusCompleted, model.ConclusionSuccess)},
		artifactList: []model.Artifact{cliArtifact(901, "coverage", false)},
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"artifacts", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("artifacts default-run exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		RunID int64 `json:"run_id"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v", jsonErr)
	}
	if decoded.RunID != 908 {
		t.Fatalf("run_id = %d, want latest run 908", decoded.RunID)
	}
}

func TestRunsArtifactsFlagIsOptIn(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{
		runs:         []model.Run{cliRun(908, "CI", model.StatusCompleted, model.ConclusionSuccess)},
		artifactList: []model.Artifact{cliArtifact(901, "coverage", false)},
	}
	runtime := commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}

	cmd := newRootCommandWithRuntime(runtime, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--no-tui", "--json"})
	if code, err := executeCommand(cmd); err != nil || code != 0 {
		t.Fatalf("runs exit = %d, err = %v", code, err)
	}
	if github.listArtifacts != 0 {
		t.Fatalf("default runs path must make zero artifact calls, got %d", github.listArtifacts)
	}

	out.Reset()
	cmd = newRootCommandWithRuntime(runtime, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--artifacts", "--no-tui", "--json"})
	if code, err := executeCommand(cmd); err != nil || code != 0 {
		t.Fatalf("runs --artifacts exit = %d, err = %v", code, err)
	}
	var decoded struct {
		Runs []struct {
			Artifacts []struct {
				Name string `json:"name"`
			} `json:"artifacts"`
		} `json:"runs"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if len(decoded.Runs) != 1 || len(decoded.Runs[0].Artifacts) != 1 || decoded.Runs[0].Artifacts[0].Name != "coverage" {
		t.Fatalf("runs --artifacts metadata missing:\n%s", out.String())
	}
}

func TestRunsRunFlagNarrowsAndAttemptEnriches(t *testing.T) {
	var out bytes.Buffer
	attemptRun := cliRun(950, "Release", model.StatusCompleted, model.ConclusionFailure)
	github := &cliGitHub{
		attemptRun: attemptRun,
		attemptJobs: []model.Job{{
			ID: 88, Name: "gh extension precompile", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure,
			Steps: []model.Step{{Name: "Run cli/gh-extension-precompile", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, Number: 2}},
		}},
		jobLog: "non-200 OK status code: 401 Unauthorized\n##[error]Process completed with exit code 1.",
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out, Stderr: &bytes.Buffer{}, Env: emptyEnv, IsTTY: false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--run", "950", "--attempt", "2", "--no-tui", "--json"})

	code, _ := executeCommand(cmd)
	if code != 1 {
		t.Fatalf("failed attempt should exit 1, got %d\n%s", code, out.String())
	}
	var decoded struct {
		Runs []struct {
			ID     int64 `json:"id"`
			Failed []struct {
				Job        string `json:"job"`
				LogExcerpt string `json:"log_excerpt"`
			} `json:"failed"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(decoded.Runs) != 1 || decoded.Runs[0].ID != 950 {
		t.Fatalf("--run must narrow to the requested run:\n%s", out.String())
	}
	if len(decoded.Runs[0].Failed) != 1 || decoded.Runs[0].Failed[0].Job != "gh extension precompile" {
		t.Fatalf("attempt jobs must drive failed[]:\n%s", out.String())
	}
	if !strings.Contains(decoded.Runs[0].Failed[0].LogExcerpt, "401 Unauthorized") {
		t.Fatalf("attempt log must drive the excerpt:\n%s", out.String())
	}
	if github.attemptJobCalls != 1 {
		t.Fatalf("attempt-scoped job listing must be used, got %d calls", github.attemptJobCalls)
	}
}

func TestAttemptWithoutRunIsUsageError(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out, Stderr: &bytes.Buffer{}, Env: emptyEnv, IsTTY: false,
		GitHub: &cliGitHub{},
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "x/y", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--attempt", "2", "--no-tui", "--json"})
	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("--attempt without --run must exit 2, got %d", code)
	}
}

func (g *cliGitHub) GetRunAttempt(context.Context, string, int64, int) (model.Run, error) {
	return g.attemptRun, nil
}

func (g *cliGitHub) ListJobsForAttempt(context.Context, string, int64, int) ([]model.Job, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.attemptJobCalls++
	return g.attemptJobs, nil
}

func runMutationCommand(t *testing.T, args ...string) (int, map[string]any, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs(append(args, "--no-tui", "--json", "--fake-scenario", "failure", "-R", "indrasvat/gh-hound"))
	code, err := executeCommand(cmd)
	decoded := map[string]any{}
	if out.Len() > 0 {
		if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
			t.Fatalf("output is not JSON: %v\n%s", jsonErr, out.String())
		}
	}
	return code, decoded, err
}

func TestRerunCommandEmitsAcceptedJSON(t *testing.T) {
	code, decoded, err := runMutationCommand(t, "rerun", "--run", "571", "--debug")
	if err != nil || code != 0 {
		t.Fatalf("rerun exit = %d err = %v", code, err)
	}
	if decoded["action"] != "rerun" || decoded["accepted"] != true {
		t.Fatalf("rerun result = %#v", decoded)
	}
	if decoded["run_id"] != float64(571) {
		t.Fatalf("run_id = %v", decoded["run_id"])
	}
	if decoded["html_url"] != "https://github.com/indrasvat/gh-hound/actions/runs/571" {
		t.Fatalf("html_url = %v (must be reconstructed, never fetched)", decoded["html_url"])
	}
}

func TestRerunFailedOnlyAction(t *testing.T) {
	code, decoded, err := runMutationCommand(t, "rerun", "--run", "571", "--failed-only", "--debug")
	if err != nil || code != 0 {
		t.Fatalf("rerun --failed-only exit = %d err = %v", code, err)
	}
	if decoded["action"] != "rerun_failed" {
		t.Fatalf("action = %v, want rerun_failed", decoded["action"])
	}
}

func TestRerunJobAction(t *testing.T) {
	code, decoded, err := runMutationCommand(t, "rerun", "--run", "571", "--job", "399")
	if err != nil || code != 0 {
		t.Fatalf("rerun --job exit = %d err = %v", code, err)
	}
	if decoded["action"] != "rerun_job" || decoded["job_id"] != float64(399) {
		t.Fatalf("job rerun result = %#v", decoded)
	}
}

func TestRerunJobRejectsFailedOnly(t *testing.T) {
	code, decoded, _ := runMutationCommand(t, "rerun", "--run", "571", "--job", "399", "--failed-only")
	if code != 2 {
		t.Fatalf("conflicting flags exit = %d, want 2\n%v", code, decoded)
	}
}

func TestRerunRequiresRunID(t *testing.T) {
	code, _, _ := runMutationCommand(t, "rerun")
	if code != 2 {
		t.Fatalf("missing --run exit = %d, want 2", code)
	}
}

func TestCancelCommandActions(t *testing.T) {
	code, decoded, err := runMutationCommand(t, "cancel", "--run", "571")
	if err != nil || code != 0 {
		t.Fatalf("cancel exit = %d err = %v", code, err)
	}
	if decoded["action"] != "cancel" {
		t.Fatalf("action = %v, want cancel", decoded["action"])
	}
	code, decoded, err = runMutationCommand(t, "cancel", "--run", "571", "--force")
	if err != nil || code != 0 {
		t.Fatalf("force cancel exit = %d err = %v", code, err)
	}
	if decoded["action"] != "force_cancel" {
		t.Fatalf("action = %v, want force_cancel", decoded["action"])
	}
}

func TestMutationAPIErrorExitsTwoWithTypedError(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"rerun", "--run", "571", "--no-tui", "--json", "--fake-scenario", "api_error", "-R", "indrasvat/gh-hound"})
	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("api error exit = %d, want 2\n%s", code, out.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("api error wrote no envelope: %v\n%s", err, out.String())
	}
	if decoded["accepted"] != false || decoded["action"] != "rerun" {
		t.Fatalf("api error envelope = %#v", decoded)
	}
	errObj, ok := decoded["error"].(map[string]any)
	if !ok || errObj["kind"] != "network" {
		t.Fatalf("api error kind = %#v, want network", decoded["error"])
	}
}

func TestMutationValidationEmitsEnvelope(t *testing.T) {
	for name, args := range map[string][]string{
		"missing run":   {"rerun"},
		"negative run":  {"rerun", "--run", "-5"},
		"job+failed":    {"rerun", "--run", "571", "--job", "399", "--failed-only"},
		"negative job":  {"rerun", "--run", "571", "--job", "-1"},
		"cancel no run": {"cancel"},
	} {
		code, decoded, _ := runMutationCommand(t, args...)
		if code != 2 {
			t.Fatalf("%s: exit = %d, want 2", name, code)
		}
		errObj, ok := decoded["error"].(map[string]any)
		if !ok || errObj["kind"] != "validation" {
			t.Fatalf("%s: error = %#v, want validation kind", name, decoded["error"])
		}
		if decoded["accepted"] != false {
			t.Fatalf("%s: accepted = %v", name, decoded["accepted"])
		}
	}
}

func TestMutationConflictEmitsTypedErrorEnvelope(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	// The fake adapter refuses to cancel a completed run with a typed
	// conflict — agents must see error.kind in the JSON envelope.
	cmd.SetArgs([]string{"cancel", "--run", "569", "--no-tui", "--json", "--fake-scenario", "conflict", "-R", "indrasvat/gh-hound"})
	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("conflict exit = %d, want 2\n%s", code, out.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("error path emitted no JSON envelope: %v\n%s", err, out.String())
	}
	if decoded["accepted"] != false {
		t.Fatalf("accepted = %v, want false", decoded["accepted"])
	}
	errObj, ok := decoded["error"].(map[string]any)
	if !ok || errObj["kind"] == "" || errObj["kind"] == nil {
		t.Fatalf("missing typed error: %#v", decoded)
	}
}

// refAwareGitHub layers the 230 capabilities over the cli stub.
type refAwareGitHub struct {
	*cliGitHub
	defaultBranch string
	existingRefs  map[string]bool
	branchLookups int
}

func (g *refAwareGitHub) DefaultBranch(context.Context, string) (string, error) {
	g.branchLookups++
	return g.defaultBranch, nil
}

func (g *refAwareGitHub) RefExists(_ context.Context, _ string, ref string) (bool, error) {
	return g.existingRefs[ref], nil
}

func TestDispatchForeignRepoPrefillsTargetDefaultBranch(t *testing.T) {
	github := &refAwareGitHub{
		cliGitHub: &cliGitHub{
			workflows: []model.Workflow{{ID: 99, Name: "Deploy", Path: ".github/workflows/deploy.yml", State: "active"}},
			workflowFiles: map[string]string{
				".github/workflows/deploy.yml": "on:\n  workflow_dispatch:\n    inputs:\n      env:\n        type: string\n",
			},
		},
		defaultBranch: "trunk",
		existingRefs:  map[string]bool{"trunk": true},
	}
	// Local checkout is gh-hound, target is openclaw: the local branch
	// must not leak into the dispatch ref (issue #15).
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
		IsTTY:  true,
		GitHub: github,
		Repo: &cliRepo{context: usecase.RepositoryContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "fix/local-work",
			Actor:  "indrasvat",
		}},
	}, tui.BuildInfo{Version: "v0.1.0"}, cliOptions{Repo: "openclaw/openclaw"})
	if err != nil {
		t.Fatalf("defaultTUIApp: %v", err)
	}
	app, _ = app.Update(tui.KeyMsg{Key: "D"})
	app, settled := app.SettleLoads(2 * time.Second)
	if !settled {
		t.Fatal("dispatch load did not settle")
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	if !strings.Contains(view, "trunk") {
		t.Fatalf("dispatch form missing target default branch:\n%s", view)
	}
	if strings.Contains(view, "fix/local-work") {
		t.Fatalf("local branch leaked into foreign dispatch:\n%s", view)
	}
	if github.branchLookups != 1 {
		t.Fatalf("default-branch lookups = %d, want exactly 1", github.branchLookups)
	}
}

func TestDispatchInvalidRefRefusedBeforeMutation(t *testing.T) {
	github := &refAwareGitHub{
		cliGitHub: &cliGitHub{
			workflows: []model.Workflow{{ID: 99, Name: "Deploy", Path: ".github/workflows/deploy.yml", State: "active"}},
			workflowFiles: map[string]string{
				".github/workflows/deploy.yml": "on:\n  workflow_dispatch:\n",
			},
		},
		defaultBranch: "trunk",
		existingRefs:  map[string]bool{}, // nothing exists: every ref is a typo
	}
	app, err := defaultTUIApp(context.Background(), commandRuntime{
		Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat"}},
	}, tui.BuildInfo{Version: "v0.1.0"}, cliOptions{Repo: "openclaw/openclaw"})
	if err != nil {
		t.Fatalf("defaultTUIApp: %v", err)
	}
	app, _ = app.Update(tui.KeyMsg{Key: "D"})
	app, _ = app.SettleLoads(2 * time.Second)
	// Submit the form: enter triggers the confirm, y confirms.
	app, _ = app.Update(tui.KeyMsg{Key: "enter"})
	app, _ = app.Update(tui.KeyMsg{Key: "y"})
	view := ansi.Strip(app.ViewSize(120, 36))
	if !strings.Contains(view, "isn't in this yard") {
		t.Fatalf("invalid ref not refused with the typed message:\n%s", view)
	}
	if github.dispatches() != 0 {
		t.Fatalf("dispatch mutation fired despite invalid ref")
	}
}
