package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/confirm"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/help"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/palette"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/timejump"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/empty"
	"github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/welcome"
	"github.com/indrasvat/gh-hound/internal/tui/toast"
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
	OverlayNone     Overlay = ""
	OverlayHelp     Overlay = "help"
	OverlayPalette  Overlay = "palette"
	OverlayConfirm  Overlay = "confirm"
	OverlayTimeJump Overlay = "time_jump"
)

type KeyMsg struct {
	Key string
}

type Options struct {
	Config                    config.Config
	Build                     BuildInfo
	Launch                    usecase.LaunchContext
	DetailResolver            func(model.Run) (detail.Model, error)
	RunsResolver              func(usecase.RunFilter) ([]model.Run, error)
	FailureResolver           func(model.Run, model.Job) (failure.Model, logscreen.Model, error)
	LogResolver               func(model.Run, model.Job) (logscreen.Model, error)
	WatchResolver             func(model.Run) (watch.Model, error)
	DispatchResolver          func() (dispatch.Model, error)
	DispatchWorkflowsResolver func() ([]dispatch.Workflow, error)
	RunsMetadata              func() (usecase.RequestMeta, bool)
	LogRefetchNotice          func(int64) (usecase.LogRefetchNotice, bool)
	ActionHandler             func(ActionRequest) (usecase.ActionResult, error)
	ArtifactsResolver         func(model.Run) ([]model.Artifact, error)
	ArtifactDownloader        func(model.Artifact, string) (usecase.DownloadResult, error)
	OpenURL                   func(string) error
	CopyText                  func(string) error
}

type ActionRequest struct {
	Action   usecase.Action
	Run      model.Run
	Job      model.Job
	Workflow dispatch.Workflow
	Dispatch usecase.DispatchRequest
	Debug    bool
}

type App struct {
	config                    config.Config
	build                     BuildInfo
	theme                     theme.Theme
	routes                    []Route
	viewportHeight            int
	overlays                  []Overlay
	inputMode                 bool
	quit                      bool
	welcomeDismissed          bool
	refreshCount              int
	pollInterval              time.Duration
	launchRoute               Route
	runs                      runs.Model
	detail                    detail.Model
	failure                   failure.Model
	log                       logscreen.Model
	watch                     watch.Model
	dispatch                  dispatch.Model
	palette                   palette.Model
	confirm                   confirm.Model
	toasts                    toast.Model
	runsMeta                  usecase.RequestMeta
	dispatchWorkflows         []dispatch.Workflow
	pendingAction             *pendingAction
	routeErrors               map[Route]string
	detailResolver            func(model.Run) (detail.Model, error)
	runsResolver              func(usecase.RunFilter) ([]model.Run, error)
	failureResolver           func(model.Run, model.Job) (failure.Model, logscreen.Model, error)
	logResolver               func(model.Run, model.Job) (logscreen.Model, error)
	watchResolver             func(model.Run) (watch.Model, error)
	dispatchResolver          func() (dispatch.Model, error)
	dispatchWorkflowsResolver func() ([]dispatch.Workflow, error)
	runsMetadata              func() (usecase.RequestMeta, bool)
	logRefetchNotice          func(int64) (usecase.LogRefetchNotice, bool)
	actionHandler             func(ActionRequest) (usecase.ActionResult, error)
	artifactsResolver         func(model.Run) ([]model.Artifact, error)
	artifactDownloader        func(model.Artifact, string) (usecase.DownloadResult, error)
	artifactsFetch            *artifactsFetchState
	artifactDownload          *artifactDownloadState
	pendingDownload           *model.Artifact
	timeJump                  timejump.Model
	lastToastTick             time.Time
	openURL                   func(string) error
	copyText                  func(string) error
}

// artifactsFetchState carries an async artifacts listing for the
// detail screen. Pointer-held so goroutine completion survives App
// value copies; drained on Refresh ticks.
type artifactsFetchState struct {
	mu        sync.Mutex
	runID     int64
	artifacts []model.Artifact
	err       error
	done      bool
}

type artifactDownloadState struct {
	mu     sync.Mutex
	name   string
	result usecase.DownloadResult
	err    error
	done   bool
}

type pendingAction struct {
	route   Route
	request ActionRequest
}

func NewApp(options Options) App {
	cfg := options.Config
	if cfg.Theme == "" {
		cfg = config.Default()
	}
	launch := options.Launch
	if !hasLaunchContext(launch) {
		launch = usecase.LaunchContext{
			State:        usecase.LaunchStateError,
			ErrorMessage: "repository context is not loaded; run gh hound -R owner/repo",
		}
	}
	launchRoute := routeForLaunch(options.Launch)
	route := launchRoute
	if cfg.Welcome {
		route = RouteWelcome
	}
	runsModel := runs.NewModel(launch)
	routeErrors := map[Route]string{}
	initialDetail := detail.Model{}
	if run, ok := runsModel.SelectedRun(); ok {
		if options.DetailResolver != nil {
			resolved, err := options.DetailResolver(run)
			if err != nil {
				routeErrors[RouteDetail] = "detail unavailable: " + err.Error()
			} else {
				initialDetail = resolved
			}
		} else {
			initialDetail = DetailModelForRun(run)
		}
	}
	watchModel := watchModelForLaunch(launch, runsModel)
	var runsMeta usecase.RequestMeta
	if options.RunsMetadata != nil {
		if meta, ok := options.RunsMetadata(); ok {
			runsMeta = meta
		}
	}
	return App{
		config:                    cfg,
		build:                     options.Build,
		theme:                     theme.ForMode(theme.Mode(cfg.Theme)),
		routes:                    []Route{route},
		pollInterval:              initialPollInterval(cfg),
		launchRoute:               launchRoute,
		runs:                      runsModel,
		detail:                    initialDetail,
		routeErrors:               routeErrors,
		watch:                     watchModel,
		palette:                   palette.New(paletteItems(nil)),
		toasts:                    toast.New(),
		runsMeta:                  runsMeta,
		detailResolver:            options.DetailResolver,
		runsResolver:              options.RunsResolver,
		failureResolver:           options.FailureResolver,
		logResolver:               options.LogResolver,
		watchResolver:             options.WatchResolver,
		dispatchResolver:          options.DispatchResolver,
		dispatchWorkflowsResolver: options.DispatchWorkflowsResolver,
		runsMetadata:              options.RunsMetadata,
		logRefetchNotice:          options.LogRefetchNotice,
		actionHandler:             options.ActionHandler,
		artifactsResolver:         options.ArtifactsResolver,
		artifactDownloader:        options.ArtifactDownloader,
		openURL:                   options.OpenURL,
		copyText:                  options.CopyText,
	}
}

func NewScenarioApp(scenario string, build BuildInfo) App {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, Build: build})
	app.runs = sampleRunsModel()
	app.detail = sampleDetailModel()
	app.failure = sampleFailureModel()
	app.log = sampleLogModel()
	app.watch = sampleWatchModel()
	app.dispatch = sampleDispatchModel()
	app.routeErrors = map[Route]string{}
	app.detailResolver = func(model.Run) (detail.Model, error) {
		return sampleDetailModel(), nil
	}
	app.runsResolver = func(usecase.RunFilter) ([]model.Run, error) {
		return sampleRunsModel().Context.Runs, nil
	}
	app.failureResolver = func(model.Run, model.Job) (failure.Model, logscreen.Model, error) {
		return sampleFailureModel(), sampleLogModel(), nil
	}
	app.logResolver = func(model.Run, model.Job) (logscreen.Model, error) {
		return sampleLogModel(), nil
	}
	app.watchResolver = func(model.Run) (watch.Model, error) {
		return sampleWatchModel(), nil
	}
	app.dispatchResolver = func() (dispatch.Model, error) {
		return sampleDispatchModel(), nil
	}
	app.dispatchWorkflowsResolver = func() ([]dispatch.Workflow, error) {
		return []dispatch.Workflow{sampleDispatchModel().Workflow}, nil
	}
	app.actionHandler = func(ActionRequest) (usecase.ActionResult, error) {
		return usecase.ActionResult{Message: "accepted"}, nil
	}
	switch strings.ToLower(scenario) {
	case "green", "ok", "success":
		app.runs = sampleAllGreenModel()
	}
	return app
}

