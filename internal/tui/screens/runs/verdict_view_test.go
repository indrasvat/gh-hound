package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func verdictRows() Model {
	at := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	mk := func(number int, name, title, event string, conclusion model.Conclusion, took time.Duration) model.Run {
		return model.Run{ID: int64(number), RunNumber: number, Name: name, DisplayTitle: title, Event: event,
			Status: model.StatusCompleted, Conclusion: conclusion, Actor: "new-dev",
			RunStartedAt: at, UpdatedAt: at.Add(took)}
	}
	held := mk(9412, "Unit tests", "fix typo in docs", "pull_request", model.ConclusionActionRequired, 0)
	held.FromFork = true
	missed := mk(9408, "Unit tests", "add arm64 build", "pull_request", model.ConclusionFailure, 4*time.Hour+14*time.Minute)
	lost := mk(9392, "Integration", "docker 29 bump", "push", model.ConclusionFailure, 8*time.Minute+55*time.Second)
	m := NewModel(usecase.LaunchContext{Repo: "acme/forge", State: usecase.LaunchStateRuns, Scope: usecase.LaunchScopeRepo, Runs: []model.Run{
		held, missed,
		mk(9405, "Unit tests", "cache config parse", "push", model.ConclusionFailure, 3*time.Minute+41*time.Second),
		mk(9401, "Labeler", "triage new issues", "pull_request_target", model.ConclusionStartupFailure, 0),
		lost,
		mk(9396, "Release", "v3.2.0", "push", model.ConclusionSuccess, 6*time.Minute+2*time.Second),
	}})
	m.Context.RepoRuns = m.Context.Runs
	m.Verdicts = map[int64]model.RunVerdict{
		missed.ID: {Verdict: model.VerdictNeverRan, Reason: model.ReasonApprovalMissed},
		lost.ID:   {Verdict: model.VerdictInfra, Reason: model.ReasonRunnerLost},
		9405:      {Verdict: model.VerdictCode},
	}
	return m
}

func lineFor(t *testing.T, view string, number string) string {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, number) {
			return line
		}
	}
	t.Fatalf("no row %s in\n%s", number, view)
	return ""
}

// A run that never ran shows why instead of a made-up duration, at
// every breakpoint, and the summary counts it apart from failures.
func TestRunsListSaysWhyARedRunIsRed(t *testing.T) {
	now := time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC)
	for _, width := range []int{78, 118, 198} {
		view := ansi.Strip(ViewSize(verdictRows(), width, 20, now))
		for line := range strings.SplitSeq(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
		held := lineFor(t, view, "#9412")
		if !strings.Contains(held, "∅") || !strings.Contains(held, "approve") {
			t.Errorf("width %d held row = %q, want ∅ and approve", width, held)
		}
		missed := lineFor(t, view, "#9408")
		if !strings.Contains(missed, "missed") || strings.Contains(missed, "4h14m") {
			t.Errorf("width %d missed row = %q, want missed and no duration", width, missed)
		}
		blocked := lineFor(t, view, "#9401")
		if !strings.Contains(blocked, "∅") || !strings.Contains(blocked, "blocked") {
			t.Errorf("width %d blocked row = %q, want ∅ and the reason", width, blocked)
		}
		lost := lineFor(t, view, "#9392")
		if !strings.Contains(lost, "↯") || !strings.Contains(lost, "runner lost") {
			t.Errorf("width %d infra row = %q, want ↯ and runner lost", width, lost)
		}
		code := lineFor(t, view, "#9405")
		if !strings.Contains(code, "✗") {
			t.Errorf("width %d code row = %q, want ✗", width, code)
		}
		summary := lineFor(t, view, "failing")
		for _, want := range []string{"1 failing", "1 infra", "1 need approval", "2 never ran"} {
			if !strings.Contains(summary, want) {
				t.Errorf("width %d summary = %q, want %q", width, summary, want)
			}
		}
	}
}

// The new words wear theme colours: warn for a run waiting on a person,
// neutral for one nobody can act on, info for a runner failure.
func TestRunsListColoursVerdictsWithThemeTokens(t *testing.T) {
	m := verdictRows()
	st := stylesFor(m.Theme.OrBramble())
	view := ViewSize(m, 118, 20, time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC))
	for _, want := range []string{st.warn + "approve", st.neutral + "missed", st.info + "runner lost", st.info + "↯", st.warn + "∅"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}

// At 80 columns the paging hint outranks the A affordance and idle
// counts: a long red list must still say G loads more.
func TestSummaryKeepsTheLoadMoreHintAtEightyColumns(t *testing.T) {
	st := stylesFor(verdictRows().Theme.OrBramble())
	summary := Summary{NeedApproval: 2, Approvable: 2, NeverRan: 5}
	line := ansi.Strip(summaryLine(st, summary, true, 78-len("rows 113-127/150+")-2))
	if !strings.Contains(line, "G load more") || !strings.Contains(line, "2 need approval") || !strings.Contains(line, "5 never ran") {
		t.Fatalf("summary at 80 = %q", line)
	}
	if wide := ansi.Strip(summaryLine(st, summary, true, 200)); !strings.Contains(wide, "A approve") {
		t.Fatalf("summary with room = %q, want A approve", wide)
	}
}
