package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
)

// A run GitHub could not start has no jobs and never will: the detail
// screen says so instead of promising job data that is not coming.
func TestDetailNamesARunGitHubCouldNotStart(t *testing.T) {
	run := model.Run{ID: 1, Name: "Docs", RunNumber: 574, HeadBranch: "docs/fallback", Status: model.StatusCompleted, Conclusion: model.ConclusionStartupFailure}
	m := NewModel(run, nil).WithRepo("indrasvat/gh-hound")
	for _, width := range []int{78, 118, 198} {
		view := ansi.Strip(ViewSize(m, width, 20))
		if !strings.Contains(view, "GitHub could not start this run") {
			t.Fatalf("width %d does not name the startup failure:\n%s", width, view)
		}
		if strings.Contains(view, "after GitHub returns job data") {
			t.Fatalf("width %d still promises job data:\n%s", width, view)
		}
		for line := range strings.SplitSeq(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
	}

	// Any other zero-job run keeps the generic wording.
	run.Conclusion = model.ConclusionFailure
	view := ansi.Strip(ViewSize(NewModel(run, nil), 118, 20))
	if strings.Contains(view, "could not start") || !strings.Contains(view, "No jobs returned by GitHub") {
		t.Fatalf("a plain zero-job failure changed wording:\n%s", view)
	}
}
