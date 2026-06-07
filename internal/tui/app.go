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
	runs             runs.Model
	detail           detail.Model
	failure          failure.Model
	log              logscreen.Model
	watch            watch.Model
	dispatch         dispatch.Model
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
		config:   cfg,
		build:    options.Build,
		theme:    theme.ForMode(theme.Mode(cfg.Theme)),
		routes:   []Route{route},
		runs:     sampleRunsModel(),
		detail:   sampleDetailModel(),
		failure:  sampleFailureModel(),
		log:      sampleLogModel(),
		watch:    sampleWatchModel(),
		dispatch: sampleDispatchModel(),
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

	if a.routeInputMode() {
		return a.updateRoute(msg)
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
	return a.updateRoute(msg)
}

func (a App) View() string {
	return a.ViewSized(80)
}

func (a App) ViewSized(width int) string {
	return a.ViewSize(width, 0)
}

func (a App) ViewSize(width, height int) string {
	title, context, right := a.chromeParts()
	body := a.screenBody(width)
	footer := keys.FooterForScreen(a.footerScreen())
	if a.TopOverlay() != OverlayNone {
		body = overlayBox(a.theme, a.overlayTitle(), a.overlayView(width), width)
		footer = keys.FooterForScreen(keys.ScreenHelp)
		if a.TopOverlay() == OverlayPalette {
			footer = keys.FooterForScreen(keys.ScreenPalette)
		}
	}
	return frameViewSize(a.theme, title, context, right, body, footer, width, height, true)
}

func RenderFixture(screen string, width int) string {
	return RenderFixtureSize(screen, width, 0)
}

func RenderFixtureSize(screen string, width, height int) string {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "v0.1.0"}})
	bodyWidth := contentWidth(width)
	switch screen {
	case "welcome":
		return NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}}).ViewSize(width, height)
	case "all_green":
		return frameViewSize(app.theme, "hound", "git main · @indrasvat", "live · cache 304", runs.View(sampleAllGreenModel(), bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenAllGreen), width, height, true)
	case "runs":
		return frameViewSize(app.theme, "hound", "git fix/parser · @indrasvat", "◔ live · cache 304", runs.View(sampleRunsModel(), bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenRunsList), width, height, true)
	case "detail":
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "a1b2c3d", detail.View(sampleDetailModel(), bodyWidth), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
	case "failure":
		return frameViewSize(app.theme, "hound", "build › failed step", "exit 1", failure.View(sampleFailureModel(), bodyWidth), keys.FooterForScreen(keys.ScreenFailure), width, height, true)
	case "watch":
		return frameViewSize(app.theme, "hound", "CI #570", "streaming · follow ●", watch.View(sampleWatchModel(), bodyWidth), keys.FooterForScreen(keys.ScreenWatch), width, height, true)
	case "log":
		return frameViewSize(app.theme, "hound", "full log", "match 1/1", logscreen.View(sampleLogModel(), bodyWidth), keys.FooterForScreen(keys.ScreenLog), width, height, true)
	case "dispatch":
		return frameViewSize(app.theme, "hound", "workflow_dispatch", "Release", dispatch.View(sampleDispatchModel(), bodyWidth), keys.FooterForScreen(keys.ScreenDispatch), width, height, true)
	case "palette":
		app, _ = app.Update(KeyMsg{Key: ":"})
		return app.ViewSize(width, height)
	case "help":
		app, _ = app.Update(KeyMsg{Key: "?"})
		return app.ViewSize(width, height)
	default:
		return app.ViewSize(width, height)
	}
}

func RenderInteractionFixture(scenario string, width int) string {
	return RenderInteractionFixtureSize(scenario, width, 0)
}