func (a App) Update(msg KeyMsg) (App, bool) {
	// Async artifact results apply on the next keypress too, not only
	// on poll ticks, so the section appears as soon as it is ready.
	if next, ok := a.drainArtifactsFetch(); ok {
		a = next
	}
	if a.inputMode {
		if msg.Key == "esc" {
			a.inputMode = false
			return a, true
		}
		return a, false
	}

	if len(a.overlays) > 0 {
		if a.TopOverlay() == OverlayConfirm {
			return a.updateConfirm(msg)
		}
		if a.TopOverlay() == OverlayPalette {
			return a.updatePalette(msg)
		}
		if a.TopOverlay() == OverlayTimeJump {
			return a.updateTimeJump(msg)
		}
		switch msg.Key {
		case "esc":
			a.overlays = a.overlays[:len(a.overlays)-1]
			return a, true
		case "?":
			a.overlays = append(a.overlays, OverlayHelp)
			return a, true
		case ":":
			a = a.openPalette()
			return a, true
		case "q", "ctrl+c":
			a.quit = true
			return a, true
		default:
			return a, false
		}
	}

	if a.routeInputMode() {
		return a.updateRoute(msg)
	}

	var toastHandled bool
	a.toasts, toastHandled = a.toasts.Update(toast.KeyMsg{Key: msg.Key})
	if toastHandled {
		return a, true
	}

	switch msg.Key {
	case "T":
		a.toggleTheme()
		return a, true
	case "?":
		a.overlays = append(a.overlays, OverlayHelp)
		return a, true
	case ":":
		a = a.openPalette()
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
		if a.TopOverlay() == OverlayConfirm {
			footer = "y confirm · enter/n/esc cancel"
		}
		if a.TopOverlay() == OverlayTimeJump {
			footer = "j/k pick · type time · ⏎ go · ⎋ cancel"
		}
	} else {
		body = toastLayer(a.theme, body, a.toasts, contentWidth(width))
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
		return frameViewSize(app.theme, "hound", "⎇ branch main · @indrasvat", "◔ 4,981/5k live", runs.View(sampleAllGreenModel(), bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenAllGreen), width, height, true)
	case "runs":
		return frameViewSize(app.theme, "hound", "⎇ branch fix/parser · @indrasvat", "◔ 4,981/5k live 304", runs.View(sampleRunsModel(), bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenRunsList), width, height, true)
	case "rate_limit_toast":
		app.runs = sampleRunsModel()
		app.routes = []Route{RouteRuns}
		app.pushToast("rate-limit", usecase.ResilienceFor(usecase.APIError{
			Kind:       usecase.APIErrorRateLimit,
			Status:     403,
			Message:    "API rate limit exceeded",
			RetryAfter: 42 * time.Second,
			ResetAt:    time.Date(2026, 6, 9, 20, 4, 0, 0, time.UTC),
		}, usecase.ErrorContext{}))
		return app.ViewSize(width, height)
	case "detail":
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "a1b2c3d", detail.ViewSize(sampleDetailModel(), bodyWidth, bodyHeight(height)), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
	case "detail-artifacts":
		m := sampleDetailModel().Update(detail.KeyMsg{Key: "a"})
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "artifacts", detail.ViewSize(m, bodyWidth, bodyHeight(height)), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
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
	app := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
	bodyWidth := contentWidth(width)
	switch scenario {
	case "welcome-enter":
		app = NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		app.config.Welcome = true
		app.routes = []Route{RouteWelcome}
		app.launchRoute = RouteRuns
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
		for _, key := range []string{"/", "f", "a", "i", "l", "enter"} {
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
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "a1b2c3d", detail.ViewSize(m, bodyWidth, bodyHeight(height)), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
	case "detail-artifacts":
		m := sampleDetailModel()
		for _, key := range []string{"a", "j"} {
			m = m.Update(detail.KeyMsg{Key: key})
		}
		return frameViewSize(app.theme, "hound", "CI #571 › fix/parser", "artifacts", detail.ViewSize(m, bodyWidth, bodyHeight(height)), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
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

func (a App) PollInterval() time.Duration {
	base := a.pollInterval
	if base <= 0 {
		base = initialPollInterval(a.config)
	}
	// Tighten the loop while async artifact work or timed toasts are
	// pending so completions and TTL expiry surface promptly.
	if a.artifactsFetch != nil || a.artifactDownload != nil || len(a.toasts.Toasts) > 0 {
		if base > time.Second {
			return time.Second
		}
	}
	return base
}

func (a App) Refresh() (App, bool) {
	changed := false
	// Always keep the ticked app: discarding it when no toast expired
	// would reset the elapsed clock and make TTLs unreachable.
	var expired bool
	a, expired = a.tickToasts()
	if expired {
		changed = true
	}
	if next, ok := a.drainArtifactDownload(); ok {
		a = next
		changed = true
	}
	if a.TopOverlay() != OverlayNone || a.routeInputMode() {
		return a, changed
	}
	if next, ok := a.drainArtifactsFetch(); ok {
		a = next
		changed = true
	}
	switch a.Route() {
	case RouteRuns:
		next, refreshed := a.refreshRuns()
		return next, refreshed || changed
	case RouteWatch:
		next, refreshed := a.refreshWatch()
		return next, refreshed || changed
	default:
		return a, changed
	}
}

// tickToasts advances toast TTLs on poll ticks so timed toasts
// actually expire; before this the TickMsg path was never driven.
func (a App) tickToasts() (App, bool) {
	now := time.Now()
	if a.lastToastTick.IsZero() || len(a.toasts.Toasts) == 0 {
		a.lastToastTick = now
		return a, false
	}
	elapsed := now.Sub(a.lastToastTick)
	a.lastToastTick = now
	before := len(a.toasts.Toasts)
	a.toasts, _ = a.toasts.Update(toast.TickMsg{Elapsed: elapsed})
	return a, len(a.toasts.Toasts) != before
}

func (a App) drainArtifactsFetch() (App, bool) {
	fetch := a.artifactsFetch
	if fetch == nil {
		return a, false
	}
	fetch.mu.Lock()
	defer fetch.mu.Unlock()
	if !fetch.done {
		return a, false
	}
	a.artifactsFetch = nil
	if fetch.err != nil {
		// Artifacts are auxiliary: the screen stays usable, but the
		// failure is surfaced so it cannot masquerade as "no artifacts".
		a.pushToast("artifacts-unavailable", usecase.Resilience{
			Severity: usecase.SeverityWarn,
			Title:    "artifacts unavailable",
			Message:  "could not list this run's artifacts; retry by reopening the run",
		})
		return a, true
	}
	if a.detail.Run.ID != fetch.runID || len(fetch.artifacts) == 0 {
		return a, false
	}
	a.detail = a.detail.WithArtifacts(fetch.artifacts)
	return a, a.Route() == RouteDetail
}

func (a App) drainArtifactDownload() (App, bool) {
	download := a.artifactDownload
	if download == nil {
		return a, false
	}
	download.mu.Lock()
	defer download.mu.Unlock()
	if !download.done {
		return a, false
	}
	a.artifactDownload = nil
	a.toasts = a.toasts.Dismiss("artifact-downloading")
	if download.err != nil {
		a.pushErrorToast("artifact-download-failed", usecase.ResilienceFor(download.err, usecase.ErrorContext{}))
		return a, true
	}
	files := "files"
	if download.result.FileCount == 1 {
		files = "file"
	}
	a.pushToast("artifact-downloaded", usecase.Resilience{
		Severity: usecase.SeverityOK,
		Title:    "artifact downloaded",
		Message:  fmt.Sprintf("%s extracted to %s (%d %s)", download.name, download.result.Path, download.result.FileCount, files),
	})
	return a, true
}

func (a App) startArtifactsFetch(run model.Run) App {
	if a.artifactsResolver == nil {
		return a
	}
	if pending := a.artifactsFetch; pending != nil && pending.runID == run.ID {
		return a
	}
	state := &artifactsFetchState{runID: run.ID}
	a.artifactsFetch = state
	resolver := a.artifactsResolver
	go func() {
		artifacts, err := resolver(run)
		state.mu.Lock()
		state.artifacts = artifacts
		state.err = err
		state.done = true
		state.mu.Unlock()
	}()
	return a
}

func (a App) startArtifactDownload(artifact model.Artifact) App {
	if a.artifactDownloader == nil {
		a.pushErrorToast("artifact-download-unavailable", usecase.ResilienceFor(errors.New("artifact download is not configured"), usecase.ErrorContext{}))
		return a
	}
	// One download at a time: overwriting the pending state would
	// orphan the first goroutine's completion and let the UI lie.
	if a.artifactDownload != nil {
		a.pushToast("artifact-download-busy", usecase.Resilience{
			Severity: usecase.SeverityWarn,
			Title:    "download in progress",
			Message:  "wait for the current artifact download to finish",
		})
		return a
	}
	state := &artifactDownloadState{name: artifact.Name}
	a.artifactDownload = state
	downloader := a.artifactDownloader
	go func() {
		result, err := downloader(artifact, ".")
		state.mu.Lock()
		state.result = result
		state.err = err
		state.done = true
		state.mu.Unlock()
	}()
	a.pushToast("artifact-downloading", usecase.Resilience{
		Severity: usecase.SeverityInfo,
		Title:    "downloading artifact",
		Message:  fmt.Sprintf("%s (%s)", artifact.Name, humanArtifactSize(artifact.SizeInBytes)),
	})
	return a
}

func humanArtifactSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (a App) WelcomeDismissed() bool {
	return a.welcomeDismissed
}

func (a App) DetailModel() detail.Model {
	return a.detail
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
	switch a.Route() {
	case RouteRuns:
		return a.runs.InputMode
	case RouteLog:
		return a.log.InputMode
	case RouteDispatch:
		return true
	default:
		return false
	}
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
			a = a.loadDetail(run)
		}
		a.PushRoute(RouteDetail)
	case runs.IntentOpenLogs:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.loadLog(run, model.Job{})
		}
		a.PushRoute(RouteLog)
	case runs.IntentWatch:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.loadWatch(run)
		}
		a.PushRoute(RouteWatch)
	case runs.IntentDispatch:
		a = a.openDispatch()
	case runs.IntentBrowser:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.openExternal(selectedRunURL(a.runs.Context.Repo, run))
		}
	case runs.IntentCopy:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.copyExternal("run-url", selectedRunURL(a.runs.Context.Repo, run))
		}
	case runs.IntentRerun:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.handleAction(RouteRuns, ActionRequest{Action: usecase.ActionRerunRun, Run: run})
		}
	case runs.IntentRerunFailed:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.handleAction(RouteRuns, ActionRequest{Action: usecase.ActionRerunFailedJobs, Run: run})
		}
	case runs.IntentCancel:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.handleAction(RouteRuns, ActionRequest{Action: usecase.ActionCancelRun, Run: run})
		}
	case runs.IntentForceCancel:
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.handleAction(RouteRuns, ActionRequest{Action: usecase.ActionForceCancelRun, Run: run})
		}
	case runs.IntentFilter:
		a = a.reloadRuns(a.runs.Intent.Filter)
		_, serverSide := serverRunFilter(a.runs.Context, a.config.PerPage, a.runs.Intent.Filter)
		a.runs.ServerFiltered = serverSide
	case runs.IntentLoadMore:
		a = a.loadMoreRuns()
	}
	return a, runsHandled(msg.Key) || before.Selected != a.runs.Selected || before.Filter != a.runs.Filter || before.InputMode != a.runs.InputMode || a.runs.Intent.Kind != runs.IntentNone
}

