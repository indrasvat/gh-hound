package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
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
			"HOUND_REPO":   "indrasvat/gh-ghent",
			"HOUND_BRANCH": "fix/parser",
			"HOUND_STATUS": "failure",
			"HOUND_NO_TUI": "true",
			"HOUND_FORMAT": "json",
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

func TestLaunchFlagsRouteRepoAllAndWatch(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env: mapEnv(map[string]string{
			"GH_REPO": "indrasvat/env-repo",
		}),
		IsTTY: false,
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
