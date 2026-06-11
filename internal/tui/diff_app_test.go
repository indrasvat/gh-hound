package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func diffTestVerdict() usecase.RegressionVerdict {
	return usecase.RegressionVerdict{
		Repo:     "indrasvat/gh-hound",
		Workflow: "CI",
		Branch:   "main",
		Status:   usecase.RegressionLocated,
		LastGood: model.Run{ID: 572, Name: "CI", RunNumber: 572, RunAttempt: 2, HeadSHA: "c2b3a49", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
		FirstBad: model.Run{ID: 573, Name: "CI", RunNumber: 573, HeadSHA: "d3c4b5a", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure},
		SuspectCommits: []model.Commit{
			{SHA: "d3c4b5a9f0e1", Author: "indrasvat", Message: "feat: sharpen the lexer"},
		},
		TotalSuspects: 1,
		CompareURL:    "https://github.com/indrasvat/gh-hound/compare/c2b3a49...d3c4b5a",
		RunsScanned:   4,
		Verdict:       "scent picked up: #572 was clean, #573 wasn't.",
	}
}

func diffReadyApp(t *testing.T) App {
	t.Helper()
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.diffResolver = func(context.Context, model.Run) (usecase.RegressionVerdict, error) {
		return diffTestVerdict(), nil
	}
	return app
}

func paletteJumpToDiff(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"d", "i", "f", "f"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	return app
}

func TestPaletteDiffOpensTrailScreenAsync(t *testing.T) {
	app := paletteJumpToDiff(t, diffReadyApp(t))
	if app.Route() != RouteDiff {
		t.Fatalf("route = %q, want diff", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindDiff {
		t.Fatal("palette diff must start the shared async load")
	}
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("diff load never settled")
	}
	view := app.ViewSized(100)
	if !strings.Contains(view, "scent picked up: #572 was clean, #573 wasn't.") {
		t.Fatalf("trail screen missing verdict:\n%s", view)
	}
	if !strings.Contains(view, "the trail") {
		t.Fatalf("chrome missing trail title:\n%s", view)
	}
}

// The Task 220 invariant: the keystroke that triggers the scan must
// not block on the resolver.
func TestPaletteDiffKeystrokeNeverBlocks(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	release := make(chan struct{})
	app.diffResolver = func(context.Context, model.Run) (usecase.RegressionVerdict, error) {
		<-release
		return diffTestVerdict(), nil
	}
	started := time.Now()
	app = paletteJumpToDiff(t, app)
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("palette diff blocked the keystroke for %v", elapsed)
	}
	if app.load == nil {
		t.Fatal("no pending load registered")
	}
	close(release)
	if _, ok := app.SettleLoads(time.Second); !ok {
		t.Fatal("diff load never settled after release")
	}
}

func TestDiffEnterOpensFirstBadDetail(t *testing.T) {
	app := paletteJumpToDiff(t, diffReadyApp(t))
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.Route() != RouteDetail {
		t.Fatalf("route after enter = %q, want detail", app.Route())
	}
	if app.detail.Run.ID != 573 {
		t.Fatalf("detail run = %d, want the first bad run 573", app.detail.Run.ID)
	}
	app, _ = app.SettleLoads(time.Second)
}

func TestDiffOpensCompareURLInBrowser(t *testing.T) {
	app := diffReadyApp(t)
	var opened string
	app.openURL = func(url string) error {
		opened = url
		return nil
	}
	app = paletteJumpToDiff(t, app)
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "o"})
	if opened != "https://github.com/indrasvat/gh-hound/compare/c2b3a49...d3c4b5a" {
		t.Fatalf("opened = %q, want the compare URL", opened)
	}
}

func TestDiffEscReturnsToRuns(t *testing.T) {
	app := paletteJumpToDiff(t, diffReadyApp(t))
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteRuns {
		t.Fatalf("route after esc = %q, want runs", app.Route())
	}
}

func TestDiffResolverErrorBecomesRouteError(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.diffResolver = func(context.Context, model.Run) (usecase.RegressionVerdict, error) {
		return usecase.RegressionVerdict{}, usecase.APIError{Kind: usecase.APIErrorRateLimit, Message: "API rate limit exceeded"}
	}
	app = paletteJumpToDiff(t, app)
	app, _ = app.SettleLoads(time.Second)
	view := app.ViewSized(100)
	if !strings.Contains(view, "diff unavailable") {
		t.Fatalf("route error not surfaced:\n%s", view)
	}
}

func TestDiffFixturesRender(t *testing.T) {
	located := RenderFixtureSize("diff", 80, 24)
	for _, want := range []string{"the trail", "scent picked up", "suspects ·"} {
		if !strings.Contains(located, want) {
			t.Fatalf("diff fixture missing %q:\n%s", want, located)
		}
	}
	cold := RenderFixtureSize("diff-inconclusive", 80, 24)
	if !strings.Contains(cold, "trail went cold") {
		t.Fatalf("diff-inconclusive fixture missing cold-trail verdict:\n%s", cold)
	}
}

func TestPaletteAdvertisesDiffHonestly(t *testing.T) {
	app := diffReadyApp(t)
	app, _ = app.Update(KeyMsg{Key: ":"})
	view := app.ViewSized(100)
	if strings.Contains(view, "diff (v2)") {
		t.Fatalf("palette still shows the v2 stub:\n%s", view)
	}
	if !strings.Contains(view, "diff") {
		t.Fatalf("palette missing the diff entry:\n%s", view)
	}
}