func (a App) updateDetail(msg KeyMsg) (App, bool) {
	beforeFocus := a.detail.Focus
	beforeJob := a.detail.SelectedJob
	beforeStep := a.detail.SelectedStep
	beforeArtifact := a.detail.SelectedArtifact
	a.detail = a.detail.Update(detail.KeyMsg{Key: msg.Key})
	switch a.detail.Intent.Kind {
	case detail.IntentFailure:
		a = a.loadFailure(a.detail.Run, a.selectedDetailJob())
		a.PushRoute(RouteFailure)
	case detail.IntentLog:
		a = a.loadLog(a.detail.Run, a.selectedDetailJob())
		a.PushRoute(RouteLog)
	case detail.IntentWatch:
		a = a.loadWatch(a.detail.Run)
		a.PushRoute(RouteWatch)
	case detail.IntentRerunJob:
		a = a.handleAction(RouteDetail, ActionRequest{Action: usecase.ActionRerunJob, Run: a.detail.Run, Job: a.selectedDetailJob()})
	case detail.IntentRerunFailed:
		a = a.handleAction(RouteDetail, ActionRequest{Action: usecase.ActionRerunFailedJobs, Run: a.detail.Run})
	case detail.IntentCancel:
		a = a.handleAction(RouteDetail, ActionRequest{Action: usecase.ActionCancelRun, Run: a.detail.Run})
	case detail.IntentForceCancel:
		a = a.handleAction(RouteDetail, ActionRequest{Action: usecase.ActionForceCancelRun, Run: a.detail.Run})
	case detail.IntentBrowser:
		a = a.openExternal(detailBrowserURL(a.detail))
	case detail.IntentCopyURL:
		a = a.copyExternal("detail-url", selectedRunURL(a.detail.Repo, a.detail.Run))
	case detail.IntentCopySHA:
		a = a.copyExternal("detail-sha", a.detail.Run.HeadSHA)
	case detail.IntentDownloadArtifact:
		a = a.requestArtifactDownload(a.detail.Intent.ArtifactID)
	case detail.IntentBack:
		a.PopRoute()
	}
	return a, detailHandled(msg.Key) || beforeFocus != a.detail.Focus || beforeJob != a.detail.SelectedJob || beforeStep != a.detail.SelectedStep || beforeArtifact != a.detail.SelectedArtifact || a.detail.Intent.Kind != detail.IntentNone
}

func (a App) requestArtifactDownload(artifactID int64) App {
	for _, artifact := range a.detail.Artifacts {
		if artifact.ID != artifactID {
			continue
		}
		if artifact.Expired {
			a.pushErrorToast("artifact-expired", usecase.ResilienceFor(usecase.ArtifactExpiredError{Name: artifact.Name}, usecase.ErrorContext{}))
			return a
		}
		return a.openDownloadConfirm(artifact)
	}
	return a
}

func (a App) openDownloadConfirm(artifact model.Artifact) App {
	a.clearRouteError(RouteDetail)
	selected := artifact
	a.pendingDownload = &selected
	a.confirm = confirm.New(fmt.Sprintf("Download artifact %q (%s) to ./%s/?", artifact.Name, humanArtifactSize(artifact.SizeInBytes), artifact.Name))
	if a.TopOverlay() != OverlayConfirm {
		a.overlays = append(a.overlays, OverlayConfirm)
	}
	return a
}

func (a App) updateFailure(msg KeyMsg) (App, bool) {
	a.failure = a.failure.Update(failure.KeyMsg{Key: msg.Key})
	switch a.failure.Intent.Kind {
	case failure.IntentFullLog:
		a.clearRouteError(RouteLog)
		a.log = logscreen.NewModel(a.failure.Report.Log, a.failure.Offset, 6)
		a.PushRoute(RouteLog)
	case failure.IntentRerunJob:
		a = a.handleAction(RouteFailure, ActionRequest{Action: usecase.ActionRerunJob, Run: a.detail.Run, Job: a.failure.Report.Job})
	case failure.IntentRerunFailed:
		a = a.handleAction(RouteFailure, ActionRequest{Action: usecase.ActionRerunFailedJobs, Run: a.detail.Run})
	case failure.IntentBrowser:
		a = a.openExternal(failureBrowserURL(a.failure))
	case failure.IntentCopyExcerpt:
		a = a.copyExternal("failure-excerpt", failureExcerptText(a.failure))
	case failure.IntentBack:
		a.PopRoute()
	}
	return a, failureHandled(msg.Key) || a.failure.Intent.Kind != failure.IntentNone
}

// WithViewport records the terminal size so key handling (G, ctrl+d)
// scrolls against the real viewport, not the resolver's default.
func (a App) WithViewport(width, height int) App {
	a.viewportHeight = height
	return a
}

