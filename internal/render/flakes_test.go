package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixtureFlakesResult() FlakesResult {
	return FlakesResult{
		Repo:             "indrasvat/gh-hound",
		Workflow:         "ci.yml",
		Branch:           "main",
		Status:           "flaky",
		SampleSize:       48,
		Window:           50,
		RunsScanned:      50,
		SignalsEvaluated: []string{"attempt_flips", "cross_run_flaps", "retry_masks"},
		Jobs: []FlakeJob{
			{
				Job:        "build",
				FlakeScore: 0.9,
				Verdict:    "flaky",
				Flips:      2,
				Flaps:      0,
				Masks:      0,
				FlakedRuns: 2,
				Evidence: []FlakeEvidence{
					{RunID: 2110, RunNumber: 110, Attempt: 1, Kind: "attempt_flip", Detail: "#110: failed on attempt 1, passed on a later attempt"},
					{RunID: 2106, RunNumber: 106, Attempt: 1, Kind: "attempt_flip", Detail: "#106: failed on attempt 1, passed on a later attempt"},
				},
			},
		},
		Verdict: "seen this one before: it's a squirrel — build flaked 2 of the last 50 runs.",
	}
}

func TestFlakesJSONMatchesPublicContract(t *testing.T) {
	var out bytes.Buffer
	if err := WriteFlakes(&out, FormatJSON, fixtureFlakesResult()); err != nil {
		t.Fatalf("WriteFlakes returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{"repo", "workflow", "branch", "status", "sample_size", "window", "runs_scanned", "signals_evaluated", "jobs", "verdict"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("flakes result missing key %q in %s", key, out.String())
		}
	}
	job := decoded["jobs"].([]any)[0].(map[string]any)
	for _, key := range []string{"job", "flake_score", "verdict", "attempt_flips", "cross_run_flaps", "retry_masks", "flaked_runs", "evidence"} {
		if _, ok := job[key]; !ok {
			t.Fatalf("flake job missing key %q in %s", key, out.String())
		}
	}
	evidence := job["evidence"].([]any)[0].(map[string]any)
	for _, key := range []string{"run_id", "run_number", "attempt", "kind", "detail"} {
		if _, ok := evidence[key]; !ok {
			t.Fatalf("flake evidence missing key %q in %s", key, out.String())
		}
	}
}

