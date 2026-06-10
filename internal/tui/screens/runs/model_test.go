package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestModelSelectionFilterAndRouteIntents(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "fix/parser",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{
			run(571, "CI", "pull_request", model.StatusCompleted, model.ConclusionFailure),
			run(570, "CI", "push", model.StatusInProgress, model.ConclusionNone),
			run(569, "Release", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
	})

	m = m.Update(KeyMsg{Key: "j"})
	if m.Selected != 1 || m.Intent.Kind != IntentNone {
		t.Fatalf("j selected=%d intent=%s", m.Selected, m.Intent.Kind)
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentOpenDetail || m.Intent.RunID != 570 {
		t.Fatalf("enter intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "/"})
	if !m.InputMode || m.Filter != "" {
		t.Fatalf("filter mode = %v filter=%q", m.InputMode, m.Filter)
	}
	m = m.Update(KeyMsg{Key: "f"})
	m = m.Update(KeyMsg{Key: "a"})
	m = m.Update(KeyMsg{Key: "i"})
	m = m.Update(KeyMsg{Key: "l"})
	if m.Filter != "fail" {
		t.Fatalf("filter = %q", m.Filter)
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.InputMode || m.Intent.Kind != IntentFilter || m.Intent.Filter != "fail" {
		t.Fatalf("filter submit model = %#v", m)
	}
}

func TestSummaryAndAllGreenVariant(t *testing.T) {
	green := NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		State:  usecase.LaunchStateAllGreen,
		Runs: []model.Run{
			run(569, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
			run(568, "Release", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
	})
	summary := green.Summary()
	if !green.AllGreen() || summary.Failing != 0 || summary.Running != 0 || summary.Passed != 2 {
		t.Fatalf("green summary = %#v allGreen=%v", summary, green.AllGreen())
	}

	mixed := green
	mixed.Context.State = usecase.LaunchStateRuns
	mixed.Context.Runs[0].Conclusion = model.ConclusionFailure
	if mixed.AllGreen() {
		t.Fatalf("mixed runs should not be all-green")
	}
}

func TestAllGreenHeadlineDistinguishesStrictSuccessFromNonFailingRuns(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:   "openclaw/openclaw",
		Branch: "main",
		State:  usecase.LaunchStateAllGreen,
		Runs: []model.Run{
			run(101, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
			run(102, "Command Reactions", "push", model.StatusCompleted, model.ConclusionNeutral),
			run(103, "Dispatch", "push", model.StatusCompleted, model.ConclusionCancelled),
		},
	})
	view := ansi.Strip(ViewSize(m, 100, 14, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC)))
	if !strings.Contains(view, "No failing checks on branch main") {
		t.Fatalf("non-failing mixed conclusions should use precise headline:\n%s", view)
	}
	if strings.Contains(view, "All checks passing on branch main") {
		t.Fatalf("non-success conclusions should not claim all checks passing:\n%s", view)
	}
}

func TestSummaryCountsQueuedWaitingPendingAndRequestedAsRunning(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:   "openclaw/openclaw",
		Branch: "main",
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{
			run(1, "Queued", "push", model.StatusQueued, model.ConclusionNone),
			run(2, "Waiting", "push", model.StatusWaiting, model.ConclusionNone),
			run(3, "Pending", "push", model.StatusPending, model.ConclusionNone),
			run(4, "Requested", "push", model.StatusRequested, model.ConclusionNone),
		},
	})
	if got := m.Summary().Running; got != 4 {
		t.Fatalf("running summary = %d, want queued/waiting/pending/requested counted", got)
	}
	if m.AllGreen() {
		t.Fatalf("queued/waiting/pending/requested runs must not render as all-green")
	}
}

func TestViewUsesRealRunIdentifiersWhenWorkflowNameIsMissing(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:  "openclaw/openclaw",
		State: usecase.LaunchStateRuns,
		Runs: []model.Run{{
			ID:         9001,
			RunNumber:  44,
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionSuccess,
		}},
	})
	view := ansi.Strip(ViewSize(m, 100, 12, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC)))
	if strings.Contains(view, "workflow") || strings.Contains(view, "unknown") {
		t.Fatalf("runs view invented fallback metadata:\n%s", view)
	}
	if !strings.Contains(view, "#44") {
		t.Fatalf("runs view did not render real run number:\n%s", view)
	}
}

func TestViewMatchesRunsAndAllGreenMocks(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "fix/parser",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{
			run(571, "CI", "pull_request", model.StatusCompleted, model.ConclusionFailure),
			run(570, "CI", "push", model.StatusInProgress, model.ConclusionNone),
			run(569, "Release", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
	})
	view := View(m, 80, time.Date(2026, 6, 7, 17, 45, 0, 0, time.UTC))
	visible := ansi.Strip(view)
	for _, want := range []string{
		"Workflow / detail",
		"Event",
		"Dur.",
		"▌#571     ✗ CI",
		"CI · fix/parser",
		"1 failing · 1 running · 1 passed",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("runs view missing %q\n%s", want, visible)
		}
	}
	assertMaxWidth(t, view, 80)

	m.Context.State = usecase.LaunchStateAllGreen
	for i := range m.Context.Runs {
		m.Context.Runs[i].Status = model.StatusCompleted
		m.Context.Runs[i].Conclusion = model.ConclusionSuccess
	}
	view = View(m, 80, time.Date(2026, 6, 7, 17, 45, 0, 0, time.UTC))
	visible = ansi.Strip(view)
	for _, want := range []string{
		"✔",
		"All checks passing on branch fix/parser",
		"3 recent runs · 0 failing",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("all-green view missing %q\n%s", want, visible)
		}
	}
	assertMaxWidth(t, view, 80)
}

