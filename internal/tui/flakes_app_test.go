package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func paletteJumpToFlakes(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"f", "l", "a", "k", "e", "s"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	return app
}

func settleFlakes(t *testing.T, app App) App {
	t.Helper()
	app, ok := app.SettleLoads(2 * time.Second)
	if !ok {
		t.Fatal("flakes load never settled")
	}
	return app
}

func TestPaletteFlakesOpensScentCheckAsync(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app = paletteJumpToFlakes(t, app)
	if app.Route() != RouteFlakes {
		t.Fatalf("route = %q, want flakes", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindFlakes {
		t.Fatal("palette flakes must start the shared async load")
	}
	app = settleFlakes(t, app)
	view := app.View()
	for _, want := range []string{"squirrel", "build", "attempt_flip"} {
		if !strings.Contains(view, want) {
			t.Fatalf("flakes view missing %q:\n%s", want, view)
		}
	}
}

func TestFlakesEvidenceEnterOpensThatRunsDetail(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app = settleFlakes(t, paletteJumpToFlakes(t, app))
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("evidence enter should route to detail: handled=%v route=%s", handled, app.Route())
	}
	// The skeleton paints from the evidence run before the resolver
	// lands — the drill-down targets THAT run, not the selected row.
	if app.detail.Run.ID != 30433570 {
		t.Fatalf("detail run = %d, want the evidence run 30433570", app.detail.Run.ID)
	}
	app = settleFlakes(t, app)
}

func TestFlakesEscReturnsToRuns(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app = settleFlakes(t, paletteJumpToFlakes(t, app))
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteRuns {
		t.Fatalf("route = %q, want runs after esc", app.Route())
	}
}

// The second :flakes jump for the same workflow+branch answers from
// the session cache — no second scan is spent.
func TestFlakesSessionCacheAnswersWithoutASecondScan(t *testing.T) {
	var calls atomic.Int32
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.flakesResolver = func(context.Context, model.Run) (usecase.FlakeReport, error) {
		calls.Add(1)
		return sampleFlakeReport(), nil
	}
	app = settleFlakes(t, paletteJumpToFlakes(t, app))
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app = paletteJumpToFlakes(t, app)
	if app.load != nil {
		t.Fatal("cached verdict must not start a new load")
	}
	if app.Route() != RouteFlakes {
		t.Fatalf("route = %q, want flakes", app.Route())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls = %d, want 1 (session cache)", got)
	}
}

func openFailureScreen(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: "enter"}) // runs -> detail
	app = settleFlakes(t, app)
	app, _ = app.Update(KeyMsg{Key: "enter"}) // detail -> failure
	app = settleFlakes(t, app)
	if app.Route() != RouteFailure {
		t.Fatalf("route = %q, want failure", app.Route())
	}
	return app
}

// The failure screen's scent check rides off the load slot, lands on
// a poll tick, and never blocks the failure paint (Task 220).
func TestFailureScreenFlakePanelArrivesAsync(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app = openFailureScreen(t, app)
	deadline := time.Now().Add(2 * time.Second)
	for app.failure.Flake == nil {
		if time.Now().After(deadline) {
			t.Fatal("flake panel never arrived")
		}
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	view := app.View()
	for _, want := range []string{"seen this one before", "build flaked 2 of last 6 runs"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failure view missing %q:\n%s", want, view)
		}
	}
}

