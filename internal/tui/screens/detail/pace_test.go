package detail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// paceBaseline backs jobs(): its green step 5 (18.7s) is slow against ~6s.
func paceBaseline(samples int) *usecase.PaceJobBaseline {
	jb := &usecase.PaceJobBaseline{BaselineBranch: "fix/parser", Runs: samples, Jobs: map[string][]time.Duration{}, Steps: map[string][]time.Duration{}}
	for i := range samples {
		jb.Jobs["build"] = append(jb.Jobs["build"], time.Duration(88+i%7)*time.Second)
		jb.Steps["build\x00go build ./..."] = append(jb.Steps["build\x00go build ./..."], time.Duration(5+i%3)*time.Second)
		jb.Steps["build\x00Checkout"] = append(jb.Steps["build\x00Checkout"], time.Second)
	}
	return jb
}

func pacedModel(samples int) Model {
	m := NewModel(run(), jobs())
	m.Pace = paceBaseline(samples)
	m.PaceHint = true
	m.Focus = FocusSteps
	m.SelectedStep = 4 // go build ./...
	return m
}

func TestDetailPaceFlagsSlowStepsAndExplainsTheFocusedOne(t *testing.T) {
	for _, width := range []int{80, 120, 200} {
		view := ansi.Strip(ViewSize(pacedModel(20), width, 30))
		if !strings.Contains(view, "↑ 18s") {
			t.Errorf("width %d: slow step must read ↑ 18s:\n%s", width, view)
		}
		if !strings.Contains(view, "p50 1m31s · 2m14s") {
			t.Errorf("width %d: steps pane header must carry the job's p50:\n%s", width, view)
		}
		if !strings.Contains(view, "pace ↑ 18s vs p50 6s · p90 7s · 3.0× · 20 runs · fix/parser") {
			t.Errorf("width %d: focused step must explain its flag:\n%s", width, view)
		}
		if !strings.Contains(view, "p pace") {
			t.Errorf("width %d: hint must teach p:\n%s", width, view)
		}
		for line := range strings.SplitSeq(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d: overflow %q", width, line)
			}
		}
	}
}

// The pace line explains only what is focused: jobs focus has none, and
// an on-pace step says it is on pace.
func TestDetailPaceLineFollowsFocus(t *testing.T) {
	m := pacedModel(20)
	m.Focus = FocusJobs
	if view := ansi.Strip(View(m, 120)); strings.Contains(view, "pace ↑") {
		t.Fatalf("jobs focus must not show a step's pace line:\n%s", view)
	}
	m = pacedModel(20)
	m.SelectedStep = 1 // Checkout, on pace
	if view := ansi.Strip(View(m, 120)); !strings.Contains(view, "pace 1s vs p50 1s · p90 1s · 1.0× · 20 runs · fix/parser") {
		t.Fatalf("an on-pace step reads its numbers without ↑:\n%s", view)
	}
	m.SelectedStep = 5 // go test failed: not judged, no line
	if view := ansi.Strip(View(m, 120)); strings.Contains(view, "  pace ") {
		t.Fatalf("a failed step is not judged:\n%s", view)
	}
}

func TestDetailPaceThinTrailFlagsNothing(t *testing.T) {
	view := ansi.Strip(View(pacedModel(3), 120))
	if strings.Contains(view, "↑") || strings.Contains(view, "p50") {
		t.Fatalf("a thin trail must not flag or estimate:\n%s", view)
	}
	if !strings.Contains(view, "pace thin trail · 3 green runs · needs 5 for an estimate") {
		t.Fatalf("the focused step must say why:\n%s", view)
	}
	m := pacedModel(20)
	m.SelectedStep = 2 // Setup Go: in no sample run
	if view := ansi.Strip(View(m, 120)); !strings.Contains(view, "pace no trail · not in the last 20 green runs") {
		t.Fatalf("a step with no samples must say so:\n%s", view)
	}
}

