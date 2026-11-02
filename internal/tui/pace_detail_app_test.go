package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type jobPaceSpy struct {
	mu        sync.Mutex
	calls     int
	baselines []usecase.PaceBaseline
}

func (s *jobPaceSpy) resolve(_ context.Context, run model.Run, b usecase.PaceBaseline) (usecase.PaceJobBaseline, error) {
	s.mu.Lock()
	s.calls++
	s.baselines = append(s.baselines, b)
	s.mu.Unlock()
	jb := usecase.PaceJobBaseline{BaselineBranch: run.HeadBranch, Runs: 20, Jobs: map[string][]time.Duration{}, Steps: map[string][]time.Duration{}}
	for range 20 {
		jb.Steps["build\x00go build"] = append(jb.Steps["build\x00go build"], 6*time.Second)
	}
	return jb, nil
}

// A failed run's detail: no run-level verdict, but its green steps are
// judged against the workflow's green runs.
func pacedDetailApp(t *testing.T, runs *paceResolverSpy, jobs *jobPaceSpy) App {
	t.Helper()
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, PaceResolver: runs.resolve, PaceJobResolver: jobs.resolve})
	run := paceRow(700, ".github/workflows/ci.yml", "fix/parser", "push", model.StatusCompleted, model.ConclusionFailure, 3*time.Minute)
	start := run.RunStartedAt
	build := model.Job{ID: 1, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, StartedAt: start, CompletedAt: start.Add(2 * time.Minute),
		Steps: []model.Step{
			{Number: 1, Name: "go build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: start, CompletedAt: start.Add(19 * time.Second)},
			{Number: 2, Name: "go test", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, StartedAt: start, CompletedAt: start.Add(41 * time.Second)},
		}}
	app.detail = detail.NewModel(run, []model.Job{build}).WithRepo("o/r")
	app.routes = []Route{RouteRuns, RouteDetail}
	return app
}

func TestDetailPaceFetchesRunThenJobBaselinesOnTicks(t *testing.T) {
	runs, jobs := newPaceSpy(), &jobPaceSpy{}
	app := pacedDetailApp(t, runs, jobs)
	for range 3 {
		_ = app.ViewSize(120, 30)
	}
	if runs.total() != 0 || jobs.calls != 0 {
		t.Fatalf("render fetched: runs %d jobs %d", runs.total(), jobs.calls)
	}
	app = settlePace(t, app)
	if runs.total() != 1 || jobs.calls != 1 {
		t.Fatalf("calls: runs %d jobs %d, want one run baseline then one job baseline", runs.total(), jobs.calls)
	}
	if len(jobs.baselines[0].Samples) != 10 {
		t.Fatalf("the job baseline must be built on the run baseline: %+v", jobs.baselines[0])
	}
	view := ansi.Strip(app.ViewSize(120, 30))
	if !strings.Contains(view, "↑ 19s") || !strings.Contains(view, "p pace") {
		t.Fatalf("detail must flag the slow step and teach p:\n%s", view)
	}
	// Cached: more ticks spend nothing.
	if settlePace(t, app); runs.total() != 1 || jobs.calls != 1 {
		t.Fatalf("refetched a fresh baseline: runs %d jobs %d", runs.total(), jobs.calls)
	}
}

func TestDetailPaceToggleHidesAndSpendsNothing(t *testing.T) {
	runs, jobs := newPaceSpy(), &jobPaceSpy{}
	app := pacedDetailApp(t, runs, jobs)
	app, handled := app.Update(KeyMsg{Key: "p"})
	if !handled || !app.paceHidden {
		t.Fatalf("p on detail: handled %v hidden %v", handled, app.paceHidden)
	}
	if app = settlePace(t, app); runs.total() != 0 || jobs.calls != 0 {
		t.Fatalf("hidden Pace fetched: runs %d jobs %d", runs.total(), jobs.calls)
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "↑") {
		t.Fatalf("hidden Pace drew a mark:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "p"})
	app = settlePace(t, app)
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "↑ 19s") {
		t.Fatalf("p again must bring the marks back:\n%s", view)
	}
}

func TestDetailPaceConfigOffShowsNoHint(t *testing.T) {
	runs, jobs := newPaceSpy(), &jobPaceSpy{}
	app := pacedDetailApp(t, runs, jobs)
	app.config.Pace = false
	app = settlePace(t, app)
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "p pace") || runs.total()+jobs.calls != 0 {
		t.Fatalf("pace = false: calls %d, view:\n%s", runs.total()+jobs.calls, view)
	}
}
