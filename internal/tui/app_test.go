package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	failurescreen "github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	watchscreen "github.com/indrasvat/gh-hound/internal/tui/screens/watch"
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

func TestProductionChromeDoesNotInventMissingGitHubMetadata(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:  "openclaw/openclaw",
			State: usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         9001,
				RunNumber:  44,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}},
		},
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{{
				ID:         7001,
				RunID:      run.ID,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}}).WithRepo("openclaw/openclaw"), nil
		},
	})
	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("enter did not open detail: handled=%v route=%s", handled, app.Route())
	}
	// The skeleton must already show the real repo, never invented
	// metadata; resolved identifiers are asserted after settle.
	skeleton := ansi.Strip(app.ViewSize(120, 32))
	if !strings.Contains(skeleton, "openclaw/openclaw") {
		t.Fatalf("loading skeleton lost the repo breadcrumb:\n%s", skeleton)
	}
	app = settleApp(t, app)
	view := ansi.Strip(app.ViewSize(120, 32))
	for _, banned := range []string{"branch", "@sha", "unknown", "workflow"} {
		if strings.Contains(view, banned) {
			t.Fatalf("production chrome/detail invented fallback %q\n%s", banned, view)
		}
	}
	for _, want := range []string{"openclaw/openclaw", "#44", "job 7001"} {
		if !strings.Contains(view, want) {
			t.Fatalf("production chrome/detail missing real identifier %q\n%s", want, view)
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
	for _, want := range []string{"hound", "⎇ branch fix/parser · @indrasvat", "⏎ open · ↻ rerun · ✗ cancel"} {
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

func TestDispatchPickerSelectsExactWorkflow(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	var called []ActionRequest
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Actor:  "indrasvat",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         1001,
				Name:       "CI",
				RunNumber:  1001,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionSuccess,
				HeadBranch: "main",
			}},
		},
		DispatchWorkflowsResolver: func(context.Context) ([]dispatch.Workflow, error) {
			return []dispatch.Workflow{
				{Name: "Release", ID: "release.yml", Ref: "main"},
				{Name: "Blacksmith Build Artifacts Testbox", ID: "blacksmith.yml", Ref: "main", Inputs: []dispatch.Input{{
					Name:     "testbox_id",
					Required: true,
					Type:     dispatch.InputText,
				}}},
			}, nil
		},
		ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
			called = append(called, request)
			return usecase.ActionResult{Action: request.Action, WorkflowID: request.Workflow.ID, Message: "queued"}, nil
		},
	})

	app, handled := app.Update(KeyMsg{Key: "D"})
	app = settleApp(t, app)
	if !handled || app.TopOverlay() != OverlayPalette || app.Route() != RouteRuns {
		t.Fatalf("D should open dispatch workflow picker: handled=%v route=%s overlay=%s\n%s", handled, app.Route(), app.TopOverlay(), app.View())
	}
	picker := ansi.Strip(app.ViewSize(140, 36))
	for _, want := range []string{"dispatch: Release", "dispatch: Blacksmith Build Artifacts Testbox", "workflow_dispatch"} {
		if !strings.Contains(picker, want) {
			t.Fatalf("dispatch picker missing %q\n%s", want, picker)
		}
	}

	app, handled = app.Update(KeyMsg{Key: "down"})
	if !handled {
		t.Fatalf("down should move workflow picker selection")
	}
	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.TopOverlay() != OverlayNone || app.Route() != RouteDispatch {
		t.Fatalf("enter should open selected workflow form: handled=%v route=%s overlay=%s\n%s", handled, app.Route(), app.TopOverlay(), app.View())
	}
	form := ansi.Strip(app.ViewSize(140, 36))
	for _, want := range []string{"dispatch · Blacksmith Build Artifacts Testbox", "testbox_id", "POST …/workflows/blacksmith.yml/dispatches"} {
		if !strings.Contains(form, want) {
			t.Fatalf("selected workflow form missing %q\n%s", want, form)
		}
	}

	for _, key := range []string{"t", "b", "x", "-", "1", "2", "3", "enter"} {
		var stepHandled bool
		app, stepHandled = app.Update(KeyMsg{Key: key})
		if !stepHandled {
			t.Fatalf("dispatch form key %q was not handled", key)
		}
	}
	if len(called) != 1 || called[0].Workflow.ID != "blacksmith.yml" || called[0].Dispatch.Inputs["testbox_id"] != "tbx-123" {
		t.Fatalf("dispatch call = %#v", called)
	}
}

