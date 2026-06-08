package tui

import (
	"fmt"
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
	"github.com/indrasvat/gh-hound/internal/tui/screens/empty"
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
	Config         config.Config
	Build          BuildInfo
	Launch         usecase.LaunchContext
	DetailResolver func(model.Run) detail.Model
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
	launchRoute      Route
	runs             runs.Model
	detail           detail.Model
	failure          failure.Model
	log              logscreen.Model
	watch            watch.Model
	dispatch         dispatch.Model
	detailResolver   func(model.Run) detail.Model
}

func NewApp(options Options) App {
	cfg := options.Config
	if cfg.Theme == "" {
		cfg = config.Default()
	}
	launchRoute := routeForLaunch(options.Launch)
	route := launchRoute
	if cfg.Welcome {
		route = RouteWelcome
	}
	runsModel := sampleRunsModel()
	if hasLaunchContext(options.Launch) {
		runsModel = runs.NewModel(options.Launch)
	}
	detailResolver := options.DetailResolver
	if detailResolver == nil {
		detailResolver = DetailModelForRun
	}
	initialDetail := sampleDetailModel()
	if run, ok := runsModel.SelectedRun(); ok {
		initialDetail = detailResolver(run)
	}
	watchModel := watchModelForLaunch(options.Launch, runsModel)
	return App{
		config:         cfg,
		build:          options.Build,
		theme:          theme.ForMode(theme.Mode(cfg.Theme)),
		routes:         []Route{route},
		launchRoute:    launchRoute,
		runs:           runsModel,
		detail:         initialDetail,
		failure:        sampleFailureModel(),
		log:            sampleLogModel(),
		watch:          watchModel,
		dispatch:       sampleDispatchModel(),
		detailResolver: detailResolver,
	}
}

func NewScenarioApp(scenario string, build BuildInfo) App {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: build})
	switch strings.ToLower(scenario) {
	case "green", "ok", "success":
		app.runs = sampleAllGreenModel()
	}
	return app
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
			a.routes[len(a.routes)-1] = a.launchRoute
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
	body := a.screenBody(width, height)
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
		return frameViewSize(app.theme, "hound", "⌥ main · @indrasvat", "◔ 4,981/5k live", runs.View(sampleAllGreenModel(), bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenAllGreen), width, height, true)
	case "runs":
		return frameViewSize(app.theme, "hound", "⌥ fix/parser · @indrasvat", "◔ 4,981/5k live 304", runs.View(sampleRunsModel(), bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenRunsList), width, height, true)
	case "detail":
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "a1b2c3d", detail.View(sampleDetailModel(), bodyWidth), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
	case "failure":
		return frameViewSize(app.theme, "hound", "build › failed step", "exit 1", failure.View(sampleFailureModel(), bodyWidth), keys.FooterForScreen(keys.ScreenFailure), width, height, true)
	case "watch":
		return frameViewSize(app.theme, "hound", "CI #570", "streaming · follow ●", watch.View(sampleWatchModel(), bodyWidth), keys.FooterForScreen(keys.ScreenWatch), width, height, true)
	case "log":
		m := sampleLogModel()
		if rows := bodyHeight(height) - 1; rows > 0 {
			m.Height = rows
		}
		return frameViewSize(app.theme, "hound", "full log", "match 1/1", logscreen.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenLog), width, height, true)
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
	case "runs-long":
		m := sampleLongRunsModel(1000)
		m.Selected = 500
		return frameViewSize(app.theme, "hound", "git main · @indrasvat", "1,000 loaded", runs.ViewSize(m, bodyWidth, bodyHeight(height), time.Now()), keys.FooterForScreen(keys.ScreenAllGreen), width, height, true)
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
		if rows := bodyHeight(height) - 1; rows > 0 {
			m.Height = rows
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
		if run, ok := a.runs.SelectedRun(); ok {
			a.detail = a.detailResolver(run)
		}
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
	case "ctrl+d", "ctrl+u":
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
	case "ctrl+d", "ctrl+u":
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
		if a.Route() == RouteRuns && a.runs.AllGreen() {
			return keys.ScreenAllGreen
		}
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
		return "hound", detailContext(a.detail.Run), shortSHA(a.detail.Run.HeadSHA)
	case RouteFailure:
		return "hound", "build › failed step", "exit 1"
	case RouteLog:
		return "hound", "full log", "match 1/1"
	case RouteWatch:
		run := a.watch.State.Run
		return "hound", fmt.Sprintf("%s #%d", firstNonEmpty(run.Name, "workflow"), run.RunNumber), "streaming · follow ●"
	case RouteDispatch:
		return "hound", "workflow_dispatch", "Release"
	default:
		if a.runs.AllGreen() {
			return "hound", branchContext(a.runs.Context.Branch, a.runs.Context.Actor), "◔ 4,981/5k live"
		}
		return "hound", branchContext(a.runs.Context.Branch, a.runs.Context.Actor), "◔ 4,981/5k live 304"
	}
}

