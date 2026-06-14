package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGroupEventWritesOneCompactNDJSONLine(t *testing.T) {
	var out bytes.Buffer
	ts := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	err := WriteGroupEvent(&out, GroupEvent{
		TS:         ts,
		RunID:      30433701,
		Workflow:   "CI",
		Status:     "in_progress",
		Conclusion: "",
	})
	if err != nil {
		t.Fatalf("write event: %v", err)
	}
	got := out.String()
	want := `{"type":"run","ts":"2026-06-11T10:00:00Z","run_id":30433701,"workflow":"CI","status":"in_progress","conclusion":""}` + "\n"
	if got != want {
		t.Fatalf("event line = %q, want %q", got, want)
	}
	if strings.Count(got, "\n") != 1 || strings.Contains(strings.TrimSuffix(got, "\n"), "\n") {
		t.Fatalf("event must be exactly one line: %q", got)
	}
}

func TestGroupSummaryClosesTheStream(t *testing.T) {
	var out bytes.Buffer
	ts := time.Date(2026, 6, 11, 10, 5, 0, 0, time.UTC)
	err := WriteGroupSummary(&out, GroupSummary{
		TS:      ts,
		Repo:    "indrasvat/gh-hound",
		HeadSHA: "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432",
		Event:   "push",
		Runs:    3,
		Running: 0,
		Home:    2,
		Lost:    1,
	})
	if err != nil {
		t.Fatalf("write summary: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("summary not valid JSON: %v", err)
	}
	if decoded["type"] != "summary" || decoded["lost"] != float64(1) || decoded["event"] != "push" {
		t.Fatalf("summary = %s", out.String())
	}
}

// TestSchemaPublishesWatchGroupContract pins the public NDJSON
// contract: run-level events only, plus the terminal summary.
func TestSchemaPublishesWatchGroupContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema.json invalid: %v", err)
	}
	event, ok := schema.Defs["watch_group_event"]
	if !ok {
		t.Fatal("schema.json must define $defs.watch_group_event")
	}
	for _, needle := range []string{"run_id", "workflow", "status", "conclusion"} {
		if !strings.Contains(string(event), needle) {
			t.Fatalf("watch_group_event schema missing %q", needle)
		}
	}
	// The board budget forbids job fetches: job/step fields must never
	// leak into the group event contract.
	for _, forbidden := range []string{`"job"`, `"step"`} {
		if strings.Contains(string(event), forbidden) {
			t.Fatalf("watch_group_event schema must stay run-level, found %s", forbidden)
		}
	}
	summary, ok := schema.Defs["watch_group_summary"]
	if !ok {
		t.Fatal("schema.json must define $defs.watch_group_summary")
	}
	for _, needle := range []string{"head_sha", "running", "home", "lost", "timed_out"} {
		if !strings.Contains(string(summary), needle) {
			t.Fatalf("watch_group_summary schema missing %q", needle)
		}
	}
}

// TestGroupSummaryCarriesTimedOutMarker pins the bounded-timeout
// contract: a settled summary reports timed_out:false, a timed-out one
// reports true so agents can tell a clean conclusion from an aborted
// wait without re-deriving it from the running tally.
func TestGroupSummaryCarriesTimedOutMarker(t *testing.T) {
	ts := time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		timedOut bool
	}{
		{"settled", false},
		{"timed out", true},
	} {
		var out bytes.Buffer
		if err := WriteGroupSummary(&out, GroupSummary{
			TS: ts, Repo: "indrasvat/gh-hound", HeadSHA: "abc123", Event: "push",
			Runs: 3, Running: 1, Home: 2, Lost: 0, TimedOut: tc.timedOut,
		}); err != nil {
			t.Fatalf("%s: write summary: %v", tc.name, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
			t.Fatalf("%s: summary not valid JSON: %v", tc.name, err)
		}
		got, ok := decoded["timed_out"].(bool)
		if !ok {
			t.Fatalf("%s: summary missing timed_out marker: %s", tc.name, out.String())
		}
		if got != tc.timedOut {
			t.Fatalf("%s: timed_out = %v, want %v", tc.name, got, tc.timedOut)
		}
	}
}
