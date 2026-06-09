package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/usecase"
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

func TestProductionAppWithoutLaunchDoesNotRenderSampleRuns(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	view := ansi.Strip(app.ViewSize(120, 32))
	assertNoProductionSampleData(t, view)
	for _, want := range []string{"Repository needed", "gh hound -R owner/repo"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing repository guidance %q\n%s", want, view)
		}
	}
}

func TestProductionDeepRoutesDoNotRenderUnloadedSamples(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Actor:  "indrasvat",
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         9001,
				Name:       "Nightly",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
				RunNumber:  77,
				HeadBranch: "main",
				HeadSHA:    "deadbee",
			}},
		},
	})
	for _, route := range []Route{RouteFailure, RouteLog, RouteWatch, RouteDispatch} {
		app := app
		app.PushRoute(route)
		view := ansi.Strip(app.ViewSize(120, 32))
		assertNoProductionSampleData(t, view)
		if !strings.Contains(view, "unavailable") && !strings.Contains(view, "No workflow") {
			t.Fatalf("%s should render an explicit unavailable/empty state, not blank or sample data:\n%s", route, view)
		}
	}
}

func TestProductionRunsLogShortcutDoesNotReuseSampleLog(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Actor:  "indrasvat",
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         9002,
				Name:       "CI",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
				RunNumber:  78,
				HeadBranch: "main",
				HeadSHA:    "cafebabe",
			}},
		},
	})
	app, handled := app.Update(KeyMsg{Key: "l"})
	if !handled || app.Route() != RouteLog {
		t.Fatalf("l should open the log route through live loading: handled=%v route=%s", handled, app.Route())
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	assertNoProductionSampleData(t, view)
	if !strings.Contains(view, "log unavailable") {
		t.Fatalf("missing explicit log unavailable state:\n%s", view)
	}
}

