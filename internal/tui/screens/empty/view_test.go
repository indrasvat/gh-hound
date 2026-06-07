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
