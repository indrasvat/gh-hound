package runs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		raw  string
		want Query
	}{
		{"", Query{}},
		{"actor:@me", Query{Actor: MeActor}},
		{"actor:@ME", Query{Actor: MeActor}},
		{"actor:@alice", Query{Actor: "alice"}},
		{"actor:Alice", Query{Actor: "Alice"}},
		{"actor:dependabot[bot]", Query{Actor: "dependabot[bot]"}},
		{"actor:@me status:failure", Query{Actor: MeActor, Status: "failure"}},
		{"  actor:@me   status:failing  ", Query{Actor: MeActor, Status: "failure"}},
		{"conclusion:passed branch:main event:push", Query{Status: "success", Branch: "main", Event: "push"}},
		{"status:running", Query{Status: "in_progress"}},
		{"sha:A1B2C3D", Query{SHA: "a1b2c3d"}},
		{"head_sha:abc", Query{SHA: "abc"}},
		{"failing", Query{Status: "failure"}},
		{"push", Query{Event: "push"}},
		{"actor:@me fix #571", Query{Actor: MeActor, Text: []string{"fix", "#571"}}},
		{"Fix", Query{Text: []string{"fix"}}},
		{"12:30", Query{Text: []string{"12:30"}}},
		{"feat: bump", Query{Text: []string{"feat:", "bump"}}},
		{"chore(deps):", Query{Text: []string{"chore(deps):"}}},
		{"https://x.test/run", Query{Text: []string{"https://x.test/run"}}},
		{"foo:bar", Query{Text: []string{"foo:bar"}}},
		{"failing passed", Query{Text: []string{"failing", "passed"}}},
		{"deploy green", Query{Text: []string{"deploy", "green"}}},
		{"actor:@me failing", Query{Actor: MeActor, Status: "failure"}},
		{"status:failure push", Query{Status: "failure", Event: "push"}},
	}
	for _, tt := range tests {
		got, err := ParseQuery(tt.raw)
		if err != nil {
			t.Fatalf("ParseQuery(%q) error = %v", tt.raw, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("ParseQuery(%q) = %+v, want %+v", tt.raw, got, tt.want)
		}
	}
}

func TestParseQueryRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"actor:", "actor: needs a value"},
		{"status:", "status: needs a value"},
		{"actor:alice actor:bob", "only one actor: per filter"},
		{"status:failure conclusion:success", "only one status: per filter"},
		{"actor:not_a_login", "isn't a GitHub login"},
		{"actor:-alice", "isn't a GitHub login"},
		{"actor:" + strings.Repeat("a", 40), "isn't a GitHub login"},
		{"status:sideways", `unknown status "sideways"`},
	}
	for _, tt := range tests {
		_, err := ParseQuery(tt.raw)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("ParseQuery(%q) error = %v, want %q", tt.raw, err, tt.want)
		}
	}
}

func TestQueryServerKeyIgnoresTextAndActorCase(t *testing.T) {
	a, _ := ParseQuery("actor:Alice status:failure fix")
	b, _ := ParseQuery("status:failed actor:alice")
	if a.ServerKey() != b.ServerKey() {
		t.Fatalf("keys differ: %q vs %q", a.ServerKey(), b.ServerKey())
	}
	text, _ := ParseQuery("fix #571")
	if text.Server() || text.ServerKey() != "" {
		t.Fatalf("text-only query must be client-side, got key %q", text.ServerKey())
	}
}

func TestQueryMatches(t *testing.T) {
	run := model.Run{
		Name: "CI", DisplayTitle: "fix parser", Actor: "Alice", Status: model.StatusCompleted,
		Conclusion: model.ConclusionFailure, HeadBranch: "main", Event: "push", HeadSHA: "A1B2C3D", RunNumber: 571,
	}
	tests := []struct {
		raw    string
		viewer string
		want   bool
	}{
		{"actor:alice", "", true},
		{"actor:bob", "", false},
		{"actor:@me", "alice", true},
		{"actor:@me", "", false},
		{"actor:@me", "bob", false},
		{"status:failure branch:main event:push sha:a1b", "", true},
		{"status:success", "", false},
		{"branch:release", "", false},
		{"fix 571", "", true},
		{"fix 572", "", false},
	}
	for _, tt := range tests {
		q, err := ParseQuery(tt.raw)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tt.raw, err)
		}
		if got := q.Matches(run, tt.viewer); got != tt.want {
			t.Fatalf("%q viewer=%q Matches = %v, want %v", tt.raw, tt.viewer, got, tt.want)
		}
	}
}

func TestWithTokenReplacesOnlyItsKey(t *testing.T) {
	tests := []struct {
		raw, key, value, want string
	}{
		{"", "status", "failure", "status:failure"},
		{"actor:@me", "status", "failure", "actor:@me status:failure"},
		{"actor:@me status:failure fix", "status", "in_progress", "actor:@me fix status:in_progress"},
		{"failing actor:@me", "status", "success", "actor:@me status:success"},
		{"actor:@me status:success", "status", "", "actor:@me"},
		{"actor:alice status:failure", "actor", "@me", "status:failure actor:@me"},
		{"actor:@me status:failure", "actor", "", "status:failure"},
		{"conclusion:failure", "status", "", ""},
		{"deploy green", "status", "failure", "deploy green status:failure"},
		{"feat: bump", "actor", "@me", "feat: bump actor:@me"},
	}
	for _, tt := range tests {
		if got := WithToken(tt.raw, tt.key, tt.value); got != tt.want {
			t.Fatalf("WithToken(%q, %q, %q) = %q, want %q", tt.raw, tt.key, tt.value, got, tt.want)
		}
	}
}