func hasLaunchContext(ctx usecase.LaunchContext) bool {
	return ctx.Repo != "" || ctx.Branch != "" || ctx.Actor != "" || ctx.State != "" || len(ctx.Runs) > 0 || len(ctx.Workflows) > 0 || ctx.Notice != "" || ctx.ErrorMessage != ""
}

func branchContext(branch, actor string) string {
	if branch == "" {
		branch = "all branches"
	}
	if actor == "" {
		actor = "indrasvat"
	}
	return "⌥ " + branch + " · @" + actor
}

func detailContext(run model.Run) string {
	name := firstNonEmpty(run.Name, run.DisplayTitle, run.Path, "workflow")
	branch := firstNonEmpty(run.HeadBranch, "branch")
	return fmt.Sprintf("%s #%d › %s", name, run.RunNumber, branch)
}

func shortSHA(sha string) string {
	if sha == "" {
		return "sha"
	}
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (a App) screenBody(width, height int) string {
	bodyWidth := contentWidth(width)
	switch a.Route() {
	case RouteWelcome:
		return welcome.View(welcome.Model{Build: a.build}, bodyWidth, max(height-6, 0))
	case RouteRuns:
		if body, ok := a.launchStateBody(bodyWidth); ok {
			return body
		}
		return runs.ViewSize(a.runs, bodyWidth, bodyHeight(height), time.Now())
	case RouteDetail:
		return detail.View(a.detail, bodyWidth)
	case RouteFailure:
		return failure.View(a.failure, bodyWidth)
	case RouteLog:
		logModel := a.log
		if rows := bodyHeight(height) - 1; rows > 0 {
			logModel.Height = rows
		}
		return logscreen.View(logModel, bodyWidth)
	case RouteWatch:
		return watch.View(a.watch, bodyWidth)
	case RouteDispatch:
		return dispatch.View(a.dispatch, bodyWidth)
	default:
		return string(a.Route())
	}
}

func routeForLaunch(ctx usecase.LaunchContext) Route {
	switch ctx.State {
	case usecase.LaunchStateWatch:
		return RouteWatch
	case usecase.LaunchStateDispatch:
		return RouteDispatch
	default:
		return RouteRuns
	}
}

func watchModelForLaunch(ctx usecase.LaunchContext, runsModel runs.Model) watch.Model {
	if ctx.State != usecase.LaunchStateWatch {
		return sampleWatchModel()
	}
	run, ok := runsModel.SelectedRun()
	if !ok {
		return sampleWatchModel()
	}
	sample := sampleWatchModel()
	return watch.NewModel(watch.State{
		Repo:    ctx.Repo,
		Branch:  firstNonEmpty(ctx.Branch, run.HeadBranch),
		Run:     run,
		Lines:   sample.State.Lines,
		Elapsed: sample.State.Elapsed,
	})
}

func (a App) launchStateBody(width int) (string, bool) {
	ctx := a.runs.Context
	switch ctx.State {
	case usecase.LaunchStateError:
		return empty.View(empty.Model{
			Kind:    empty.KindError,
			Repo:    ctx.Repo,
			Branch:  ctx.Branch,
			Message: ctx.ErrorMessage,
		}, width), true
	case usecase.LaunchStateEmpty:
		kind := empty.KindNoRuns
		if len(ctx.Workflows) == 0 {
			kind = empty.KindNoWorkflows
		}
		return empty.View(empty.Model{
			Kind:    kind,
			Repo:    ctx.Repo,
			Branch:  ctx.Branch,
			Message: ctx.Notice,
		}, width), true
	default:
		return "", false
	}
}

func contentWidth(width int) int {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	bodyWidth := max(width-2, 1)
	return bodyWidth
}

func bodyHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return 0
	}
	return max(frameHeight-6, 1)
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
	jobs := []model.Job{
		{
			ID:          100,
			Name:        "build",
			Status:      model.StatusCompleted,
			Conclusion:  model.ConclusionFailure,
			Labels:      []string{"ubuntu-latest"},
			StartedAt:   start,
			CompletedAt: start.Add(134 * time.Second),
			Steps: []model.Step{
				step(1, "Set up job", model.ConclusionSuccess, start, 400*time.Millisecond),
				step(2, "Checkout", model.ConclusionSuccess, start.Add(1*time.Second), 1200*time.Millisecond),
				step(3, "Setup Go 1.26", model.ConclusionSuccess, start.Add(3*time.Second), 3100*time.Millisecond),
				step(4, "Cache modules", model.ConclusionSuccess, start.Add(7*time.Second), 2*time.Second),
				step(5, "go build ./...", model.ConclusionSuccess, start.Add(10*time.Second), 18700*time.Millisecond),
				step(6, "go test ./...", model.ConclusionFailure, start.Add(31*time.Second), 41300*time.Millisecond),
				step(7, "Upload coverage", model.ConclusionSkipped, start.Add(74*time.Second), 0),
				step(8, "Complete job", model.ConclusionSuccess, start.Add(133*time.Second), 100*time.Millisecond),
			},
		},
		job(101, "lint", model.ConclusionSuccess, start.Add(5*time.Second), 31*time.Second),
		job(102, "test (1.25)", model.ConclusionSuccess, start.Add(7*time.Second), 108*time.Second),
		job(103, "test (1.26)", model.ConclusionSuccess, start.Add(9*time.Second), 112*time.Second),
		{ID: 104, Name: "deploy", Status: model.StatusQueued, Conclusion: model.ConclusionNone, Labels: []string{"ubuntu-latest"}},
	}
	return detail.NewModel(run, jobs)
}

