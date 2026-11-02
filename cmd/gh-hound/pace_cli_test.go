package main

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
)

func runPaceCommand(t *testing.T, env map[string]string, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	runtime := commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		IsTTY:  true,
		Env: func(key string) (string, bool) {
			v, ok := env[key]
			return v, ok
		},
	}
	cmd := newRootCommandWithRuntime(runtime, testBuildInfo())
	cmd.SetArgs(append([]string{"pace", "--no-tui"}, args...))
	code, _ := executeCommand(cmd)
	return code, out.String()
}

func pacedRunID(number int) string { return strconv.Itoa(30433000 + number) }

func decodePace(t *testing.T, raw string) (map[string]any, map[string]any) {
	t.Helper()
	decoded := decodeJSON(t, []byte(raw))
	run, _ := decoded["run"].(map[string]any)
	return decoded, run
}

// No --run: the newest run of the latest workflow on the branch, here
// #600, 90s into a ~4m workflow — pending, with time left.
func TestPaceRunningRunExitsPendingWithAnETA(t *testing.T) {
	code, out := runPaceCommand(t, nil, "--json", "--fake-scenario", "paced")
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (pending)\n%s", code, out)
	}
	decoded, run := decodePace(t, out)
	if decoded["status"] != "running" || run["number"].(float64) != fake.PacedRunningNumber {
		t.Fatalf("status %v run %v, want running #600\n%s", decoded["status"], run["number"], out)
	}
	if decoded["baseline_branch"] != "main" || decoded["baseline_event"] != "push" || decoded["sample_size"].(float64) != 20 {
		t.Fatalf("baseline %v/%v/%v, want main/push/20\n%s", decoded["baseline_branch"], decoded["baseline_event"], decoded["sample_size"], out)
	}
	eta, ok := run["eta_seconds"].(float64)
	if !ok || eta < 150 || eta > 160 {
		t.Fatalf("eta_seconds = %v, want about 246-90\n%s", run["eta_seconds"], out)
	}
	if run["queued_seconds"].(float64) != 14 {
		t.Fatalf("queued_seconds = %v, want 14", run["queued_seconds"])
	}
}

// The slow run is judged without itself: 19 samples, p50 4m06s, p90
// 4m22s, so 9m31s is 2.32x — slow, exit 1. Slow schedule runs, the
// re-run attempt and the failure never enter the baseline.
func TestPaceSlowRunExitsOne(t *testing.T) {
	code, out := runPaceCommand(t, nil, "--json", "--fake-scenario", "paced", "--run", pacedRunID(fake.PacedSlowNumber))
	if code != 1 {
		t.Fatalf("exit = %d, want 1\n%s", code, out)
	}
	decoded, run := decodePace(t, out)
	if decoded["status"] != "slow" || run["slow"] != true {
		t.Fatalf("status %v slow %v\n%s", decoded["status"], run["slow"], out)
	}
	if decoded["sample_size"].(float64) != 19 || run["p50_seconds"].(float64) != 246 || run["p90_seconds"].(float64) != 262 || run["ratio"].(float64) != 2.32 {
		t.Fatalf("sample %v p50 %v p90 %v ratio %v, want 19, 246, 262, 2.32\n%s", decoded["sample_size"], run["p50_seconds"], run["p90_seconds"], run["ratio"], out)
	}
	if run["eta_seconds"] != nil {
		t.Fatalf("a finished run has no ETA: %v", run["eta_seconds"])
	}
	if !strings.Contains(decoded["verdict"].(string), "ran slow: 9m31s against its usual 4m06s") {
		t.Fatalf("verdict = %v", decoded["verdict"])
	}
}

// A PR branch with two green runs widens to pull_request runs on any
// branch, and says so with a null baseline_branch.
func TestPacePRBranchWidensToTheSameEventAnywhere(t *testing.T) {
	code, out := runPaceCommand(t, nil, "--json", "--fake-scenario", "paced", "--branch", "fix/parser")
	decoded, run := decodePace(t, out)
	if code != 0 || decoded["status"] != "on_pace" || run["number"].(float64) != fake.PacedPRNumber {
		t.Fatalf("exit %d status %v run %v, want 0 on_pace #620\n%s", code, decoded["status"], run["number"], out)
	}
	if decoded["baseline_branch"] != nil || decoded["baseline_event"] != "pull_request" || decoded["sample_size"].(float64) != 7 {
		t.Fatalf("baseline %v/%v/%v, want null/pull_request/7\n%s", decoded["baseline_branch"], decoded["baseline_event"], decoded["sample_size"], out)
	}
}

