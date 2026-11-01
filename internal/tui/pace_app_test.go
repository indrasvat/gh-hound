package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// paceResolverSpy counts calls per key and the most ever in flight.
type paceResolverSpy struct {
	mu       sync.Mutex
	calls    map[string]int
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	err      error
	gate     chan struct{}
}

func newPaceSpy() *paceResolverSpy { return &paceResolverSpy{calls: map[string]int{}} }

func (s *paceResolverSpy) resolve(ctx context.Context, run model.Run) (usecase.PaceBaseline, error) {
	n := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		seen := s.maxSeen.Load()
		if n <= seen || s.maxSeen.CompareAndSwap(seen, n) {
			break
		}
	}
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return usecase.PaceBaseline{}, ctx.Err()
		}
	}
	s.mu.Lock()
	s.calls[run.Path+"|"+run.HeadBranch+"|"+run.Event]++
	s.mu.Unlock()
	if s.err != nil {
		return usecase.PaceBaseline{}, s.err
	}
	b := usecase.PaceBaseline{Event: run.Event, BaselineBranch: run.HeadBranch}
	for i, sec := range []int{200, 220, 230, 235, 240, 245, 250, 260, 270, 280} {
		b.Samples = append(b.Samples, usecase.PaceSample{RunID: int64(9000 + i), Duration: time.Duration(sec) * time.Second})
	}
	return b, nil
}

func (s *paceResolverSpy) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		n += c
	}
	return n
}

func paceRow(id int64, path, branch, event string, status model.Status, conclusion model.Conclusion, took time.Duration) model.Run {
	start := time.Now().Add(-time.Hour)
	run := model.Run{
		ID: id, RunNumber: int(id), Name: "CI", Path: path, WorkflowID: 1,
		HeadBranch: branch, Event: event, Status: status, Conclusion: conclusion, RunAttempt: 1,
		CreatedAt: start.Add(-10 * time.Second), RunStartedAt: start, UpdatedAt: start.Add(took),
	}
	if status == model.StatusInProgress {
		run.RunStartedAt = time.Now().Add(-60 * time.Second)
		run.UpdatedAt = time.Now()
	}
	return run
}

func pacedApp(t *testing.T, spy *paceResolverSpy, cfg config.Config) App {
	t.Helper()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, PaceResolver: spy.resolve})
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Runs: []model.Run{
		paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0),
		paceRow(599, ".github/workflows/ci.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, 571*time.Second),
		paceRow(598, ".github/workflows/ci.yml", "main", "push", model.StatusCompleted, model.ConclusionFailure, 40*time.Second),
		paceRow(597, ".github/workflows/docs.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, 30*time.Second),
		paceRow(596, ".github/workflows/ci.yml", "fix/x", "pull_request", model.StatusCompleted, model.ConclusionCancelled, 30*time.Second),
	}})
	app.routes = []Route{RouteRuns}
	return app
}

