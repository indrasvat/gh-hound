package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
)

func queueReadyApp(t *testing.T) App {
	t.Helper()
	app := NewScenarioApp("failure", BuildInfo{Version: "test"})
	app.queueResolver = func(context.Context, model.Run) (model.ConcurrencyReport, error) {
		return sampleQueueReport(), nil
	}
	app.detailResolver = func(_ context.Context, run model.Run) (detail.Model, error) {
		return detail.NewModel(run, nil).WithRepo("indrasvat/gh-hound"), nil
	}
	return app
}

func paletteJumpToQueue(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"q", "u", "e", "u", "e"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	return app
}

func TestPaletteQueueOpensScreenAsync(t *testing.T) {
	app := paletteJumpToQueue(t, queueReadyApp(t))
	if app.Route() != RouteQueue {
		t.Fatalf("route = %q, want queue", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindQueue {
		t.Fatal("palette queue must start the shared async load")
	}
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("queue load never settled")
	}
	view := app.ViewSize(120, 40)
	for _, want := range []string{"GitHub queue", "blocked", "deploy-prod", "1 item(s) ahead"} {
		if !strings.Contains(view, want) {
			t.Fatalf("queue screen missing %q:\n%s", want, view)
		}
	}
}

func TestPaletteQFiltersInsteadOfQuitting(t *testing.T) {
	app := queueReadyApp(t)
	app, _ = app.Update(KeyMsg{Key: ":"})
	app, _ = app.Update(KeyMsg{Key: "q"})
	if app.ShouldQuit() {
		t.Fatal("typing q in the palette must filter, not quit")
	}
	if !strings.Contains(app.ViewSize(120, 40), "queue") {
		t.Fatalf("palette did not filter toward queue:\n%s", app.ViewSize(120, 40))
	}
}

func TestQueueCopyJSONUsesConfiguredClipboard(t *testing.T) {
	app := queueReadyApp(t)
	var copied string
	app.copyText = func(value string) error {
		copied = value
		return nil
	}
	app = paletteJumpToQueue(t, app)
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "y"})
	if !strings.Contains(copied, "blocked") || !strings.Contains(copied, "deploy-prod") {
		t.Fatalf("copied queue JSON = %q", copied)
	}
	if !strings.Contains(app.ViewSize(120, 40), "⎋ dismiss") {
		t.Fatalf("active toast must switch footer to toast controls:\n%s", app.ViewSize(120, 40))
	}
}

func TestQueueEnterOpensSelectedRunDetail(t *testing.T) {
	app := paletteJumpToQueue(t, queueReadyApp(t))
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.Route() != RouteDetail {
		t.Fatalf("route = %q, want detail", app.Route())
	}
	if app.detail.Run.ID != 30433642 {
		t.Fatalf("detail run = %d, want holder run 30433642", app.detail.Run.ID)
	}
}

func TestQueueEscReturnsToRuns(t *testing.T) {
	app := paletteJumpToQueue(t, queueReadyApp(t))
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteRuns {
		t.Fatalf("route = %q, want runs", app.Route())
	}
}
