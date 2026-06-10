package detail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
)

func TestModelPreselectsFailureFocusAndIntents(t *testing.T) {
	m := NewModel(run(), jobs()).WithRepo("indrasvat/gh-hound")
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
	m := NewModel(run(), jobs()).WithRepo("indrasvat/gh-hound")
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
	view := View(NewModel(run(), jobs()).WithRepo("indrasvat/gh-hound"), 120)
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

func TestViewDoesNotInventMissingGitHubMetadata(t *testing.T) {
	m := NewModel(model.Run{
		ID:         9001,
		RunNumber:  44,
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
	}, []model.Job{{
		ID:         7001,
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
	}})
	view := ansi.Strip(View(m, 100))
	for _, banned := range []string{"repository", "branch", "@sha", "unknown", "runner"} {
		if strings.Contains(view, banned) {
			t.Fatalf("detail view invented fallback %q\n%s", banned, view)
		}
	}
	for _, want := range []string{"#44", "job 7001"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail view missing real identifier %q\n%s", want, view)
		}
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

func artifactFixtures() []model.Artifact {
	return []model.Artifact{
		{ID: 901, Name: "coverage", SizeInBytes: 1262848},
		{ID: 902, Name: "old-report", SizeInBytes: 52480, Expired: true},
	}
}

func TestArtifactsKeyFocusesArtifactsPane(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m = m.Update(KeyMsg{Key: "a"})
	if m.Focus != FocusArtifacts {
		t.Fatalf("focus = %q, want artifacts", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if m.SelectedArtifact != 1 {
		t.Fatalf("selected artifact = %d, want 1", m.SelectedArtifact)
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentDownloadArtifact || m.Intent.ArtifactID != 902 {
		t.Fatalf("intent = %#v, want download for 902", m.Intent)
	}
}

func TestArtifactsKeyIsNoOpWithoutArtifacts(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil)
	m = m.Update(KeyMsg{Key: "a"})
	if m.Focus == FocusArtifacts {
		t.Fatal("artifacts focus must be unreachable when no artifacts exist")
	}
}

func TestTabCyclesThroughArtifactsWhenPresent(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m.Focus = FocusJobs
	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusSteps {
		t.Fatalf("first tab = %q, want steps", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusArtifacts {
		t.Fatalf("second tab = %q, want artifacts", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusJobs {
		t.Fatalf("third tab = %q, want jobs", m.Focus)
	}
}

func TestDownloadKeyTriggersFromAnyFocusWithSelection(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m = m.Update(KeyMsg{Key: "d"})
	if m.Intent.Kind != IntentDownloadArtifact || m.Intent.ArtifactID != 901 {
		t.Fatalf("intent = %#v, want download for selected artifact 901", m.Intent)
	}
}

func TestWithArtifactsClampsSelection(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m.SelectedArtifact = 5
	m = m.WithArtifacts(artifactFixtures()[:1])
	if m.SelectedArtifact != 0 {
		t.Fatalf("selection not clamped: %d", m.SelectedArtifact)
	}
}

func TestViewSizeKeepsArtifactsVisibleWithManySteps(t *testing.T) {
	steps := make([]model.Step, 0, 14)
	for i := 1; i <= 14; i++ {
		steps = append(steps, model.Step{Name: fmt.Sprintf("step %d", i), Number: i, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess})
	}
	jobs := []model.Job{{ID: 1, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Steps: steps}}
	m := NewModel(model.Run{ID: 1}, jobs).WithArtifacts(artifactFixtures())

	view := ansi.Strip(ViewSize(m, 80, 20))
	lines := strings.Split(view, "\n")
	if len(lines) > 20 {
		t.Fatalf("ViewSize must fit the height budget: %d lines > 20\n%s", len(lines), view)
	}
	if !strings.Contains(view, "Artifacts (2)") || !strings.Contains(view, "coverage") {
		t.Fatalf("artifacts must stay visible when steps overflow:\n%s", view)
	}
	if !strings.Contains(view, "more") {
		t.Fatalf("step overflow must be indicated:\n%s", view)
	}
	if !strings.Contains(view, "a artifacts") {
		t.Fatalf("hint line must survive the height budget:\n%s", view)
	}
}

func TestViewSizeKeepsSelectedStepVisible(t *testing.T) {
	steps := make([]model.Step, 0, 20)
	for i := 1; i <= 20; i++ {
		steps = append(steps, model.Step{Name: fmt.Sprintf("step %d", i), Number: i, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess})
	}
	jobs := []model.Job{{ID: 1, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Steps: steps}}
	m := NewModel(model.Run{ID: 1}, jobs).WithArtifacts(artifactFixtures())
	m.Focus = FocusSteps
	m.SelectedStep = 17

	view := ansi.Strip(ViewSize(m, 80, 20))
	if !strings.Contains(view, "step 18") {
		t.Fatalf("selected step must stay in the window:\n%s", view)
	}
}