func TestDispatchTextInputConsumesGlobalShortcutLetters(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         1,
				Name:       "CI",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionSuccess,
			}},
		},
		DispatchWorkflowsResolver: func(context.Context) ([]dispatch.Workflow, error) {
			return []dispatch.Workflow{{
				Name: "CI",
				ID:   "ci.yml",
				Ref:  "main",
				Inputs: []dispatch.Input{{
					Name: "version",
					Type: dispatch.InputText,
				}},
			}}, nil
		},
	})
	app, handled := app.Update(KeyMsg{Key: "D"})
	app = settleApp(t, app)
	if !handled || app.Route() != RouteDispatch {
		t.Fatalf("D did not open dispatch form: handled=%v route=%s\n%s", handled, app.Route(), app.View())
	}
	app, handled = app.Update(KeyMsg{Key: "T"})
	if !handled {
		t.Fatal("dispatch text input did not handle uppercase T")
	}
	if app.Theme().Mode != theme.ModeBramble {
		t.Fatalf("uppercase T toggled global theme while editing dispatch input")
	}
	if len(app.dispatch.Fields) == 0 || app.dispatch.Fields[0].Value != "T" {
		t.Fatalf("dispatch field value = %#v", app.dispatch.Fields)
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
	app = settleApp(t, app)

	app, handled = app.Update(KeyMsg{Key: "tab"})
	if !handled || app.detail.Focus != detail.FocusArtifacts {
		t.Fatalf("detail tab from steps should focus artifacts when present: handled=%v focus=%s route=%s\n%s", handled, app.detail.Focus, app.Route(), app.View())
	}

	app, handled = app.Update(KeyMsg{Key: "tab"})
	if !handled || app.detail.Focus != detail.FocusJobs {
		t.Fatalf("detail tab should cycle back to jobs: handled=%v focus=%s route=%s\n%s", handled, app.detail.Focus, app.Route(), app.View())
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
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
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
	app = settleApp(t, app)
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

func TestDestructiveActionsRequireExplicitConfirmation(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	calls := []ActionRequest{}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusInProgress,
				Conclusion: model.ConclusionNone,
				HeadBranch: "main",
			}},
		},
		ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
			calls = append(calls, request)
			return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, Message: "accepted"}, nil
		},
	})

	app, handled := app.Update(KeyMsg{Key: "x"})
	if !handled || app.TopOverlay() != OverlayConfirm {
		t.Fatalf("cancel should open confirm overlay before mutation: handled=%v top=%s\n%s", handled, app.TopOverlay(), app.View())
	}
	if len(calls) != 0 {
		t.Fatalf("action handler called before confirmation: %#v", calls)
	}
	if view := ansi.Strip(app.ViewSize(120, 32)); !strings.Contains(view, "Confirm action") || !strings.Contains(view, "cancel run #44") {
		t.Fatalf("confirm modal missing action context:\n%s", view)
	}

	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.TopOverlay() != OverlayNone || len(calls) != 0 {
		t.Fatalf("enter should keep default no: handled=%v top=%s calls=%d", handled, app.TopOverlay(), len(calls))
	}

	app, _ = app.Update(KeyMsg{Key: "x"})
	app, handled = app.Update(KeyMsg{Key: "y"})
	if !handled || app.TopOverlay() != OverlayNone {
		t.Fatalf("y should confirm and close overlay: handled=%v top=%s", handled, app.TopOverlay())
	}
	if len(calls) != 1 || calls[0].Action != usecase.ActionCancelRun || calls[0].Run.ID != 9001 {
		t.Fatalf("confirmed cancel call = %#v", calls)
	}

	app, _ = app.Update(KeyMsg{Key: "X"})
	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.TopOverlay() != OverlayNone || len(calls) != 1 {
		t.Fatalf("force-cancel enter should abort, not confirm: handled=%v top=%s calls=%d", handled, app.TopOverlay(), len(calls))
	}
}

func TestRunsMutationShortcutsRequireConfirmation(t *testing.T) {
	tests := []struct {
		key    string
		action usecase.Action
		label  string
	}{
		{key: "r", action: usecase.ActionRerunRun, label: "rerun run #44"},
		{key: "R", action: usecase.ActionRerunFailedJobs, label: "rerun failed jobs for run #44"},
		{key: "x", action: usecase.ActionCancelRun, label: "cancel run #44"},
		{key: "X", action: usecase.ActionForceCancelRun, label: "force-cancel run #44"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			cfg := config.Default()
			cfg.Welcome = false
			calls := []ActionRequest{}
			app := NewApp(Options{
				Config: cfg,
				Launch: usecase.LaunchContext{
					Repo:   "openclaw/openclaw",
					Branch: "main",
					State:  usecase.LaunchStateRuns,
					Runs: []model.Run{{
						ID:         9001,
						Name:       "CI",
						RunNumber:  44,
						Status:     model.StatusInProgress,
						Conclusion: model.ConclusionNone,
						HeadBranch: "main",
					}},
				},
				ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
					calls = append(calls, request)
					return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, Message: "accepted"}, nil
				},
			})

			app, handled := app.Update(KeyMsg{Key: tt.key})
			if !handled || app.TopOverlay() != OverlayConfirm || len(calls) != 0 {
				t.Fatalf("%s should open confirm before mutation: handled=%v top=%s calls=%d", tt.key, handled, app.TopOverlay(), len(calls))
			}
			if view := ansi.Strip(app.ViewSize(120, 32)); !strings.Contains(view, tt.label) {
				t.Fatalf("confirm modal missing %q:\n%s", tt.label, view)
			}
			app, handled = app.Update(KeyMsg{Key: "y"})
			if !handled || app.TopOverlay() != OverlayNone {
				t.Fatalf("%s y should confirm and close overlay: handled=%v top=%s", tt.key, handled, app.TopOverlay())
			}
			if len(calls) != 1 || calls[0].Action != tt.action || calls[0].Run.ID != 9001 {
				t.Fatalf("%s confirmed call = %#v", tt.key, calls)
			}
		})
	}
}

