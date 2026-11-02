package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
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
	b := usecase.PaceBaseline{BaselineBranch: run.HeadBranch}
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
	app.runsConfirmed = confirmedAt(nil, app.runs.Context.Runs, time.Now())
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
	if spy.total() != 0 {
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
	// Past paceMinRefresh, so the new green run may refresh it.
	for key, entry := range app.paceBaselines {
		entry.fetchedAt = entry.fetchedAt.Add(-paceMinRefresh - time.Second)
		app.paceBaselines[key] = entry
	}
	app = settlePace(t, app)
	for range 20 {
		app, _ = app.Refresh()
	}
	app = settlePace(t, app)
	if spy.calls[".github/workflows/ci.yml|main|push"] != 2 {
		t.Fatalf("calls = %v, want ci refetched once its running run went green", spy.calls)
	}
}

// The off toast names the key that brings Pace back, moved or not.
func TestPaceToastNamesTheRemappedKey(t *testing.T) {
	spy := newPaceSpy()
	keymap, err := keys.NewKeymap(map[string]string{"pace_toggle": "e"})
	if err != nil {
		t.Fatal(err)
	}
	app := pacedApp(t, spy, config.Default())
	app.keymap = keymap
	app, handled := app.Update(KeyMsg{Key: "e"})
	if !handled {
		t.Fatal("the moved key must toggle pace")
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "pace off · e brings it back") {
		t.Fatalf("toast must name e:\n%s", view)
	}
}

// On a long list the selected running run is served first, and busy
// workflows going green do not refetch faster than paceMinRefresh.
func TestPaceServesTheSelectedRunningRowFirst(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	rows := []model.Run{}
	for i := range 40 {
		rows = append(rows, paceRow(int64(1000+i), fmt.Sprintf(".github/workflows/bot%d.yml", i), "main", "push", model.StatusCompleted, model.ConclusionSuccess, 30*time.Second))
	}
	rows = append(rows, paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0))
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Runs: rows})
	app.runs.Selected = len(rows) - 1
	app, _ = app.Refresh()
	if app.paceFetch == nil || !strings.Contains(app.paceFetch.key, "ci.yml") {
		t.Fatalf("first fetch = %+v, want the selected running CI row", app.paceFetch)
	}
	app = settlePace(t, app)
	before := spy.total()
	// A bot workflow goes green again right after its fetch: too soon
	// to spend another call on it.
	fresh := paceRow(2000, ".github/workflows/bot0.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, 30*time.Second)
	app.runs.Context.Runs = append([]model.Run{fresh}, app.runs.Context.Runs...)
	app.runs.Selected++ // the selection stays on its run, as after a poll
	app = settlePace(t, app)
	if spy.total() != before {
		t.Fatalf("calls %d → %d: a fresh baseline refetched on a new green run", before, spy.total())
	}
}

// pollRuns runs one runs poll to completion and applies it.
func pollRuns(t *testing.T, app App) App {
	t.Helper()
	app.lastPollStart = time.Time{} // due now: each call is one real poll
	app = app.startRunsPoll()
	if app.tickPoll == nil {
		t.Fatal("no runs poll started")
	}
	deadline := time.Now().Add(2 * time.Second)
	for app.tickPoll != nil && !app.tickPoll.ready() {
		if time.Now().After(deadline) {
			t.Fatal("poll never completed")
		}
		time.Sleep(time.Millisecond)
	}
	app, _ = app.drainTickPoll()
	return app
}