func DetailModelForRun(run model.Run) detail.Model {
	if run.ID == 571 && run.Name == "CI" && run.RunNumber == 571 {
		return sampleDetailModel()
	}
	now := time.Now().UTC().Truncate(time.Second)
	started := run.RunStartedAt
	if started.IsZero() {
		started = run.UpdatedAt.Add(-90 * time.Second)
	}
	if started.IsZero() {
		started = now.Add(-90 * time.Second)
	}
	completed := run.UpdatedAt
	if completed.IsZero() {
		completed = started.Add(90 * time.Second)
	}
	jobStatus := run.Status
	jobConclusion := run.Conclusion
	if jobStatus == "" {
		jobStatus = model.StatusCompleted
	}
	job := model.Job{
		ID:          run.ID*100 + 1,
		RunID:       run.ID,
		Name:        firstNonEmpty(run.Name, run.DisplayTitle, "workflow"),
		Status:      jobStatus,
		Conclusion:  jobConclusion,
		Labels:      []string{"ubuntu-latest"},
		StartedAt:   started,
		CompletedAt: completed,
		Steps: []model.Step{
			stepForRun(1, "Set up job", model.StatusCompleted, model.ConclusionSuccess, started, 500*time.Millisecond),
			stepForRun(2, "Run "+firstNonEmpty(run.Name, "workflow"), jobStatus, jobConclusion, started.Add(time.Second), completed.Sub(started)-time.Second),
		},
	}
	if jobStatus != model.StatusCompleted {
		job.CompletedAt = time.Time{}
		job.Steps[1].CompletedAt = time.Time{}
	}
	return detail.NewModel(run, []model.Job{job})
}

