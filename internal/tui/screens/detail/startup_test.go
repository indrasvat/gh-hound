package detail

import (
	"strings"
	"testing"
	"time"

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

	// A failed push run with no job says it never started, not that it
	// could not start; a run that is not red keeps the generic wording.
	run.Conclusion, run.Event = model.ConclusionFailure, "push"
	view := ansi.Strip(ViewSize(NewModel(run, nil), 118, 20))
	if strings.Contains(view, "could not start") || !strings.Contains(view, "before any job started") {
		t.Fatalf("a zero-job failure does not say no job started:\n%s", view)
	}
	run.Conclusion = model.ConclusionCancelled
	view = ansi.Strip(ViewSize(NewModel(run, nil), 118, 20))
	if !strings.Contains(view, "No jobs returned by GitHub") {
		t.Fatalf("a cancelled zero-job run changed wording:\n%s", view)
	}
}

// A held fork run names who has to act, how long it has waited and how
// many runs wait with it, and offers A in place of keys that can't help.
func TestDetailOffersToApproveAHeldForkRun(t *testing.T) {
	now := time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC)
	run := model.Run{ID: 1, Name: "Unit tests", RunNumber: 9412, HeadBranch: "patch-1", Event: "pull_request", FromFork: true,
		Status: model.StatusCompleted, Conclusion: model.ConclusionActionRequired, CreatedAt: now.Add(-2 * time.Hour)}
	m := NewModel(run, nil).WithRepo("acme/forge")
	m.Now, m.HeldRuns = now, 2
	for _, width := range []int{78, 118, 198} {
		view := ansi.Strip(ViewSize(m, width, 20))
		for _, want := range []string{"Why this run never ran", "Waiting for a maintainer to approve", "waiting 2h", "2 runs held on this commit", "A approve and run"} {
			if !strings.Contains(view, want) {
				t.Fatalf("width %d lacks %q:\n%s", width, want, view)
			}
		}
		for line := range strings.SplitSeq(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
	}
	// Not from a fork: GitHub's endpoint cannot approve it, so no A.
	m.Run.FromFork = false
	if view := ansi.Strip(ViewSize(m, 118, 20)); strings.Contains(view, "A approve") {
		t.Fatalf("a held non-fork run offers A:\n%s", view)
	}
}
