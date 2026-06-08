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
		"Workflow",
		"Event",
		"Duration",
		"▌✗ CI",
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
		"All checks passing on fix/parser",
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
	if !strings.Contains(visible, "▌ ✔       CodeQL") {
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
	if !strings.Contains(visible, "▌ ✔       CI") || !strings.Contains(visible, "rows ") || !strings.Contains(visible, "/1000") {
		t.Fatalf("long all-green list did not show selected viewport/page marker:\n%s", visible)
	}
	if strings.Contains(visible, "1000 recent runs") && strings.Count(visible, "\n") > 18 {
		t.Fatalf("long list rendered beyond requested viewport:\n%s", visible)
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
