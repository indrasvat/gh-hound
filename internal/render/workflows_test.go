package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixtureWorkflowsResult() WorkflowsResult {
	return WorkflowsResult{
		Repo: "indrasvat/gh-hound",
		Workflows: []WorkflowInfo{
			{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
			{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: "disabled_inactivity"},
		},
	}
}

func TestWorkflowsJSONCarriesStateVerbatim(t *testing.T) {
	var out bytes.Buffer
	if err := WriteWorkflows(&out, FormatJSON, fixtureWorkflowsResult()); err != nil {
		t.Fatalf("json: %v", err)
	}
	var decoded struct {
		Repo      string `json:"repo"`
		Workflows []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Path  string `json:"path"`
			State string `json:"state"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Repo != "indrasvat/gh-hound" || len(decoded.Workflows) != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}
	if decoded.Workflows[1].State != "disabled_inactivity" {
		t.Fatalf("state = %q", decoded.Workflows[1].State)
	}
}

func TestWorkflowsToggleEnvelopeWritesAcceptedAndRefusal(t *testing.T) {
	accepted := true
	result := WorkflowsResult{
		Repo:      "indrasvat/gh-hound",
		Workflows: []WorkflowInfo{},
		Accepted:  &accepted,
		Toggled:   &WorkflowToggle{Target: "ci.yml", Action: "enable", State: "active"},
	}
	var out bytes.Buffer
	if err := WriteWorkflows(&out, FormatJSON, result); err != nil {
		t.Fatalf("json: %v", err)
	}
	for _, want := range []string{`"accepted": true`, `"target": "ci.yml"`, `"action": "enable"`, `"state": "active"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("toggle envelope missing %s:\n%s", want, out.String())
		}
	}

	refusedFlag := false
	refusal := WorkflowsResult{
		Repo:      "indrasvat/gh-hound",
		Workflows: []WorkflowInfo{},
		Accepted:  &refusedFlag,
		Error:     &MutationError{Kind: "validation", Field: "workflow", Message: "not a toggle selector"},
	}
	out.Reset()
	if err := WriteWorkflows(&out, FormatJSON, refusal); err != nil {
		t.Fatalf("json refusal: %v", err)
	}
	for _, want := range []string{`"accepted": false`, `"kind": "validation"`, `"field": "workflow"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("refusal envelope missing %s:\n%s", want, out.String())
		}
	}
}

func TestWorkflowsMarkdownAndXMLAreStructured(t *testing.T) {
	result := fixtureWorkflowsResult()
	var md bytes.Buffer
	if err := WriteWorkflows(&md, FormatMarkdown, result); err != nil {
		t.Fatalf("markdown: %v", err)
	}
	for _, want := range []string{"# gh-hound workflows", "Nightly Sweep", "disabled_inactivity"} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("markdown missing %q:\n%s", want, md.String())
		}
	}
	var xmlOut bytes.Buffer
	if err := WriteWorkflows(&xmlOut, FormatXML, result); err != nil {
		t.Fatalf("xml: %v", err)
	}
	if !strings.Contains(xmlOut.String(), "<workflows_result") || !strings.Contains(xmlOut.String(), "disabled_inactivity") {
		t.Fatalf("xml missing structure:\n%s", xmlOut.String())
	}
}

// The schema is the public contract: $defs.workflows_result must pin
// the envelope's required keys, keep `state` an OPEN string (unknown
// future states pass through verbatim, never rejected), and pin the
// toggle action and error-kind enums.
func TestSchemaPinsWorkflowsResult(t *testing.T) {
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
	workflows, ok := schema.Defs["workflows_result"]
	if !ok {
		t.Fatal("schema missing $defs.workflows_result")
	}
	required := map[string]bool{}
	for _, key := range workflows.Required {
		required[key] = true
	}
	for _, key := range []string{"repo", "workflows"} {
		if !required[key] {
			t.Fatalf("workflows_result.required missing %q", key)
		}
	}
	items := string(workflows.Properties["workflows"])
	for _, want := range []string{`"id"`, `"name"`, `"path"`, `"state"`} {
		if !strings.Contains(items, want) {
			t.Fatalf("workflows items missing %s: %s", want, items)
		}
	}
	if strings.Contains(items, `"enum"`) {
		t.Fatalf("workflow state must stay an open string, found enum: %s", items)
	}
	toggled := string(workflows.Properties["toggled"])
	for _, action := range []string{"enable", "disable"} {
		if !strings.Contains(toggled, action) {
			t.Fatalf("toggled action enum missing %q: %s", action, toggled)
		}
	}
	errorProp := string(workflows.Properties["error"])
	for _, kind := range []string{"validation", "permission", "conflict", "rate_limit", "network", "unknown"} {
		if !strings.Contains(errorProp, kind) {
			t.Fatalf("error kind enum missing %q: %s", kind, errorProp)
		}
	}
}