// A run that slid past page 1 while running is never in a page-1 poll
// again: the poll re-reads unfinished rows it did not return, so a
// finished run stops reading as running.
func TestRunsPollRefreshesUnfinishedRowsPastPageOne(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	slid := app.runs.Context.Runs[0] // #600, running
	page := []model.Run{paceRow(700, ".github/workflows/ci.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, 30*time.Second)}
	app.runsResolver = func(context.Context, usecase.RunFilter) ([]model.Run, error) { return page, nil }
	var asked []int64
	app.runRefresher = func(_ context.Context, id int64) (model.Run, error) {
		asked = append(asked, id)
		done := slid
		done.Status, done.Conclusion, done.UpdatedAt = model.StatusCompleted, model.ConclusionSuccess, time.Now()
		return done, nil
	}
	app = pollRuns(t, app)
	if len(asked) != 1 || asked[0] != slid.ID {
		t.Fatalf("refreshed %v, want only the unfinished row #600", asked)
	}
	for _, run := range app.runs.Context.Runs {
		if run.ID == slid.ID && run.Status != model.StatusCompleted {
			t.Fatalf("#600 still reads %s after the refresh", run.Status)
		}
	}
	// Nothing unfinished is left: the next poll spends no extra call.
	asked = nil
	app = pollRuns(t, app)
	if len(asked) != 0 {
		t.Fatalf("refreshed %v with no unfinished rows left", asked)
	}
}

// A running row no fetch has confirmed lately gets no estimate and no
// slow flag: Pace must not make a confident claim on stale data.
func TestPaceSkipsUnconfirmedRunningRows(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "3m left") {
		t.Fatalf("a freshly loaded running row must carry its ETA:\n%s", view)
	}
	c := app.runsConfirmed[600]
	c.at = time.Now().Add(-paceConfirmWindow - time.Second)
	app.runsConfirmed = map[int64]runConfirmation{600: c}
	view := ansi.Strip(app.ViewSize(120, 30))
	if strings.Contains(view, "left") {
		t.Fatalf("an unconfirmed running row carries an ETA:\n%s", view)
	}
	if !strings.Contains(view, "↑ 9m31s") {
		t.Fatalf("a finished slow row needs no confirmation:\n%s", view)
	}
}

// A refreshed row that left a status filter leaves the filtered list.
func TestRunsPollDropsRefreshedRowsThatLeaveTheStatusFilter(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	app.runs.Filter = "status:in_progress"
	slid := app.runs.Context.Runs[0]
	app.runsResolver = func(context.Context, usecase.RunFilter) ([]model.Run, error) { return nil, nil }
	app.runRefresher = func(context.Context, int64) (model.Run, error) {
		done := slid
		done.Status, done.Conclusion = model.StatusCompleted, model.ConclusionSuccess
		return done, nil
	}
	app = pollRuns(t, app)
	for _, run := range app.runs.Context.Runs {
		if run.ID == slid.ID {
			t.Fatalf("#600 finished but stays under status:in_progress: %+v", run.Status)
		}
	}
}

// A row whose re-read keeps failing must not starve the rows behind it.
func TestRunsPollRotatesPastFailingRefreshes(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	rows := []model.Run{}
	for i := range maxRunRefreshes + 1 {
		rows = append(rows, paceRow(int64(800+i), ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0))
	}
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Runs: rows})
	app.runsConfirmed = nil
	app.runsResolver = func(context.Context, usecase.RunFilter) ([]model.Run, error) { return nil, nil }
	asked := map[int64]int{}
	app.runRefresher = func(_ context.Context, id int64) (model.Run, error) {
		asked[id]++
		return model.Run{}, errors.New("not found")
	}
	app = pollRuns(t, app)
	app = pollRuns(t, app)
	if asked[int64(800+maxRunRefreshes)] == 0 {
		t.Fatalf("asked %v: the last row was starved by failing ones", asked)
	}
}

// A confirmation vouches for the snapshot it saw: an older snapshot of
// the same run (the other scope's cache) gets no estimate.
func TestPaceConfirmationIsBoundToTheSnapshot(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	running := app.runs.Context.Runs[0]
	done := running
	done.Status, done.Conclusion, done.UpdatedAt = model.StatusCompleted, model.ConclusionSuccess, running.UpdatedAt.Add(time.Minute)
	app.runsConfirmed = confirmedAt(nil, []model.Run{done}, time.Now())
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "left") {
		t.Fatalf("a completed snapshot vouched for a running one:\n%s", view)
	}
}

