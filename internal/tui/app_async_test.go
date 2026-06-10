package tui

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
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
