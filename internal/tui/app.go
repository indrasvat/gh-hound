package tui

import (
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/tui/screens/welcome"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type BuildInfo = banner.BuildInfo

type Route string

const (
	RouteWelcome Route = "welcome"
	RouteRuns    Route = "runs"
	RouteDetail  Route = "detail"
	RouteFailure Route = "failure"
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
		out.WriteString(runs.View(runs.NewModel(usecase.LaunchContext{
			Repo:   "indrasvat/gh-hound",
			Branch: "main",
			Actor:  "indrasvat",
			State:  usecase.LaunchStateRuns,
		}), 80, time.Now()))
	} else if a.Route() == RouteDetail {
		out.WriteString(detail.View(sampleDetailModel(), 80))
	} else {
		out.WriteString(string(a.Route()))
	}
	out.WriteString("\n\n")
	out.WriteString(keys.FooterForScreen(a.footerScreen()))
	return out.String()
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
	default:
		return keys.ScreenRunsList
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
