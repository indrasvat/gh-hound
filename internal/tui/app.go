package tui

import (
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/help"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/palette"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/welcome"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type BuildInfo = banner.BuildInfo

type Route string

const (
	RouteWelcome  Route = "welcome"
	RouteRuns     Route = "runs"
	RouteDetail   Route = "detail"
	RouteFailure  Route = "failure"
	RouteLog      Route = "log"
	RouteWatch    Route = "watch"
	RouteDispatch Route = "dispatch"
)

type Overlay string

const (
	OverlayNone    Overlay = ""
	OverlayHelp    Overlay = "help"
	OverlayPalette Overlay = "palette"
)

type KeyMsg struct {
	Key string
}

type Options struct {
	Config config.Config
	Build  BuildInfo
}

type App struct {
	config           config.Config
	build            BuildInfo
	theme            theme.Theme
	routes           []Route
	overlays         []Overlay
	inputMode        bool
	quit             bool
	welcomeDismissed bool
}

func NewApp(options Options) App {
	cfg := options.Config
	if cfg.Theme == "" {
		cfg = config.Default()
	}
	route := RouteRuns
	if cfg.Welcome {
		route = RouteWelcome
	}
	return App{
		config: cfg,
		build:  options.Build,
		theme:  theme.ForMode(theme.Mode(cfg.Theme)),
		routes: []Route{route},
	}
}

func (a App) Update(msg KeyMsg) (App, bool) {
	if a.inputMode {
		if msg.Key == "esc" {
			a.inputMode = false
			return a, true
		}
		return a, false
	}

	if len(a.overlays) > 0 {
		switch msg.Key {
		case "esc":
			a.overlays = a.overlays[:len(a.overlays)-1]
			return a, true
		case "?":
			a.overlays = append(a.overlays, OverlayHelp)
			return a, true
		case ":":
			a.overlays = append(a.overlays, OverlayPalette)
			return a, true
		case "q", "ctrl+c":
			a.quit = true
			return a, true
		default:
			return a, false
		}
	}

	switch msg.Key {
	case "T":
		a.toggleTheme()
		return a, true
	case "?":
		a.overlays = append(a.overlays, OverlayHelp)
		return a, true
	case ":":
		a.overlays = append(a.overlays, OverlayPalette)
		return a, true
	case "q", "ctrl+c":
		a.quit = true
		return a, true
	case "enter":
		if a.Route() == RouteWelcome {
			a.welcomeDismissed = true
			a.routes[len(a.routes)-1] = RouteRuns
			return a, true
		}
	}
	return a, false
}

func (a App) View() string {
	var out strings.Builder
	out.WriteString("hound · ")
	out.WriteString(string(a.Route()))
	out.WriteString("\n\n")
	if a.Route() == RouteWelcome {
		out.WriteString(welcome.View(welcome.Model{Build: a.build}))
	} else if a.Route() == RouteRuns {
		out.WriteString(runs.View(sampleRunsModel(), 80, time.Now()))
	} else if a.Route() == RouteDetail {
		out.WriteString(detail.View(sampleDetailModel(), 80))
	} else if a.Route() == RouteFailure {
		out.WriteString(failure.View(sampleFailureModel(), 80))
	} else if a.Route() == RouteLog {
		out.WriteString(logscreen.View(sampleLogModel(), 80))
	} else if a.Route() == RouteWatch {
		out.WriteString(watch.View(sampleWatchModel(), 80))
	} else if a.Route() == RouteDispatch {
		out.WriteString(dispatch.View(sampleDispatchModel(), 80))
	} else {
		out.WriteString(string(a.Route()))
	}
	out.WriteString("\n\n")
	out.WriteString(keys.FooterForScreen(a.footerScreen()))
	if a.TopOverlay() != OverlayNone {
		out.WriteString("\n\n")
		out.WriteString(a.overlayView())
	}
	return out.String()
}

func RenderFixture(screen string, width int) string {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "v0.1.0"}})
	switch screen {
	case "welcome":
		return NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}}).View()
	case "all_green":
		return runs.View(sampleAllGreenModel(), width, time.Now())
	case "runs":
		return runs.View(sampleRunsModel(), width, time.Now())
	case "detail":
		return detail.View(sampleDetailModel(), width)
	case "failure":
		return failure.View(sampleFailureModel(), width)
	case "watch":
		return watch.View(sampleWatchModel(), width)
	case "log":
		return logscreen.View(sampleLogModel(), width)
	case "dispatch":
		return dispatch.View(sampleDispatchModel(), width)
	case "palette":
		app, _ = app.Update(KeyMsg{Key: ":"})
		return app.View()
	case "help":
		app, _ = app.Update(KeyMsg{Key: "?"})
		return app.View()
	default:
		return app.View()
	}
}

