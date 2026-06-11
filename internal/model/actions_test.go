package model

import (
	"encoding/json"
	"testing"
)

func TestStatusValidationAcceptsOnlyGitHubValues(t *testing.T) {
	for _, raw := range []string{"queued", "in_progress", "completed", "requested", "waiting", "pending"} {
		status, err := ParseStatus(raw)
		if err != nil {
			t.Fatalf("ParseStatus(%q) returned error: %v", raw, err)
		}
		if string(status) != raw {
			t.Fatalf("ParseStatus(%q) = %q", raw, status)
		}
		if !status.Valid() {
			t.Fatalf("status %q should be valid", status)
		}
	}

	if _, err := ParseStatus("running"); err == nil {
		t.Fatal("ParseStatus accepted invented status value running")
	}
}

func TestConclusionValidationAcceptsNullAsEmptyConclusion(t *testing.T) {
	for _, raw := range []string{"success", "failure", "cancelled", "skipped", "neutral", "timed_out", "action_required", "stale"} {
		conclusion, err := ParseConclusion(raw)
		if err != nil {
			t.Fatalf("ParseConclusion(%q) returned error: %v", raw, err)
		}
		if string(conclusion) != raw {
			t.Fatalf("ParseConclusion(%q) = %q", raw, conclusion)
		}
		if !conclusion.Valid() {
			t.Fatalf("conclusion %q should be valid", conclusion)
		}
	}

	conclusion, err := ParseConclusion("null")
	if err != nil {
		t.Fatalf("ParseConclusion(null) returned error: %v", err)
	}
	if conclusion != ConclusionNone {
		t.Fatalf("ParseConclusion(null) = %q, want empty ConclusionNone", conclusion)
	}

	if _, err := ParseConclusion("red"); err == nil {
		t.Fatal("ParseConclusion accepted invented conclusion value red")
	}
}

func TestConclusionJSONNullRoundTrip(t *testing.T) {
	type payload struct {
		Conclusion Conclusion `json:"conclusion"`
	}

	var got payload
	if err := json.Unmarshal([]byte(`{"conclusion":null}`), &got); err != nil {
		t.Fatalf("unmarshal null conclusion: %v", err)
	}
	if got.Conclusion != ConclusionNone {
		t.Fatalf("null conclusion = %q, want ConclusionNone", got.Conclusion)
	}

	data, err := json.Marshal(payload{Conclusion: ConclusionNone})
	if err != nil {
		t.Fatalf("marshal none conclusion: %v", err)
	}
	if string(data) != `{"conclusion":null}` {
		t.Fatalf("marshal none conclusion = %s", data)
	}
}

func TestWorkflowToggleabilityCoversAllDocumentedStates(t *testing.T) {
	cases := []struct {
		state string
		want  bool
	}{
		{WorkflowStateActive, true},
		{WorkflowStateDisabledManually, true},
		{WorkflowStateDisabledInactivity, true},
		{WorkflowStateDisabledFork, false},
		{WorkflowStateDeleted, false},
		// Open-string semantics: unknown future states are rendered
		// verbatim elsewhere and never guessed at here.
		{"disabled_by_dependabot_v9", false},
		{"", false},
	}
	for _, tc := range cases {
		workflow := Workflow{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: tc.state}
		if got := workflow.Toggleable(); got != tc.want {
			t.Fatalf("Workflow{State: %q}.Toggleable() = %t, want %t", tc.state, got, tc.want)
		}
	}
}