func TestWelcomeDismissesToResolvedLaunchState(t *testing.T) {
	cfg := config.Default()
	launch := usecase.LaunchContext{
		Repo:   "openclaw/openclaw",
		Branch: "main",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateWatch,
		Runs: []model.Run{{
			ID:         658258,
			Name:       "ClawSweeper Dispatch",
			Status:     model.StatusInProgress,
			Conclusion: model.ConclusionNone,
			RunNumber:  658258,
			HeadBranch: "main",
		}},
	}
	app := NewApp(Options{Config: cfg, Launch: launch})
	if app.Route() != RouteWelcome {
		t.Fatalf("initial route = %s, want welcome", app.Route())
	}

	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteWatch {
		t.Fatalf("welcome enter handled=%v route=%s, want watch", handled, app.Route())
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	for _, want := range []string{"watch · ClawSweeper Dispatch #658258 · main", "streaming"} {
		if !strings.Contains(view, want) {
			t.Fatalf("watch view missing launch run %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "CI #570") {
		t.Fatalf("watch view used sample run after welcome:\n%s", view)
	}
}

func assertNoProductionSampleData(t *testing.T, view string) {
	t.Helper()
	for _, banned := range []string{
		"fix/parser",
		"parser fix validation",
		"TestLexIdent",
		"internal/parser/lexer.go",
		"a1b2c3d",
		"CI #571",
		"Release #42",
		"dispatch · Release",
		"release.yml",
		"Run go test ./...",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("production TUI rendered sample sentinel %q:\n%s", banned, view)
		}
	}
}

func TestLaunchErrorIsVisibleInsteadOfEmptyRunsList(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:         "openclaw/openclaw",
			Branch:       "main",
			State:        usecase.LaunchStateError,
			ErrorMessage: "github api GET /repos/openclaw/openclaw/actions/runs: API rate limit exceeded",
		},
	})
	view := ansi.Strip(app.ViewSize(120, 32))
	for _, want := range []string{"Runs unavailable", "API rate limit exceeded"} {
		if !strings.Contains(view, want) {
			t.Fatalf("error launch view missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "no runs match") {
		t.Fatalf("error launch was rendered as an empty filtered runs list:\n%s", view)
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
	view := RenderFixtureSize("runs", 80, 0)
	for _, want := range []string{"hound", "⌥ branch fix/parser · @indrasvat", "⏎ open · ↻ rerun · ✗ cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("runs route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersDetailRouteContract(t *testing.T) {
	view := ansi.Strip(RenderFixtureSize("detail", 80, 0))
	for _, want := range []string{"CI #571", "build [failure]", "go test ./...", "⏎ expand · ↻ rerun job"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersFailureRouteContract(t *testing.T) {
	view := RenderFixtureSize("failure", 80, 0)
	for _, want := range []string{"Annotations", "error window", "copy excerpt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failure route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersLogRouteContract(t *testing.T) {
	view := RenderFixtureSize("log", 80, 0)
	for _, want := range []string{"log", "001", "go test", "j/k scroll"} {
		if !strings.Contains(view, want) {
			t.Fatalf("log route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersWatchRouteContract(t *testing.T) {
	view := RenderFixtureSize("watch", 80, 0)
	for _, want := range []string{"watch · CI #570", "streaming", "follow", "✗ cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("watch route view missing %q\n%s", want, view)
		}
	}
}

func TestRootViewRendersDispatchRouteContract(t *testing.T) {
	view := RenderFixtureSize("dispatch", 80, 0)
	for _, want := range []string{"dispatch · Release", "POST …/workflows/release.yml/dispatches", "⏎ run"} {
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

func TestSelectedLineReappliesBackgroundAfterNestedReset(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	style := sgrHex(th.FG, false) + sgrHex(th.Surface2, true)
	line := selectedLine(th, "\x1b[38;2;79;211;122m✔\x1b[0m CI", 24)
	if !strings.HasPrefix(line, style) {
		t.Fatalf("selected line should start with fg+bg SGR: %q", line)
	}
	if !strings.Contains(line, sgrReset+style) {
		t.Fatalf("selected line should reapply fg+bg after nested reset: %q", line)
	}
	if !strings.HasSuffix(line, sgrReset) {
		t.Fatalf("selected line should reset at final boundary: %q", line)
	}
}

func TestFixtureBackgroundLinesDoNotBleedAfterNestedReset(t *testing.T) {
	screens := []string{
		"welcome",
		"all_green",
		"runs",
		"detail",
		"failure",
		"watch",
		"log",
		"dispatch",
		"palette",
		"help",
	}
	breakpoints := []struct {
		width  int
		height int
	}{
		{width: 80, height: 24},
		{width: 120, height: 40},
		{width: 200, height: 60},
	}
	for _, screen := range screens {
		for _, bp := range breakpoints {
			view := RenderFixtureSize(screen, bp.width, bp.height)
			for lineNumber, line := range strings.Split(view, "\n") {
				if !strings.Contains(line, "\x1b[48;2;") {
					continue
				}
				assertBackgroundLineSafe(t, screen, bp.width, bp.height, lineNumber+1, line)
			}
		}
	}
}

func assertBackgroundLineSafe(t *testing.T, screen string, width int, height int, lineNumber int, line string) {
	t.Helper()
	for index := 0; index < len(line); {
		resetAt := strings.Index(line[index:], sgrReset)
		if resetAt == -1 {
			return
		}
		absoluteReset := index + resetAt
		remaining := line[absoluteReset+len(sgrReset):]
		visibleRemaining := strings.TrimSpace(ansi.Strip(remaining))
		if visibleRemaining == "" || strings.HasPrefix(visibleRemaining, "│") {
			index = absoluteReset + len(sgrReset)
			continue
		}
		prefix := remaining[:clampPrefix(len(remaining), 40)]
		if !strings.HasPrefix(remaining, "\x1b[38;2;") || !strings.Contains(prefix, "\x1b[48;2;") {
			t.Fatalf("%s %dx%d line %d loses background after reset before visible content: %q", screen, width, height, lineNumber, line)
		}
		index = absoluteReset + len(sgrReset)
	}
}

func clampPrefix(length, maxLength int) int {
	if length < maxLength {
		return length
	}
	return maxLength
}

func TestScreenBodiesDoNotRenderFrameFooters(t *testing.T) {
	tests := map[string]string{
		"detail":   "⏎ expand · ↻ rerun job",
		"failure":  "↻ rerun failed · r rerun job",
		"watch":    "✗ cancel · f follow",
		"log":      "j/k scroll · g/G ends",
		"dispatch": "⏎ run · ⇥ next",
	}
	for screen, footer := range tests {
		view := RenderFixtureSize(screen, 120, 40)
		if count := strings.Count(ansi.Strip(view), footer); count != 1 {
			t.Fatalf("%s rendered footer %d times, want frame-only footer once\n%s", screen, count, view)
		}
	}
}

func TestRootShellDelegatesScreenKeysAndRoutes(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})

	app, handled := app.Update(KeyMsg{Key: "j"})
	if view := ansi.Strip(app.View()); !handled || !strings.Contains(view, "▌#570") || !strings.Contains(view, "CI · push smoke") {
		t.Fatalf("runs j should move selection to running row: handled=%v\n%s", handled, view)
	}

	app, handled = app.Update(KeyMsg{Key: "k"})
	if view := ansi.Strip(app.View()); !handled || !strings.Contains(view, "▌#571") || !strings.Contains(view, "CI · parser fix validation") {
		t.Fatalf("runs k should move selection back to failing row: handled=%v\n%s", handled, view)
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

func TestRunsArrowKeysNavigateAndSelectedRunOpensDistinctDetail(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	release := model.Run{ID: 2001, Name: "Release", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunNumber: 42, HeadBranch: "main", HeadSHA: "rel1234"}
	codeQL := model.Run{ID: 2002, Name: "CodeQL", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 43, HeadBranch: "main", HeadSHA: "ql56789"}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
			Actor:  "indrasvat",
			State:  usecase.LaunchStateRuns,
			Runs:   []model.Run{release, codeQL},
		},
		DetailResolver: func(run model.Run) (detail.Model, error) {
			job := model.Job{ID: run.ID + 10, RunID: run.ID, Name: run.Name + " job", Status: run.Status, Conclusion: run.Conclusion}
			return detail.NewModel(run, []model.Job{job}), nil
		},
	})

	app, handled := app.Update(KeyMsg{Key: "down"})
	if !handled {
		t.Fatal("down key was not handled on the runs screen")
	}
	if view := ansi.Strip(app.View()); !strings.Contains(view, "▌#43") || !strings.Contains(view, "CodeQL · main") {
		t.Fatalf("down key did not select CodeQL row:\n%s", view)
	}

	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("enter did not open selected detail: handled=%v route=%s", handled, app.Route())
	}
	view := ansi.Strip(app.View())
	for _, want := range []string{"CodeQL #43", "CodeQL job", "ql56789"} {
		if !strings.Contains(view, want) {
			t.Fatalf("selected detail missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "CI #571") || strings.Contains(view, "Release #42") {
		t.Fatalf("selected detail reused a different run:\n%s", view)
	}
}

func TestLogRouteUsesAvailableFrameHeight(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "line content"
	}
	app.log = logscreen.NewModel(logs.Parse(strings.Join(lines, "\n")), 1, 6)
	app.PushRoute(RouteLog)

	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "033 line content") {
		t.Fatalf("log view did not expand to available height:\n%s", view)
	}
	if strings.Contains(view, "006 line content") && !strings.Contains(view, "020 line content") {
		t.Fatalf("log view appears fixed to the model default height:\n%s", view)
	}
}