func TestRunsFilterReloadsServerSupportedQueries(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	cfg.PerPage = 50
	calls := []usecase.RunFilter{}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:           1001,
				Name:         "CI",
				DisplayTitle: "green tip",
				RunNumber:    101,
				Status:       model.StatusCompleted,
				Conclusion:   model.ConclusionSuccess,
				HeadBranch:   "main",
			}},
		},
		RunsResolver: func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
			calls = append(calls, filter)
			return []model.Run{{
				ID:           2002,
				Name:         "Integration",
				DisplayTitle: "staging failure",
				RunNumber:    202,
				Status:       model.StatusCompleted,
				Conclusion:   model.ConclusionFailure,
				HeadBranch:   "main",
				HTMLURL:      "https://github.com/openclaw/openclaw/actions/runs/2002",
				RunStartedAt: time.Now().Add(-5 * time.Minute),
				UpdatedAt:    time.Now().Add(-2 * time.Minute),
			}}, nil
		},
	})

	for _, key := range []string{"/", "f", "a", "i", "l", "i", "n", "g", "enter"} {
		var handled bool
		app, handled = app.Update(KeyMsg{Key: key})
		if !handled {
			t.Fatalf("key %q was not handled", key)
		}
	}
	app = settleApp(t, app)

	if len(calls) != 1 {
		t.Fatalf("runs resolver calls = %d, want 1", len(calls))
	}
	if calls[0].Repo != "openclaw/openclaw" || calls[0].Branch != "main" || calls[0].Status != "failure" || calls[0].PerPage != 50 {
		t.Fatalf("resolver filter = %#v", calls[0])
	}
	view := ansi.Strip(app.ViewSize(120, 32))
	if !strings.Contains(view, "/failing  1 matches") || !strings.Contains(view, "#202") || !strings.Contains(view, "Integration · staging failure") || strings.Contains(view, "#101") {
		t.Fatalf("server-filtered runs did not replace visible list:\n%s", view)
	}
}

func TestRunsEndLoadsNextGitHubPage(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	cfg.PerPage = 3
	calls := []usecase.RunFilter{}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:    "openclaw/openclaw",
			Scope:   usecase.LaunchScopeRepo,
			State:   usecase.LaunchStateRuns,
			PerPage: 3,
			Runs: []model.Run{
				{ID: 1003, Name: "CI", RunNumber: 1003, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
				{ID: 1002, Name: "CI", RunNumber: 1002, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
				{ID: 1001, Name: "CI", RunNumber: 1001, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
			},
			RepoRuns: []model.Run{
				{ID: 1003, Name: "CI", RunNumber: 1003, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
				{ID: 1002, Name: "CI", RunNumber: 1002, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
				{ID: 1001, Name: "CI", RunNumber: 1001, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
			},
		},
		RunsResolver: func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
			calls = append(calls, filter)
			return []model.Run{
				{ID: 1001, Name: "CI", RunNumber: 1001, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
				{ID: 1000, Name: "CI", RunNumber: 1000, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
				{ID: 999, Name: "Release", RunNumber: 999, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
			}, nil
		},
	})

	app, handled := app.Update(KeyMsg{Key: "G"})
	if !handled {
		t.Fatal("G should be handled")
	}
	app = settleApp(t, app)
	if len(calls) != 1 {
		t.Fatalf("pagination resolver calls = %d, want 1", len(calls))
	}
	if calls[0].Repo != "openclaw/openclaw" || calls[0].Page != 2 || calls[0].PerPage != 3 {
		t.Fatalf("pagination filter = %#v", calls[0])
	}
	view := ansi.Strip(app.ViewSize(120, 20))
	if !strings.Contains(view, "#999") || !strings.Contains(view, "rows 1-") || !strings.Contains(view, "/5") {
		t.Fatalf("next page was not appended to visible runs:\n%s", view)
	}
	if strings.Count(view, "#1001") != 1 {
		t.Fatalf("pagination should deduplicate overlapping live pages:\n%s", view)
	}
}

func TestRefreshReloadsVisibleRunsWithoutKeypress(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	calls := 0
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:    "openclaw/openclaw",
			Branch:  "main",
			Scope:   usecase.LaunchScopeBranch,
			State:   usecase.LaunchStateRuns,
			PerPage: 30,
			Runs: []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusInProgress,
				Conclusion: model.ConclusionNone,
				HeadBranch: "main",
			}},
			BranchRuns: []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusInProgress,
				Conclusion: model.ConclusionNone,
				HeadBranch: "main",
			}},
		},
		RunsResolver: func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
			calls++
			if filter.Page != 1 || filter.Branch != "main" {
				t.Fatalf("refresh filter = %#v", filter)
			}
			return []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionSuccess,
				HeadBranch: "main",
			}}, nil
		},
	})

	app, changed := app.Refresh()
	if !changed || calls != 1 {
		t.Fatalf("refresh changed=%v calls=%d", changed, calls)
	}
	view := ansi.Strip(app.ViewSize(100, 20))
	if !strings.Contains(view, "✔") || !strings.Contains(view, "live") || strings.Contains(view, "⠹") {
		t.Fatalf("refresh did not update visible run status:\n%s", view)
	}
}