func RenderInteractionFixtureSize(scenario string, width, height int) string {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "v0.1.0"}})
	bodyWidth := contentWidth(width)
	switch scenario {
	case "welcome-enter":
		app = NewApp(Options{Config: config.Default(), Build: BuildInfo{Version: "v0.1.0"}})
		app, _ = app.Update(KeyMsg{Key: "enter"})
		return app.ViewSize(width, height)
	case "global-help":
		app, _ = app.Update(KeyMsg{Key: "?"})
		return app.ViewSize(width, height)
	case "global-palette":
		app, _ = app.Update(KeyMsg{Key: ":"})
		return app.ViewSize(width, height)
	case "overlay-esc":
		app, _ = app.Update(KeyMsg{Key: "?"})
		app, _ = app.Update(KeyMsg{Key: ":"})
		app, _ = app.Update(KeyMsg{Key: "esc"})
		return app.ViewSize(width, height)
	case "runs-select":
		m := sampleRunsModel()
		m = m.Update(runs.KeyMsg{Key: "j"})
		return frameViewSize(app.theme, "hound", "git fix/parser · @indrasvat", "◔ live · cache 304", runs.View(m, bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenRunsList), width, height, true)
	case "runs-filter":
		m := sampleRunsModel()
		for _, key := range []string{"/", "f", "a", "i", "l"} {
			m = m.Update(runs.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "git fix/parser · @indrasvat", "filter /fail", runs.View(m, bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenRunsList), width, height, true)
	case "detail-nav":
		m := sampleDetailModel()
		for _, key := range []string{"tab", "j", "n"} {
			m = m.Update(detail.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "a1b2c3d", detail.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
	case "failure-actions":
		m := sampleFailureModel()
		for _, key := range []string{"l", "y", "o", "r", "R"} {
			m = m.Update(failure.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "build › failed step", "actions queued", failure.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenFailure), width, height, true)
	case "log-search-fold":
		m := sampleLogModel()
		for _, key := range []string{"/", "t", "r", "a", "i", "l", "enter", "z"} {
			m = m.Update(logscreen.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "full log", "search /trail", logscreen.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenLog), width, height, true)
	case "watch-toggle":
		m := sampleWatchModel()
		for _, key := range []string{"f", "d"} {
			m = m.Update(watch.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "CI #570", "debug · follow", watch.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenWatch), width, height, true)
	case "dispatch-fill":
		m := sampleDispatchModel()
		for _, key := range []string{"T", "v", "0", ".", "1", "2", ".", "0", "tab", "right", "tab", "right"} {
			m = m.Update(dispatch.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "workflow_dispatch", "Release", dispatch.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenDispatch), width, height, true)
	default:
		return app.ViewSize(width, height)
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

func (a App) routeInputMode() bool {
	if a.Route() == RouteRuns && a.runs.InputMode {
		return true
	}
	return false
}

func (a App) updateRoute(msg KeyMsg) (App, bool) {
	switch a.Route() {
	case RouteRuns:
		return a.updateRuns(msg)
	case RouteDetail:
		return a.updateDetail(msg)
	case RouteFailure:
		return a.updateFailure(msg)
	case RouteLog:
		return a.updateLog(msg)
	case RouteWatch:
		return a.updateWatch(msg)
	case RouteDispatch:
		return a.updateDispatch(msg)
	default:
		return a, false
	}
}

func (a App) updateRuns(msg KeyMsg) (App, bool) {
	before := a.runs
	a.runs = a.runs.Update(runs.KeyMsg{Key: msg.Key})
	switch a.runs.Intent.Kind {
	case runs.IntentOpenDetail:
		a.PushRoute(RouteDetail)
	case runs.IntentOpenLogs:
		a.PushRoute(RouteLog)
	case runs.IntentWatch:
		a.PushRoute(RouteWatch)
	case runs.IntentDispatch:
		a.PushRoute(RouteDispatch)
	}
	return a, runsHandled(msg.Key) || before.Selected != a.runs.Selected || before.Filter != a.runs.Filter || before.InputMode != a.runs.InputMode || a.runs.Intent.Kind != runs.IntentNone
}

func (a App) updateDetail(msg KeyMsg) (App, bool) {
	beforeFocus := a.detail.Focus
	beforeJob := a.detail.SelectedJob
	beforeStep := a.detail.SelectedStep
	a.detail = a.detail.Update(detail.KeyMsg{Key: msg.Key})
	switch a.detail.Intent.Kind {
	case detail.IntentFailure:
		a.PushRoute(RouteFailure)
	case detail.IntentLog:
		a.PushRoute(RouteLog)
	case detail.IntentWatch:
		a.PushRoute(RouteWatch)
	case detail.IntentBack:
		a.PopRoute()
	}
	return a, detailHandled(msg.Key) || beforeFocus != a.detail.Focus || beforeJob != a.detail.SelectedJob || beforeStep != a.detail.SelectedStep || a.detail.Intent.Kind != detail.IntentNone
}

func (a App) updateFailure(msg KeyMsg) (App, bool) {
	a.failure = a.failure.Update(failure.KeyMsg{Key: msg.Key})
	switch a.failure.Intent.Kind {
	case failure.IntentFullLog:
		a.PushRoute(RouteLog)
	case failure.IntentBack:
		a.PopRoute()
	}
	return a, failureHandled(msg.Key) || a.failure.Intent.Kind != failure.IntentNone
}

func (a App) updateLog(msg KeyMsg) (App, bool) {
	before := a.log
	a.log = a.log.Update(logscreen.KeyMsg{Key: msg.Key})
	if msg.Key == "esc" {
		a.PopRoute()
		return a, true
	}
	searchChanged := before.Search.Query != a.log.Search.Query || before.Search.Current != a.log.Search.Current || before.Search.Total != a.log.Search.Total
	return a, logHandled(msg.Key) || before.Offset != a.log.Offset || before.Wrap != a.log.Wrap || searchChanged
}

func (a App) updateWatch(msg KeyMsg) (App, bool) {
	before := a.watch
	a.watch = a.watch.Update(watch.KeyMsg{Key: msg.Key})
	switch a.watch.Intent.Kind {
	case watch.IntentDetach:
		a.PopRoute()
	}
	return a, watchHandled(msg.Key) || before.Follow != a.watch.Follow || before.Debug != a.watch.Debug || a.watch.Intent.Kind != watch.IntentNone
}

func (a App) updateDispatch(msg KeyMsg) (App, bool) {
	beforeFocused := a.dispatch.Focused
	a.dispatch = a.dispatch.Update(dispatch.KeyMsg{Key: msg.Key})
	if a.dispatch.Intent.Kind == dispatch.IntentCancel {
		a.PopRoute()
		return a, true
	}
	return a, dispatchHandled(msg.Key) || beforeFocused != a.dispatch.Focused || a.dispatch.Intent.Kind != dispatch.IntentNone
}

func runsHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "g", "G", "/", "enter", "l", "w", "D", "o", "y", "r", "R", "x", "X", "esc", "backspace":
		return true
	default:
		return len([]rune(key)) == 1
	}
}

func detailHandled(key string) bool {
	switch key {
	case "tab", "j", "k", "down", "up", "n", "enter", "l", "w", "r", "R", "x", "X", "o", "J", "K", "esc":
		return true
	default:
		return false
	}
}

func failureHandled(key string) bool {
	switch key {
	case "l", "y", "o", "r", "R", "esc":
		return true
	default:
		return false
	}
}

func logHandled(key string) bool {
	switch key {
	case "j", "k", "g", "G", "/", "n", "N", "z", "Z", "w", "enter", "backspace", "esc":
		return true
	default:
		return len([]rune(key)) == 1
	}
}

func watchHandled(key string) bool {
	switch key {
	case "f", "d", "x", "esc":
		return true
	default:
		return false
	}
}

func dispatchHandled(key string) bool {
	switch key {
	case "tab", "shift+tab", "right", "space", "left", "backspace", "enter", "esc":
		return true
	default:
		return len([]rune(key)) == 1
	}
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

func (a App) overlayTitle() string {
	switch a.TopOverlay() {
	case OverlayHelp:
		return "help · gh hound"
	case OverlayPalette:
		return ": jump to…"
	default:
		return ""
	}
}

func (a App) overlayView(width int) string {
	switch a.TopOverlay() {
	case OverlayHelp:
		return help.View(a.footerScreen(), width-20)
	case OverlayPalette:
		return palette.View(palette.New(palette.DefaultItems()), width-20)
	default:
		return ""
	}
}

func (a App) chromeParts() (string, string, string) {
	switch a.Route() {
	case RouteWelcome:
		return "hound", "welcome · first run", a.build.Version
	case RouteDetail:
		return "hound", "CI #571 › fix/parser", "a1b2c3d"
	case RouteFailure:
		return "hound", "build › failed step", "exit 1"
	case RouteLog:
		return "hound", "full log", "match 1/1"
	case RouteWatch:
		return "hound", "CI #570", "streaming · follow ●"
	case RouteDispatch:
		return "hound", "workflow_dispatch", "Release"
	default:
		return "hound", "runs · git fix/parser · @indrasvat", "◔ live · cache 304"
	}
}

func (a App) screenBody(width int) string {
	bodyWidth := contentWidth(width)
	switch a.Route() {
	case RouteWelcome:
		return welcome.View(welcome.Model{Build: a.build})
	case RouteRuns:
		return runs.View(a.runs, bodyWidth, time.Now())
	case RouteDetail:
		return detail.View(a.detail, bodyWidth)
	case RouteFailure:
		return failure.View(a.failure, bodyWidth)
	case RouteLog:
		return logscreen.View(a.log, bodyWidth)
	case RouteWatch:
		return watch.View(a.watch, bodyWidth)
	case RouteDispatch:
		return dispatch.View(a.dispatch, bodyWidth)
	default:
		return string(a.Route())
	}
}

func contentWidth(width int) int {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	bodyWidth := max(width-2, 1)
	return bodyWidth
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
