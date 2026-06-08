package detail

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
)

func TestModelPreselectsFailureFocusAndIntents(t *testing.T) {
	m := NewModel(run(), jobs())
	if m.SelectedJob != 0 || m.SelectedStep != 5 || m.Focus != FocusSteps {
		t.Fatalf("initial model = %#v", m)
	}

	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusJobs {
		t.Fatalf("focus after tab = %s", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if m.SelectedJob != 1 {
		t.Fatalf("selected job = %d", m.SelectedJob)
	}
	m = m.Update(KeyMsg{Key: "n"})
	if m.SelectedJob != 0 || m.SelectedStep != 5 || m.Focus != FocusSteps {
		t.Fatalf("n did not jump to failed step: %#v", m)
	}
	m = m.Update(KeyMsg{Key: "l"})
	if m.Intent.Kind != IntentLog || m.Intent.JobID != 100 {
		t.Fatalf("log intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "J"})
	if m.Intent.Kind != IntentNextRun {
		t.Fatalf("J intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "K"})
	if m.Intent.Kind != IntentPreviousRun {
		t.Fatalf("K intent = %#v", m.Intent)
	}
}

func TestViewMatchesMasterDetailAndNarrowCollapse(t *testing.T) {
	m := NewModel(run(), jobs())
	view := View(m, 120)
	plain := ansi.Strip(view)
	for _, want := range []string{
		"indrasvat/gh-hound › CI #571 › fix/parser · @a1b2c3d",
		"Jobs",
		"build [failure] [ubuntu-latest]",
		"2m14s",
		"▌ ✗ 6  go test ./...",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("detail view missing %q\n%s", want, view)
		}
	}
	assertWidth(t, view, 120)

	narrow := View(m, 80)
	plainNarrow := ansi.Strip(narrow)
	for _, want := range []string{"build [failure]", "▌ ✗ 6  go test ./..."} {
		if !strings.Contains(plainNarrow, want) {
			t.Fatalf("narrow view missing %q\n%s", want, narrow)
		}
	}
	if strings.Contains(plainNarrow, "Jobs |") {
		t.Fatalf("narrow view should collapse the jobs pane:\n%s", narrow)
	}
	assertWidth(t, narrow, 80)
}

func TestViewMatchesMockPaneRowsAndFailHighlight(t *testing.T) {
	view := View(NewModel(run(), jobs()), 120)
	plain := ansi.Strip(view)
	for _, banned := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(plain, banned) {
			t.Fatalf("detail view should use row panes, not ASCII boxes %q\n%s", banned, view)
		}
	}
	if !strings.Contains(view, "\x1b[48;2;36;39;30m") {
		t.Fatalf("selected job should use surface-2 background fill\n%s", view)
	}
	if !strings.Contains(view, "\x1b[48;2;40;19;18m") {
		t.Fatalf("failed step should use fail-tinted background fill\n%s", view)
	}
	if !strings.Contains(view, "\x1b[38;2;226;86;75m▌") {
		t.Fatalf("failed step should have fail-colored left bar\n%s", view)
	}
	if strings.Count(plain, "n jump to failure") != 1 {
		t.Fatalf("detail hint should render once in the steps pane\n%s", view)
	}
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line too wide (%d): %q\n%s", got, line, view)
		}
	}
}

func run() model.Run {
	return model.Run{
		ID:         571,
		Name:       "CI",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
		HeadBranch: "fix/parser",
		HeadSHA:    "a1b2c3d",
		RunNumber:  571,
	}
}

func jobs() []model.Job {
	start := time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC)
	return []model.Job{
		{
			ID:          100,
			Name:        "build",
			Status:      model.StatusCompleted,
			Conclusion:  model.ConclusionFailure,
			Labels:      []string{"ubuntu-latest"},
			StartedAt:   start,
			CompletedAt: start.Add(134 * time.Second),
			Steps: []model.Step{
				step(1, "Set up job", model.ConclusionSuccess, 400*time.Millisecond),
				step(2, "Checkout", model.ConclusionSuccess, 1200*time.Millisecond),
				step(3, "Setup Go 1.26", model.ConclusionSuccess, 3100*time.Millisecond),
				step(4, "Cache modules", model.ConclusionSuccess, 2*time.Second),
				step(5, "go build ./...", model.ConclusionSuccess, 18700*time.Millisecond),
				step(6, "go test ./...", model.ConclusionFailure, 41300*time.Millisecond),
			},
		},
		{ID: 101, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: start, CompletedAt: start.Add(31 * time.Second)},
	}
}

func step(number int, name string, conclusion model.Conclusion, duration time.Duration) model.Step {
	start := time.Date(2026, 6, 7, 17, 43, 0, 0, time.UTC)
	return model.Step{
		Number:      number,
		Name:        name,
		Status:      model.StatusCompleted,
		Conclusion:  conclusion,
		StartedAt:   start,
		CompletedAt: start.Add(duration),
	}
}