func TestRefreshBacksOffIdleRunsAndResetsWhenRunning(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	cfg.PollMin = time.Second
	cfg.PollMax = 8 * time.Second
	responses := [][]model.Run{
		{{
			ID:         9001,
			Name:       "CI",
			RunNumber:  44,
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionSuccess,
			HeadBranch: "main",
		}},
		{{
			ID:         9001,
			Name:       "CI",
			RunNumber:  44,
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionSuccess,
			HeadBranch: "main",
		}},
		{{
			ID:         9002,
			Name:       "CI",
			RunNumber:  45,
			Status:     model.StatusInProgress,
			Conclusion: model.ConclusionNone,
			HeadBranch: "main",
		}},
	}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:    "openclaw/openclaw",
			Branch:  "main",
			Scope:   usecase.LaunchScopeBranch,
			State:   usecase.LaunchStateRuns,
			PerPage: 30,
			Runs: []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionSuccess,
				HeadBranch: "main",
			}},
		},
		RunsResolver: func(context.Context, usecase.RunFilter) ([]model.Run, error) {
			next := responses[0]
			responses = responses[1:]
			return next, nil
		},
	})

	if got := app.PollInterval(); got != time.Second {
		t.Fatalf("initial poll interval = %s, want 1s", got)
	}
	app, _ = app.Refresh()
	if got := app.PollInterval(); got != 2*time.Second {
		t.Fatalf("first idle poll interval = %s, want 2s", got)
	}
	app, _ = app.Refresh()
	if got := app.PollInterval(); got != 4*time.Second {
		t.Fatalf("second idle poll interval = %s, want 4s", got)
	}
	app, _ = app.Refresh()
	if got := app.PollInterval(); got != time.Second {
		t.Fatalf("running poll interval = %s, want reset to 1s", got)
	}
}

func TestRunsChromeShowsRealRateAndCacheMetadata(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	meta := usecase.RequestMeta{}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:    "openclaw/openclaw",
			Branch:  "main",
			Scope:   usecase.LaunchScopeBranch,
			State:   usecase.LaunchStateRuns,
			PerPage: 30,
			Runs: []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionSuccess,
				HeadBranch: "main",
			}},
		},
		RunsResolver: func(context.Context, usecase.RunFilter) ([]model.Run, error) {
			meta = usecase.RequestMeta{Status: 304, Cache: "hit", RateRemaining: "4998"}
			return []model.Run{{
				ID:         9001,
				Name:       "CI",
				RunNumber:  44,
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionSuccess,
				HeadBranch: "main",
			}}, nil
		},
		RunsMetadata: func() (usecase.RequestMeta, bool) {
			return meta, meta.Status != 0 || meta.RateRemaining != ""
		},
	})

	before := ansi.Strip(app.ViewSize(120, 20))
	if strings.Contains(before, "4998/5k") || strings.Contains(before, "304") {
		t.Fatalf("header invented metadata before refresh:\n%s", before)
	}
	app, _ = app.Refresh()
	after := ansi.Strip(app.ViewSize(120, 20))
	for _, want := range []string{"1 runs loaded", "live", "4998/5k", "304"} {
		if !strings.Contains(after, want) {
			t.Fatalf("header missing %q:\n%s", want, after)
		}
	}
}

func TestRefreshErrorKeepsCachedRunsAndShowsToast(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:    "openclaw/openclaw",
			Branch:  "integration",
			Scope:   usecase.LaunchScopeBranch,
			State:   usecase.LaunchStateRuns,
			PerPage: 30,
			Runs: []model.Run{{
				ID:           8100,
				Name:         "CI",
				DisplayTitle: "cached branch run",
				RunNumber:    8100,
				Status:       model.StatusCompleted,
				Conclusion:   model.ConclusionSuccess,
				HeadBranch:   "integration",
			}},
			BranchRuns: []model.Run{{
				ID:           8100,
				Name:         "CI",
				DisplayTitle: "cached branch run",
				RunNumber:    8100,
				Status:       model.StatusCompleted,
				Conclusion:   model.ConclusionSuccess,
				HeadBranch:   "integration",
			}},
		},
		RunsResolver: func(context.Context, usecase.RunFilter) ([]model.Run, error) {
			return nil, usecase.APIError{
				Kind:    usecase.APIErrorPermission,
				Status:  403,
				Message: "Resource not accessible by personal access token",
			}
		},
	})

	app, changed := app.Refresh()
	if !changed {
		t.Fatal("refresh error should repaint cached rows with toast")
	}
	view := ansi.Strip(app.ViewSize(120, 24))
	for _, want := range []string{"cached branch run", "GitHub access denied", "Resource not accessible by personal access token"} {
		if !strings.Contains(view, want) {
			t.Fatalf("refresh error view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "hound · empty") {
		t.Fatalf("refresh error should not replace cached runs with an empty screen:\n%s", view)
	}
}

func TestRefreshRateLimitToastShowsAutoResumeMetadata(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "integration",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:           8120,
				Name:         "CI",
				DisplayTitle: "cached integration run",
				RunNumber:    8120,
				Status:       model.StatusCompleted,
				Conclusion:   model.ConclusionSuccess,
				HeadBranch:   "integration",
			}},
		},
		RunsResolver: func(context.Context, usecase.RunFilter) ([]model.Run, error) {
			return nil, usecase.APIError{
				Kind:       usecase.APIErrorRateLimit,
				Status:     403,
				Message:    "API rate limit exceeded",
				RetryAfter: 42 * time.Second,
				ResetAt:    time.Date(2026, 6, 9, 20, 4, 0, 0, time.UTC),
			}
		},
	})

	app, changed := app.Refresh()
	if !changed {
		t.Fatal("rate-limit refresh should repaint cached rows with toast")
	}
	view := ansi.Strip(app.ViewSize(140, 24))
	for _, want := range []string{"cached integration run", "GitHub API · 403", "API rate limit exceeded", "auto-resume in 42s", "reset 20:04 UTC"} {
		if !strings.Contains(view, want) {
			t.Fatalf("rate-limit view missing %q:\n%s", want, view)
		}
	}
}

