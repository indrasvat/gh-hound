package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The pack pipe surface: NDJSON transitions until the group settles,
// terminal summary, worst-outcome exit code.
func TestWatchGroupEmitsNDJSONUntilThePackSettles(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: nil,
		Env: mapEnv(map[string]string{
			"HOUND_POLL_MIN_MS": "5",
			"HOUND_POLL_MAX_MS": "5",
		}),
		IsTTY: true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "--group", "--no-tui", "--fake-scenario", "pack"})

	code, err := executeCommand(cmd)
	if code != 1 || err == nil {
		t.Fatalf("watch --group code=%d err=%v out=%s", code, err, out.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected several NDJSON lines, got %d:\n%s", len(lines), out.String())
	}
	type line struct {
		Type       string `json:"type"`
		RunID      int64  `json:"run_id"`
		Workflow   string `json:"workflow"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		Repo       string `json:"repo"`
		HeadSHA    string `json:"head_sha"`
		Event      string `json:"event"`
		Runs       int    `json:"runs"`
		Running    int    `json:"running"`
		Home       int    `json:"home"`
		Lost       int    `json:"lost"`
	}
	var decoded []line
	for i, raw := range lines {
		var l line
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\n%s", i, err, raw)
		}
		if raw != strings.TrimSpace(raw) || strings.Contains(raw, "\n") {
			t.Fatalf("line %d is not a single compact NDJSON line: %q", i, raw)
		}
		decoded = append(decoded, l)
	}

	// Every line but the last is a run-level transition.
	transitions := map[string][]string{}
	for _, l := range decoded[:len(decoded)-1] {
		if l.Type != "run" {
			t.Fatalf("mid-stream line type = %q, want run:\n%s", l.Type, out.String())
		}
		transitions[l.Workflow] = append(transitions[l.Workflow], l.Status+"/"+l.Conclusion)
	}
	// The chained workflow_run deploy shares the sha but not the event:
	// it must never appear on the stream.
	if _, leaked := transitions["Deploy Pages"]; leaked {
		t.Fatalf("foreign-event run leaked into the group stream:\n%s", out.String())
	}
	// Each pack member walks to completion; Docs is the lost one.
	for _, workflow := range []string{"CI", "Release", "Docs"} {
		states := transitions[workflow]
		if len(states) < 2 {
			t.Fatalf("%s transitions = %v, want at least start and settle", workflow, states)
		}
		final := states[len(states)-1]
		want := "completed/success"
		if workflow == "Docs" {
			want = "completed/failure"
		}
		if final != want {
			t.Fatalf("%s final state = %s, want %s", workflow, final, want)
		}
	}

	summary := decoded[len(decoded)-1]
	if summary.Type != "summary" {
		t.Fatalf("stream must close with the summary, got %q", lines[len(lines)-1])
	}
	if summary.Runs != 3 || summary.Running != 0 || summary.Home != 2 || summary.Lost != 1 {
		t.Fatalf("summary = %+v, want 3 runs · 0 running · 2 home · 1 lost", summary)
	}
	if summary.Event != "push" || summary.Repo == "" || summary.HeadSHA == "" {
		t.Fatalf("summary scent incomplete: %+v", summary)
	}
}

func TestWatchGroupRefusesNonJSONFormats(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "--group", "--format", "md", "--fake-scenario", "pack"})

	code, err := executeCommand(cmd)
	if code != 2 || err == nil || !strings.Contains(err.Error(), "NDJSON") {
		t.Fatalf("watch --group --format md code=%d err=%v", code, err)
	}
}
