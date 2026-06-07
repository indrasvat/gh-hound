package tui

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/theme"
)

func TestAppShellHandlesGlobalsAndStacks(t *testing.T) {
	app := NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}})
	if app.Route() != RouteWelcome {
		t.Fatalf("initial route = %s, want welcome", app.Route())
	}

	app, handled := app.Update(KeyMsg{Key: "T"})
	if !handled || app.Theme().Mode != theme.ModeBone {
		t.Fatalf("theme update handled=%v theme=%s", handled, app.Theme().Mode)
	}

	app, handled = app.Update(KeyMsg{Key: "?"})
	if !handled || app.TopOverlay() != OverlayHelp {
		t.Fatalf("help overlay handled=%v top=%s", handled, app.TopOverlay())
	}
	app, handled = app.Update(KeyMsg{Key: ":"})
	if !handled || app.TopOverlay() != OverlayPalette {
		t.Fatalf("palette overlay handled=%v top=%s", handled, app.TopOverlay())
	}
	app, handled = app.Update(KeyMsg{Key: "esc"})
	if !handled || app.TopOverlay() != OverlayHelp {
		t.Fatalf("esc should pop exactly one overlay: handled=%v top=%s", handled, app.TopOverlay())
	}

	app, handled = app.Update(KeyMsg{Key: "enter"})
	if handled || app.Route() != RouteWelcome {
		t.Fatalf("covered welcome should not receive enter while overlay is active")
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteRuns || !app.WelcomeDismissed() {
		t.Fatalf("welcome enter handled=%v route=%s dismissed=%v", handled, app.Route(), app.WelcomeDismissed())
	}

	app.PushRoute(RouteDetail)
	app.PushRoute(RouteFailure)
	app.PopRoute()
	if app.Route() != RouteDetail {
		t.Fatalf("route stack pop = %s, want detail", app.Route())
	}

	app, handled = app.Update(KeyMsg{Key: "ctrl+c"})
	if !handled || !app.ShouldQuit() {
		t.Fatalf("ctrl+c handled=%v quit=%v", handled, app.ShouldQuit())
	}
}

func TestInputModeBlocksPrintableGlobals(t *testing.T) {
	app := NewApp(Options{Config: config.Default()})
	app.SetInputMode(true)
	app, handled := app.Update(KeyMsg{Key: "T"})
	if handled || app.Theme().Mode != theme.ModeBramble {
		t.Fatalf("input-mode T handled=%v theme=%s", handled, app.Theme().Mode)
	}
	app, handled = app.Update(KeyMsg{Key: "esc"})
	if !handled || app.InputMode() {
		t.Fatalf("esc should leave input mode: handled=%v input=%v", handled, app.InputMode())
	}
}

func TestWelcomeCanBeDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	if app.Route() != RouteRuns {
		t.Fatalf("route with welcome disabled = %s, want runs", app.Route())
	}
}

func TestRootViewContainsChromeAndFooter(t *testing.T) {
	app := NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}})
	view := app.View()
	for _, want := range []string{"hound", "welcome", "WATCH", "DIAGNOSE", "RERUN", "⏎ continue · ? help · q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersRunsRoutePlaceholderContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "v0.1.0"}})
	view := app.View()
	for _, want := range []string{"hound", "runs", "⏎ open · ↻ rerun · ✗ cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("runs route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersDetailRouteContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	app.PushRoute(RouteDetail)
	view := app.View()
	for _, want := range []string{"CI #571", "Steps", "go test ./...", "⏎ expand · ↻ rerun job"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersFailureRouteContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	app.PushRoute(RouteFailure)
	view := app.View()
	for _, want := range []string{"Annotations", "error window", "copy excerpt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failure route view missing %q\n%s", want, view)
		}
	}
}