func (a App) updateLog(msg KeyMsg) (App, bool) {
	if a.viewportHeight > 0 {
		if rows := bodyHeight(a.viewportHeight) - 1; rows > 0 {
			a.log.Height = rows
		}
	}
	if msg.Key == "t" && !a.log.InputMode {
		a.timeJump = timejump.New(a.log.Document)
		a.overlays = append(a.overlays, OverlayTimeJump)
		return a, true
	}
	before := a.log
	a.log = a.log.Update(logscreen.KeyMsg{Key: msg.Key})
	if msg.Key == "esc" {
		if before.InputMode || before.RangeLabel != "" {
			// Esc only cancelled the input layer or range filter;
			// stay on the log.
			return a, true
		}
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
	case watch.IntentCancel:
		a = a.handleAction(RouteWatch, ActionRequest{Action: usecase.ActionCancelRun, Run: a.watch.State.Run})
	case watch.IntentDetach:
		a.PopRoute()
	}
	return a, watchHandled(msg.Key) || before.Follow != a.watch.Follow || before.Debug != a.watch.Debug || a.watch.Intent.Kind != watch.IntentNone
}

func (a App) updateDispatch(msg KeyMsg) (App, bool) {
	beforeFocused := a.dispatch.Focused
	a.dispatch = a.dispatch.Update(dispatch.KeyMsg{Key: msg.Key})
	switch a.dispatch.Intent.Kind {
	case dispatch.IntentSubmit:
		a = a.handleAction(RouteDispatch, ActionRequest{Action: usecase.ActionDispatch, Workflow: a.dispatch.Workflow, Dispatch: a.dispatch.Intent.Request})
	case dispatch.IntentCancel:
		a.PopRoute()
		return a, true
	}
	return a, dispatchHandled(msg.Key) || beforeFocused != a.dispatch.Focused || a.dispatch.Intent.Kind != dispatch.IntentNone
}

func (a App) updateConfirm(msg KeyMsg) (App, bool) {
	a.confirm = a.confirm.Update(confirm.KeyMsg{Key: msg.Key})
	switch a.confirm.Intent.Kind {
	case confirm.IntentConfirm:
		pending := a.pendingAction
		pendingDownload := a.pendingDownload
		a = a.closeConfirm()
		if pendingDownload != nil {
			return a.startArtifactDownload(*pendingDownload), true
		}
		if pending != nil {
			a = a.executeAction(pending.route, pending.request)
		}
		return a, true
	case confirm.IntentCancel:
		a = a.closeConfirm()
		return a, true
	default:
		return a, false
	}
}

func (a App) updateTimeJump(msg KeyMsg) (App, bool) {
	switch msg.Key {
	case "esc":
		a.PopOverlay()
		a.timeJump = timejump.Model{}
		return a, true
	case "enter":
		next, action := a.timeJump.Commit()
		a.timeJump = next
		switch action.Kind {
		case timejump.ActionJump:
			a.PopOverlay()
			a.log = a.log.JumpToLine(action.Line)
			a.log.LastJump = a.timeJumpBreadcrumb()
			a.timeJump = timejump.Model{}
		case timejump.ActionRelative:
			a.PopOverlay()
			a.log = a.log.JumpRelative(action.DeltaSeconds)
			a.log.LastJump = a.timeJumpBreadcrumb()
			a.timeJump = timejump.Model{}
		case timejump.ActionRange:
			a.PopOverlay()
			a.log = a.log.SetRange(action.Line, action.EndLine, strings.TrimSpace(a.timeJump.Input))
			a.timeJump = timejump.Model{}
		case timejump.ActionInvalid:
			// Feedback is set on the model; the modal stays open.
		}
		return a, true
	default:
		a.timeJump = a.timeJump.Update(msg.Key)
		return a, true
	}
}

// timeJumpBreadcrumb names what was jumped to for the log header.
func (a App) timeJumpBreadcrumb() string {
	if input := strings.TrimSpace(a.timeJump.Input); input != "" {
		return input
	}
	if len(a.timeJump.Entries) > a.timeJump.Selected {
		entry := a.timeJump.Entries[a.timeJump.Selected]
		if entry.Clock != "" {
			return entry.Clock
		}
		return entry.Label
	}
	return ""
}

func (a App) updatePalette(msg KeyMsg) (App, bool) {
	switch msg.Key {
	case "esc":
		a.PopOverlay()
		return a, true
	case "?":
		a.overlays = append(a.overlays, OverlayHelp)
		return a, true
	case "q", "ctrl+c":
		a.quit = true
		return a, true
	}
	before := a.palette
	a.palette = a.palette.Update(palette.KeyMsg{Key: msg.Key})
	if a.palette.Intent.Route != "" {
		a = a.handlePaletteIntent(a.palette.Intent)
		return a, true
	}
	return a, before.Query != a.palette.Query || before.Selected != a.palette.Selected || paletteHandled(msg.Key)
}

func (a App) handlePaletteIntent(intent palette.Intent) App {
	switch intent.Route {
	case "runs":
		a.PopOverlay()
		a.routes = []Route{RouteRuns}
	case "runs --all":
		a.PopOverlay()
		a.routes = []Route{RouteRuns}
		a.runs.Context.Scope = usecase.LaunchScopeRepo
		if len(a.runs.Context.RepoRuns) > 0 {
			a.runs.Context.Runs = a.runs.Context.RepoRuns
		} else {
			a = a.reloadRuns("")
		}
	case "run:failed":
		a.PopOverlay()
		a.routes = []Route{RouteRuns}
		a.runs.Filter = "failure"
		a = a.reloadRuns("failure")
	case "artifacts":
		a.PopOverlay()
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.loadDetail(run)
			a.routes = []Route{RouteRuns, RouteDetail}
			a.detail = a.detail.Update(detail.KeyMsg{Key: "a"})
		}
	case string(RouteDispatch):
		a = a.openDispatchFromPalette(intent.Value)
	}
	return a
}

func (a App) openPalette() App {
	// Dispatch resolution can fail in repo-wide sessions (no branch
	// ref); that is not the palette's problem. The generic dispatch
	// item stays available and openDispatch surfaces the error when
	// the user actually selects it.
	workflows, _ := a.dispatchWorkflowChoices()
	a.palette = palette.New(paletteItems(workflows))
	if a.TopOverlay() != OverlayPalette {
		a.overlays = append(a.overlays, OverlayPalette)
	}
	return a
}

func (a App) openDispatchFromPalette(value string) App {
	if strings.TrimSpace(value) != "" {
		if workflow, ok := a.dispatchWorkflowByValue(value); ok {
			a.PopOverlay()
			a.dispatch = dispatch.NewModel(workflow)
			a.PushRoute(RouteDispatch)
			return a
		}
		a.pushErrorToast("dispatch-workflow-missing", usecase.ResilienceFor(fmt.Errorf("dispatch workflow %q is no longer available", value), usecase.ErrorContext{}))
		return a
	}
	a.PopOverlay()
	return a.openDispatch()
}

func (a *App) PopOverlay() {
	if len(a.overlays) > 0 {
		a.overlays = a.overlays[:len(a.overlays)-1]
	}
}

func (a App) loadDetail(run model.Run) App {
	a.clearRouteError(RouteDetail)
	if a.detailResolver == nil {
		a.detail = DetailModelForRun(run)
		return a.startArtifactsFetch(run)
	}
	resolved, err := a.detailResolver(run)
	if err != nil {
		a.detail = DetailModelForRun(run)
		a.setRouteError(RouteDetail, "detail unavailable: "+err.Error())
		return a
	}
	a.detail = resolved
	return a.startArtifactsFetch(run)
}

func (a App) reloadRuns(query string) App {
	filter, ok := serverRunFilter(a.runs.Context, a.config.PerPage, query)
	if !ok {
		if strings.TrimSpace(query) == "" {
			// Clearing a server-backed filter must restore the unfiltered
			// listing immediately, not leave the filtered subset on
			// screen until the next poll tick.
			filter = baseRunsFilter(a.runs.Context, a.config.PerPage)
		} else {
			return a
		}
	}
	a.clearRouteError(RouteRuns)
	if a.runsResolver == nil {
		a.setRouteError(RouteRuns, "runs filter unavailable: live GitHub runs loader is not configured")
		return a
	}
	resolved, err := a.runsResolver(filter)
	if err != nil {
		a = a.handleRunsError(RouteRuns, "runs-filter", "runs filter failed: "+err.Error(), err)
		return a
	}
	a.runs.Context.Runs = resolved
	switch a.runs.Context.Scope {
	case usecase.LaunchScopeBranch:
		a.runs.Context.BranchRuns = resolved
	case usecase.LaunchScopeRepo:
		a.runs.Context.RepoRuns = resolved
	}
	a.runs.Context.Page = 1
	a.runs.Context.PerPage = perPageFor(a.runs.Context, a.config.PerPage)
	a.runs.Context.HasMore = len(resolved) >= a.runs.Context.PerPage
	a.runs.Selected = 0
	return a
}

