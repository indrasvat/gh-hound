package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
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
	for _, want := range []string{"hound", "welcome", "WATCH", "DIAGNOSE", "RERUN", "enter continue · ? help · q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q\n%s", want, view)
		}
	}
}

func TestSizedViewDoesNotScrollOrWrapAtTerminalGeometry(t *testing.T) {
	app := NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}})
	view := app.ViewSize(80, 24)
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("80x24 view rendered %d lines, want 24\n%s", len(lines), view)
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 80 {
			t.Fatalf("line %d width=%d, want <=80: %q\n%s", i+1, width, ansi.Strip(line), view)
		}
	}
}

func TestRootViewRendersRunsRoutePlaceholderContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "v0.1.0"}})
	view := app.View()
	for _, want := range []string{"hound", "runs", "enter open · r rerun · x cancel"} {
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
	for _, want := range []string{"CI #571", "Steps", "go test ./...", "enter expand · r rerun job"} {
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

func TestRootViewRendersLogRouteContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	app.PushRoute(RouteLog)
	view := app.View()
	for _, want := range []string{"log", "001", "go test", "j/k scroll"} {
		if !strings.Contains(view, want) {
			t.Fatalf("log route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersWatchRouteContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	app.PushRoute(RouteWatch)
	view := app.View()
	for _, want := range []string{"watch · CI #570", "streaming", "follow", "x cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("watch route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersDispatchRouteContract(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	app.PushRoute(RouteDispatch)
	view := app.View()
	for _, want := range []string{"dispatch · Release", "POST …/workflows/release.yml/dispatches", "enter run"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dispatch route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersHelpAndPaletteOverlays(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	app, _ = app.Update(KeyMsg{Key: "?"})
	if view := app.View(); !strings.Contains(view, "help · gh hound") || !strings.Contains(view, "Legend") {
		t.Fatalf("help overlay missing\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: ":"})
	if view := app.View(); !strings.Contains(view, ": jump to…") || !strings.Contains(view, "runs --all") {
		t.Fatalf("palette overlay missing\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.TopOverlay() != OverlayHelp {
		t.Fatalf("esc should pop only palette, top=%s", app.TopOverlay())
	}
}

func TestRootShellDelegatesScreenKeysAndRoutes(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})

	app, handled := app.Update(KeyMsg{Key: "j"})
	if !handled || !strings.Contains(app.View(), "▌> CI") {
		t.Fatalf("runs j should move selection to running row: handled=%v\n%s", handled, app.View())
	}

	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("runs enter should route to detail: handled=%v route=%s", handled, app.Route())
	}

	app, handled = app.Update(KeyMsg{Key: "tab"})
	if !handled || app.detail.Focus != detail.FocusJobs {
		t.Fatalf("detail tab should update focused pane: handled=%v focus=%s route=%s\n%s", handled, app.detail.Focus, app.Route(), app.View())
	}

	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteFailure {
		t.Fatalf("detail enter should route to failure: handled=%v route=%s", handled, app.Route())
	}

	app, handled = app.Update(KeyMsg{Key: "l"})
	if !handled || app.Route() != RouteLog {
		t.Fatalf("failure l should route to log: handled=%v route=%s", handled, app.Route())
	}

	app, handled = app.Update(KeyMsg{Key: "esc"})
	if !handled || app.Route() != RouteFailure {
		t.Fatalf("log esc should return to failure: handled=%v route=%s", handled, app.Route())
	}
}