func stepForRun(number int, name string, status model.Status, conclusion model.Conclusion, started time.Time, elapsed time.Duration) model.Step {
	completed := started.Add(maxDuration(elapsed, time.Second))
	if status != model.StatusCompleted {
		completed = time.Time{}
	}
	return model.Step{
		Number:      number,
		Name:        name,
		Status:      status,
		Conclusion:  conclusion,
		StartedAt:   started,
		CompletedAt: completed,
	}
}

func maxDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func job(id int64, name string, conclusion model.Conclusion, started time.Time, elapsed time.Duration) model.Job {
	return model.Job{
		ID:          id,
		Name:        name,
		Status:      model.StatusCompleted,
		Conclusion:  conclusion,
		Labels:      []string{"ubuntu-latest"},
		StartedAt:   started,
		CompletedAt: started.Add(elapsed),
		Steps: []model.Step{
			step(1, "Set up job", model.ConclusionSuccess, started, 500*time.Millisecond),
			step(2, "Run "+name, conclusion, started.Add(time.Second), elapsed-time.Second),
		},
	}
}

func step(number int, name string, conclusion model.Conclusion, started time.Time, elapsed time.Duration) model.Step {
	status := model.StatusCompleted
	completed := started.Add(elapsed)
	if conclusion == model.ConclusionNone {
		status = model.StatusQueued
		completed = time.Time{}
	}
	return model.Step{
		Number:      number,
		Name:        name,
		Status:      status,
		Conclusion:  conclusion,
		StartedAt:   started,
		CompletedAt: completed,
	}
}

func sampleRunsModel() runs.Model {
	now := time.Now().UTC().Truncate(time.Second)
	return runs.NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "fix/parser",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{
			{ID: 571, Name: "CI", DisplayTitle: "parser fix validation", Event: "pull_request", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunNumber: 571, Actor: "indrasvat", HeadBranch: "fix/parser", HeadSHA: "a1b2c3d", UpdatedAt: now.Add(-12 * time.Second), RunStartedAt: now.Add(-2 * time.Minute)},
			{ID: 570, Name: "CI", DisplayTitle: "push smoke", Event: "push", Status: model.StatusInProgress, Conclusion: model.ConclusionNone, RunNumber: 570, Actor: "indrasvat", HeadBranch: "fix/parser", HeadSHA: "b4c5d6e", UpdatedAt: now, RunStartedAt: now.Add(-48 * time.Second)},
			{ID: 569, Name: "Release", DisplayTitle: "snapshot release", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 569, Actor: "indrasvat", HeadBranch: "fix/parser", HeadSHA: "c7d8e9f", UpdatedAt: now.Add(-3 * time.Minute), RunStartedAt: now.Add(-4 * time.Minute)},
			{ID: 568, Name: "CodeQL", DisplayTitle: "weekly scan", Event: "schedule", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 568, Actor: "github", HeadBranch: "fix/parser", HeadSHA: "d0e1f2a", UpdatedAt: now.Add(-2 * time.Hour), RunStartedAt: now.Add(-121 * time.Minute)},
			{ID: 567, Name: "CI", DisplayTitle: "manual retry", Event: "workflow_dispatch", Status: model.StatusCompleted, Conclusion: model.ConclusionCancelled, RunNumber: 567, Actor: "indrasvat", HeadBranch: "fix/parser", HeadSHA: "1029384", UpdatedAt: now.Add(-3 * time.Hour), RunStartedAt: now.Add(-181 * time.Minute)},
			{ID: 566, Name: "Deploy", DisplayTitle: "staging deploy", Event: "push", Status: model.StatusQueued, Conclusion: model.ConclusionNone, RunNumber: 566, Actor: "indrasvat", HeadBranch: "fix/parser", HeadSHA: "5647382", UpdatedAt: now.Add(-3 * time.Hour), RunStartedAt: now.Add(-3 * time.Hour)},
			{ID: 565, Name: "Docs", DisplayTitle: "docs refresh", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 565, Actor: "indrasvat", HeadBranch: "fix/parser", HeadSHA: "e3f4a5b", UpdatedAt: now.Add(-3 * time.Hour), RunStartedAt: now.Add(-181 * time.Minute)},
			{ID: 564, Name: "Security", DisplayTitle: "dependency audit", Event: "workflow_dispatch", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 564, Actor: "dependabot", HeadBranch: "fix/parser", HeadSHA: "f6a7b8c", UpdatedAt: now.Add(-4 * time.Hour), RunStartedAt: now.Add(-241 * time.Minute)},
		},
	})
}