func (a App) loadMoreRuns() App {
	filter := baseRunsFilter(a.runs.Context, a.config.PerPage)
	if strings.TrimSpace(a.runs.Filter) != "" {
		if serverFilter, ok := serverRunFilter(a.runs.Context, a.config.PerPage, a.runs.Filter); ok {
			filter = serverFilter
		}
	}
	perPage := perPageFor(a.runs.Context, a.config.PerPage)
	nextPage := a.runs.Context.Page + 1
	if nextPage <= 1 {
		nextPage = 2
	}
	filter.PerPage = perPage
	filter.Page = nextPage
	a.clearRouteError(RouteRuns)
	if a.runsResolver == nil {
		a.setRouteError(RouteRuns, "next page unavailable: live GitHub runs loader is not configured")
		return a
	}
	resolved, err := a.runsResolver(filter)
	if err != nil {
		a = a.handleRunsError(RouteRuns, "runs-page", "next page failed: "+err.Error(), err)
		return a
	}
	a.runs.Context.Page = nextPage
	a.runs.Context.PerPage = perPage
	_ = a.appendActiveRuns(resolved)
	// A full page that deduped to nothing still means deeper pages
	// exist (high-velocity repos shift runs between pages); latching
	// HasMore=false there froze pagination on openclaw-sized repos.
	a.runs.Context.HasMore = len(resolved) >= perPage
	return a
}

func (a App) refreshRuns() (App, bool) {
	if a.runsResolver == nil {
		return a, false
	}
	filter := baseRunsFilter(a.runs.Context, a.config.PerPage)
	if strings.TrimSpace(a.runs.Filter) != "" {
		if serverFilter, ok := serverRunFilter(a.runs.Context, a.config.PerPage, a.runs.Filter); ok {
			filter = serverFilter
		}
	}
	filter.Page = 1
	selectedID := int64(0)
	if selected, ok := a.runs.SelectedRun(); ok {
		selectedID = selected.ID
	}
	resolved, err := a.runsResolver(filter)
	if err != nil {
		a = a.handleRunsError(RouteRuns, "runs-refresh", "refresh failed: "+err.Error(), err)
		a.pollInterval = nextPollIntervalForRuns(nil, a.pollInterval, a.config)
		a.refreshCount++
		return a, true
	}
	a.clearRouteError(RouteRuns)
	a.refreshCount++
	perPage := perPageFor(a.runs.Context, a.config.PerPage)
	merged := append(slices.Clone(resolved), dedupeNewRuns(resolved, a.runs.Context.Runs)...)
	a.runs.Context.Runs = merged
	switch a.runs.Context.Scope {
	case usecase.LaunchScopeBranch:
		a.runs.Context.BranchRuns = merged
	case usecase.LaunchScopeRepo:
		a.runs.Context.RepoRuns = merged
	}
	a.runs.Context.Page = max(a.runs.Context.Page, 1)
	a.runs.Context.PerPage = perPage
	a.runs.Context.HasMore = len(resolved) >= perPage || a.runs.Context.HasMore
	a.runs.Selected = indexOfRun(a.runs.Context.Runs, selectedID)
	if a.runsMetadata != nil {
		if meta, ok := a.runsMetadata(); ok {
			a.runsMeta = meta
		}
	}
	a.pollInterval = nextPollIntervalForRuns(resolved, a.pollInterval, a.config)
	return a, true
}

func (a App) refreshWatch() (App, bool) {
	if a.watchResolver == nil || a.watch.State.Run.ID == 0 {
		return a, false
	}
	resolved, err := a.watchResolver(a.watch.State.Run)
	if err != nil {
		a.setRouteError(RouteWatch, "watch refresh failed: "+err.Error())
		a.refreshCount++
		return a, true
	}
	a.clearRouteError(RouteWatch)
	a.refreshCount++
	a.watch = resolved
	a.pollInterval = nextPollIntervalForRuns([]model.Run{resolved.State.Run}, a.pollInterval, a.config)
	return a, true
}

func initialPollInterval(cfg config.Config) time.Duration {
	if cfg.PollMin > 0 {
		return cfg.PollMin
	}
	return config.Default().PollMin
}

func maxPollInterval(cfg config.Config) time.Duration {
	if cfg.PollMax > 0 {
		return cfg.PollMax
	}
	return config.Default().PollMax
}

func nextPollIntervalForRuns(runs []model.Run, previous time.Duration, cfg config.Config) time.Duration {
	minPoll := initialPollInterval(cfg)
	maxPoll := max(maxPollInterval(cfg), minPoll)
	for _, run := range runs {
		if run.Status == model.StatusInProgress || run.Status == model.StatusQueued || run.Status == model.StatusPending || run.Status == model.StatusRequested || run.Status == model.StatusWaiting {
			return minPoll
		}
	}
	if previous < minPoll {
		previous = minPoll
	}
	next := previous * 2
	if next > maxPoll {
		return maxPoll
	}
	return next
}

func indexOfRun(runs []model.Run, id int64) int {
	if id == 0 {
		return 0
	}
	for index, run := range runs {
		if run.ID == id {
			return index
		}
	}
	return 0
}

func (a *App) appendActiveRuns(next []model.Run) int {
	if len(next) == 0 {
		return 0
	}
	next = dedupeNewRuns(a.runs.Context.Runs, next)
	if len(next) == 0 {
		return 0
	}
	switch a.runs.Context.Scope {
	case usecase.LaunchScopeBranch:
		a.runs.Context.BranchRuns = append(a.runs.Context.BranchRuns, next...)
		a.runs.Context.Runs = a.runs.Context.BranchRuns
	case usecase.LaunchScopeRepo:
		a.runs.Context.RepoRuns = append(a.runs.Context.RepoRuns, next...)
		a.runs.Context.Runs = a.runs.Context.RepoRuns
	default:
		a.runs.Context.Runs = append(a.runs.Context.Runs, next...)
	}
	return len(next)
}

func dedupeNewRuns(existing, next []model.Run) []model.Run {
	seen := map[int64]bool{}
	for _, run := range existing {
		if run.ID != 0 {
			seen[run.ID] = true
		}
	}
	out := make([]model.Run, 0, len(next))
	for _, run := range next {
		if run.ID != 0 && seen[run.ID] {
			continue
		}
		if run.ID != 0 {
			seen[run.ID] = true
		}
		out = append(out, run)
	}
	return out
}

func baseRunsFilter(ctx usecase.LaunchContext, defaultPerPage int) usecase.RunFilter {
	filter := usecase.RunFilter{Repo: ctx.Repo, PerPage: perPageFor(ctx, defaultPerPage)}
	if ctx.Scope == usecase.LaunchScopeBranch && strings.TrimSpace(ctx.Branch) != "" {
		filter.Branch = ctx.Branch
	}
	return filter
}

func perPageFor(ctx usecase.LaunchContext, fallback int) int {
	if ctx.PerPage > 0 {
		return ctx.PerPage
	}
	if fallback > 0 {
		return fallback
	}
	return config.Default().PerPage
}

func serverRunFilter(ctx usecase.LaunchContext, perPage int, query string) (usecase.RunFilter, bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || strings.TrimSpace(ctx.Repo) == "" {
		return usecase.RunFilter{}, false
	}
	filter := baseRunsFilter(ctx, perPage)
	key, value, tagged := strings.Cut(query, ":")
	if tagged {
		value = strings.TrimSpace(value)
		if value == "" {
			return usecase.RunFilter{}, false
		}
		switch strings.TrimSpace(key) {
		case "actor":
			filter.Actor = value
			return filter, true
		case "branch":
			filter.Branch = value
			return filter, true
		case "event":
			filter.Event = value
			return filter, true
		case "sha", "head", "head_sha":
			filter.HeadSHA = value
			return filter, true
		case "status", "conclusion":
			status, ok := normalizedRunStatus(value)
			if !ok {
				return usecase.RunFilter{}, false
			}
			filter.Status = status
			return filter, true
		default:
			return usecase.RunFilter{}, false
		}
	}
	if status, ok := normalizedRunStatus(query); ok {
		filter.Status = status
		return filter, true
	}
	if isKnownEvent(query) {
		filter.Event = query
		return filter, true
	}
	return usecase.RunFilter{}, false
}