func TestActionPermissionErrorKeepsCurrentScreenAndShowsToast(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:           8200,
				Name:         "CI",
				DisplayTitle: "read only run",
				RunNumber:    8200,
				Status:       model.StatusCompleted,
				Conclusion:   model.ConclusionFailure,
			}},
		},
		ActionHandler: func(ActionRequest) (usecase.ActionResult, error) {
			return usecase.ActionResult{}, usecase.ActionError{
				Kind:    usecase.ActionErrorPermission,
				Status:  403,
				Message: "Must have admin rights to Repository.",
			}
		},
	})

	app, _ = app.executeAction(RouteRuns, ActionRequest{
		Action: usecase.ActionRerunFailedJobs,
		Run:    model.Run{ID: 8200, RunNumber: 8200},
	})

	view := ansi.Strip(app.ViewSize(120, 24))
	for _, want := range []string{"read only run", "Mutation rejected", "Must have admin rights"} {
		if !strings.Contains(view, want) {
			t.Fatalf("action permission view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "hound · empty") {
		t.Fatalf("action error should not replace current screen:\n%s", view)
	}
}

func TestRunsOpenBrowserAndCopyUseSelectedRunURL(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	var opened, copied string
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:   "openclaw/openclaw",
			Branch: "main",
			Scope:  usecase.LaunchScopeBranch,
			State:  usecase.LaunchStateRuns,
			Runs: []model.Run{{
				ID:         8300,
				Name:       "CI",
				RunNumber:  8300,
				HTMLURL:    "https://github.com/openclaw/openclaw/actions/runs/8300",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}},
		},
		OpenURL:  func(value string) error { opened = value; return nil },
		CopyText: func(value string) error { copied = value; return nil },
	})

	app, handled := app.Update(KeyMsg{Key: "o"})
	if !handled || opened != "https://github.com/openclaw/openclaw/actions/runs/8300" {
		t.Fatalf("open handled=%v opened=%q", handled, opened)
	}
	app, handled = app.Update(KeyMsg{Key: "y"})
	if !handled || copied != opened {
		t.Fatalf("copy handled=%v copied=%q want %q", handled, copied, opened)
	}
	view := ansi.Strip(app.ViewSize(120, 24))
	if !strings.Contains(view, "Copied") {
		t.Fatalf("copy toast missing:\n%s", view)
	}
}