func sampleAllGreenModel() runs.Model {
	now := time.Now().UTC().Truncate(time.Second)
	return runs.NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateAllGreen,
		Runs: []model.Run{
			{ID: 569, Name: "CI", DisplayTitle: "linux matrix", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 569, HeadBranch: "main", UpdatedAt: now.Add(-3 * time.Minute), RunStartedAt: now.Add(-4*time.Minute - 2*time.Second)},
			{ID: 568, Name: "Release", DisplayTitle: "snapshot artifacts", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 568, HeadBranch: "main", UpdatedAt: now.Add(-1 * time.Hour), RunStartedAt: now.Add(-61*time.Minute - 2*time.Second)},
			{ID: 567, Name: "CodeQL", DisplayTitle: "weekly scan", Event: "schedule", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 567, HeadBranch: "main", UpdatedAt: now.Add(-2 * time.Hour), RunStartedAt: now.Add(-121*time.Minute - 2*time.Second)},
			{ID: 566, Name: "Docs", DisplayTitle: "reference refresh", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 566, HeadBranch: "main", UpdatedAt: now.Add(-3 * time.Hour), RunStartedAt: now.Add(-181*time.Minute - 2*time.Second)},
			{ID: 565, Name: "Security", DisplayTitle: "dependency audit", Event: "workflow_dispatch", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 565, HeadBranch: "main", UpdatedAt: now.Add(-4 * time.Hour), RunStartedAt: now.Add(-241*time.Minute - 2*time.Second)},
			{ID: 564, Name: "Deploy", DisplayTitle: "staging deploy", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 564, HeadBranch: "main", UpdatedAt: now.Add(-5 * time.Hour), RunStartedAt: now.Add(-301*time.Minute - 2*time.Second)},
		},
	})
}

func sampleLongRunsModel(count int) runs.Model {
	now := time.Now().UTC().Truncate(time.Second)
	items := make([]model.Run, count)
	for i := range items {
		number := count - i
		items[i] = model.Run{
			ID:           int64(900000 + number),
			Name:         "CI",
			DisplayTitle: fmt.Sprintf("batch %03d", number),
			Event:        "push",
			Status:       model.StatusCompleted,
			Conclusion:   model.ConclusionSuccess,
			RunNumber:    number,
			Actor:        "indrasvat",
			HeadBranch:   "main",
			HeadSHA:      fmt.Sprintf("long%03d", number),
			UpdatedAt:    now.Add(-time.Duration(i) * time.Minute),
			RunStartedAt: now.Add(-time.Duration(i)*time.Minute - 90*time.Second),
		}
	}
	return runs.NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Actor:  "indrasvat",
		State:  usecase.LaunchStateAllGreen,
		Runs:   items,
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
			Message:   "identifier lexer drops trailing underscore",
		}, {
			Path:      "internal/parser/lexer_test.go",
			StartLine: 88,
			Message:   "FAIL TestLexIdent/trailing_underscore",
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
		"17:43:02.781Z go test ./... -race -count=1",
		"ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
		"ok    github.com/indrasvat/gh-hound/internal/render 0.331s",
		"=== RUN   TestLexIdent",
		"=== RUN   TestLexIdent/basic",
		"--- PASS: TestLexIdent/basic (0.00s)",
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