func normalizedRunStatus(query string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "failure", "failed", "failing", "red":
		return string(model.ConclusionFailure), true
	case "success", "passed", "passing", "green":
		return string(model.ConclusionSuccess), true
	case "cancelled", "canceled":
		return string(model.ConclusionCancelled), true
	case "running", "live":
		return string(model.StatusInProgress), true
	}
	if status, err := model.ParseStatus(query); err == nil {
		return string(status), true
	}
	if conclusion, err := model.ParseConclusion(query); err == nil && conclusion != model.ConclusionNone {
		return string(conclusion), true
	}
	return "", false
}

func isKnownEvent(query string) bool {
	switch query {
	case "push", "pull_request", "workflow_dispatch", "schedule", "workflow_run", "repository_dispatch", "merge_group":
		return true
	default:
		return false
	}
}

func (a App) loadFailure(run model.Run, job model.Job) App {
	a.clearRouteError(RouteFailure)
	if a.failureResolver == nil {
		a.setRouteError(RouteFailure, "failure unavailable: live failure loader is not configured")
		return a
	}
	resolved, fullLog, err := a.failureResolver(run, job)
	if err != nil {
		a.setRouteError(RouteFailure, "failure unavailable: "+err.Error())
		return a
	}
	a.failure = resolved
	a.log = fullLog
	a.clearRouteError(RouteLog)
	a.pushLogRefetchToast(job.ID)
	return a
}

func (a App) loadLog(run model.Run, job model.Job) App {
	a.clearRouteError(RouteLog)
	if a.logResolver == nil {
		a.setRouteError(RouteLog, "log unavailable: live log loader is not configured")
		return a
	}
	resolved, err := a.logResolver(run, job)
	if err != nil {
		a.setRouteError(RouteLog, "log unavailable: "+err.Error())
		return a
	}
	a.log = resolved
	a.pushLogRefetchToast(job.ID)
	return a
}

func (a *App) pushLogRefetchToast(jobID int64) {
	if a.logRefetchNotice == nil || jobID == 0 {
		return
	}
	notice, ok := a.logRefetchNotice(jobID)
	if !ok {
		return
	}
	message := strings.TrimSpace(notice.Message)
	if message == "" {
		message = "link had expired; re-requested job log"
	}
	if notice.ExpiredStatus != 0 {
		message = fmt.Sprintf("%s · HTTP %d", message, notice.ExpiredStatus)
	}
	a.pushToast("log-refetch", usecase.Resilience{
		Class:          usecase.ErrorClassLogRender,
		Severity:       usecase.SeverityWarn,
		Title:          "Log render failed",
		Message:        message,
		RetryAction:    "refetch_log",
		KeepCachedView: true,
	})
}

func (a App) loadWatch(run model.Run) App {
	a.clearRouteError(RouteWatch)
	if a.watchResolver == nil {
		a.setRouteError(RouteWatch, "watch unavailable: live watch loader is not configured")
		return a
	}
	resolved, err := a.watchResolver(run)
	if err != nil {
		a.setRouteError(RouteWatch, "watch unavailable: "+err.Error())
		return a
	}
	a.watch = resolved
	return a
}

func (a App) openDispatch() App {
	workflows, err := a.dispatchWorkflowChoices()
	if err != nil {
		a.setRouteError(RouteDispatch, "dispatch unavailable: "+err.Error())
		a.PushRoute(RouteDispatch)
		return a
	}
	switch len(workflows) {
	case 0:
		a = a.loadDispatch()
		a.PushRoute(RouteDispatch)
	case 1:
		a.dispatch = dispatch.NewModel(workflows[0])
		a.PushRoute(RouteDispatch)
	default:
		a.palette = palette.New(dispatchPaletteItems(workflows))
		if a.TopOverlay() != OverlayPalette {
			a.overlays = append(a.overlays, OverlayPalette)
		}
	}
	return a
}

func (a App) loadDispatch() App {
	a.clearRouteError(RouteDispatch)
	if workflows, err := a.dispatchWorkflowChoices(); err == nil && len(workflows) > 0 {
		a.dispatch = dispatch.NewModel(workflows[0])
		return a
	}
	if a.dispatchResolver == nil {
		a.setRouteError(RouteDispatch, "dispatch unavailable: live workflow loader is not configured")
		return a
	}
	resolved, err := a.dispatchResolver()
	if err != nil {
		a.setRouteError(RouteDispatch, "dispatch unavailable: "+err.Error())
		return a
	}
	a.dispatch = resolved
	return a
}

func (a *App) dispatchWorkflowChoices() ([]dispatch.Workflow, error) {
	if len(a.dispatchWorkflows) > 0 {
		return append([]dispatch.Workflow(nil), a.dispatchWorkflows...), nil
	}
	if a.dispatchWorkflowsResolver == nil {
		return nil, nil
	}
	workflows, err := a.dispatchWorkflowsResolver()
	if err != nil {
		return nil, err
	}
	a.dispatchWorkflows = append([]dispatch.Workflow(nil), workflows...)
	return workflows, nil
}

func (a *App) dispatchWorkflowByValue(value string) (dispatch.Workflow, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return dispatch.Workflow{}, false
	}
	workflows, err := a.dispatchWorkflowChoices()
	if err != nil {
		return dispatch.Workflow{}, false
	}
	for _, workflow := range workflows {
		if workflowValue(workflow) == value {
			return workflow, true
		}
	}
	return dispatch.Workflow{}, false
}

func (a App) handleAction(route Route, request ActionRequest) App {
	if actionRequiresConfirmation(request.Action) {
		return a.openConfirm(route, request)
	}
	return a.executeAction(route, request)
}