func failureWithPanel(t *testing.T, app App) App {
	t.Helper()
	app = openFailureScreen(t, app)
	deadline := time.Now().Add(2 * time.Second)
	for app.failure.Flake == nil {
		if time.Now().After(deadline) {
			t.Fatal("flake panel never arrived")
		}
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	return app
}

func TestFailurePanelTabFocusAndEvidenceDrillDown(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app = failureWithPanel(t, app)

	// j/k drive the excerpt viewport while it has focus.
	beforeOffset := app.failure.Offset
	app, handled := app.Update(KeyMsg{Key: "j"})
	if !handled {
		t.Fatal("failure j must be handled (excerpt scroll)")
	}
	if app.failure.Offset == beforeOffset {
		t.Fatalf("excerpt offset = %d, want scrolled from %d", app.failure.Offset, beforeOffset)
	}
	app, _ = app.Update(KeyMsg{Key: "k"})

	// tab hands focus to the panel; j/k then move the evidence cursor.
	app, handled = app.Update(KeyMsg{Key: "tab"})
	if !handled || !app.failure.PanelFocus {
		t.Fatalf("tab should focus the flake panel: handled=%v focus=%v", handled, app.failure.PanelFocus)
	}
	app, _ = app.Update(KeyMsg{Key: "j"})
	if app.failure.PanelSelected != 1 {
		t.Fatalf("panel selected = %d, want 1", app.failure.PanelSelected)
	}
	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("panel enter should open the evidence run: handled=%v route=%s", handled, app.Route())
	}
	if app.detail.Run.ID != 30433570 {
		t.Fatalf("detail run = %d, want evidence run 30433570", app.detail.Run.ID)
	}
	app = settleFlakes(t, app)
}

// flake_badges=false is the zero-cost path: opening a failure spends
// no flake scan calls at all.
func TestFailureScreenSpendsNoFlakeCallsWhenBadgesOff(t *testing.T) {
	var calls atomic.Int32
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.config.FlakeBadges = false
	app.flakesResolver = func(context.Context, model.Run) (usecase.FlakeReport, error) {
		calls.Add(1)
		return sampleFlakeReport(), nil
	}
	app = openFailureScreen(t, app)
	for range 5 {
		app, _ = app.Refresh()
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("flake calls = %d, want 0 with flake_badges=false", got)
	}
	if app.failure.Flake != nil {
		t.Fatal("panel must stay absent with flake_badges=false")
	}
}

// Once a verdict lands, failing rows of that workflow+branch carry
// the badge on the runs list.
func TestFlakeBadgeMarksKnownFlakerRows(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	// The scenario list's failing run carries no Path; align the
	// sample report key with the row's workflow identity.
	app = failureWithPanel(t, app)
	app, _ = app.Update(KeyMsg{Key: "esc"}) // failure -> detail
	app, _ = app.Update(KeyMsg{Key: "esc"}) // detail -> runs
	if app.Route() != RouteRuns {
		t.Fatalf("route = %q, want runs", app.Route())
	}
	if !app.runs.FlakyRuns[571] {
		t.Fatalf("runs.FlakyRuns = %v, want run 571 badged", app.runs.FlakyRuns)
	}
	view := ansi.Strip(app.ViewSize(80, 24))
	if !strings.Contains(view, "~ CI") {
		t.Fatalf("runs view missing the flake badge at 80 cols:\n%s", view)
	}
}