func RenderInteractionFixture(scenario string, width int) string {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "v0.1.0"}})
	switch scenario {
	case "welcome-enter":
		app = NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}})
		app, _ = app.Update(KeyMsg{Key: "enter"})
		return app.View()
	case "global-help":
		app, _ = app.Update(KeyMsg{Key: "?"})
		return app.View()
	case "global-palette":
		app, _ = app.Update(KeyMsg{Key: ":"})
		return app.View()
	case "overlay-esc":
		app, _ = app.Update(KeyMsg{Key: "?"})
		app, _ = app.Update(KeyMsg{Key: ":"})
		app, _ = app.Update(KeyMsg{Key: "esc"})
		return app.View()
	case "runs-select":
		m := sampleRunsModel()
		m = m.Update(runs.KeyMsg{Key: "j"})
		return runs.View(m, width, time.Now())
	case "runs-filter":
		m := sampleRunsModel()
		for _, key := range []string{"/", "f", "a", "i", "l"} {
			m = m.Update(runs.KeyMsg{Key: key})
		}
		return runs.View(m, width, time.Now())
	case "detail-nav":
		m := sampleDetailModel()
		for _, key := range []string{"tab", "j", "n"} {
			m = m.Update(detail.KeyMsg{Key: key})
		}
		return detail.View(m, width)
	case "failure-actions":
		m := sampleFailureModel()
		for _, key := range []string{"l", "y", "o", "r", "R"} {
			m = m.Update(failure.KeyMsg{Key: key})
		}
		return failure.View(m, width)
	case "log-search-fold":
		m := sampleLogModel()
		for _, key := range []string{"/", "t", "r", "a", "i", "l", "enter", "z"} {
			m = m.Update(logscreen.KeyMsg{Key: key})
		}
		return logscreen.View(m, width)
	case "watch-toggle":
		m := sampleWatchModel()
		for _, key := range []string{"f", "d"} {
			m = m.Update(watch.KeyMsg{Key: key})
		}
		return watch.View(m, width)
	case "dispatch-fill":
		m := sampleDispatchModel()
		for _, key := range []string{"T", "v", "0", ".", "1", "2", ".", "0", "tab", "right", "tab", "right"} {
			m = m.Update(dispatch.KeyMsg{Key: key})
		}
		return dispatch.View(m, width)
	default:
		return app.View()
	}
}

func (a App) Route() Route {
	if len(a.routes) == 0 {
		return RouteRuns
	}
	return a.routes[len(a.routes)-1]
}

func (a App) Theme() theme.Theme {
	return a.theme
}

func (a App) TopOverlay() Overlay {
	if len(a.overlays) == 0 {
		return OverlayNone
	}
	return a.overlays[len(a.overlays)-1]
}

func (a App) ShouldQuit() bool {
	return a.quit
}

func (a App) WelcomeDismissed() bool {
	return a.welcomeDismissed
}

func (a App) InputMode() bool {
	return a.inputMode
}

func (a *App) SetInputMode(enabled bool) {
	a.inputMode = enabled
}

func (a *App) PushRoute(route Route) {
	a.routes = append(a.routes, route)
}

func (a *App) PopRoute() {
	if len(a.routes) > 1 {
		a.routes = a.routes[:len(a.routes)-1]
	}
}

func (a *App) toggleTheme() {
	if a.theme.Mode == theme.ModeBramble {
		a.config.Theme = config.ThemeBone
		a.theme = theme.ForMode(theme.ModeBone)
		return
	}
	a.config.Theme = config.ThemeBramble
	a.theme = theme.ForMode(theme.ModeBramble)
}

func (a App) footerScreen() keys.Screen {
	switch a.Route() {
	case RouteWelcome:
		return keys.ScreenWelcome
	case RouteDetail:
		return keys.ScreenDetail
	case RouteFailure:
		return keys.ScreenFailure
	case RouteLog:
		return keys.ScreenLog
	case RouteWatch:
		return keys.ScreenWatch
	case RouteDispatch:
		return keys.ScreenDispatch
	default:
		return keys.ScreenRunsList
	}
}

func (a App) overlayView() string {
	switch a.TopOverlay() {
	case OverlayHelp:
		return help.View(a.footerScreen(), 80)
	case OverlayPalette:
		return palette.View(palette.New(palette.DefaultItems()), 80)
	default:
		return ""
	}
}