func TestAllGreenViewKeepsSelectedRowAndFilterVisible(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		State:  usecase.LaunchStateAllGreen,
		Runs: []model.Run{
			run(12, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
			run(11, "CodeQL", "schedule", model.StatusCompleted, model.ConclusionSuccess),
			run(10, "Docs", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
	})
	m = m.Update(KeyMsg{Key: "down"})
	view := ViewSize(m, 80, 12, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC))
	visible := ansi.Strip(view)
	if !strings.Contains(visible, "▌ #11") || !strings.Contains(visible, "CodeQL · fix/parser") {
		t.Fatalf("all-green view did not highlight selected row:\n%s", visible)
	}

	m = m.Update(KeyMsg{Key: "/"})
	m = m.Update(KeyMsg{Key: "d"})
	m = m.Update(KeyMsg{Key: "o"})
	m = m.Update(KeyMsg{Key: "c"})
	view = ViewSize(m, 80, 12, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC))
	visible = ansi.Strip(view)
	if !strings.Contains(visible, "/doc  1 matches") || !strings.Contains(visible, "Docs") || strings.Contains(visible, "CodeQL") {
		t.Fatalf("filter prompt/results not reflected in all-green view:\n%s", visible)
	}
}

func TestScopeToggleSwitchesBetweenBranchAndRepoRuns(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:   "openclaw/openclaw",
		Branch: "main",
		Actor:  "indrasvat",
		Scope:  usecase.LaunchScopeBranch,
		State:  usecase.LaunchStateAllGreen,
		Notice: "repo activity: 1 running across release/2026.6.5 · s scope",
		BranchRuns: []model.Run{
			run(101, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
		RepoRuns: []model.Run{
			run(202, "Release", "workflow_dispatch", model.StatusInProgress, model.ConclusionNone),
			run(101, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
	})
	view := ansi.Strip(ViewSize(m, 100, 14, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC)))
	if !strings.Contains(view, "All checks passing on branch main") || !strings.Contains(view, "repo activity") {
		t.Fatalf("branch scope did not show all-green notice:\n%s", view)
	}

	m = m.Update(KeyMsg{Key: "s"})
	if m.Context.Scope != usecase.LaunchScopeRepo {
		t.Fatalf("scope = %s, want repo", m.Context.Scope)
	}
	view = ansi.Strip(ViewSize(m, 100, 14, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC)))
	if !strings.Contains(view, "Release") || !strings.Contains(view, "1 running") || strings.Contains(view, "All checks passing") {
		t.Fatalf("repo scope did not show repo activity rows:\n%s", view)
	}
	if strings.Contains(view, "repo activity") {
		t.Fatalf("repo scope should not repeat branch-scope activity notice:\n%s", view)
	}
}

func TestRunsViewVirtualizesLongListsAroundSelection(t *testing.T) {
	runs := make([]model.Run, 1000)
	for i := range runs {
		number := 1000 - i
		runs[i] = run(number, "CI", "push", model.StatusCompleted, model.ConclusionSuccess)
	}
	m := NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		State:  usecase.LaunchStateAllGreen,
		Runs:   runs,
	})
	m.Selected = 500
	view := ViewSize(m, 120, 18, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC))
	visible := ansi.Strip(view)
	if !strings.Contains(visible, "▌ #500") || !strings.Contains(visible, "CI · fix/parser") || !strings.Contains(visible, "rows ") || !strings.Contains(visible, "/1000") {
		t.Fatalf("long all-green list did not show selected viewport/page marker:\n%s", visible)
	}
	if strings.Contains(visible, "1000 recent runs") && strings.Count(visible, "\n") > 18 {
		t.Fatalf("long list rendered beyond requested viewport:\n%s", visible)
	}
}

func TestRunsViewShowsMorePagesAffordance(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo:    "openclaw/openclaw",
		Scope:   usecase.LaunchScopeRepo,
		State:   usecase.LaunchStateRuns,
		PerPage: 3,
		Runs: []model.Run{
			run(3, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
			run(2, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
			run(1, "CI", "push", model.StatusCompleted, model.ConclusionSuccess),
		},
	})
	view := ansi.Strip(ViewSize(m, 100, 12, time.Date(2026, 6, 8, 21, 42, 0, 0, time.UTC)))
	if !strings.Contains(view, "G load more") || !strings.Contains(view, "/3+") {
		t.Fatalf("more-pages affordance missing:\n%s", view)
	}
}

func TestAllGreenBandReappliesBackgroundAfterNestedReset(t *testing.T) {
	line := allGreenBandLine(sgrOK+"✔"+sgrReset+" All checks passing", 40)
	if !strings.HasPrefix(line, sgrBandFG+sgrBandBG) {
		t.Fatalf("band should start with fg+bg SGR: %q", line)
	}
	if !strings.Contains(line, sgrReset+sgrBandFG+sgrBandBG) {
		t.Fatalf("band should reapply fg+bg after nested reset: %q", line)
	}
	if !strings.HasSuffix(line, sgrReset) {
		t.Fatalf("band should reset once at final boundary: %q", line)
	}
}

func assertMaxWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line too wide (%d): %q\n%s", got, ansi.Strip(line), view)
		}
	}
}

func run(number int, name, event string, status model.Status, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:           int64(number),
		Name:         name,
		Event:        event,
		Status:       status,
		Conclusion:   conclusion,
		HeadBranch:   "fix/parser",
		RunNumber:    number,
		RunStartedAt: time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 7, 17, 44, 0, 0, time.UTC),
	}
}
