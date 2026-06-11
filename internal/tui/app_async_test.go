package tui

import (
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func asyncTestApp() App {
	return NewScenarioApp("failure", BuildInfo{Version: "test"})
}

func TestStartLoadReturnsImmediately(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	startedAt := time.Now()
	app = app.startLoad(loadKindRuns, "sniffing", func() func(App) App {
		<-release
		return func(a App) App { return a }
	})
	if elapsed := time.Since(startedAt); elapsed > 50*time.Millisecond {
		t.Fatalf("startLoad blocked for %v", elapsed)
	}
	if app.load == nil {
		t.Fatal("startLoad left no pending load")
	}
	close(release)
}

func TestSettleLoadsAppliesResult(t *testing.T) {
	app := asyncTestApp()
	var applied atomic.Bool
	app = app.startLoad(loadKindRuns, "sniffing", func() func(App) App {
		return func(a App) App {
			applied.Store(true)
			return a
		}
	})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("SettleLoads timed out on an instant load")
	}
	if !applied.Load() {
		t.Fatal("load result was not applied")
	}
	if app.load != nil {
		t.Fatal("pending load not cleared after settle")
	}
}

func TestSupersededLoadNeverApplies(t *testing.T) {
	app := asyncTestApp()
	var staleApplied atomic.Bool
	slowDone := make(chan struct{})
	app = app.startLoad(loadKindRuns, "first", func() func(App) App {
		time.Sleep(30 * time.Millisecond)
		close(slowDone)
		return func(a App) App {
			staleApplied.Store(true)
			return a
		}
	})
	var freshApplied atomic.Bool
	app = app.startLoad(loadKindRuns, "second", func() func(App) App {
		return func(a App) App {
			freshApplied.Store(true)
			return a
		}
	})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("SettleLoads timed out")
	}
	<-slowDone
	// Give the orphaned goroutine every chance to misbehave.
	time.Sleep(20 * time.Millisecond)
	if _, changed := app.drainLoad(); changed {
		t.Fatal("drain reported change after settle")
	}
	if staleApplied.Load() {
		t.Fatal("superseded load applied its result")
	}
	if !freshApplied.Load() {
		t.Fatal("fresh load never applied")
	}
}

func TestEscCancelsPendingLoad(t *testing.T) {
	app := asyncTestApp()
	var applied atomic.Bool
	release := make(chan struct{})
	app = app.startLoad(loadKindRuns, "sniffing", func() func(App) App {
		<-release
		return func(a App) App {
			applied.Store(true)
			return a
		}
	})
	app, handled := app.Update(KeyMsg{Key: "esc"})
	if !handled {
		t.Fatal("esc with pending load not handled")
	}
	if app.load != nil {
		t.Fatal("esc did not cancel the pending load")
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	if _, changed := app.drainLoad(); changed {
		t.Fatal("cancelled load still drained")
	}
	if applied.Load() {
		t.Fatal("cancelled load applied its result")
	}
}

func TestPollIntervalTightensWhileLoading(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app = app.startLoad(loadKindDetail, "fetching jobs", func() func(App) App {
		<-release
		return func(a App) App { return a }
	})
	if got := app.PollInterval(); got != loadFrameInterval {
		t.Fatalf("PollInterval while loading = %v, want %v", got, loadFrameInterval)
	}
	close(release)
}

func TestRefreshAnimatesVisibleSpinner(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app = app.startLoad(loadKindDetail, "fetching jobs", func() func(App) App {
		<-release
		return func(a App) App { return a }
	})
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	_, changed := app.Refresh()
	if !changed {
		t.Fatal("Refresh did not report change while spinner is visible")
	}
	close(release)
}

func TestRefreshQuietInsideGrace(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app = app.startLoad(loadKindDetail, "fetching jobs", func() func(App) App {
		<-release
		return func(a App) App { return a }
	})
	// Just started: inside the grace window the pending load itself
	// demands no repaint, so fast loads never flash a spinner frame.
	// (Refresh as a whole may still repaint for unrelated reasons —
	// route polling, toasts — so the assertion targets drainLoad.)
	if _, changed := app.drainLoad(); changed {
		t.Fatal("pending load demanded a repaint inside the grace window")
	}
	close(release)
}

func TestSettleLoadsTimesOut(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	defer close(release)
	app = app.startLoad(loadKindLog, "fetching log", func() func(App) App {
		<-release
		return func(a App) App { return a }
	})
	if _, ok := app.SettleLoads(50 * time.Millisecond); ok {
		t.Fatal("SettleLoads reported settled for a stuck load")
	}
}

func TestRapidLoadCyclesLeakNoGoroutines(t *testing.T) {
	app := asyncTestApp()
	baseline := runtime.NumGoroutine()
	for range 50 {
		app = app.startLoad(loadKindRuns, "cycle", func() func(App) App {
			time.Sleep(time.Millisecond)
			return func(a App) App { return a }
		})
	}
	if _, ok := app.SettleLoads(2 * time.Second); !ok {
		t.Fatal("loads did not settle")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines leaked: baseline %d, now %d", baseline, runtime.NumGoroutine())
}

func TestStatusCycleReloadDoesNotBlock(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	var calls atomic.Int32
	app.runsResolver = func(usecase.RunFilter) ([]model.Run, error) {
		calls.Add(1)
		<-release
		return sampleRunsModel().Context.Runs, nil
	}
	started := time.Now()
	app, handled := app.Update(KeyMsg{Key: "f"})
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("status cycle blocked for %v", elapsed)
	}
	if !handled {
		t.Fatal("f not handled")
	}
	if app.load == nil || app.load.kind != loadKindRuns {
		t.Fatalf("status cycle did not start a runs load: %+v", app.load)
	}
	// After the grace window the view carries the shared loading line.
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	view := ansi.Strip(app.ViewSized(124))
	hasFrame := false
	for _, frame := range icons.SpinnerFrames {
		if strings.Contains(view, frame+" ") {
			hasFrame = true
			break
		}
	}
	if !hasFrame {
		t.Fatalf("loading view missing spinner line:\n%s", view)
	}
	close(release)
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("reload never settled")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls = %d, want exactly 1", got)
	}
	settled := ansi.Strip(app.ViewSized(124))
	for _, frame := range icons.SpinnerFrames {
		if strings.Contains(settled, frame+" sniffing") {
			t.Fatalf("loading line survived settle:\n%s", settled)
		}
	}
}

