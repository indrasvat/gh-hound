package render

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

//go:fix inline

func fixturePaceResult() PaceResult {
	return PaceResult{
		Repo: "indrasvat/gh-hound", Workflow: ".github/workflows/ci.yml", Branch: "fix/parser",
		BaselineBranch: nil, BaselineEvent: "pull_request",
		Status: "slow", Window: 20, SampleSize: 20, RunsScanned: 23, Statistic: "p50_p90",
		Run: &PaceRun{
			ID: 1234567890, Number: 573, Attempt: 1, Event: "pull_request",
			Status: "completed", Conclusion: "success",
			ElapsedSeconds: 571, QueuedSeconds: 14,
			P50Seconds: new(int64(241)), P90Seconds: new(int64(298)), Ratio: new(2.37), Slow: true,
		},
		Verdict: "the hound ran slow: 9m31s against its usual 4m01s",
	}
}

func TestPaceJSONMatchesGolden(t *testing.T) {
	var out bytes.Buffer
	if err := WritePace(&out, FormatJSON, fixturePaceResult()); err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/pace.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(golden) {
		t.Fatalf("pace JSON drifted from testdata/pace.golden.json:\n%s", out.String())
	}
}

type schemaNode struct {
	Required   []string              `json:"required"`
	Properties map[string]schemaNode `json:"properties"`
	Enum       []any                 `json:"enum"`
	Type       any                   `json:"type"`
}

func paceSchema(t *testing.T) schemaNode {
	t.Helper()
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]schemaNode `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	node, ok := schema.Defs["pace_result"]
	if !ok {
		t.Fatal("schema missing $defs.pace_result")
	}
	return node
}

// conform checks a decoded object against a schema node: required keys
// present, no undeclared keys, enum values and JSON types allowed.
func conform(t *testing.T, path string, value any, node schemaNode) {
	t.Helper()
	if len(node.Enum) > 0 && !slices.Contains(node.Enum, value) {
		t.Fatalf("%s = %v, not in enum %v", path, value, node.Enum)
	}
	if node.Type != nil && !typeAllowed(node.Type, value) {
		t.Fatalf("%s = %v (%T), want type %v", path, value, value, node.Type)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return
	}
	for _, key := range node.Required {
		if _, ok := obj[key]; !ok {
			t.Fatalf("%s missing required key %q", path, key)
		}
	}
	for key, v := range obj {
		child, ok := node.Properties[key]
		if !ok {
			t.Fatalf("%s.%s is not declared in the schema", path, key)
		}
		conform(t, path+"."+key, v, child)
	}
}

func typeAllowed(want any, value any) bool {
	names := []string{}
	switch w := want.(type) {
	case string:
		names = append(names, w)
	case []any:
		for _, n := range w {
			names = append(names, n.(string))
		}
	}
	for _, name := range names {
		switch v := value.(type) {
		case nil:
			if name == "null" {
				return true
			}
		case string:
			if name == "string" {
				return true
			}
		case bool:
			if name == "boolean" {
				return true
			}
		case float64:
			if name == "number" || name == "integer" && v == float64(int64(v)) {
				return true
			}
		case map[string]any:
			if name == "object" {
				return true
			}
		case []any:
			if name == "array" {
				return true
			}
		}
	}
	return false
}

// Every status the verb can emit conforms to $defs.pace_result: nulls
// where there is no baseline, a typed error, a widened baseline.
func TestPaceEnvelopesConformToSchema(t *testing.T) {
	schema := paceSchema(t)
	running := fixturePaceResult()
	running.Status, running.BaselineBranch = "running", new("fix/parser")
	running.Run.Status, running.Run.Conclusion, running.Run.Slow, running.Run.ETASeconds = "in_progress", "", false, new(int64(151))
	thin := fixturePaceResult()
	thin.Status, thin.SampleSize = "insufficient_data", 2
	thin.Run.P50Seconds, thin.Run.P90Seconds, thin.Run.Ratio, thin.Run.Slow = nil, nil, nil, false
	rerun := thin
	rerun.Status = "not_comparable"
	rerun.Run = &PaceRun{ID: 1, Number: 2, Attempt: 2, Event: "push", Status: "completed", Conclusion: "success", Reason: "attempt 2 is a re-run"}
	failed := PaceResult{Repo: "o/r", Status: "error", Statistic: "p50_p90", Verdict: "", Error: &PaceError{Kind: "not_found", Message: "no workflow"}}

	for name, result := range map[string]PaceResult{"slow": fixturePaceResult(), "running": running, "thin": thin, "rerun": rerun, "error": failed} {
		var out bytes.Buffer
		if err := WritePace(&out, FormatJSON, result); err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
			t.Fatalf("%s: invalid JSON: %v", name, err)
		}
		conform(t, name, decoded, schema)
	}
}

func TestPaceExitCodes(t *testing.T) {
	cases := map[string]int{
		"on_pace": ExitOK, "insufficient_data": ExitOK, "not_comparable": ExitOK,
		"slow": ExitActionNeeded, "overrun": ExitActionNeeded,
		"running": ExitPending, "error": ExitError,
	}
	for status, want := range cases {
		if got := PaceExitCode(PaceResult{Status: status}); got != want {
			t.Errorf("%s exit = %d, want %d", status, got, want)
		}
	}
	if got := PaceExitCode(PaceResult{Status: "running", Error: &PaceError{Kind: "network"}}); got != ExitError {
		t.Fatalf("an error wins over any status: exit %d", got)
	}
}

func TestPaceMarkdownAndXML(t *testing.T) {
	var md bytes.Buffer
	if err := WritePace(&md, FormatMarkdown, fixturePaceResult()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# gh-hound pace", "Baseline: any branch, `pull_request` events, 20 green runs", "| #573 | 1 | completed | 571s | 14s | — | 241s | 298s | 2.37× | true |", "ran slow"} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("markdown missing %q:\n%s", want, md.String())
		}
	}
	var xmlOut bytes.Buffer
	if err := WritePace(&xmlOut, FormatXML, fixturePaceResult()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<pace_result", `p50_seconds="241"`, `slow="true"`} {
		if !strings.Contains(xmlOut.String(), want) {
			t.Fatalf("xml missing %q:\n%s", want, xmlOut.String())
		}
	}
	if strings.Contains(xmlOut.String(), "eta_seconds") || strings.Contains(xmlOut.String(), "baseline_branch") {
		t.Fatalf("xml must omit absent values, not print empty attributes:\n%s", xmlOut.String())
	}
}
