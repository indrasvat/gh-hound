package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func successRun(id int64, number int, name string) model.Run {
	return model.Run{
		ID: id, RunNumber: number, Name: name,
		Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess,
		Event: "push", HeadBranch: "main",
		RunStartedAt: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 10, 9, 2, 0, 0, time.UTC),
	}
}

// Issue #12: a filtered view must keep the standard table columns even
// when every match is green; the all-green celebration is a home state,
// not a filter-result state.
func TestFilteredAllGreenKeepsStandardTableColumns(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo: "openclaw/openclaw", State: usecase.LaunchStateRuns,
		Runs: []model.Run{successRun(1, 100, "CI"), successRun(2, 101, "Release")},
	})
	m.Filter = "passed"

	view := ansi.Strip(ViewSize(m, 120, 30, time.Date(2026, 6, 10, 9, 5, 0, 0, time.UTC)))
	if strings.Contains(view, "All checks passing") {
		t.Fatalf("filtered results must not render the all-green band:\n%s", view)
	}
	if !strings.Contains(view, "Event") || !strings.Contains(view, "Duration") {
		t.Fatalf("filtered view must keep Event/Duration columns:\n%s", view)
	}

	m.Filter = ""
	home := ansi.Strip(ViewSize(m, 120, 30, time.Date(2026, 6, 10, 9, 5, 0, 0, time.UTC)))
	if !strings.Contains(home, "All checks passing") {
		t.Fatalf("unfiltered all-green home must keep the band:\n%s", home)
	}
}

// Issue #12: six-digit run numbers must not shift the workflow column.
func TestRunNumberWidthDoesNotJitterColumns(t *testing.T) {
	failing := successRun(3, 5, "CI")
	failing.Conclusion = model.ConclusionFailure
	wide := successRun(4, 241811, "Release")
	wide.Conclusion = model.ConclusionFailure
	m := NewModel(usecase.LaunchContext{
		Repo: "openclaw/openclaw", State: usecase.LaunchStateRuns,
		Runs: []model.Run{failing, wide},
	})
	view := ansi.Strip(ViewSize(m, 120, 30, time.Now()))
	var labelCols []int
	for line := range strings.SplitSeq(view, "\n") {
		if !strings.Contains(line, "push") {
			continue // only data rows
		}
		before, _, ok := strings.Cut(line, "✗ ")
		if !ok {
			t.Fatalf("status glyph missing in row %q", line)
		}
		labelCols = append(labelCols, len([]rune(before)))
	}
	if len(labelCols) != 2 {
		t.Fatalf("expected 2 data rows, got %d:\n%s", len(labelCols), view)
	}
	if labelCols[0] != labelCols[1] {
		t.Fatalf("workflow column jitters with run-number width: %v\n%s", labelCols, view)
	}
}

// Issue #12: the all-green header's Age label must align with the
// right-aligned age values.
func TestAllGreenAgeHeaderAlignsWithValues(t *testing.T) {
	m := NewModel(usecase.LaunchContext{
		Repo: "x/y", State: usecase.LaunchStateRuns,
		Runs: []model.Run{successRun(1, 100, "CI")},
	})
	view := ansi.Strip(ViewSize(m, 120, 30, time.Date(2026, 6, 10, 9, 5, 0, 0, time.UTC)))
	lines := strings.Split(view, "\n")
	headerAge, valueAge := -1, -1
	runeCol := func(line, needle string) int {
		idx := strings.LastIndex(line, needle)
		if idx < 0 {
			return -1
		}
		return len([]rune(line[:idx]))
	}
	for _, line := range lines {
		if strings.Contains(line, "Workflow / detail") {
			headerAge = runeCol(line, "Age")
		}
		if strings.Contains(line, "#100") {
			valueAge = runeCol(strings.TrimRight(line, " "), "3m")
		}
	}
	if headerAge < 0 || valueAge < 0 {
		t.Fatalf("header or value row not found:\n%s", view)
	}
	// Age header should end where values end (right-aligned together).
	if diff := (valueAge + 2) - (headerAge + 3); diff < -1 || diff > 1 {
		t.Fatalf("Age header at %d misaligned with values at %d:\n%s", headerAge, valueAge, view)
	}
}