// settlePace drives poll ticks until no Pace fetch is in flight.
func settlePace(t *testing.T, app App) App {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app, _ = app.Refresh()
		if app.paceFetch == nil {
			app, _ = app.Refresh()
			if app.paceFetch == nil {
				return app
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("pace fetches never settled")
	return app
}

// Only rows with something to compare spend a baseline, one key at a
// time; the failure and the cancelled PR run need none.
func TestPaceFetchesOnlyNeededKeysOneAtATime(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	if spy.total() != 2 || spy.calls[".github/workflows/ci.yml|main|push"] != 1 || spy.calls[".github/workflows/docs.yml|main|push"] != 1 {
		t.Fatalf("calls = %v, want one each for ci and docs on main/push", spy.calls)
	}
	if spy.maxSeen.Load() != 1 {
		t.Fatalf("%d fetches ran at once, want at most one in flight", spy.maxSeen.Load())
	}
	view := ansi.Strip(app.ViewSize(120, 30))
	if !strings.Contains(view, "3m left") || !strings.Contains(view, "↑ 9m31s") {
		t.Fatalf("runs list missing pace marks:\n%s", view)
	}
	// The session cache answers every later tick.
	for range 5 {
		app, _ = app.Refresh()
	}
	if spy.total() != 2 {
		t.Fatalf("calls = %d after more ticks, want the cache to answer", spy.total())
	}
}

// Rendering never spends a request: a view before any tick has no
// baselines and calls nothing.
func TestPaceRenderNeverFetches(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	for range 3 {
		_ = app.ViewSize(120, 30)
	}
	if spy.total() != 0 || app.paceFetch != nil {
		t.Fatalf("render spent %d calls", spy.total())
	}
}

func TestPaceToggleHidesAndRestoresWithoutRefetching(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	app, handled := app.Update(KeyMsg{Key: "p"})
	if !handled {
		t.Fatal("p must be handled on the runs list")
	}
	view := ansi.Strip(app.ViewSize(120, 30))
	if strings.Contains(view, "left") || strings.Contains(view, "↑") {
		t.Fatalf("pace off must hide every mark:\n%s", view)
	}
	if !strings.Contains(view, "pace off · p brings it back") {
		t.Fatalf("toggle off must say how to bring it back:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "p"})
	for _, toast := range app.toasts.Toasts {
		if strings.Contains(toast.Title, "pace") {
			t.Fatalf("p back on must clear the off toast, not stack another: %+v", app.toasts.Toasts)
		}
	}
	app = settlePace(t, app)
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "3m left") {
		t.Fatalf("p again must restore the marks:\n%s", view)
	}
	if spy.total() != 2 {
		t.Fatalf("calls = %d, want the toggle to reuse the cache", spy.total())
	}
}

// Hidden for the session means no new requests either.
func TestPaceToggledOffSpendsNothing(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	app, _ = app.Update(KeyMsg{Key: "p"})
	if app = settlePace(t, app); spy.total() != 0 || app.paceFetch != nil {
		t.Fatalf("calls = %d with pace toggled off, want 0", spy.total())
	}
}

func TestPaceConfigOffMakesZeroRequests(t *testing.T) {
	spy := newPaceSpy()
	cfg := config.Default()
	cfg.Pace = false
	app := settlePace(t, pacedApp(t, spy, cfg))
	if spy.total() != 0 {
		t.Fatalf("calls = %d with pace = false, want 0", spy.total())
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "left") || strings.Contains(view, "↑") {
		t.Fatalf("pace = false must show no marks:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "p"})
	app = settlePace(t, app)
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "pace is off in config") || spy.total() != 0 {
		t.Fatalf("p with pace = false must explain and spend nothing (calls %d):\n%s", spy.total(), view)
	}
}

// A failed key toasts once and is not retried on every tick.
func TestPaceErrorToastsOnceAndDoesNotStorm(t *testing.T) {
	spy := newPaceSpy()
	spy.err = errors.New("rate limited")
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	for range 10 {
		app, _ = app.Refresh()
	}
	app = settlePace(t, app)
	if spy.total() != 2 {
		t.Fatalf("calls = %d, want one per key and no retries", spy.total())
	}
	toasts := 0
	for _, toast := range app.toasts.Toasts {
		if strings.Contains(toast.Title, "pace unavailable") {
			toasts++
		}
	}
	if toasts != 1 {
		t.Fatalf("pace toasts = %d, want exactly 1", toasts)
	}
}

// Leaving the screen cancels the fetch in flight; a cancelled fetch is
// not an error.
func TestPaceCancelledFetchIsSilent(t *testing.T) {
	spy := newPaceSpy()
	spy.gate = make(chan struct{})
	app := pacedApp(t, spy, config.Default())
	app, _ = app.Refresh()
	if app.paceFetch == nil {
		t.Fatal("a tick must start a pace fetch")
	}
	app.abandonPaceFetch()
	app, _ = app.Refresh() // drains the cancelled fetch; the next starts
	close(spy.gate)
	app = settlePace(t, app)
	for _, toast := range app.toasts.Toasts {
		if strings.Contains(toast.Title, "pace") {
			t.Fatalf("a cancelled fetch toasted: %+v", toast)
		}
	}
}

// A rerun invalidates baselines: the next ticks fetch again.
func TestPaceRerunResetsBaselines(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	app.actionHandler = func(ActionRequest) (usecase.ActionResult, error) { return usecase.ActionResult{Message: "ok"}, nil }
	app, _ = app.executeAction(RouteRuns, ActionRequest{Action: usecase.ActionRerunRun, Run: app.runs.Context.Runs[0]})
	if len(app.paceBaselines) != 0 {
		t.Fatalf("baselines = %d after a rerun, want cleared", len(app.paceBaselines))
	}
	app = settlePace(t, app)
	if spy.total() != 4 {
		t.Fatalf("calls = %d, want both keys fetched again", spy.total())
	}
}

// A baseline goes stale when a run of its key finishes green after the
// fetch: the new run belongs in the sample.
func TestPaceBaselineRefreshesWhenARunOfItsKeyGoesGreen(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	finished := app.runs.Context.Runs[0]
	// Ahead of the local clock on purpose: staleness must not hang on
	// timestamps, or a skewed clock refetches on every tick.
	finished.Status, finished.Conclusion, finished.UpdatedAt = model.StatusCompleted, model.ConclusionSuccess, time.Now().Add(time.Hour)
	app.runs.Context.Runs[0] = finished
	app = settlePace(t, app)
	for range 20 {
		app, _ = app.Refresh()
	}
	app = settlePace(t, app)
	if spy.calls[".github/workflows/ci.yml|main|push"] != 2 {
		t.Fatalf("calls = %v, want ci refetched once its running run went green", spy.calls)
	}
}

// A fetch already past cancellation when a rerun clears the cache must
// not reinstall the pre-rerun baseline.
func TestPaceRerunDropsAFetchThatFinishesAfterTheReset(t *testing.T) {
	spy := newPaceSpy()
	release := make(chan struct{})
	app := pacedApp(t, spy, config.Default())
	app.paceResolver = func(_ context.Context, run model.Run) (usecase.PaceBaseline, error) {
		<-release // ignores cancellation, as a request already on the wire does
		return spy.resolve(context.Background(), run)
	}
	app, _ = app.Refresh()
	if app.paceFetch == nil {
		t.Fatal("a tick must start a pace fetch")
	}
	app.actionHandler = func(ActionRequest) (usecase.ActionResult, error) { return usecase.ActionResult{Message: "ok"}, nil }
	app, _ = app.executeAction(RouteRuns, ActionRequest{Action: usecase.ActionRerunRun, Run: app.runs.Context.Runs[0]})
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for spy.total() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(5 * time.Millisecond)
	app, _ = app.drainPaceFetch()
	if len(app.paceBaselines) != 0 {
		t.Fatalf("a pre-rerun fetch reinstalled %d baselines", len(app.paceBaselines))
	}
}