func TestFlakesCleanKeepsEmptyJobsArray(t *testing.T) {
	var out bytes.Buffer
	result := FlakesResult{
		Repo: "o/r", Workflow: "ci.yml", Branch: "main",
		Status:  "clean",
		Verdict: "fresh scent — worth chasing: no flake history in the last 50 runs.",
	}
	if err := WriteFlakes(&out, FormatJSON, result); err != nil {
		t.Fatalf("WriteFlakes returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if jobs, ok := decoded["jobs"].([]any); !ok || len(jobs) != 0 {
		t.Fatalf("jobs must be an empty array, got %v", decoded["jobs"])
	}
	if signals, ok := decoded["signals_evaluated"].([]any); !ok || len(signals) != 0 {
		t.Fatalf("signals_evaluated must be an empty array, got %v", decoded["signals_evaluated"])
	}
}

func TestFlakesErrorEnvelope(t *testing.T) {
	var out bytes.Buffer
	result := FlakesResult{
		Repo: "o/r", Workflow: "ci.yml", Branch: "main", Status: "error",
		Error: &FlakeError{Kind: "not_found", Message: "no workflow named \"ghost\" in this yard"},
	}
	if err := WriteFlakes(&out, FormatJSON, result); err != nil {
		t.Fatalf("WriteFlakes returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "not_found" {
		t.Fatalf("error.kind = %v", errObj["kind"])
	}
}

// Exit contract: 0 no action (clean OR insufficient_data — status is
// the source of truth), 1 action needed (flaky OR suspect — rerun vs
// investigate is distinguished in JSON), 2 typed error. 3 is NOT used.
func TestFlakesExitCodes(t *testing.T) {
	if got := FlakesExitCode(FlakesResult{Status: "flaky"}); got != ExitActionNeeded {
		t.Fatalf("flaky exit = %d, want %d", got, ExitActionNeeded)
	}
	if got := FlakesExitCode(FlakesResult{Status: "suspect"}); got != ExitActionNeeded {
		t.Fatalf("suspect exit = %d, want %d", got, ExitActionNeeded)
	}
	if got := FlakesExitCode(FlakesResult{Status: "clean"}); got != ExitOK {
		t.Fatalf("clean exit = %d, want %d", got, ExitOK)
	}
	if got := FlakesExitCode(FlakesResult{Status: "insufficient_data"}); got != ExitOK {
		t.Fatalf("insufficient_data exit = %d, want %d", got, ExitOK)
	}
	if got := FlakesExitCode(FlakesResult{Status: "error", Error: &FlakeError{Kind: "network"}}); got != ExitError {
		t.Fatalf("error exit = %d, want %d", got, ExitError)
	}
	// Pinned by the spec: an underfilled window with clear flips is
	// still flaky — the verdict, not the sample size, drives the exit.
	if got := FlakesExitCode(FlakesResult{Status: "flaky", SampleSize: 2, Window: 50}); got != ExitActionNeeded {
		t.Fatalf("underfilled flaky exit = %d, want %d", got, ExitActionNeeded)
	}
}

func TestFlakesMarkdownAndXMLAreStructured(t *testing.T) {
	result := fixtureFlakesResult()
	var md bytes.Buffer
	if err := WriteFlakes(&md, FormatMarkdown, result); err != nil {
		t.Fatalf("markdown: %v", err)
	}
	for _, want := range []string{"# gh-hound flakes", "squirrel", "build", "attempt_flip"} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("markdown missing %q:\n%s", want, md.String())
		}
	}
	var xmlOut bytes.Buffer
	if err := WriteFlakes(&xmlOut, FormatXML, result); err != nil {
		t.Fatalf("xml: %v", err)
	}
	if !strings.Contains(xmlOut.String(), "<flakes_result") {
		t.Fatalf("xml missing flakes_result element:\n%s", xmlOut.String())
	}
}

// The schema is the public contract: $defs.flakes_result must exist,
// pin the envelope's required keys, the status enum agents branch on,
// the typed-error taxonomy (not_found included), and document both
// the scoring thresholds and the annotations-latest-attempt caveat.
func TestSchemaPinsFlakesResult(t *testing.T) {
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			Description string                     `json:"description"`
			Required    []string                   `json:"required"`
			Properties  map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	flakes, ok := schema.Defs["flakes_result"]
	if !ok {
		t.Fatal("schema missing $defs.flakes_result")
	}
	required := map[string]bool{}
	for _, key := range flakes.Required {
		required[key] = true
	}
	for _, key := range []string{"repo", "workflow", "branch", "status", "sample_size", "window", "runs_scanned", "signals_evaluated", "jobs", "verdict"} {
		if !required[key] {
			t.Fatalf("flakes_result.required missing %q", key)
		}
	}
	statusEnum := string(flakes.Properties["status"])
	for _, status := range []string{"flaky", "suspect", "clean", "insufficient_data", "error"} {
		if !strings.Contains(statusEnum, status) {
			t.Fatalf("flakes_result status enum missing %q: %s", status, statusEnum)
		}
	}
	errorDef := string(flakes.Properties["error"])
	if !strings.Contains(errorDef, "not_found") {
		t.Fatalf("flakes_result error kind enum missing not_found: %s", errorDef)
	}
	for _, needle := range []string{"0.45", "0.30", "0.20", "0.6", "latest attempt"} {
		if !strings.Contains(flakes.Description, needle) {
			t.Fatalf("flakes_result description must document thresholds and the annotations caveat; missing %q in %q", needle, flakes.Description)
		}
	}
}