func TestPaceRerunIsNotComparable(t *testing.T) {
	code, out := runPaceCommand(t, nil, "--json", "--fake-scenario", "paced", "--run", pacedRunID(578))
	decoded, run := decodePace(t, out)
	if code != 0 || decoded["status"] != "not_comparable" || run["reason"] != "attempt 2 is a re-run" || run["p50_seconds"] != nil {
		t.Fatalf("exit %d status %v reason %v p50 %v\n%s", code, decoded["status"], run["reason"], run["p50_seconds"], out)
	}
}

func TestPaceThinHistoryIsInsufficientExitZero(t *testing.T) {
	code, out := runPaceCommand(t, nil, "--json", "--fake-scenario", "green")
	decoded, _ := decodePace(t, out)
	if code != 0 || decoded["status"] != "insufficient_data" {
		t.Fatalf("exit %d status %v, want 0 insufficient_data\n%s", code, decoded["status"], out)
	}
	if !strings.Contains(decoded["verdict"].(string), "needs 5") {
		t.Fatalf("verdict = %v", decoded["verdict"])
	}
}

func TestPaceErrorsAreTyped(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		args []string
		kind string
	}{
		{"api error", nil, []string{"--fake-scenario", "api_error"}, "network"},
		{"window below range", nil, []string{"--fake-scenario", "paced", "--window", "4"}, "validation"},
		{"run and workflow", nil, []string{"--fake-scenario", "paced", "--run", pacedRunID(599), "--workflow", "CI"}, "validation"},
		{"unknown run", nil, []string{"--fake-scenario", "paced", "--run", "7"}, "unknown"},
		{"bad env", map[string]string{"HOUND_PACE_WINDOW": "lots"}, []string{"--fake-scenario", "paced"}, "validation"},
	}
	for _, tc := range cases {
		code, out := runPaceCommand(t, tc.env, append([]string{"--json"}, tc.args...)...)
		decoded := decodeJSON(t, []byte(out))
		errObj, _ := decoded["error"].(map[string]any)
		if code != 2 || decoded["status"] != "error" || errObj == nil || errObj["kind"] != tc.kind {
			t.Fatalf("%s: exit %d status %v error %v, want 2 and kind %s\n%s", tc.name, code, decoded["status"], errObj, tc.kind, out)
		}
	}
}

// --window beats HOUND_PACE_WINDOW, which beats the default.
func TestPaceWindowPrecedence(t *testing.T) {
	env := map[string]string{"HOUND_PACE_WINDOW": "8"}
	_, out := runPaceCommand(t, env, "--json", "--fake-scenario", "paced")
	if decoded, _ := decodePace(t, out); decoded["window"].(float64) != 8 || decoded["sample_size"].(float64) != 8 {
		t.Fatalf("env window: %v/%v, want 8/8\n%s", decoded["window"], decoded["sample_size"], out)
	}
	_, out = runPaceCommand(t, env, "--json", "--fake-scenario", "paced", "--window", "12")
	if decoded, _ := decodePace(t, out); decoded["window"].(float64) != 12 {
		t.Fatalf("flag window: %v, want 12\n%s", decoded["window"], out)
	}
}

// pace = false turns off the TUI annotations and their requests; the
// verb is asked for by name, so it still answers.
func TestPaceVerbAnswersWithPaceOff(t *testing.T) {
	code, out := runPaceCommand(t, map[string]string{"HOUND_PACE": "false"}, "--json", "--fake-scenario", "paced", "--run", pacedRunID(fake.PacedSlowNumber))
	if code != 1 || !strings.Contains(out, `"status": "slow"`) {
		t.Fatalf("exit %d, want the verb to answer with pace off\n%s", code, out)
	}
}

func TestPaceMarkdownAndXML(t *testing.T) {
	_, md := runPaceCommand(t, nil, "--format", "md", "--fake-scenario", "paced", "--run", pacedRunID(fake.PacedSlowNumber))
	if !strings.Contains(md, "# gh-hound pace") || !strings.Contains(md, "| #599 |") {
		t.Fatalf("markdown:\n%s", md)
	}
	_, xmlOut := runPaceCommand(t, nil, "--format", "xml", "--fake-scenario", "paced", "--run", pacedRunID(fake.PacedSlowNumber))
	if !strings.Contains(xmlOut, `<pace_result`) || !strings.Contains(xmlOut, `status="slow"`) {
		t.Fatalf("xml:\n%s", xmlOut)
	}
}

// A small window still judges the slow run against a full window, not
// one short because the run itself was sampled.
func TestPaceSmallWindowStillJudges(t *testing.T) {
	code, out := runPaceCommand(t, nil, "--json", "--fake-scenario", "paced", "--run", pacedRunID(fake.PacedSlowNumber), "--window", "5")
	decoded, _ := decodePace(t, out)
	if code != 1 || decoded["status"] != "slow" || decoded["sample_size"].(float64) != 5 {
		t.Fatalf("exit %d status %v sample %v, want 1, slow, 5\n%s", code, decoded["status"], decoded["sample_size"], out)
	}
}
