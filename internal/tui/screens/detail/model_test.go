package detail

import (
	"strings"
	"testing"
	"time"

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
	for _, want := range []string{
		"indrasvat/gh-hound › CI #571 › fix/parser · @a1b2c3d",
		"Jobs",
		"Steps",
		"build failure ubuntu-latest 2m14s",
		"▌✗ 6 go test ./...",
		"⏎ expand · ↻ rerun job · R rerun failed · ✗ cancel · ⎋ back · ?",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail view missing %q\n%s", want, view)
		}
	}
	assertWidth(t, view, 120)

	narrow := View(m, 80)
	for _, want := range []string{"Steps", "build failure", "▌✗ 6 go test ./..."} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("narrow view missing %q\n%s", want, narrow)
		}
	}
	if strings.Contains(narrow, "Jobs |") {
		t.Fatalf("narrow view should collapse the jobs pane:\n%s", narrow)
	}
	assertWidth(t, narrow, 80)
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if len([]rune(line)) > width {
			t.Fatalf("line too wide (%d): %q\n%s", len([]rune(line)), line, view)
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