// A confirmed rerun invalidates the session verdict cache: new
// attempts are about to land and a cached verdict would lie.
func TestRerunInvalidatesFlakeVerdictCache(t *testing.T) {
	var calls atomic.Int32
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.flakesResolver = func(context.Context, model.Run) (usecase.FlakeReport, error) {
		calls.Add(1)
		return sampleFlakeReport(), nil
	}
	app = settleFlakes(t, paletteJumpToFlakes(t, app))
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls = %d, want 1", got)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	// Seed derived state the rerun must also clear (ghent Codex P2):
	// stale badges and a stale failure panel would outlive the cache.
	app.runs.FlakyRuns = map[int64]bool{570: true}
	app.failure.Flake = &sampleFlakeReport().Jobs[0]

	// Confirmed rerun on the selected run.
	app, _ = app.Update(KeyMsg{Key: "r"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("overlay = %q, want confirm", app.TopOverlay())
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	if app.runs.FlakyRuns != nil {
		t.Fatal("rerun must clear the stale flake badges")
	}
	if app.failure.Flake != nil {
		t.Fatal("rerun must clear the stale failure panel verdict")
	}

	app = paletteJumpToFlakes(t, app)
	app = settleFlakes(t, app)
	if got := calls.Load(); got != 2 {
		t.Fatalf("resolver calls = %d, want 2 (rerun invalidates the cache)", got)
	}
}

// The fixtures behind make vqa must keep rendering the panel, the
// badge, and the scent screen.
func TestFlakeFixturesRender(t *testing.T) {
	cases := map[string][]string{
		"flakes":        {"squirrel", "build", "attempt_flip", "j/k move"},
		"failure-flaky": {"seen this one before", "build flaked 2 of last 6 runs", "retry_mask"},
		"runs-flaky":    {"~ CI"},
	}
	for screen, needles := range cases {
		view := ansi.Strip(RenderFixtureSize(screen, 120, 40))
		for _, want := range needles {
			if !strings.Contains(view, want) {
				t.Fatalf("fixture %s missing %q:\n%s", screen, want, view)
			}
		}
	}
}

// TestFlakeScanStartsOnlyAfterTheFailureResolves pins the codex
// blocker: the GitHub queue is serial, so the scent check must not
// race the failure fetch — it starts in the failure load's apply
// callback, after the paintable result exists.
func TestFlakeScanStartsOnlyAfterTheFailureResolves(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	var failureDone, scanStarted atomic.Bool
	innerFailure := app.failureResolver
	app.failureResolver = func(ctx context.Context, run model.Run, job model.Job) (failure.Model, logscreen.Model, error) {
		defer failureDone.Store(true)
		return innerFailure(ctx, run, job)
	}
	innerFlakes := app.flakesResolver
	app.flakesResolver = func(ctx context.Context, run model.Run) (usecase.FlakeReport, error) {
		scanStarted.Store(true)
		if !failureDone.Load() {
			t.Error("flake scan started before the failure resolved")
		}
		return innerFlakes(ctx, run)
	}
	app = failureWithPanel(t, app)
	if !scanStarted.Load() {
		t.Fatal("flake scan never started")
	}
}

// TestEscFromFailureCancelsTheInFlightScan pins the second half of
// the codex blocker: leaving the failure screen abandons the scan —
// no API spend continues behind the user's back.
func TestEscFromFailureCancelsTheInFlightScan(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	release := make(chan struct{})
	var sawCancel atomic.Bool
	app.flakesResolver = func(ctx context.Context, run model.Run) (usecase.FlakeReport, error) {
		select {
		case <-ctx.Done():
			sawCancel.Store(true)
			return usecase.FlakeReport{}, ctx.Err()
		case <-release:
			return usecase.FlakeReport{}, nil
		}
	}
	app = openFailureScreen(t, app)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	deadline := time.Now().Add(2 * time.Second)
	for !sawCancel.Load() {
		if time.Now().After(deadline) {
			close(release)
			t.Fatal("esc did not cancel the in-flight scan")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestAbandonedScanDrainsSilently pins codex round-2: an intentionally
// cancelled scan (esc or a palette jump away from failure) must not
// surface the "scent check unavailable" warning.
func TestAbandonedScanDrainsSilently(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.flakesResolver = func(ctx context.Context, run model.Run) (usecase.FlakeReport, error) {
		<-ctx.Done()
		return usecase.FlakeReport{}, ctx.Err()
	}
	app = openFailureScreen(t, app)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	deadline := time.Now().Add(2 * time.Second)
	for app.flakesFetch != nil {
		if time.Now().After(deadline) {
			t.Fatal("cancelled scan never drained")
		}
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
	view := app.View()
	if strings.Contains(view, "scent check unavailable") {
		t.Fatalf("abandoned scan must not toast:\n%s", view)
	}
}

// TestPaletteJumpFromFailureCancelsTheScan pins the second codex
// round-2 blocker: palette jumps replace the route stack without
// PopRoute, and must abandon the scan too.
func TestPaletteJumpFromFailureCancelsTheScan(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	var sawCancel atomic.Bool
	app.flakesResolver = func(ctx context.Context, run model.Run) (usecase.FlakeReport, error) {
		<-ctx.Done()
		sawCancel.Store(true)
		return usecase.FlakeReport{}, ctx.Err()
	}
	app = openFailureScreen(t, app)
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"r", "u", "n", "s"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	deadline := time.Now().Add(2 * time.Second)
	for !sawCancel.Load() {
		if time.Now().After(deadline) {
			t.Fatal("palette jump did not cancel the in-flight scan")
		}
		time.Sleep(time.Millisecond)
	}
}