func (a App) executeAction(route Route, request ActionRequest) App {
	a.clearRouteError(route)
	if a.actionHandler == nil {
		a.setRouteError(route, "action unavailable: live GitHub mutation handler is not configured")
		return a
	}
	if result, err := a.actionHandler(request); err != nil {
		a.pushErrorToast("action-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
	} else {
		a.pushToast("action-ok", usecase.ResilienceForSuccess(result))
	}
	return a
}

func (a App) openConfirm(route Route, request ActionRequest) App {
	a.clearRouteError(route)
	a.pendingAction = &pendingAction{route: route, request: request}
	a.confirm = confirm.New(confirmMessage(request))
	if a.TopOverlay() != OverlayConfirm {
		a.overlays = append(a.overlays, OverlayConfirm)
	}
	return a
}

func (a App) closeConfirm() App {
	if a.TopOverlay() == OverlayConfirm {
		a.overlays = a.overlays[:len(a.overlays)-1]
	}
	a.pendingAction = nil
	a.pendingDownload = nil
	a.confirm = confirm.Model{}
	return a
}

func actionRequiresConfirmation(action usecase.Action) bool {
	switch action {
	case usecase.ActionRerunRun,
		usecase.ActionRerunFailedJobs,
		usecase.ActionRerunJob,
		usecase.ActionCancelRun,
		usecase.ActionForceCancelRun:
		return true
	default:
		return false
	}
}

func confirmMessage(request ActionRequest) string {
	switch request.Action {
	case usecase.ActionRerunRun:
		return "rerun " + runTarget(request.Run)
	case usecase.ActionRerunFailedJobs:
		return "rerun failed jobs for " + runTarget(request.Run)
	case usecase.ActionRerunJob:
		return "rerun " + jobTarget(request.Job)
	case usecase.ActionCancelRun:
		return "cancel " + runTarget(request.Run)
	case usecase.ActionForceCancelRun:
		return "force-cancel " + runTarget(request.Run)
	default:
		return string(request.Action)
	}
}

func runTarget(run model.Run) string {
	if run.RunNumber > 0 {
		return fmt.Sprintf("run #%d", run.RunNumber)
	}
	if run.ID > 0 {
		return fmt.Sprintf("run %d", run.ID)
	}
	return "selected run"
}

func jobTarget(job model.Job) string {
	if strings.TrimSpace(job.Name) != "" {
		return "job " + strings.TrimSpace(job.Name)
	}
	if job.ID > 0 {
		return fmt.Sprintf("job %d", job.ID)
	}
	return "selected job"
}

func (a App) selectedDetailJob() model.Job {
	job, _ := a.detail.SelectedJobModel()
	return job
}

func (a *App) clearRouteError(route Route) {
	if a.routeErrors != nil {
		delete(a.routeErrors, route)
	}
}

func (a *App) setRouteError(route Route, message string) {
	if a.routeErrors == nil {
		a.routeErrors = map[Route]string{}
	}
	a.routeErrors[route] = message
}

func (a App) handleRunsError(route Route, id, routeMessage string, err error) App {
	if !a.hasRuns() {
		a.setRouteError(route, routeMessage)
		return a
	}
	a.clearRouteError(route)
	a.pushErrorToast(id, usecase.ResilienceFor(err, usecase.ErrorContext{}))
	return a
}

func (a App) hasRuns() bool {
	return len(a.runs.Context.Runs) > 0 || len(a.runs.Context.BranchRuns) > 0 || len(a.runs.Context.RepoRuns) > 0
}

func (a *App) pushErrorToast(id string, resilience usecase.Resilience) {
	a.pushToast(id, resilience)
}

func (a *App) pushToast(id string, resilience usecase.Resilience) {
	if a.toasts.Toasts == nil {
		a.toasts = toast.New()
	}
	a.toasts = a.toasts.Push(toast.FromResilience(id, resilience, 8*time.Second))
}

func (a App) openExternal(url string) App {
	url = strings.TrimSpace(url)
	if url == "" {
		a.pushErrorToast("open-url-missing", usecase.ResilienceFor(fmt.Errorf("browser URL is not available for this selection"), usecase.ErrorContext{}))
		return a
	}
	if a.openURL == nil {
		a.pushErrorToast("open-url-unavailable", usecase.ResilienceFor(fmt.Errorf("browser opener is not configured"), usecase.ErrorContext{}))
		return a
	}
	if err := a.openURL(url); err != nil {
		a.pushErrorToast("open-url-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
		return a
	}
	a.pushToast("open-url-ok", usecase.Resilience{
		Class:          usecase.ErrorClassSuccess,
		Severity:       usecase.SeverityOK,
		Title:          "Opened in browser",
		Message:        url,
		RetryAction:    "open",
		KeepCachedView: true,
	})
	return a
}

func (a App) copyExternal(id, value string) App {
	value = strings.TrimSpace(value)
	if value == "" {
		a.pushErrorToast(id+"-missing", usecase.ResilienceFor(fmt.Errorf("copy value is not available for this selection"), usecase.ErrorContext{}))
		return a
	}
	if a.copyText == nil {
		a.pushErrorToast(id+"-unavailable", usecase.ResilienceFor(fmt.Errorf("clipboard copier is not configured"), usecase.ErrorContext{}))
		return a
	}
	if err := a.copyText(value); err != nil {
		a.pushErrorToast(id+"-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
		return a
	}
	a.pushToast(id+"-ok", usecase.Resilience{
		Class:          usecase.ErrorClassSuccess,
		Severity:       usecase.SeverityOK,
		Title:          "Copied",
		Message:        copySummary(value),
		RetryAction:    "copy",
		KeepCachedView: true,
	})
	return a
}

func selectedRunURL(repo string, run model.Run) string {
	if strings.TrimSpace(run.HTMLURL) != "" {
		return strings.TrimSpace(run.HTMLURL)
	}
	if strings.TrimSpace(repo) != "" && run.ID > 0 {
		return fmt.Sprintf("https://github.com/%s/actions/runs/%d", strings.TrimSpace(repo), run.ID)
	}
	return ""
}

func detailBrowserURL(m detail.Model) string {
	if job, ok := m.SelectedJobModel(); ok && strings.TrimSpace(job.HTMLURL) != "" {
		return strings.TrimSpace(job.HTMLURL)
	}
	return selectedRunURL(m.Repo, m.Run)
}

func failureBrowserURL(m failure.Model) string {
	if strings.TrimSpace(m.Report.Job.HTMLURL) != "" {
		return strings.TrimSpace(m.Report.Job.HTMLURL)
	}
	if strings.TrimSpace(m.Repo) != "" && m.RunID > 0 && m.Report.Job.ID > 0 {
		return fmt.Sprintf("https://github.com/%s/actions/runs/%d/job/%d", strings.TrimSpace(m.Repo), m.RunID, m.Report.Job.ID)
	}
	if strings.TrimSpace(m.Repo) != "" && m.RunID > 0 {
		return fmt.Sprintf("https://github.com/%s/actions/runs/%d", strings.TrimSpace(m.Repo), m.RunID)
	}
	return ""
}

func failureExcerptText(m failure.Model) string {
	lines := m.Excerpt
	if len(lines) == 0 {
		lines = m.Report.Log.Failure.Lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, fmt.Sprintf("%03d %s", line.Number, strings.TrimSpace(line.Text)))
	}
	return strings.Join(out, "\n")
}

func copySummary(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if len([]rune(value)) > 72 {
		value = string([]rune(value)[:71]) + "…"
	}
	return value
}

func runsHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "g", "G", "s", "/", "enter", "l", "w", "D", "o", "y", "r", "R", "x", "X", "esc", "backspace":
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

func paletteHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "enter", "backspace", "esc":
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
	case OverlayConfirm:
		return "Confirm action"
	case OverlayTimeJump:
		return "Jump to time"
	default:
		return ""
	}
}

func (a App) overlayView(width int) string {
	switch a.TopOverlay() {
	case OverlayHelp:
		return help.View(a.footerScreen(), width-20)
	case OverlayPalette:
		return palette.View(a.palette, width-20)
	case OverlayConfirm:
		return confirm.View(a.confirm, width-20)
	case OverlayTimeJump:
		return timejump.View(a.timeJump, width-20)
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
		return "hound", "failed step", failureRight(a.failure)
	case RouteLog:
		return "hound", "full log", logRight(a.log)
	case RouteWatch:
		run := a.watch.State.Run
		return "hound", runChromeTitle(run), "streaming · follow ●"
	case RouteDispatch:
		return "hound", "workflow_dispatch", firstNonEmpty(a.dispatch.Workflow.Name, a.dispatch.Workflow.ID)
	default:
		if a.runs.AllGreen() {
			return "hound", branchContext(a.runs.Context.Scope, a.runs.Context.Branch, a.runs.Context.Actor), runsRight(a.runs, a.refreshCount, a.runsMeta)
		}
		return "hound", branchContext(a.runs.Context.Scope, a.runs.Context.Branch, a.runs.Context.Actor), runsRight(a.runs, a.refreshCount, a.runsMeta)
	}
}

func hasLaunchContext(ctx usecase.LaunchContext) bool {
	return ctx.Repo != "" || ctx.Branch != "" || ctx.Actor != "" || ctx.State != "" || len(ctx.Runs) > 0 || len(ctx.Workflows) > 0 || ctx.Notice != "" || ctx.ErrorMessage != ""
}

func branchContext(scope usecase.LaunchScope, branch, actor string) string {
	label := "repo all branches"
	if scope == usecase.LaunchScopeBranch && strings.TrimSpace(branch) != "" {
		label = "branch " + strings.TrimSpace(branch)
	}
	if strings.TrimSpace(actor) == "" {
		return "⎇ " + label
	}
	return "⎇ " + label + " · @" + strings.TrimSpace(actor)
}

func runChromeTitle(run model.Run) string {
	name := firstNonEmpty(run.Name, run.DisplayTitle, run.Path)
	switch {
	case name != "" && run.RunNumber > 0:
		return fmt.Sprintf("%s #%d", name, run.RunNumber)
	case name != "":
		return name
	case run.RunNumber > 0:
		return fmt.Sprintf("#%d", run.RunNumber)
	case run.ID > 0:
		return fmt.Sprintf("run %d", run.ID)
	default:
		return ""
	}
}

func runsRight(m runs.Model, refreshCount int, meta usecase.RequestMeta) string {
	count := len(m.Context.Runs)
	if m.Context.Scope == usecase.LaunchScopeRepo && len(m.Context.RepoRuns) > 0 {
		count = len(m.Context.RepoRuns)
	}
	if m.Context.Scope == usecase.LaunchScopeBranch && len(m.Context.BranchRuns) > 0 {
		count = len(m.Context.BranchRuns)
	}
	if count == 0 {
		return "no runs loaded"
	}
	value := fmt.Sprintf("%d runs loaded", count)
	if refreshCount > 0 {
		value += " · live"
	}
	if meta.RateRemaining != "" {
		value += fmt.Sprintf(" · %s/5k", meta.RateRemaining)
	}
	if meta.Status == 304 || meta.Cache == "hit" {
		value += " · 304"
	}
	return value
}

