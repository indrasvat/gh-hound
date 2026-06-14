package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
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

// TestWatchGroupTimeoutClosesTheHunt pins the bounded --timeout: the
// running scenario never settles, so a short --timeout must close the
// stream with a timed_out:true summary and exit 3 (pending) instead of
// blocking forever.
func TestWatchGroupTimeoutClosesTheHunt(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Env: mapEnv(map[string]string{
			"HOUND_POLL_MIN_MS": "5",
			"HOUND_POLL_MAX_MS": "5",
		}),
		IsTTY: true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "--group", "--no-tui", "--timeout", "40ms", "--fake-scenario", "pending"})

	code, err := executeCommand(cmd)
	if code != 3 || err == nil {
		t.Fatalf("watch --group --timeout code=%d err=%v out=%s", code, err, out.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var summary struct {
		Type     string `json:"type"`
		Running  int    `json:"running"`
		TimedOut bool   `json:"timed_out"`
	}
	last := lines[len(lines)-1]
	if uerr := json.Unmarshal([]byte(last), &summary); uerr != nil {
		t.Fatalf("final line is not valid JSON: %v\n%s", uerr, last)
	}
	if summary.Type != "summary" {
		t.Fatalf("stream must close with a summary, got %q", last)
	}
	if !summary.TimedOut {
		t.Fatalf("timed-out hunt must carry timed_out:true, got %s", last)
	}
	if summary.Running < 1 {
		t.Fatalf("timed-out summary must still report live running members, got %s", last)
	}
}

// TestWatchTimeoutWithoutGroupIsRefused pins that --timeout only bounds
// the blocking --group hunt; on the snapshot path it is meaningless and
// must be refused up front (exit 2) rather than silently ignored.
func TestWatchTimeoutWithoutGroupIsRefused(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "--timeout", "5m", "--fake-scenario", "pending"})

	code, err := executeCommand(cmd)
	if code != 2 || err == nil || !strings.Contains(err.Error(), "--group") {
		t.Fatalf("watch --timeout (no --group) code=%d err=%v", code, err)
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

// TestWatchGroupRunAnchorKeepsItsOwnBranch pins the ghent Codex P2: a
// --run anchor fetched via GetRun can live on a branch the launch
// never resolved; the group ticks must list THAT branch or the anchor
// is never refreshed and an in-progress hunt waits forever.
func TestWatchGroupRunAnchorKeepsItsOwnBranch(t *testing.T) {
	anchor := model.Run{
		ID: 9100, RunNumber: 91, Name: "CI", Event: "push",
		HeadBranch: "feat/elsewhere", HeadSHA: "feedface",
		Status: model.StatusInProgress,
	}
	settled := anchor
	settled.Status = model.StatusCompleted
	settled.Conclusion = model.ConclusionSuccess
	github := &cliGitHub{
		// Batch 0: the anchor-resolution listing (anchor not in it →
		// GetRun fallback). Batch 1: the first tick settles the hunt.
		runBatches: [][]model.Run{
			{{ID: 1, RunNumber: 1, Name: "CI", Event: "push", HeadBranch: "main", HeadSHA: "aaa", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}},
			{settled},
		},
		runByID: map[int64]model.Run{9100: anchor},
	}
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Env: mapEnv(map[string]string{
			"HOUND_POLL_MIN_MS": "5",
			"HOUND_POLL_MAX_MS": "5",
		}),
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"watch", "--group", "--no-tui", "--run", "9100"})
	if code, err := executeCommand(cmd); code != 0 || err != nil {
		t.Fatalf("watch --group --run code=%d err=%v\n%s", code, err, out.String())
	}
	sawAnchorBranch := false
	for _, filter := range github.filters {
		if filter.HeadSHA == "feedface" && filter.Branch != "feat/elsewhere" {
			t.Fatalf("group tick listed branch %q for the anchor, want feat/elsewhere", filter.Branch)
		}
		if filter.Branch == "feat/elsewhere" {
			sawAnchorBranch = true
		}
	}
	if !sawAnchorBranch {
		t.Fatal("no tick listed the anchor's branch")
	}
}