func TestDetailOpenBrowserAndCopyUseSelectedJobAndRun(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	var opened, copied string
	run := model.Run{
		ID:         8400,
		Name:       "CI",
		RunNumber:  8400,
		HeadSHA:    "abcdef1234567890",
		HTMLURL:    "https://github.com/openclaw/openclaw/actions/runs/8400",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
	}
	job := model.Job{
		ID:         8401,
		RunID:      8400,
		Name:       "build",
		HTMLURL:    "https://github.com/openclaw/openclaw/actions/runs/8400/job/8401",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
	}
	app := NewApp(Options{
		Config:   cfg,
		Launch:   usecase.LaunchContext{Repo: "openclaw/openclaw", Branch: "main", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, Runs: []model.Run{run}},
		OpenURL:  func(value string) error { opened = value; return nil },
		CopyText: func(value string) error { copied = value; return nil },
		DetailResolver: func(context.Context, model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{job}).WithRepo("openclaw/openclaw"), nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)

	app, handled := app.Update(KeyMsg{Key: "o"})
	if !handled || opened != job.HTMLURL {
		t.Fatalf("detail open handled=%v opened=%q", handled, opened)
	}
	app, handled = app.Update(KeyMsg{Key: "y"})
	if !handled || copied != run.HTMLURL {
		t.Fatalf("detail copy url handled=%v copied=%q", handled, copied)
	}
	_, handled = app.Update(KeyMsg{Key: "Y"})
	if !handled || copied != run.HeadSHA {
		t.Fatalf("detail copy sha handled=%v copied=%q", handled, copied)
	}
}

func TestFailureOpenBrowserAndCopyExcerpt(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	var opened, copied string
	run := model.Run{ID: 8500, Name: "CI", RunNumber: 8500, HTMLURL: "https://github.com/openclaw/openclaw/actions/runs/8500"}
	report := externalFailureReport()
	report.Job.HTMLURL = "https://github.com/openclaw/openclaw/actions/runs/8500/job/8501"
	app := NewApp(Options{
		Config:   cfg,
		Launch:   usecase.LaunchContext{Repo: "openclaw/openclaw", Branch: "main", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, Runs: []model.Run{run}},
		OpenURL:  func(value string) error { opened = value; return nil },
		CopyText: func(value string) error { copied = value; return nil },
		DetailResolver: func(context.Context, model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{report.Job}).WithRepo("openclaw/openclaw"), nil
		},
		FailureResolver: func(context.Context, model.Run, model.Job) (failurescreen.Model, logscreen.Model, error) {
			return failurescreen.NewModel("openclaw/openclaw", run.ID, report), logscreen.NewModel(report.Log, 1, 6), nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)

	app, handled := app.Update(KeyMsg{Key: "o"})
	if !handled || opened != report.Job.HTMLURL {
		t.Fatalf("failure open handled=%v opened=%q", handled, opened)
	}
	_, handled = app.Update(KeyMsg{Key: "y"})
	if !handled || !strings.Contains(copied, "trailing_underscore") || !strings.Contains(copied, "Process completed") {
		t.Fatalf("failure copy handled=%v copied=%q", handled, copied)
	}
}

func TestNestedMutationShortcutsRequireConfirmation(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	run := model.Run{ID: 9001, Name: "CI", RunNumber: 44, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, HeadBranch: "main"}
	job := model.Job{ID: 7001, RunID: run.ID, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	tests := []struct {
		name   string
		key    string
		action usecase.Action
		setup  func(App) App
		label  string
	}{
		{
			name:   "detail rerun job",
			key:    "r",
			action: usecase.ActionRerunJob,
			setup: func(app App) App {
				app.detail = detail.NewModel(run, []model.Job{job})
				app.PushRoute(RouteDetail)
				return app
			},
			label: "rerun job build",
		},
		{
			name:   "failure rerun failed",
			key:    "R",
			action: usecase.ActionRerunFailedJobs,
			setup: func(app App) App {
				app.detail = detail.NewModel(run, []model.Job{job})
				app.failure = failurescreenModelForTest(run, job)
				app.PushRoute(RouteFailure)
				return app
			},
			label: "rerun failed jobs for run #44",
		},
		{
			name:   "watch cancel",
			key:    "x",
			action: usecase.ActionCancelRun,
			setup: func(app App) App {
				app.watch = watchModelForTest(run)
				app.PushRoute(RouteWatch)
				return app
			},
			label: "cancel run #44",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []ActionRequest{}
			app := NewApp(Options{
				Config: cfg,
				Launch: usecase.LaunchContext{
					Repo:       "openclaw/openclaw",
					Branch:     "main",
					State:      usecase.LaunchStateRuns,
					Runs:       []model.Run{run},
					BranchRuns: []model.Run{run},
				},
				ActionHandler: func(request ActionRequest) (usecase.ActionResult, error) {
					calls = append(calls, request)
					return usecase.ActionResult{Action: request.Action, RunID: request.Run.ID, JobID: request.Job.ID, Message: "accepted"}, nil
				},
			})
			app = tt.setup(app)
			app, handled := app.Update(KeyMsg{Key: tt.key})
			if !handled || app.TopOverlay() != OverlayConfirm || len(calls) != 0 {
				t.Fatalf("%s should confirm before mutation: handled=%v top=%s calls=%d", tt.name, handled, app.TopOverlay(), len(calls))
			}
			if view := ansi.Strip(app.ViewSize(120, 32)); !strings.Contains(view, tt.label) {
				t.Fatalf("confirm modal missing %q:\n%s", tt.label, view)
			}
			app, handled = app.Update(KeyMsg{Key: "y"})
			if !handled || app.TopOverlay() != OverlayNone || len(calls) != 1 || calls[0].Action != tt.action {
				t.Fatalf("%s confirmed state: handled=%v top=%s calls=%#v", tt.name, handled, app.TopOverlay(), calls)
			}
		})
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

func TestLogRefetchNoticeShowsToastWhileKeepingRecoveredLog(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	run := model.Run{ID: 9100, Name: "CI", RunNumber: 91}
	job := model.Job{ID: 9101, RunID: 9100, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:       "openclaw/openclaw",
			Branch:     "main",
			State:      usecase.LaunchStateRuns,
			Runs:       []model.Run{run},
			BranchRuns: []model.Run{run},
		},
		DetailResolver: func(context.Context, model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{job}).WithRepo("openclaw/openclaw"), nil
		},
		LogResolver: func(context.Context, model.Run, model.Job, func(read, total int64)) (logscreen.Model, error) {
			return logscreen.NewModel(logs.Parse("001 recovered log line\n##[error]still visible"), 1, 6), nil
		},
		LogRefetchNotice: func(jobID int64) (usecase.LogRefetchNotice, bool) {
			if jobID != job.ID {
				return usecase.LogRefetchNotice{}, false
			}
			return usecase.LogRefetchNotice{
				JobID:         job.ID,
				Attempts:      2,
				ExpiredStatus: 404,
				Message:       "link had expired; re-requested job log",
			}, true
		},
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, handled := app.Update(KeyMsg{Key: "l"})
	if !handled || app.Route() != RouteLog {
		t.Fatalf("log open handled=%v route=%s", handled, app.Route())
	}
	app = settleApp(t, app)
	view := ansi.Strip(app.ViewSize(120, 28))
	for _, want := range []string{"recovered log line", "Log render failed", "link had expired"} {
		if !strings.Contains(view, want) {
			t.Fatalf("log refetch view missing %q:\n%s", want, view)
		}
	}
}

func TestFailureRouteShowsLogRefetchToastAfterRecoveredFailureLoad(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	run := model.Run{ID: 9200, Name: "CI", RunNumber: 92}
	job := model.Job{ID: 9201, RunID: 9200, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	report := usecase.FailureReport{
		Job: job,
		Log: logs.Parse(strings.Join([]string{
			"001 setup",
			"002 ##[error]Process completed with exit code 1",
		}, "\n")),
	}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:       "openclaw/openclaw",
			Branch:     "main",
			State:      usecase.LaunchStateRuns,
			Runs:       []model.Run{run},
			BranchRuns: []model.Run{run},
		},
		DetailResolver: func(context.Context, model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{job}).WithRepo("openclaw/openclaw"), nil
		},
		FailureResolver: func(context.Context, model.Run, model.Job) (failurescreen.Model, logscreen.Model, error) {
			return failurescreen.NewModel("openclaw/openclaw", run.ID, report), logscreen.NewModel(report.Log, 1, 6), nil
		},
		LogRefetchNotice: func(jobID int64) (usecase.LogRefetchNotice, bool) {
			if jobID != job.ID {
				return usecase.LogRefetchNotice{}, false
			}
			return usecase.LogRefetchNotice{
				JobID:         job.ID,
				Attempts:      2,
				ExpiredStatus: 410,
				Message:       "link had expired; re-requested job log",
			}, true
		},
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteFailure {
		t.Fatalf("failure open handled=%v route=%s", handled, app.Route())
	}
	app = settleApp(t, app)
	view := ansi.Strip(app.ViewSize(120, 28))
	for _, want := range []string{"Process completed with exit code 1", "Log render failed", "HTTP 410"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failure refetch view missing %q:\n%s", want, view)
		}
	}
}

func failurescreenModelForTest(run model.Run, job model.Job) failurescreen.Model {
	raw := strings.Join([]string{
		"##[group] build",
		"FAIL package/test",
		"##[error]Process completed with exit code 1",
		"##[endgroup]",
	}, "\n")
	report := usecase.FailureReport{
		Job: job,
		Log: logs.Parse(raw),
	}
	return failurescreen.NewModel("openclaw/openclaw", run.ID, report)
}

func externalFailureReport() usecase.FailureReport {
	raw := strings.Join([]string{
		"##[group] test output",
		"RUN TestLexIdent/trailing_underscore",
		"lexer_test.go:88: got \"foo\" want \"foo_\"",
		"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"##[error]Process completed with exit code 1",
	}, "\n")
	return usecase.FailureReport{
		Job: model.Job{
			ID:         8501,
			RunID:      8500,
			Name:       "build",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
			Steps: []model.Step{{
				Name:       "go test ./...",
				Number:     6,
				Conclusion: model.ConclusionFailure,
			}},
		},
		Log: logs.Parse(raw),
	}
}

func watchModelForTest(run model.Run) watchscreen.Model {
	return watchscreen.NewModel(watchscreen.State{
		Repo:    "openclaw/openclaw",
		Branch:  "main",
		Run:     run,
		Elapsed: "1m02s",
	})
}

func TestPaletteArtifactsEntryReachesDetail(t *testing.T) {
	app := artifactsTestApp(t, nil)
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"a", "r", "t", "i"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("palette artifacts should open detail: handled=%v route=%s", handled, app.Route())
	}
	if app.detail.Focus != detail.FocusArtifacts && len(app.detail.Artifacts) == 0 {
		// artifacts may still be loading; focus request happens after load
		app = waitForArtifacts(t, app)
	}
}