// A response is as fresh as when it arrived, not when it was applied.
func TestPaceConfirmationDatesFromArrival(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	app.runsConfirmed = nil
	rows := slices.Clone(app.runs.Context.Runs)
	app = app.applyRunsRefresh(fetchedRuns{rows: rows, at: time.Now().Add(-paceConfirmWindow - time.Minute)}, nil, app.runs.ServerKey())
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "left") {
		t.Fatalf("a response parked past the window reads as fresh:\n%s", view)
	}
}

// A running row is judged as of its last confirmation: a run confirmed
// running that then finished unseen cannot grow into an overrun.
func TestPaceJudgesRunningRowsAsOfTheirConfirmation(t *testing.T) {
	spy := newPaceSpy()
	app := settlePace(t, pacedApp(t, spy, config.Default()))
	run := app.runs.Context.Runs[0] // #600: running, 60s in, p50 4m
	// Confirmed 100s ago at 60s elapsed; by now it would be 160s in.
	app.runsConfirmed = confirmedAt(nil, []model.Run{run}, time.Now().Add(-100*time.Second))
	run.RunStartedAt = time.Now().Add(-160 * time.Second)
	app.runs.Context.Runs[0] = run
	app.runsConfirmed = confirmedAt(nil, []model.Run{run}, time.Now().Add(-100*time.Second))
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "3m left") {
		t.Fatalf("want the ETA as of the confirmation (60s in → 3m left):\n%s", view)
	}
	// Confirmed at 230s, unconfirmed since: never an overrun.
	run.RunStartedAt = time.Now().Add(-400 * time.Second)
	app.runs.Context.Runs[0] = run
	app.runsConfirmed = confirmedAt(nil, []model.Run{run}, time.Now().Add(-110*time.Second))
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "↑ 6m40s") || strings.Contains(view, "↑ 4m") {
		t.Fatalf("an unconfirmed tail made an overrun:\n%s", view)
	}
}

// Re-reads and baselines stay near the selection on a long list: the
// selected unfinished row is re-read first, far rows cost nothing.
func TestPaceAndRefreshesStayNearTheSelection(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	rows := []model.Run{}
	for i := range 200 {
		rows = append(rows, paceRow(int64(3000+i), fmt.Sprintf(".github/workflows/w%d.yml", i), "main", "push", model.StatusInProgress, model.ConclusionNone, 0))
	}
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Runs: rows})
	app.runs.Selected = 150
	app.runsConfirmed = nil
	got := app.refreshCandidates(maxRunRefreshes)
	if len(got) == 0 || got[0] != 3150 {
		t.Fatalf("candidates %v, want the selected row first", got)
	}
	for _, id := range got {
		if id < 3150-paceRowSpan || id > 3150+paceRowSpan {
			t.Fatalf("candidate %d is far from the selection", id)
		}
	}
	app = settlePace(t, app)
	if spy.total() > 2*paceRowSpan+1 {
		t.Fatalf("%d baselines for a 200-row list, want only rows near the selection", spy.total())
	}
}

// The all-green home shows slow marks and keeps fetching, so p must
// work there too: hidden while a run was live, Pace has to come back
// once the list turns all green.
func TestPaceToggleWorksOnTheAllGreenHome(t *testing.T) {
	spy := newPaceSpy()
	app := pacedApp(t, spy, config.Default())
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Runs: []model.Run{
		paceRow(599, ".github/workflows/ci.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, 571*time.Second),
		paceRow(597, ".github/workflows/ci.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, 240*time.Second),
	}})
	app = settlePace(t, app)
	if app.footerScreen() != keys.ScreenAllGreen {
		t.Fatalf("screen = %s, want the all-green home", app.footerScreen())
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "↑ 9m31s") {
		t.Fatalf("the all-green home must carry the slow mark:\n%s", view)
	}
	app, handled := app.Update(KeyMsg{Key: "p"})
	if !handled || !app.paceHidden {
		t.Fatalf("p on the all-green home: handled=%v hidden=%v", handled, app.paceHidden)
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "↑") {
		t.Fatalf("pace off must hide the slow mark:\n%s", view)
	}
	if app, _ = app.Update(KeyMsg{Key: "p"}); app.paceHidden {
		t.Fatal("p again must bring Pace back on the all-green home")
	}
}