// Pace off (or not loaded): measured cells only, no hint.
func TestDetailWithoutPaceIsPlain(t *testing.T) {
	m := NewModel(run(), jobs())
	m.Focus, m.SelectedStep = FocusSteps, 4
	view := ansi.Strip(View(m, 120))
	if strings.Contains(view, "↑") || strings.Contains(view, "pace") {
		t.Fatalf("no Pace without a baseline:\n%s", view)
	}
}

// GitHub stamps steps to the second: measured cells read whole seconds.
func TestDetailDurationsAreWholeSeconds(t *testing.T) {
	m := NewModel(run(), jobs())
	m.Focus = FocusSteps
	view := ansi.Strip(View(m, 120))
	for _, want := range []string{" 0s", " 1s", " 3s", " 18s", " 41s", "2m14s"} {
		if !strings.Contains(view, want) {
			t.Errorf("want %q:\n%s", want, view)
		}
	}
	for _, decimal := range []string{"0.4s", "18.7s", "41.3s"} {
		if strings.Contains(view, decimal) {
			t.Fatalf("decimal seconds remain (%s):\n%s", decimal, view)
		}
	}
}

// The pace line is paid for from the steps budget: the frame fits its
// height, and the selected step and the line both stay on screen.
func TestDetailPaceLineFitsTheHeightBudget(t *testing.T) {
	steps := make([]model.Step, 0, 30)
	for i := 1; i <= 30; i++ {
		steps = append(steps, step(i, fmt.Sprintf("step %d", i), model.ConclusionSuccess, 2*time.Second))
	}
	jobs := []model.Job{{ID: 1, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: steps[0].StartedAt, CompletedAt: steps[0].StartedAt.Add(time.Minute), Steps: steps}}
	m := NewModel(model.Run{ID: 1}, jobs).WithArtifacts(artifactFixtures())
	jb := &usecase.PaceJobBaseline{Runs: 20, Jobs: map[string][]time.Duration{}, Steps: map[string][]time.Duration{}}
	for range 20 {
		jb.Steps["build\x00step 25"] = append(jb.Steps["build\x00step 25"], 2*time.Second)
	}
	m.Pace, m.Focus, m.SelectedStep = jb, FocusSteps, 24
	for _, width := range []int{80, 120} {
		for _, height := range []int{16, 20, 24} {
			view := ansi.Strip(ViewSize(m, width, height))
			if n := len(strings.Split(view, "\n")); n > height {
				t.Fatalf("%dx%d: %d lines over budget:\n%s", width, height, n, view)
			}
			if !strings.Contains(view, "step 25") || !strings.Contains(view, "pace 2s vs p50 2s") {
				t.Fatalf("%dx%d: selected step and its pace line must both show:\n%s", width, height, view)
			}
		}
	}
}

// At a height where the steps already sit at their minimum, the pace
// line gives way rather than push the hint off the frame.
func TestDetailPaceLineGivesWayWhenTight(t *testing.T) {
	m := pacedModel(20).WithArtifacts(artifactFixtures()[:2])
	m.Jobs[0].Steps = m.Jobs[0].Steps[:5]
	for _, width := range []int{80, 120, 200} {
		for height := 8; height <= 16; height++ {
			off := m
			off.Pace = nil
			want := len(strings.Split(ViewSize(off, width, height), "\n"))
			if got := len(strings.Split(ViewSize(m, width, height), "\n")); got > max(want, height) {
				t.Fatalf("%dx%d: %d lines with Pace, %d without", width, height, got, want)
			}
		}
	}
}

// A step that usually takes 0s has no ratio; 0.0× would read as measured.
func TestDetailPaceLineOmitsAnUndefinedRatio(t *testing.T) {
	m := pacedModel(20)
	for i := range m.Pace.Steps["build\x00go build ./..."] {
		m.Pace.Steps["build\x00go build ./..."][i] = 0
	}
	view := ansi.Strip(View(m, 120))
	if !strings.Contains(view, "pace ↑ 18s vs p50 0s · p90 0s · 20 runs") || strings.Contains(view, "×") {
		t.Fatalf("a zero p50 must drop the ratio:\n%s", view)
	}
}