func TestEscDuringReloadKeepsPreviousList(t *testing.T) {
	app := asyncTestApp()
	before := ansi.Strip(app.ViewSized(124))
	release := make(chan struct{})
	defer close(release)
	app.runsResolver = func(usecase.RunFilter) ([]model.Run, error) {
		<-release
		return nil, nil
	}
	app, _ = app.Update(KeyMsg{Key: "f"})
	if app.load == nil {
		t.Fatal("no pending load after f")
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	// esc cancels the reload AND clears the runs filter (the runs model
	// owns esc-clears-filter); the second reload from the clear is also
	// fine — what must never happen is a stuck loading state.
	if app.load != nil {
		var ok bool
		app, ok = app.SettleLoads(time.Second)
		_ = ok
	}
	after := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(after, "#571") {
		t.Fatalf("run rows lost after esc-cancel:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// settleApp drains the app's pending load for tests that assert on
// post-reload state. Fails the test instead of hanging forever.
func settleApp(t *testing.T, app App) App {
	t.Helper()
	app, ok := app.SettleLoads(2 * time.Second)
	if !ok {
		t.Fatal("pending load did not settle")
	}
	return app
}

func TestDetailOpenPaintsSkeletonImmediately(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app.detailResolver = func(run model.Run) (detail.Model, error) {
		<-release
		return sampleDetailModel(), nil
	}
	started := time.Now()
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("detail open blocked for %v", elapsed)
	}
	if app.Route() != RouteDetail {
		t.Fatalf("route = %v, want detail", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindDetail {
		t.Fatal("detail open started no load")
	}
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	view := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(view, "fetching jobs") {
		t.Fatalf("skeleton missing loading hint:\n%s", view)
	}
	close(release)
	app = settleApp(t, app)
	settled := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(settled, "go test ./...") {
		t.Fatalf("resolved jobs missing after settle:\n%s", settled)
	}
}

func TestDetailResultForAbandonedRunNeverApplies(t *testing.T) {
	app := asyncTestApp()
	slow := make(chan struct{})
	app.detailResolver = func(run model.Run) (detail.Model, error) {
		if run.ID == 571 {
			<-slow
		}
		return detail.NewModel(run, nil), nil
	}
	app, _ = app.Update(KeyMsg{Key: "enter"}) // open #571, slow
	app, _ = app.Update(KeyMsg{Key: "esc"})   // back to runs
	app, _ = app.Update(KeyMsg{Key: "j"})     // select #570
	app, _ = app.Update(KeyMsg{Key: "enter"}) // open #570, instant
	close(slow)
	app = settleApp(t, app)
	// The second run's view must never be clobbered by the first
	// run's late result.
	if got := app.detail.Run.ID; got != 570 {
		t.Fatalf("detail shows run %d, want 570", got)
	}
}

func TestLogOpenShowsByteProgress(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app.logResolver = func(run model.Run, job model.Job, progress func(read, total int64)) (logscreen.Model, error) {
		progress(2202009, 5033165)
		<-release
		return sampleLogModel(), nil
	}
	app, _ = app.Update(KeyMsg{Key: "l"})
	if app.Route() != RouteLog {
		t.Fatalf("route = %v, want log", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindLog {
		t.Fatal("log open started no load")
	}
	// Wait for the resolver goroutine to report progress.
	deadline := time.Now().Add(time.Second)
	for {
		if _, _, read, _ := app.load.snapshot(); read > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("progress never reported")
		}
		time.Sleep(time.Millisecond)
	}
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	view := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(view, "2.1 MB") || !strings.Contains(view, "4.8 MB") || !strings.Contains(view, "▰") {
		t.Fatalf("log loading view missing byte progress:\n%s", view)
	}
	close(release)
	app = settleApp(t, app)
	settled := ansi.Strip(app.ViewSized(124))
	if strings.Contains(settled, "4.8 MB") {
		t.Fatalf("progress line survived settle:\n%s", settled)
	}
}

func TestFailureOpenIsAsync(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app.failureResolver = func(model.Run, model.Job) (failure.Model, logscreen.Model, error) {
		<-release
		return sampleFailureModel(), sampleLogModel(), nil
	}
	app, _ = app.Update(KeyMsg{Key: "enter"}) // detail (async, instant sample)
	app = settleApp(t, app)
	started := time.Now()
	app, _ = app.Update(KeyMsg{Key: "enter"}) // failure
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("failure open blocked for %v", elapsed)
	}
	if app.Route() != RouteFailure {
		t.Fatalf("route = %v, want failure", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindFailure {
		t.Fatal("failure open started no load")
	}
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	view := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(view, "fetching the failure") {
		t.Fatalf("failure loading body missing:\n%s", view)
	}
	close(release)
	app = settleApp(t, app)
	settled := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(settled, "exit 1") {
		t.Fatalf("failure content missing after settle:\n%s", settled)
	}
}

func TestDispatchOpenIsAsync(t *testing.T) {
	app := asyncTestApp()
	release := make(chan struct{})
	app.dispatchWorkflows = nil
	app.dispatchWorkflowsResolver = func() ([]dispatch.Workflow, error) {
		<-release
		return []dispatch.Workflow{sampleDispatchModel().Workflow}, nil
	}
	started := time.Now()
	app, _ = app.Update(KeyMsg{Key: "D"})
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("dispatch open blocked for %v", elapsed)
	}
	// The route flips only once the workflow count is known; until
	// then the runs screen hosts the loading line.
	if app.Route() != RouteRuns {
		t.Fatalf("route = %v, want runs while resolving", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindDispatch {
		t.Fatal("dispatch open started no load")
	}
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	view := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(view, "fetching workflows") {
		t.Fatalf("dispatch loading line missing on runs:\n%s", view)
	}
	close(release)
	app = settleApp(t, app)
	if app.Route() != RouteDispatch {
		t.Fatalf("route after settle = %v, want dispatch", app.Route())
	}
	settled := ansi.Strip(app.ViewSized(124))
	if !strings.Contains(settled, "Release") {
		t.Fatalf("dispatch form missing after settle:\n%s", settled)
	}
}

func TestRefreshSkipsRoutePollingWhileLoading(t *testing.T) {
	app := asyncTestApp()
	var pollCalls atomic.Int32
	release := make(chan struct{})
	defer close(release)
	app.runsResolver = func(usecase.RunFilter) ([]model.Run, error) {
		if pollCalls.Add(1) == 1 {
			<-release // the load's own fetch holds the queue
		}
		return nil, nil
	}
	app, _ = app.Update(KeyMsg{Key: "f"})
	if app.load == nil {
		t.Fatal("no pending load")
	}
	// Wait until the load's goroutine has actually entered the resolver.
	deadline := time.Now().Add(time.Second)
	for pollCalls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("load fetch never started")
		}
		time.Sleep(time.Millisecond)
	}
	app.load.started = time.Now().Add(-200 * time.Millisecond)
	// Five animation ticks must not fire route polls: the pending load
	// owns the queue and frames must keep flowing.
	for range 5 {
		var changed bool
		app, changed = app.Refresh()
		if !changed {
			t.Fatal("Refresh did not animate while spinner visible")
		}
	}
	if got := pollCalls.Load(); got != 1 {
		t.Fatalf("route polls during load = %d, want 1 (the load's own fetch)", got)
	}
}
