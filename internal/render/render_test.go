package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestJSONRendererMatchesAppendixBShape(t *testing.T) {
	result := fixtureResult()
	var out bytes.Buffer

	if err := Write(&out, FormatJSON, result); err != nil {
		t.Fatalf("Write JSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON is invalid: %v\n%s", err, out.String())
	}

	if decoded["repo"] != "indrasvat/gh-ghent" {
		t.Fatalf("repo = %v", decoded["repo"])
	}
	runs := decoded["runs"].([]any)
	run := runs[0].(map[string]any)
	for _, key := range []string{"id", "workflow", "run_number", "event", "head_branch", "head_sha", "status", "conclusion", "created_at", "html_url", "failed"} {
		if _, ok := run[key]; !ok {
			t.Fatalf("run missing key %q in %s", key, out.String())
		}
	}
	failed := run["failed"].([]any)[0].(map[string]any)
	for _, key := range []string{"job", "step", "exit_code", "annotations", "log_excerpt"} {
		if _, ok := failed[key]; !ok {
			t.Fatalf("failure missing key %q in %s", key, out.String())
		}
	}
}

func TestJSONRendererMatchesGoldenFixture(t *testing.T) {
	var out bytes.Buffer
	if err := Write(&out, FormatJSON, fixtureResult()); err != nil {
		t.Fatalf("Write JSON returned error: %v", err)
	}
	want, err := os.ReadFile("testdata/failure.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if strings.TrimSpace(out.String()) != strings.TrimSpace(string(want)) {
		t.Fatalf("JSON golden mismatch\nwant:\n%s\ngot:\n%s", string(want), out.String())
	}
}

func TestMarkdownAndXMLRenderersAreStructured(t *testing.T) {
	result := fixtureResult()

	var md bytes.Buffer
	if err := Write(&md, FormatMarkdown, result); err != nil {
		t.Fatalf("Write markdown returned error: %v", err)
	}
	for _, want := range []string{"# gh-hound", "indrasvat/gh-ghent", "CI #571", "go test ./..."} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("markdown missing %q\n%s", want, md.String())
		}
	}

	var xmlOut bytes.Buffer
	if err := Write(&xmlOut, FormatXML, result); err != nil {
		t.Fatalf("Write XML returned error: %v", err)
	}
	for _, want := range []string{`<result`, `repo="indrasvat/gh-ghent"`, `<run`, `<failure`} {
		if !strings.Contains(xmlOut.String(), want) {
			t.Fatalf("XML missing %q\n%s", want, xmlOut.String())
		}
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name string
		res  Result
		err  error
		want int
	}{
		{name: "ok", res: Result{Runs: []Run{{Conclusion: "success", Status: "completed"}}}, want: ExitOK},
		{name: "action needed", res: Result{Runs: []Run{{Conclusion: "failure", Status: "completed"}}}, want: ExitActionNeeded},
		{name: "pending", res: Result{Runs: []Run{{Status: "in_progress"}}}, want: ExitPending},
		{name: "error", err: errFixture{}, want: ExitError},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.res, tt.err); got != tt.want {
			t.Fatalf("%s exit = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func fixtureResult() Result {
	createdAt := time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC)
	return Result{
		Repo:   "indrasvat/gh-ghent",
		Branch: "fix/parser",
		Runs: []Run{{
			ID:         30433642,
			Workflow:   "CI",
			RunNumber:  571,
			Event:      "pull_request",
			HeadBranch: "fix/parser",
			HeadSHA:    "a1b2c3d",
			Status:     "completed",
			Conclusion: "failure",
			CreatedAt:  createdAt,
			HTMLURL:    "https://github.com/indrasvat/gh-ghent/actions/runs/30433642",
			Failed: []Failure{{
				Job:      "build",
				Step:     "go test ./...",
				ExitCode: 1,
				Annotations: []Annotation{{
					Path:    "internal/parser/lexer.go",
					Line:    142,
					Level:   "failure",
					Message: "identifier mismatch",
				}},
				LogExcerpt: "--- FAIL: TestLexIdent/trailing_underscore ...",
			}},
		}},
	}
}

type errFixture struct{}

func (errFixture) Error() string { return "boom" }

func TestWriteMutationFormats(t *testing.T) {
	result := MutationResult{
		Repo:     "indrasvat/gh-hound",
		RunID:    571,
		JobID:    399,
		Action:   "rerun_job",
		Accepted: true,
		HTMLURL:  "https://github.com/indrasvat/gh-hound/actions/runs/571",
	}
	var jsonOut bytes.Buffer
	if err := WriteMutation(&jsonOut, FormatJSON, result); err != nil {
		t.Fatalf("json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	for _, key := range []string{"repo", "run_id", "job_id", "action", "accepted", "html_url"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("json missing %q: %s", key, jsonOut.String())
		}
	}
	// job_id is omitted for run-level mutations.
	var runLevel bytes.Buffer
	runResult := result
	runResult.JobID = 0
	runResult.Action = "rerun"
	if err := WriteMutation(&runLevel, FormatJSON, runResult); err != nil {
		t.Fatalf("json run-level: %v", err)
	}
	if strings.Contains(runLevel.String(), "job_id") {
		t.Fatalf("run-level mutation leaked job_id: %s", runLevel.String())
	}
	var mdOut bytes.Buffer
	if err := WriteMutation(&mdOut, FormatMarkdown, result); err != nil {
		t.Fatalf("md: %v", err)
	}
	if !strings.Contains(mdOut.String(), "rerun_job") || !strings.Contains(mdOut.String(), "actions/runs/571") {
		t.Fatalf("md output = %s", mdOut.String())
	}
	var xmlOut bytes.Buffer
	if err := WriteMutation(&xmlOut, FormatXML, result); err != nil {
		t.Fatalf("xml: %v", err)
	}
	if !strings.Contains(xmlOut.String(), "<mutation_result") || !strings.Contains(xmlOut.String(), `action="rerun_job"`) {
		t.Fatalf("xml output = %s", xmlOut.String())
	}
}