func TestServerTaggedFilterSkipsLocalSubstringMatch(t *testing.T) {
	m := runs.NewModel(usecase.LaunchContext{
		Repo:  "openclaw/openclaw",
		State: usecase.LaunchStateRuns,
		Runs:  []model.Run{cliTestRun(1, "CI", "fix/parser")},
	})
	m.Filter = "branch:fix/parser"
	m.ServerFiltered = true
	if len(m.FilteredRuns()) != 1 {
		t.Fatalf("server-filtered results must not be re-filtered locally: %d visible", len(m.FilteredRuns()))
	}
}

func TestEscClearsAppliedFilter(t *testing.T) {
	m := runs.NewModel(usecase.LaunchContext{
		Repo:  "indrasvat/gh-hound",
		State: usecase.LaunchStateRuns,
		Runs:  []model.Run{cliTestRun(1, "CI", "main")},
	})
	for _, key := range []string{"/", "z", "z", "enter"} {
		m = m.Update(runs.KeyMsg{Key: key})
	}
	m = m.Update(runs.KeyMsg{Key: "esc"})
	if m.Filter != "" {
		t.Fatalf("esc must clear the applied filter, got %q", m.Filter)
	}
	if m.Intent.Kind != runs.IntentFilter || m.Intent.Filter != "" {
		t.Fatalf("esc must emit an empty filter intent to reload: %#v", m.Intent)
	}
}

func cliTestRun(id int64, workflow, branch string) model.Run {
	return model.Run{ID: id, Name: workflow, HeadBranch: branch, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: int(id)}
}

func TestRunningVocabularyMapsToServerStatus(t *testing.T) {
	filter, ok := serverRunFilter(usecase.LaunchContext{Repo: "openclaw/openclaw"}, 30, "running")
	if !ok || filter.Status != string(model.StatusInProgress) {
		t.Fatalf("running must map to in_progress server filter: ok=%v status=%q", ok, filter.Status)
	}
	// The status bar teaches this vocabulary; every word it shows must filter.
	for _, word := range []string{"failing", "running", "passed"} {
		if _, ok := serverRunFilter(usecase.LaunchContext{Repo: "x/y"}, 30, word); !ok {
			t.Fatalf("status-bar word %q must be a valid server filter", word)
		}
	}
}

func TestRunningAliasMatchesLocally(t *testing.T) {
	m := runs.NewModel(usecase.LaunchContext{
		Repo:  "x/y",
		State: usecase.LaunchStateRuns,
		Runs:  []model.Run{{ID: 1, Name: "CI", Status: model.StatusInProgress, RunNumber: 1}},
	})
	m.Filter = "running"
	if len(m.FilteredRuns()) != 1 {
		t.Fatalf("local 'running' alias must match in_progress runs")
	}
}

