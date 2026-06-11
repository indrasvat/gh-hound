package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureDiffResult() DiffResult {
	created := time.Date(2026, 6, 9, 14, 5, 0, 0, time.UTC)
	return DiffResult{
		Repo:     "indrasvat/gh-hound",
		Workflow: "CI",
		Branch:   "main",
		Status:   "located",
		LastGood: &Run{
			ID: 101, Workflow: "CI", RunNumber: 101, Event: "push", HeadBranch: "main",
			HeadSHA: "sha0101", Status: "completed", Conclusion: "success",
			CreatedAt: created, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/runs/101",
			Failed: []Failure{},
		},
		FirstBad: &Run{
			ID: 102, Workflow: "CI", RunNumber: 102, Event: "push", HeadBranch: "main",
			HeadSHA: "sha0102", Status: "completed", Conclusion: "failure",
			CreatedAt: created.Add(time.Hour), HTMLURL: "https://github.com/indrasvat/gh-hound/actions/runs/102",
			Failed: []Failure{},
		},
		SuspectCommits: []Commit{
			{SHA: "f2b85a73d866512fa76484ee2034d46e28ab9de1", Author: "indrasvat", Message: "fix: terminal resize"},
		},
		TotalSuspects: 1,
		CompareURL:    "https://github.com/indrasvat/gh-hound/compare/sha0101...sha0102",
		RunsScanned:   12,
		Verdict:       "scent picked up: #101 was clean, #102 wasn't.",
	}
}

func TestDiffJSONMatchesPublicContract(t *testing.T) {
	var out bytes.Buffer
	if err := WriteDiff(&out, FormatJSON, fixtureDiffResult()); err != nil {
		t.Fatalf("WriteDiff returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{"repo", "workflow", "branch", "status", "last_good", "first_bad", "suspect_commits", "total_suspects", "compare_url", "runs_scanned", "verdict"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("diff result missing key %q in %s", key, out.String())
		}
	}
	commit := decoded["suspect_commits"].([]any)[0].(map[string]any)
	for _, key := range []string{"sha", "author", "message"} {
		if _, ok := commit[key]; !ok {
			t.Fatalf("suspect commit missing key %q", key)
		}
	}
	lastGood := decoded["last_good"].(map[string]any)
	if lastGood["run_number"].(float64) != 101 {
		t.Fatalf("last_good.run_number = %v", lastGood["run_number"])
	}
}

func TestDiffGreenAndInconclusiveOmitBoundary(t *testing.T) {
	var out bytes.Buffer
	result := DiffResult{
		Repo: "o/r", Workflow: "CI", Branch: "main",
		Status:         "inconclusive",
		SuspectCommits: []Commit{},
		Verdict:        "trail went cold after 1,000 runs.",
	}
	if err := WriteDiff(&out, FormatJSON, result); err != nil {
		t.Fatalf("WriteDiff returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := decoded["last_good"]; ok {
		t.Fatalf("inconclusive verdict must omit last_good: %s", out.String())
	}
	if _, ok := decoded["first_bad"]; ok {
		t.Fatalf("inconclusive verdict must omit first_bad: %s", out.String())
	}
	if commits, ok := decoded["suspect_commits"].([]any); !ok || len(commits) != 0 {
		t.Fatalf("suspect_commits must be an empty array, got %v", decoded["suspect_commits"])
	}
}

func TestDiffErrorEnvelope(t *testing.T) {
	var out bytes.Buffer
	result := DiffResult{
		Repo: "o/r", Workflow: "CI", Branch: "main", Status: "error",
		SuspectCommits: []Commit{},
		Error:          &DiffError{Kind: "rate_limit", Message: "API rate limit exceeded"},
	}
	if err := WriteDiff(&out, FormatJSON, result); err != nil {
		t.Fatalf("WriteDiff returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "rate_limit" {
		t.Fatalf("error.kind = %v", errObj["kind"])
	}
}

func TestDiffExitCodes(t *testing.T) {
	if got := DiffExitCode(DiffResult{Status: "located"}); got != ExitActionNeeded {
		t.Fatalf("located exit = %d, want %d", got, ExitActionNeeded)
	}
	if got := DiffExitCode(DiffResult{Status: "green"}); got != ExitOK {
		t.Fatalf("green exit = %d, want %d", got, ExitOK)
	}
	if got := DiffExitCode(DiffResult{Status: "inconclusive"}); got != ExitOK {
		t.Fatalf("inconclusive exit = %d, want %d", got, ExitOK)
	}
	if got := DiffExitCode(DiffResult{Status: "error", Error: &DiffError{Kind: "network"}}); got != ExitError {
		t.Fatalf("error exit = %d, want %d", got, ExitError)
	}
}

func TestDiffMarkdownAndXMLAreStructured(t *testing.T) {
	result := fixtureDiffResult()
	var md bytes.Buffer
	if err := WriteDiff(&md, FormatMarkdown, result); err != nil {
		t.Fatalf("markdown: %v", err)
	}
	for _, want := range []string{"# gh-hound diff", "scent picked up", "f2b85a7", "indrasvat"} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("markdown missing %q:\n%s", want, md.String())
		}
	}
	var xmlOut bytes.Buffer
	if err := WriteDiff(&xmlOut, FormatXML, result); err != nil {
		t.Fatalf("xml: %v", err)
	}
	if !strings.Contains(xmlOut.String(), "<diff_result") {
		t.Fatalf("xml missing diff_result element:\n%s", xmlOut.String())
	}
}

// The schema is the public contract: $defs.diff_result must exist and
// pin the envelope's required keys and status enum.
func TestSchemaPinsDiffResult(t *testing.T) {
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	diff, ok := schema.Defs["diff_result"]
	if !ok {
		t.Fatal("schema missing $defs.diff_result")
	}
	required := map[string]bool{}
	for _, key := range diff.Required {
		required[key] = true
	}
	for _, key := range []string{"repo", "workflow", "branch", "status", "suspect_commits", "total_suspects", "verdict"} {
		if !required[key] {
			t.Fatalf("diff_result.required missing %q", key)
		}
	}
	statusEnum := string(diff.Properties["status"])
	for _, status := range []string{"located", "green", "inconclusive", "error"} {
		if !strings.Contains(statusEnum, status) {
			t.Fatalf("diff_result status enum missing %q: %s", status, statusEnum)
		}
	}
}