func logRight(m logscreen.Model) string {
	if m.Search.Total > 0 {
		return fmt.Sprintf("match %d/%d", m.Search.Current, m.Search.Total)
	}
	return fmt.Sprintf("%d lines", len(m.Document.Lines))
}

func failureRight(m failure.Model) string {
	if m.TotalLines > 0 {
		return fmt.Sprintf("%d log lines", m.TotalLines)
	}
	return ""
}

func detailContext(run model.Run) string {
	title := runChromeTitle(run)
	if strings.TrimSpace(run.HeadBranch) == "" {
		return title
	}
	if title == "" {
		return strings.TrimSpace(run.HeadBranch)
	}
	return title + " › " + strings.TrimSpace(run.HeadBranch)
}

func shortSHA(sha string) string {
	if sha == "" {
		return ""
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

func paletteItems(workflows []dispatch.Workflow) []palette.Item {
	items := []palette.Item{
		{Name: "runs", Description: "workflow runs · this branch", Tag: "default", Route: "runs"},
		{Name: "runs --all", Description: "runs across all branches", Route: "runs --all"},
		{Name: "run:failed", Description: "filtered to failures", Route: "run:failed"},
		{Name: "artifacts", Description: "selected run's artifacts", Route: "artifacts"},
	}
	if len(workflows) == 0 {
		items = append(items, palette.Item{Name: "dispatch", Description: "trigger workflow_dispatch", Route: string(RouteDispatch)})
		return items
	}
	return append(items, dispatchPaletteItems(workflows)...)
}

func dispatchPaletteItems(workflows []dispatch.Workflow) []palette.Item {
	items := make([]palette.Item, 0, len(workflows))
	for _, workflow := range workflows {
		name := strings.TrimSpace(workflow.Name)
		if name == "" {
			name = workflow.ID
		}
		if name == "" {
			continue
		}
		items = append(items, palette.Item{
			Name:        "dispatch: " + name,
			Description: "workflow_dispatch · " + workflowValue(workflow),
			Route:       string(RouteDispatch),
			Value:       workflowValue(workflow),
		})
	}
	if len(items) == 0 {
		return []palette.Item{{Name: "dispatch", Description: "trigger workflow_dispatch", Route: string(RouteDispatch)}}
	}
	return items
}

func workflowValue(workflow dispatch.Workflow) string {
	if strings.TrimSpace(workflow.ID) != "" {
		return strings.TrimSpace(workflow.ID)
	}
	return strings.TrimSpace(workflow.Name)
}

func (a App) screenBody(width, height int) string {
	bodyWidth := contentWidth(width)
	if body, ok := a.routeErrorBody(a.Route(), bodyWidth); ok {
		return body
	}
	if body, ok := a.unloadedRouteBody(a.Route(), bodyWidth); ok {
		return body
	}
	switch a.Route() {
	case RouteWelcome:
		return welcome.View(welcome.Model{Build: a.build}, bodyWidth, max(height-6, 0))
	case RouteRuns:
		if body, ok := a.launchStateBody(bodyWidth); ok {
			return body
		}
		return runs.ViewSize(a.runs, bodyWidth, bodyHeight(height), time.Now())
	case RouteDetail:
		return detail.ViewSize(a.detail, bodyWidth, bodyHeight(height))
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

func (a App) routeErrorBody(route Route, width int) (string, bool) {
	if a.routeErrors == nil {
		return "", false
	}
	message := strings.TrimSpace(a.routeErrors[route])
	if message == "" {
		return "", false
	}
	return empty.View(empty.Model{
		Kind:    empty.KindError,
		Repo:    a.runs.Context.Repo,
		Branch:  a.runs.Context.Branch,
		Message: message,
	}, width), true
}

func (a App) unloadedRouteBody(route Route, width int) (string, bool) {
	message := ""
	switch route {
	case RouteFailure:
		if a.failure.RunID == 0 && len(a.failure.Report.Log.Lines) == 0 {
			message = "failure unavailable: select a failed job with live GitHub data loaded"
		}
	case RouteLog:
		if len(a.log.Document.Lines) == 0 {
			message = "log unavailable: select a job with live GitHub logs loaded"
		}
	case RouteWatch:
		if a.watch.State.Run.ID == 0 {
			message = "watch unavailable: select a live run first"
		}
	case RouteDispatch:
		if a.dispatch.Workflow.ID == "" && a.dispatch.Workflow.Name == "" {
			message = "dispatch unavailable: no workflow has been loaded"
		}
	}
	if message == "" {
		return "", false
	}
	return empty.View(empty.Model{
		Kind:    empty.KindError,
		Repo:    a.runs.Context.Repo,
		Branch:  a.runs.Context.Branch,
		Message: message,
	}, width), true
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
		return watch.Model{}
	}
	run, ok := runsModel.SelectedRun()
	if !ok {
		return watch.Model{}
	}
	return watch.NewModel(watch.State{
		Repo:   ctx.Repo,
		Branch: firstNonEmpty(ctx.Branch, run.HeadBranch),
		Run:    run,
	})
}

func (a App) launchStateBody(width int) (string, bool) {
	ctx := a.runs.Context
	switch ctx.State {
	case usecase.LaunchStateError:
		kind := empty.KindError
		if ctx.Repo == "" {
			kind = empty.KindNoRepository
		}
		return empty.View(empty.Model{
			Kind:    kind,
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

func toastLayer(th theme.Theme, body string, model toast.Model, width int) string {
	if len(model.Toasts) == 0 || width <= 0 {
		return body
	}
	bodyLines := splitLines(body)
	toasts := model.Toasts
	if len(toasts) > 2 {
		toasts = toasts[len(toasts)-2:]
	}
	for i, item := range toasts {
		line := toastText(th, item, width)
		if i >= len(bodyLines) {
			bodyLines = append(bodyLines, "")
		}
		bodyLines[i] = mergeRight(bodyLines[i], line, width)
	}
	return strings.Join(bodyLines, "\n")
}

func toastText(th theme.Theme, item toast.Toast, width int) string {
	message := strings.TrimSpace(item.Message)
	value := toast.Glyph(item.Severity) + " " + item.Title
	if message != "" {
		value += " · " + message
	}
	if visibleLen(value) > width && item.SourceClass == usecase.ErrorClassRateLimit {
		value = toast.Glyph(item.Severity) + " Rate limit"
		if message != "" {
			value += " · " + message
		}
	}
	value = fitPlain(value, width)
	color := th.Info
	switch item.Severity {
	case usecase.SeverityOK:
		color = th.OK
	case usecase.SeverityWarn:
		color = th.Run
	case usecase.SeverityError:
		color = th.Fail
	}
	return sgrHex(color, false) + value + sgrReset
}

func mergeRight(left, right string, width int) string {
	rightWidth := visibleLen(right)
	if rightWidth >= width {
		return fitPlain(right, width)
	}
	leftWidth := max(width-rightWidth-1, 0)
	left = fitPlain(left, leftWidth)
	spacer := max(width-visibleLen(left)-rightWidth, 1)
	return left + strings.Repeat(" ", spacer) + right
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
	artifacts := []model.Artifact{
		{ID: 901, Name: "coverage", SizeInBytes: 1262848, CreatedAt: start.Add(135 * time.Second), ExpiresAt: start.AddDate(0, 0, 7)},
		{ID: 902, Name: "playwright-report-shard-3-of-8-chromium-darwin-arm64", SizeInBytes: 348160, CreatedAt: start.Add(136 * time.Second), ExpiresAt: start.AddDate(0, 0, 7)},
		{ID: 903, Name: "old-report", SizeInBytes: 52480, Expired: true, CreatedAt: start.AddDate(0, -3, 0), ExpiresAt: start.AddDate(0, -3, 7)},
	}
	return detail.NewModel(run, jobs).WithRepo("indrasvat/gh-hound").WithArtifacts(artifacts)
}

func DetailModelForRun(run model.Run) detail.Model {
	return detail.NewModel(run, nil)
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