func sampleDetailModel() detail.Model {
	start := time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC)
	run := model.Run{
		ID:         571,
		Name:       "CI",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
		HeadBranch: "fix/parser",
		HeadSHA:    "a1b2c3d",
		RunNumber:  571,
	}
	jobs := []model.Job{{
		ID:          100,
		Name:        "build",
		Status:      model.StatusCompleted,
		Conclusion:  model.ConclusionFailure,
		Labels:      []string{"ubuntu-latest"},
		StartedAt:   start,
		CompletedAt: start.Add(134 * time.Second),
		Steps: []model.Step{{
			Number:      6,
			Name:        "go test ./...",
			Status:      model.StatusCompleted,
			Conclusion:  model.ConclusionFailure,
			StartedAt:   start,
			CompletedAt: start.Add(41 * time.Second),
		}},
	}}
	return detail.NewModel(run, jobs)
}

func sampleRunsModel() runs.Model {
	now := time.Date(2026, 6, 7, 17, 45, 0, 0, time.UTC)
	return runs.NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "fix/parser",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{
			{ID: 571, Name: "CI", Event: "pull_request", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunNumber: 571, UpdatedAt: now.Add(-12 * time.Second), RunStartedAt: now.Add(-2 * time.Minute)},
			{ID: 570, Name: "CI", Event: "push", Status: model.StatusInProgress, Conclusion: model.ConclusionNone, RunNumber: 570, UpdatedAt: now, RunStartedAt: now.Add(-48 * time.Second)},
			{ID: 569, Name: "Release", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 569, UpdatedAt: now.Add(-3 * time.Minute), RunStartedAt: now.Add(-4 * time.Minute)},
		},
	})
}

func sampleAllGreenModel() runs.Model {
	now := time.Date(2026, 6, 7, 17, 45, 0, 0, time.UTC)
	return runs.NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateAllGreen,
		Runs: []model.Run{
			{ID: 569, Name: "CI", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 569, UpdatedAt: now.Add(-3 * time.Minute), RunStartedAt: now.Add(-4 * time.Minute)},
			{ID: 568, Name: "Release", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 568, UpdatedAt: now.Add(-1 * time.Hour), RunStartedAt: now.Add(-61 * time.Minute)},
			{ID: 567, Name: "CodeQL", Event: "schedule", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 567, UpdatedAt: now.Add(-2 * time.Hour), RunStartedAt: now.Add(-121 * time.Minute)},
		},
	})
}

func sampleFailureModel() failure.Model {
	report := usecase.FailureReport{
		Job: model.Job{
			ID:         100,
			RunID:      571,
			Name:       "build",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
			Steps: []model.Step{{
				Number:     6,
				Name:       "go test ./...",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}},
		},
		Log: logs.Parse(strings.Join([]string{
			"17:42:53.114Z go test ./... -race",
			"=== RUN   TestLexIdent/trailing_underscore",
			"    internal/parser/lexer.go:142: got \"foo\" want \"foo_\"",
			"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
			"##[error]Process completed with exit code 1",
		}, "\n")),
		Annotations: []model.Annotation{{
			Path:      "internal/parser/lexer.go",
			StartLine: 142,
			Message:   "identifier mismatch",
		}},
	}
	return failure.NewModel("indrasvat/gh-hound", 571, report)
}

func sampleLogModel() logscreen.Model {
	doc := logs.Parse(strings.Join([]string{
		"17:42:53Z go test ./... -race",
		"##[group] Run go test ./...",
		"ok    internal/api 0.214s",
		"##[group] test output",
		"=== RUN   TestLexIdent/trailing_underscore",
		"    lexer_test.go:88: got \"foo\" want \"foo_\"",
		"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"FAIL  github.com/indrasvat/gh-hound/internal/parser  0.412s",
		"##[error]Process completed with exit code 1",
	}, "\n"))
	return logscreen.NewModel(doc, 1, 6)
}

func sampleWatchModel() watch.Model {
	doc := logs.Parse(strings.Join([]string{
		"041 17:43:02.781Z go test ./... -race -count=1",
		"042 ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
		"043 ok    github.com/indrasvat/gh-hound/internal/render 0.331s",
	}, "\n"))
	return watch.NewModel(watch.State{
		Repo:    "indrasvat/gh-hound",
		Branch:  "main",
		Elapsed: "0m48s",
		Run: model.Run{
			ID:        570,
			Name:      "CI",
			RunNumber: 570,
			Status:    model.StatusInProgress,
		},
		Lines: doc.Lines,
	})
}

func sampleDispatchModel() dispatch.Model {
	return dispatch.NewModel(dispatch.Workflow{
		Name: "Release",
		ID:   "release.yml",
		Ref:  "main",
		Inputs: []dispatch.Input{
			{Name: "version", Required: true, Type: dispatch.InputText},
			{Name: "prerelease", Type: dispatch.InputBool, Options: []string{"false", "true"}},
			{Name: "channel", Type: dispatch.InputSelect, Options: []string{"stable", "beta", "nightly"}},
		},
	})
}