func TestEscClearingServerFilterRestoresUnfilteredRuns(t *testing.T) {
	full := []model.Run{
		cliTestRun(1, "CI", "main"),
		cliTestRun(2, "Release", "main"),
		cliTestRun(3, "CI", "fix/x"),
	}
	running := []model.Run{{ID: 9, Name: "CI", HeadBranch: "main", Status: model.StatusInProgress, RunNumber: 9}}
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{Repo: "x/y", Branch: "main", State: usecase.LaunchStateRuns, Runs: full},
		RunsResolver: func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
			if filter.Status != "" {
				return running, nil
			}
			return full, nil
		},
	})
	for _, key := range []string{"/", "r", "u", "n", "n", "i", "n", "g", "enter"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app = settleApp(t, app)
	if got := len(app.runs.FilteredRuns()); got != 1 {
		t.Fatalf("server filter applied = %d rows, want 1", got)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app = settleApp(t, app)
	if got := len(app.runs.FilteredRuns()); got != len(full) {
		t.Fatalf("esc must restore the unfiltered listing immediately: %d rows, want %d", got, len(full))
	}
}

func TestPaletteOpenDoesNotToastDispatchResolutionErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{Repo: "x/y", State: usecase.LaunchStateRuns, Runs: []model.Run{cliTestRun(1, "CI", "main")}},
		DispatchWorkflowsResolver: func(context.Context) ([]dispatch.Workflow, error) {
			return nil, errors.New("dispatch ref is unavailable; pass --branch or run from a checkout")
		},
	})
	app, handled := app.Update(KeyMsg{Key: ":"})
	if !handled || app.TopOverlay() != OverlayPalette {
		t.Fatalf("palette should open: %s", app.TopOverlay())
	}
	if len(app.toasts.Toasts) != 0 {
		t.Fatalf("palette open must not push dispatch resolution toasts: %#v", app.toasts.Toasts)
	}

	// Selecting dispatch is when the failure becomes the user's problem.
	for _, key := range []string{"d", "i", "s", "p"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "dispatch unavailable") {
		t.Fatalf("selecting dispatch must surface the resolution error:\n%s", view)
	}
}

func TestTimeJumpModalFlow(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
	app.config.Welcome = false
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "l"})
	app = settleApp(t, app)
	if app.Route() != RouteLog {
		t.Fatalf("setup: expected log route, got %s", app.Route())
	}
	app, handled := app.Update(KeyMsg{Key: "t"})
	if !handled || app.TopOverlay() != OverlayTimeJump {
		t.Fatalf("t must open the time-jump modal: %s", app.TopOverlay())
	}
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "Jump to time") || !strings.Contains(view, "t→▌") {
		t.Fatalf("modal must render title and input cursor:\n%s", view)
	}
	for _, key := range []string{"1", "7", ":", "4", "2"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	view = ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "t→17:42▌") {
		t.Fatalf("modal must echo input:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.TopOverlay() != OverlayNone || app.Route() != RouteLog {
		t.Fatalf("enter must close modal and stay on log: overlay=%s route=%s", app.TopOverlay(), app.Route())
	}
	if app.log.LastJump != "17:42" {
		t.Fatalf("jump must land: LastJump=%q offset=%d", app.log.LastJump, app.log.Offset)
	}

	// esc path: opens and cancels without moving or popping the log
	beforeOffset := app.log.Offset
	app, _ = app.Update(KeyMsg{Key: "t"})
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() != RouteLog || app.log.Offset != beforeOffset {
		t.Fatalf("modal esc must cancel in place: route=%s", app.Route())
	}
}

func TestLogSearchEscStaysOnLog(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
	app.config.Welcome = false
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "l"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "/"})
	app, _ = app.Update(KeyMsg{Key: "x"})
	app, handled := app.Update(KeyMsg{Key: "esc"})
	if !handled || app.Route() != RouteLog {
		t.Fatalf("esc during search input must only cancel the input: route=%s", app.Route())
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() == RouteLog {
		t.Fatal("second esc must pop the log route")
	}
}

func TestTimeJumpPickerAndRangeFlow(t *testing.T) {
	app := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
	app.config.Welcome = false
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "l"})
	app = settleApp(t, app)
	app, _ = app.Update(KeyMsg{Key: "t"})
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "failure window") {
		t.Fatalf("picker must list the failure window entry:\n%s", view)
	}
	// picker enter with default selection jumps somewhere valid
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.TopOverlay() != OverlayNone || app.Route() != RouteLog {
		t.Fatalf("picker commit must close modal on log: overlay=%s", app.TopOverlay())
	}

	// invalid input keeps the modal open with feedback
	app, _ = app.Update(KeyMsg{Key: "t"})
	for _, key := range []string{"9", "9", ":", "9", "9"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.TopOverlay() != OverlayTimeJump {
		t.Fatal("invalid query must keep the modal open")
	}
	view = ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "no line at/after 99:99") {
		t.Fatalf("feedback must be visible:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})

	// range flow: filter to the failing second, esc clears
	app, _ = app.Update(KeyMsg{Key: "t"})
	for _, key := range []string{"1", "7", ":", "4", "2", "-", "1", "7", ":", "4", "2"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.log.RangeLabel == "" {
		t.Fatalf("range commit must set the range filter")
	}
	view = ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "[17:42-17:42]") {
		t.Fatalf("header must show the active range:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.log.RangeLabel != "" || app.Route() != RouteLog {
		t.Fatalf("first esc clears the range and stays on log: %q %s", app.log.RangeLabel, app.Route())
	}
}

func TestGUsesRealViewportHeight(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("00:%02d:00Z line content", i%60)
	}
	app.log = logscreen.NewModel(logs.Parse(strings.Join(lines, "\n")), 1, 6)
	app.PushRoute(RouteLog)
	app = app.WithViewport(120, 40)
	app, _ = app.Update(KeyMsg{Key: "G"})
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "060 ") {
		t.Fatalf("G with a 40-row viewport must reach the last line:\n%s", view)
	}
}
