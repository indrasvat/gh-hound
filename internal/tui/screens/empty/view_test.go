package empty

import (
	"strings"
	"testing"
)

func TestViewFitsEmptyAndErrorStates(t *testing.T) {
	for _, model := range []Model{
		{Kind: KindNoWorkflows, Repo: "indrasvat/gh-hound"},
		{Kind: KindNoRepository, Message: "suggest gh hound -R owner/repo"},
		{Kind: KindNoRuns, Repo: "indrasvat/gh-hound", Branch: "fix/parser"},
		{Kind: KindError, Message: "github api rate limit exceeded"},
	} {
		view := View(model, 80)
		for _, want := range []string{"hound", model.Title()} {
			if !strings.Contains(view, want) {
				t.Fatalf("view missing %q\n%s", want, view)
			}
		}
		for line := range strings.SplitSeq(view, "\n") {
			if len([]rune(line)) > 80 {
				t.Fatalf("line too wide (%d): %q\n%s", len([]rune(line)), line, view)
			}
		}
	}
}

// Task 280: the launch notice (which carries the disabled-workflows
// answer) must actually reach the empty screen, not just the model.
func TestViewSurfacesNoticeOnEmptyStates(t *testing.T) {
	for _, kind := range []Kind{KindNoRuns, KindNoWorkflows} {
		view := View(Model{
			Kind:    kind,
			Repo:    "indrasvat/gh-hound",
			Branch:  "main",
			Message: "no workflow runs yet for indrasvat/gh-hound · asleep: Nightly Sweep — :workflows holds the leash",
		}, 100)
		if !strings.Contains(view, "asleep: Nightly Sweep — :workflows holds the leash") {
			t.Fatalf("%s view missing the notice:\n%s", kind, view)
		}
	}
}
