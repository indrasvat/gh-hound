package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/approvals"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/confirm"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/help"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/palette"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/timejump"
	"github.com/indrasvat/gh-hound/internal/tui/screens/caches"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	diffscreen "github.com/indrasvat/gh-hound/internal/tui/screens/diff"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/empty"
	"github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/tui/screens/welcome"
	workflowsscreen "github.com/indrasvat/gh-hound/internal/tui/screens/workflows"
	"github.com/indrasvat/gh-hound/internal/tui/toast"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type BuildInfo = banner.BuildInfo

type Route string

const (
	RouteWelcome    Route = "welcome"
	RouteRuns       Route = "runs"
	RouteDetail     Route = "detail"
	RouteFailure    Route = "failure"
	RouteLog        Route = "log"
	RouteWatch      Route = "watch"
	RouteWatchBoard Route = "watch_board"
	RouteDispatch   Route = "dispatch"
	RouteDiff       Route = "diff"
	RouteCaches     Route = "caches"
	RouteWorkflows  Route = "workflows"
)

type Overlay string

const (
	OverlayNone      Overlay = ""
	OverlayHelp      Overlay = "help"
	OverlayPalette   Overlay = "palette"
	OverlayConfirm   Overlay = "confirm"
	OverlayTimeJump  Overlay = "time_jump"
	OverlayApprovals Overlay = "approvals"
)

type KeyMsg struct {
	Key string
}

type Options struct {
	Config                    config.Config
	Build                     BuildInfo
	Launch                    usecase.LaunchContext
	DetailResolver            func(context.Context, model.Run) (detail.Model, error)
	RunsResolver              func(context.Context, usecase.RunFilter) ([]model.Run, error)
	FailureResolver           func(context.Context, model.Run, model.Job) (failure.Model, logscreen.Model, error)
	LogResolver               func(context.Context, model.Run, model.Job, func(read, total int64)) (logscreen.Model, error)
	WatchResolver             func(context.Context, model.Run) (watch.Model, error)
	PackResolver              func(context.Context, usecase.PackState) (usecase.PackState, error)
	DispatchAttachResolver    func(ctx context.Context, workflowID, ref string, since time.Time) (model.Run, error)
	RerunAttachResolver       func(context.Context, model.Run) (model.Run, error)
	DispatchResolver          func(context.Context) (dispatch.Model, error)
	DispatchWorkflowsResolver func(context.Context) ([]dispatch.Workflow, error)
	DiffResolver              func(context.Context, model.Run) (usecase.RegressionVerdict, error)
	WorkflowsResolver         func(context.Context) ([]model.Workflow, error)
	RunsMetadata              func() (usecase.RequestMeta, bool)
	LogRefetchNotice          func(int64) (usecase.LogRefetchNotice, bool)
	ActionHandler             func(ActionRequest) (usecase.ActionResult, error)
	ApprovalsResolver         func(context.Context, model.Run) ([]model.PendingDeployment, error)
	ArtifactsResolver         func(model.Run) ([]model.Artifact, error)
	ArtifactDownloader        func(model.Artifact, string) (usecase.DownloadResult, error)
	CachesResolver            func(context.Context) (caches.Data, error)
	CacheDeleter              func(context.Context, CacheDeleteRequest) (int, error)
	OpenURL                   func(string) error
	CopyText                  func(string) error
}

// CacheDeleteRequest is the app's eviction intent: by ID (one cache)
// or by key (every cache sharing it). Exactly one field is set.
type CacheDeleteRequest struct {
	ID  int64
	Key string
}

type ActionRequest struct {
	Action   usecase.Action
	Run      model.Run
	Job      model.Job
	Workflow dispatch.Workflow
	Dispatch usecase.DispatchRequest
	Debug    bool
	// Environments and Comment carry deployment-review intent for the
	// approve/reject actions.
	Environments []string
	Comment      string
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
	board                     watch.Board
	dispatch                  dispatch.Model
	diff                      diffscreen.Model
	workflows                 workflowsscreen.Model
	palette                   palette.Model
	approvals                 approvals.Model
	confirm                   confirm.Model
	toasts                    toast.Model
	runsMeta                  usecase.RequestMeta
	dispatchWorkflows         []dispatch.Workflow
	pendingAction             *pendingAction
	routeErrors               map[Route]string
	detailResolver            func(context.Context, model.Run) (detail.Model, error)
	runsResolver              func(context.Context, usecase.RunFilter) ([]model.Run, error)
	failureResolver           func(context.Context, model.Run, model.Job) (failure.Model, logscreen.Model, error)
	logResolver               func(context.Context, model.Run, model.Job, func(read, total int64)) (logscreen.Model, error)
	watchResolver             func(context.Context, model.Run) (watch.Model, error)
	packResolver              func(context.Context, usecase.PackState) (usecase.PackState, error)
	dispatchAttachResolver    func(ctx context.Context, workflowID, ref string, since time.Time) (model.Run, error)
	rerunAttachResolver       func(context.Context, model.Run) (model.Run, error)
	dispatchResolver          func(context.Context) (dispatch.Model, error)
	dispatchWorkflowsResolver func(context.Context) ([]dispatch.Workflow, error)
	diffResolver              func(context.Context, model.Run) (usecase.RegressionVerdict, error)
	workflowsResolver         func(context.Context) ([]model.Workflow, error)
	runsMetadata              func() (usecase.RequestMeta, bool)
	logRefetchNotice          func(int64) (usecase.LogRefetchNotice, bool)
	actionHandler             func(ActionRequest) (usecase.ActionResult, error)
	approvalsResolver         func(context.Context, model.Run) ([]model.PendingDeployment, error)
	artifactsResolver         func(model.Run) ([]model.Artifact, error)
	artifactDownloader        func(model.Artifact, string) (usecase.DownloadResult, error)
	artifactsFetch            *artifactsFetchState
	artifactDownload          *artifactDownloadState
	pendingDownload           *model.Artifact
	caches                    caches.Model
	cachesResolver            func(context.Context) (caches.Data, error)
	cacheDeleter              func(context.Context, CacheDeleteRequest) (int, error)
	pendingCacheDelete        *CacheDeleteRequest
	timeJump                  timejump.Model
	lastToastTick             time.Time
	openURL                   func(string) error
	copyText                  func(string) error
	load                      *pendingLoad
}

// startLoad runs work off the paint path and records the app's single
// in-flight fetch. Starting a new load supersedes the old one by
// pointer replacement; the orphaned goroutine's result can never
// apply. work returns the apply closure that folds the result (or its
// error handling) into the App at drain time.
func (a App) startLoad(kind loadKind, label string, work func(ctx context.Context) func(App) App) App {
	return a.startLoadProgress(kind, label, func(ctx context.Context, _ func(int64, int64)) func(App) App {
		return work(ctx)
	})
}

// loadBlocked reports whether starting a load of the given kind must
// be refused: a DIFFERENT kind is already in flight. Same-kind loads
// supersede (f-cycle, opening another run's detail), but a cross-kind
// open would cancel work whose result the abandoned route still needs
// — escape back and it would be a stuck skeleton.
func (a App) loadBlocked(kind loadKind) bool {
	return a.load != nil && a.load.kind != kind
}

// startLoadProgress is startLoad for fetches that can report byte
// progress and honor cancellation; work receives a per-load context
// (cancelled on esc or supersession) and the progress callback.
func (a App) startLoadProgress(kind loadKind, label string, work func(ctx context.Context, progress func(read, total int64)) func(App) App) App {
	if a.loadBlocked(kind) {
		// Backstop: intent sites guard with loadBlocked before pushing
		// routes; refusing here keeps the single-slot invariant even if
		// a future call site forgets.
		return a
	}
	if prev := a.load; prev != nil && prev.cancel != nil {
		// Superseded work must stop burning the serial queue, not just
		// lose its seat at the table.
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &pendingLoad{kind: kind, label: label, started: time.Now(), cancel: cancel}
	a.load = state
	go func() {
		state.finish(work(ctx, state.progress))
	}()
	return a
}

// drainLoad applies a completed load, or reports an animation frame is
// due while the spinner is visible. Inside the grace window it stays
// quiet so sub-100ms fetches never flash.
func (a App) drainLoad() (App, bool) {
	load := a.load
	if load == nil {
		return a, false
	}
	done, apply, _, _ := load.snapshot()
	if !done {
		return a, time.Since(load.started) >= loadGraceDelay
	}
	a.load = nil
	if apply != nil {
		a = apply(a)
	}
	return a, true
}

// SettleLoads blocks until the pending load (and any load it chains
// into) has applied, or the timeout passes. It exists for tests and
// deterministic fixtures only — the interactive loop drains on poll
// ticks and must never call it.
func (a App) SettleLoads(timeout time.Duration) (App, bool) {
	deadline := time.Now().Add(timeout)
	for a.load != nil {
		done, _, _, _ := a.load.snapshot()
		if done {
			a, _ = a.drainLoad()
			continue
		}
		if time.Now().After(deadline) {
			return a, false
		}
		time.Sleep(time.Millisecond)
	}
	return a, true
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
			resolved, err := options.DetailResolver(context.Background(), run)
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
	app := App{
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
		packResolver:              options.PackResolver,
		dispatchAttachResolver:    options.DispatchAttachResolver,
		rerunAttachResolver:       options.RerunAttachResolver,
		dispatchResolver:          options.DispatchResolver,
		dispatchWorkflowsResolver: options.DispatchWorkflowsResolver,
		diffResolver:              options.DiffResolver,
		workflowsResolver:         options.WorkflowsResolver,
		runsMetadata:              options.RunsMetadata,
		logRefetchNotice:          options.LogRefetchNotice,
		actionHandler:             options.ActionHandler,
		approvalsResolver:         options.ApprovalsResolver,
		artifactsResolver:         options.ArtifactsResolver,
		artifactDownloader:        options.ArtifactDownloader,
		cachesResolver:            options.CachesResolver,
		cacheDeleter:              options.CacheDeleter,
		openURL:                   options.OpenURL,
		copyText:                  options.CopyText,
	}
	if route == RouteDispatch {
		// A dispatch launch (gh hound dispatch) must start the async
		// workflows fetch like the D key does: open from a runs base
		// so the 0/1/many decision routes identically.
		app.routes = []Route{RouteRuns}
		app.launchRoute = RouteRuns
		app = app.openDispatch()
	}
	return app
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
	app.detailResolver = func(context.Context, model.Run) (detail.Model, error) {
		return sampleDetailModel(), nil
	}
	app.runsResolver = func(context.Context, usecase.RunFilter) ([]model.Run, error) {
		return sampleRunsModel().Context.Runs, nil
	}
	app.failureResolver = func(context.Context, model.Run, model.Job) (failure.Model, logscreen.Model, error) {
		return sampleFailureModel(), sampleLogModel(), nil
	}
	app.logResolver = func(context.Context, model.Run, model.Job, func(read, total int64)) (logscreen.Model, error) {
		return sampleLogModel(), nil
	}
	app.watchResolver = func(context.Context, model.Run) (watch.Model, error) {
		return sampleWatchModel(), nil
	}
	app.dispatchResolver = func(context.Context) (dispatch.Model, error) {
		return sampleDispatchModel(), nil
	}
	app.dispatchWorkflowsResolver = func(context.Context) ([]dispatch.Workflow, error) {
		return []dispatch.Workflow{sampleDispatchModel().Workflow}, nil
	}
	app.actionHandler = func(ActionRequest) (usecase.ActionResult, error) {
		return usecase.ActionResult{Message: "accepted"}, nil
	}
	app.diffResolver = func(context.Context, model.Run) (usecase.RegressionVerdict, error) {
		return sampleDiffVerdict(), nil
	}
	app.workflowsResolver = func(context.Context) ([]model.Workflow, error) {
		return sampleWorkflows(), nil
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
	if load := a.load; load != nil {
		if done, _, _, _ := load.snapshot(); done {
			a, _ = a.drainLoad()
		} else if msg.Key == "esc" {
			// Cancel interest AND the underlying work: the orphaned
			// result can never apply, and the fetch stops occupying
			// the serial queue. Re-enter (load is nil now) so normal
			// esc routing still restores the prior view.
			if load.cancel != nil {
				load.cancel()
			}
			a.load = nil
			next, _ := a.Update(msg)
			return next, true
		}
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
		if a.TopOverlay() == OverlayApprovals {
			return a.updateApprovals(msg)
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
			if a.launchRoute == RouteDispatch {
				// Dispatch launches go through the standard open flow
				// so the workflows fetch starts (same as the D key).
				a.routes[len(a.routes)-1] = RouteRuns
				a.launchRoute = RouteRuns
				return a.openDispatch(), true
			}
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
			if pending := a.pendingAction; pending != nil && rerunFamily(pending.request.Action) {
				footer = "y confirm · d debug · enter/n/esc cancel"
			}
		}
		if a.TopOverlay() == OverlayTimeJump {
			footer = "j/k pick · type time · ⏎ go · ⎋ cancel"
		}
		if a.TopOverlay() == OverlayApprovals {
			footer = "j/k move · space pick · y open gate · n keep shut · c comment · ⎋ close"
			if a.approvals.CommentMode {
				footer = "type comment · ⏎ done · ⎋ cancel"
			}
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
	case "runs-loading":
		// Deterministic loading states: started is pinned 250ms back so
		// the spinner sits stably on frame 2 and the grace window has
		// passed.
		loadingApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		loadingApp.load = &pendingLoad{kind: loadKindRuns, label: "sniffing out failing runs", started: time.Now().Add(-250 * time.Millisecond)}
		return loadingApp.ViewSize(width, height)
	case "detail-loading":
		loadingApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		run := loadingApp.runs.Context.Runs[0]
		loadingApp.detail = DetailModelForRun(run).WithRepo(loadingApp.runs.Context.Repo)
		loadingApp.routes = []Route{RouteRuns, RouteDetail}
		loadingApp.load = &pendingLoad{kind: loadKindDetail, label: "fetching jobs", started: time.Now().Add(-250 * time.Millisecond)}
		return loadingApp.ViewSize(width, height)
	case "failure-loading":
		loadingApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		loadingApp.routes = []Route{RouteRuns, RouteDetail, RouteFailure}
		loadingApp.load = &pendingLoad{kind: loadKindFailure, label: "fetching the failure", started: time.Now().Add(-250 * time.Millisecond)}
		return loadingApp.ViewSize(width, height)
	case "log-progress":
		loadingApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		loadingApp.routes = []Route{RouteRuns, RouteLog}
		loadingApp.load = &pendingLoad{kind: loadKindLog, label: "fetching log", started: time.Now().Add(-250 * time.Millisecond), read: 2202009, total: 5033165}
		return loadingApp.ViewSize(width, height)
	case "runs-waiting":
		app.runs = sampleWaitingRunsModel()
		return frameViewSize(app.theme, "hound", "⎇ branch main · @indrasvat", "◔ 4,981/5k live", runs.View(app.runs, bodyWidth, time.Now()), keys.FooterForScreen(keys.ScreenRunsList), width, height, true)
	case "approvals":
		app.runs = sampleWaitingRunsModel()
		app.routes = []Route{RouteRuns}
		run, _ := app.runs.SelectedRun()
		app.approvals = approvals.NewModel(run, samplePendingDeployments())
		app.overlays = append(app.overlays, OverlayApprovals)
		return app.ViewSize(width, height)
	case "detail-pending":
		app.runs = sampleWaitingRunsModel()
		run, _ := app.runs.SelectedRun()
		m := DetailModelForRun(run).WithRepo("indrasvat/gh-hound").WithPendingDeployments(samplePendingDeployments())
		return frameViewSize(app.theme, "hound", "Deploy #572 › main", "waiting", detail.ViewSize(m, bodyWidth, bodyHeight(height)), keys.FooterForScreen(keys.ScreenDetail), width, height, true)
	case "rerun-confirm":
		loadingApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		run, _ := loadingApp.runs.SelectedRun()
		loadingApp = loadingApp.openConfirm(RouteRuns, ActionRequest{Action: usecase.ActionRerunRun, Run: run, Debug: true})
		return loadingApp.ViewSize(width, height)
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
	case "watch-board":
		boardApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		boardApp.board = sampleBoardModel()
		boardApp.routes = []Route{RouteRuns, RouteWatchBoard}
		return boardApp.ViewSize(width, height)
	case "log":
		m := sampleLogModel()
		if rows := bodyHeight(height) - 1; rows > 0 {
			m.Height = rows
		}
		return frameViewSize(app.theme, "hound", "full log", "match 1/1", logscreen.View(m, bodyWidth), keys.FooterForScreen(keys.ScreenLog), width, height, true)
	case "dispatch":
		return frameViewSize(app.theme, "hound", "workflow_dispatch", "Release", dispatch.View(sampleDispatchModel(), bodyWidth), keys.FooterForScreen(keys.ScreenDispatch), width, height, true)
	case "diff":
		trailApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		trailApp.diff = diffscreen.NewModel(sampleDiffVerdict())
		trailApp.routes = []Route{RouteRuns, RouteDiff}
		return trailApp.ViewSize(width, height)
	case "diff-inconclusive":
		trailApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		trailApp.diff = diffscreen.NewModel(sampleColdTrailVerdict())
		trailApp.routes = []Route{RouteRuns, RouteDiff}
		return trailApp.ViewSize(width, height)
	case "caches":
		m := sampleCachesModel()
		return frameViewSize(app.theme, "hound", "the kennel · caches", caches.UsageLine(m.Usage, m.Cap), caches.ViewSize(m, bodyWidth, bodyHeight(height), time.Now()), keys.FooterForScreen(keys.ScreenCaches), width, height, true)
	case "caches-pressure":
		m := sampleCachesPressureModel()
		return frameViewSize(app.theme, "hound", "the kennel · caches", caches.UsageLine(m.Usage, m.Cap), caches.ViewSize(m, bodyWidth, bodyHeight(height), time.Now()), keys.FooterForScreen(keys.ScreenCaches), width, height, true)
	case "caches-empty":
		m := caches.NewModel("indrasvat/gh-hound", caches.Data{})
		return frameViewSize(app.theme, "hound", "the kennel · caches", caches.UsageLine(m.Usage, m.Cap), caches.ViewSize(m, bodyWidth, bodyHeight(height), time.Now()), keys.FooterForScreen(keys.ScreenCaches), width, height, true)
	case "workflows":
		kennelApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		kennelApp.workflows = workflowsscreen.NewModel("indrasvat/gh-hound", sampleWorkflows())
		kennelApp.routes = []Route{RouteRuns, RouteWorkflows}
		return kennelApp.ViewSize(width, height)
	case "dispatch-picker":
		pickerApp := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		pickerApp.dispatchWorkflows = sampleDispatchPickerWorkflows()
		pickerApp.palette = palette.New(dispatchPaletteItems(pickerApp.dispatchWorkflows))
		pickerApp.overlays = append(pickerApp.overlays, OverlayPalette)
		return pickerApp.ViewSize(width, height)
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
	// A pending load animates the shared spinner: tick at frame rate.
	if a.load != nil {
		return loadFrameInterval
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
	if next, ok := a.drainLoad(); ok {
		a = next
		changed = true
	}
	if a.load != nil {
		// A pending load owns the serial queue: route polling here
		// would block the loop (starving spinner frames) and double-
		// fetch the same surface. Drain + animate only.
		return a, changed
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
	case RouteWatchBoard:
		next, refreshed := a.refreshPack()
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
	case RouteCaches:
		return a.caches.InputMode
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
	case RouteWatchBoard:
		return a.updateWatchBoard(msg)
	case RouteDispatch:
		return a.updateDispatch(msg)
	case RouteDiff:
		return a.updateDiff(msg)
	case RouteCaches:
		return a.updateCaches(msg)
	case RouteWorkflows:
		return a.updateWorkflows(msg)
	default:
		return a, false
	}
}

func (a App) updateDiff(msg KeyMsg) (App, bool) {
	before := a.diff
	a.diff = a.diff.Update(diffscreen.KeyMsg{Key: msg.Key})
	switch a.diff.Intent.Kind {
	case diffscreen.IntentOpenFirstBad:
		if a.loadBlocked(loadKindDetail) {
			break
		}
		a = a.loadDetail(a.diff.Verdict.FirstBad)
		a.PushRoute(RouteDetail)
	case diffscreen.IntentBrowser:
		a = a.openExternal(a.diff.Verdict.CompareURL)
	case diffscreen.IntentBack:
		a.PopRoute()
	}
	return a, diffScreenHandled(msg.Key) || before.Selected != a.diff.Selected || a.diff.Intent.Kind != diffscreen.IntentNone
}

// openDiff starts the regression scan for the selected run's workflow
// and routes to the trail screen. The scan is async (Task 220
// invariant): the route pushes immediately and the shared loading body
// holds the pane until the verdict lands.
func (a App) openDiff() App {
	if a.loadBlocked(loadKindDiff) {
		return a
	}
	run, ok := a.runs.SelectedRun()
	if !ok {
		a.pushErrorToast("diff-no-run", usecase.ResilienceFor(fmt.Errorf("no run selected — give the hound a trail to start from"), usecase.ErrorContext{}))
		return a
	}
	a.clearRouteError(RouteDiff)
	a.diff = diffscreen.Model{}
	if a.diffResolver == nil {
		a.setRouteError(RouteDiff, "diff unavailable: live regression scan is not configured")
		a.PushRoute(RouteDiff)
		return a
	}
	resolver := a.diffResolver
	a = a.startLoad(loadKindDiff, "picking up the scent", func(ctx context.Context) func(App) App {
		verdict, err := resolver(ctx, run)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteDiff, "diff unavailable: "+err.Error())
				return app
			}
			app.diff = diffscreen.NewModel(verdict)
			return app
		}
	})
	a.PushRoute(RouteDiff)
	return a
}

func diffScreenHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "enter", "o", "esc":
		return true
	default:
		return false
	}
}

// openWorkflows fetches the pack roster through startLoad — never
// on the keypress path (Task 220 invariant) — and routes immediately
// so the shared loading body holds the pane until the list lands.
func (a App) openWorkflows() App {
	if a.loadBlocked(loadKindWorkflows) {
		return a
	}
	a.clearRouteError(RouteWorkflows)
	a.workflows = workflowsscreen.Model{Repo: a.runs.Context.Repo}
	if a.workflowsResolver == nil {
		a.setRouteError(RouteWorkflows, "workflows unavailable: live workflow loader is not configured")
		a.PushRoute(RouteWorkflows)
		return a
	}
	resolver := a.workflowsResolver
	repo := a.runs.Context.Repo
	a = a.startLoad(loadKindWorkflows, "counting the pack", func(ctx context.Context) func(App) App {
		workflows, err := resolver(ctx)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteWorkflows, "workflows unavailable: "+err.Error())
				return app
			}
			app.workflows = workflowsscreen.NewModel(repo, workflows)
			return app
		}
	})
	a.PushRoute(RouteWorkflows)
	return a
}

func (a App) updateWorkflows(msg KeyMsg) (App, bool) {
	before := a.workflows
	a.workflows = a.workflows.Update(workflowsscreen.KeyMsg{Key: msg.Key})
	switch a.workflows.Intent.Kind {
	case workflowsscreen.IntentToggle:
		workflow := a.workflows.Intent.Workflow
		action := usecase.ActionDisableWorkflow
		if workflow.State != model.WorkflowStateActive {
			action = usecase.ActionEnableWorkflow
		}
		a = a.handleAction(RouteWorkflows, ActionRequest{
			Action: action,
			Workflow: dispatch.Workflow{
				Name:  workflowDisplayLabel(workflow),
				ID:    workflowToggleIdentifier(workflow),
				State: workflow.State,
			},
		})
	case workflowsscreen.IntentBrowser:
		a = a.openExternal(a.workflows.Intent.Workflow.HTMLURL)
	case workflowsscreen.IntentBack:
		a.PopRoute()
	}
	return a, workflowsScreenHandled(msg.Key) || before.Selected != a.workflows.Selected || a.workflows.Intent.Kind != workflowsscreen.IntentNone
}

func workflowsScreenHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "g", "G", "e", "o", "esc":
		return true
	default:
		return false
	}
}

// workflowToggleIdentifier picks the selector the API accepts: the
// workflow file path, else the numeric id.
func workflowToggleIdentifier(workflow model.Workflow) string {
	if strings.TrimSpace(workflow.Path) != "" {
		return strings.TrimSpace(workflow.Path)
	}
	if workflow.ID != 0 {
		return fmt.Sprintf("%d", workflow.ID)
	}
	return ""
}

func workflowDisplayLabel(workflow model.Workflow) string {
	if strings.TrimSpace(workflow.Name) != "" {
		return strings.TrimSpace(workflow.Name)
	}
	return strings.TrimSpace(workflow.Path)
}

func (a App) updateRuns(msg KeyMsg) (App, bool) {
	before := a.runs
	a.runs = a.runs.Update(runs.KeyMsg{Key: msg.Key})
	switch a.runs.Intent.Kind {
	case runs.IntentOpenDetail:
		if a.loadBlocked(loadKindDetail) {
			break
		}
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.loadDetail(run)
		}
		a.PushRoute(RouteDetail)
	case runs.IntentOpenLogs:
		if a.loadBlocked(loadKindLog) {
			break
		}
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.loadLog(run, model.Job{})
		}
		a.PushRoute(RouteLog)
	case runs.IntentWatch:
		// w watches the selected run's whole event group: a single-run
		// group degrades to the classic single-run watch.
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.openPackWatch(run)
		}
	case runs.IntentDispatch:
		if a.loadBlocked(loadKindDispatch) {
			break
		}
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
	case runs.IntentApprovals:
		if a.loadBlocked(loadKindApprovals) {
			break
		}
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.openApprovals(run)
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
		if a.loadBlocked(loadKindFailure) {
			break
		}
		a = a.loadFailure(a.detail.Run, a.selectedDetailJob())
		a.PushRoute(RouteFailure)
	case detail.IntentLog:
		if a.loadBlocked(loadKindLog) {
			break
		}
		a = a.loadLog(a.detail.Run, a.selectedDetailJob())
		a.PushRoute(RouteLog)
	case detail.IntentWatch:
		if a.loadBlocked(loadKindWatch) {
			break
		}
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

func (a App) updateCaches(msg KeyMsg) (App, bool) {
	before := a.caches
	a.caches = a.caches.Update(caches.KeyMsg{Key: msg.Key})
	switch a.caches.Intent.Kind {
	case caches.IntentDelete:
		a = a.openCacheDeleteConfirm(CacheDeleteRequest{ID: a.caches.Intent.CacheID}, a.caches.Intent.Key, 1)
	case caches.IntentDeleteKey:
		key := a.caches.Intent.Key
		a = a.openCacheDeleteConfirm(CacheDeleteRequest{Key: key}, key, a.caches.MatchCount(key))
	case caches.IntentBack:
		a.PopRoute()
	}
	changed := before.Selected != a.caches.Selected ||
		before.Filter != a.caches.Filter ||
		before.InputMode != a.caches.InputMode ||
		before.SortBy != a.caches.SortBy ||
		a.caches.Intent.Kind != caches.IntentNone
	return a, cachesHandled(msg.Key) || changed
}

// openCaches starts the kennel fetch off the keypress path (Task 220
// invariant) and routes to the screen; the shared loading body covers
// the gap.
func (a App) openCaches() App {
	a.clearRouteError(RouteCaches)
	if a.cachesResolver == nil {
		a.setRouteError(RouteCaches, "kennel unavailable: live GitHub cache loader is not configured")
		a.PushRoute(RouteCaches)
		return a
	}
	if a.loadBlocked(loadKindCaches) {
		return a
	}
	resolver := a.cachesResolver
	repo := a.runs.Context.Repo
	a = a.startLoad(loadKindCaches, "sniffing the kennel", func(ctx context.Context) func(App) App {
		data, err := resolver(ctx)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteCaches, "kennel unavailable: "+err.Error())
				return app
			}
			app.caches = caches.NewModel(repo, data)
			return app
		}
	})
	a.PushRoute(RouteCaches)
	return a
}

// openCacheDeleteConfirm gates every eviction behind the shared
// confirm overlay, match count first — a key can cover several
// caches and the user must see how many before anything is dug up.
func (a App) openCacheDeleteConfirm(request CacheDeleteRequest, key string, matches int) App {
	if matches <= 0 {
		return a
	}
	a.clearRouteError(RouteCaches)
	pending := request
	a.pendingCacheDelete = &pending
	a.confirm = confirm.New(cacheDeleteConfirmMessage(request, key, matches, a.caches))
	if a.TopOverlay() != OverlayConfirm {
		a.overlays = append(a.overlays, OverlayConfirm)
	}
	return a
}

func cacheDeleteConfirmMessage(request CacheDeleteRequest, key string, matches int, m caches.Model) string {
	label := cacheKeyLabel(key)
	if request.ID != 0 {
		size := ""
		for _, cache := range m.Caches {
			if cache.ID == request.ID {
				size = caches.HumanSize(cache.SizeInBytes)
			}
		}
		return fmt.Sprintf("dig up 1 cache — %q (%s)?", label, size)
	}
	noun := "caches"
	if matches == 1 {
		noun = "cache"
	}
	return fmt.Sprintf("dig up %d %s keyed %q (%s)?", matches, noun, label, caches.HumanSize(m.KeyBytes(key)))
}

// cacheKeyLabel keeps hash-suffixed cache keys readable in the
// confirm overlay.
func cacheKeyLabel(key string) string {
	runes := []rune(key)
	if len(runes) <= 48 {
		return key
	}
	return string(runes[:47]) + "…"
}

// startCacheDelete runs the eviction off the keypress path through
// the shared load slot; the result folds into the kennel locally so
// no extra listing call is spent.
func (a App) startCacheDelete(request CacheDeleteRequest) App {
	if a.cacheDeleter == nil {
		a.pushErrorToast("cache-delete-unavailable", usecase.ResilienceFor(errors.New("cache eviction is not configured"), usecase.ErrorContext{}))
		return a
	}
	deleter := a.cacheDeleter
	return a.startLoad(loadKindCaches, "digging it up", func(ctx context.Context) func(App) App {
		count, err := deleter(ctx, request)
		return func(app App) App {
			if err != nil {
				app.pushErrorToast("cache-delete-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
				return app
			}
			if request.ID != 0 {
				app.caches = app.caches.WithoutCache(request.ID)
			} else {
				app.caches = app.caches.WithoutKey(request.Key)
			}
			message := "dug that one up."
			if count > 1 {
				message = fmt.Sprintf("dug up %d caches.", count)
			}
			app.pushToast("cache-deleted", usecase.Resilience{
				Severity: usecase.SeverityOK,
				Title:    message,
				Message:  caches.UsageLine(app.caches.Usage, app.caches.Cap),
			})
			return app
		}
	})
}

func cachesHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "g", "G", "s", "/", "d", "D", "enter", "backspace", "esc":
		return true
	default:
		return len([]rune(key)) == 1
	}
}

// debugNose renders the rerun confirm's debug-logging state. Hound
// voice: the debug nose sniffs out runner diagnostics.
func debugNose(on bool) string {
	if on {
		return " · debug nose: on"
	}
	return " · debug nose: off"
}

// rerunFamily reports whether an action supports debug logging.
func rerunFamily(action usecase.Action) bool {
	switch action {
	case usecase.ActionRerunRun, usecase.ActionRerunFailedJobs, usecase.ActionRerunJob:
		return true
	default:
		return false
	}
}

func (a App) updateConfirm(msg KeyMsg) (App, bool) {
	if msg.Key == "d" {
		if pending := a.pendingAction; pending != nil && rerunFamily(pending.request.Action) {
			// Clone before mutating: App copies share the pointer, and
			// in-place edits would leak into historical values.
			toggled := *pending
			toggled.request.Debug = !toggled.request.Debug
			a.pendingAction = &toggled
			a.confirm = confirm.New(confirmMessage(toggled.request))
			return a, true
		}
	}
	a.confirm = a.confirm.Update(confirm.KeyMsg{Key: msg.Key})
	switch a.confirm.Intent.Kind {
	case confirm.IntentConfirm:
		pending := a.pendingAction
		pendingDownload := a.pendingDownload
		pendingCacheDelete := a.pendingCacheDelete
		a = a.closeConfirm()
		if pendingDownload != nil {
			return a.startArtifactDownload(*pendingDownload), true
		}
		if pendingCacheDelete != nil {
			return a.startCacheDelete(*pendingCacheDelete), true
		}
		if pending != nil {
			var accepted bool
			a, accepted = a.executeAction(pending.route, pending.request)
			if accepted && workflowToggleFamily(pending.request.Action) {
				// The landing state is derived, never re-fetched: the
				// toggle stays exactly one API call. The cached
				// dispatch roster flips too so the picker's badge and
				// refusal stay truthful without a refetch.
				enabled := pending.request.Action == usecase.ActionEnableWorkflow
				a.workflows = a.workflows.WithToggled(pending.request.Workflow.ID, enabled)
				state := model.WorkflowStateDisabledManually
				if enabled {
					state = model.WorkflowStateActive
				}
				for i := range a.dispatchWorkflows {
					if a.dispatchWorkflows[i].ID == pending.request.Workflow.ID {
						a.dispatchWorkflows[i].State = state
					}
				}
			}
			if accepted && deploymentReviewFamily(pending.request.Action) && a.TopOverlay() == OverlayApprovals {
				// The gate was acted on: the overlay beneath has
				// nothing left to offer. A refusal keeps it open with
				// the picks and comment intact for a retry.
				a.PopOverlay()
				a.approvals = approvals.Model{}
			}
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
	case "approvals":
		a.PopOverlay()
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.openApprovals(run)
		}
	case string(RouteWatch):
		a.PopOverlay()
		if run, ok := a.runs.SelectedRun(); ok {
			a = a.openPackWatch(run)
		}
	case string(RouteCaches):
		a.PopOverlay()
		a = a.openCaches()
	case string(RouteDispatch):
		a = a.openDispatchFromPalette(intent.Value)
	case string(RouteDiff):
		a.PopOverlay()
		a = a.openDiff()
	case string(RouteWorkflows):
		a.PopOverlay()
		a = a.openWorkflows()
	}
	return a
}

func (a App) openPalette() App {
	// Palette enrichment uses only already-cached workflows: the
	// invariant forbids a network fetch on the ':' keystroke. The
	// generic dispatch item stays available and openDispatch resolves
	// (async) when the user actually selects it.
	a.palette = palette.New(paletteItems(a.dispatchWorkflows))
	if a.TopOverlay() != OverlayPalette {
		a.overlays = append(a.overlays, OverlayPalette)
	}
	return a
}

func (a App) openDispatchFromPalette(value string) App {
	if strings.TrimSpace(value) != "" {
		if workflow, ok := a.dispatchWorkflowByValue(value); ok {
			if refused, refusal := dispatchOffDutyRefusal(workflow); refused {
				a.PopOverlay()
				a.pushToast("dispatch-workflow-offduty", refusal)
				return a
			}
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

// openApprovals fetches the selected waiting run's gate list through
// startLoad — never on the keypress path — and opens the overlay when
// it lands. Non-waiting runs get a truthful toast instead.
func (a App) openApprovals(run model.Run) App {
	if run.Status != model.StatusWaiting {
		a.pushToast("approvals-no-gate", usecase.Resilience{
			Severity: usecase.SeverityInfo,
			Title:    "no gate here",
			Message:  "this run is not waiting on a deploy review",
		})
		return a
	}
	if a.approvalsResolver == nil {
		a.pushErrorToast("approvals-unavailable", usecase.ResilienceFor(errors.New("deploy approvals are not configured"), usecase.ErrorContext{}))
		return a
	}
	resolver := a.approvalsResolver
	return a.startLoad(loadKindApprovals, "checking the gate", func(ctx context.Context) func(App) App {
		pending, err := resolver(ctx, run)
		return func(app App) App {
			if err != nil {
				app.pushErrorToast("approvals-fetch-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
				return app
			}
			if len(pending) == 0 {
				app.pushToast("approvals-empty", usecase.Resilience{
					Severity: usecase.SeverityInfo,
					Title:    "no gate here",
					Message:  "nothing is pending review on this run",
				})
				return app
			}
			app.approvals = approvals.NewModel(run, pending)
			if app.TopOverlay() != OverlayApprovals {
				app.overlays = append(app.overlays, OverlayApprovals)
			}
			return app
		}
	})
}

func (a App) updateApprovals(msg KeyMsg) (App, bool) {
	if !a.approvals.CommentMode {
		switch msg.Key {
		case "esc":
			a.PopOverlay()
			a.approvals = approvals.Model{}
			return a, true
		case "?":
			a.overlays = append(a.overlays, OverlayHelp)
			return a, true
		case "q", "ctrl+c":
			a.quit = true
			return a, true
		}
	}
	before := a.approvals
	a.approvals = a.approvals.Update(approvals.KeyMsg{Key: msg.Key})
	switch a.approvals.Intent.Kind {
	case approvals.IntentApprove:
		a = a.handleAction(RouteRuns, ActionRequest{
			Action:       usecase.ActionApproveDeployment,
			Run:          a.approvals.Run,
			Environments: a.approvals.Intent.Environments,
			Comment:      a.approvals.Intent.Comment,
		})
	case approvals.IntentReject:
		a = a.handleAction(RouteRuns, ActionRequest{
			Action:       usecase.ActionRejectDeployment,
			Run:          a.approvals.Run,
			Environments: a.approvals.Intent.Environments,
			Comment:      a.approvals.Intent.Comment,
		})
	}
	changed := before.Selected != a.approvals.Selected ||
		before.CommentMode != a.approvals.CommentMode ||
		before.Comment != a.approvals.Comment ||
		before.Notice != a.approvals.Notice ||
		len(before.PickedEnvironments()) != len(a.approvals.PickedEnvironments()) ||
		a.approvals.Intent.Kind != approvals.IntentNone
	return a, changed || len([]rune(msg.Key)) == 1
}

// workflowToggleFamily reports whether an action is a workflow
// enable/disable; confirming one flips the pack badge locally.
func workflowToggleFamily(action usecase.Action) bool {
	switch action {
	case usecase.ActionEnableWorkflow, usecase.ActionDisableWorkflow:
		return true
	default:
		return false
	}
}

// deploymentReviewFamily reports whether an action is a deploy-gate
// review; confirming one also closes the approvals overlay beneath.
func deploymentReviewFamily(action usecase.Action) bool {
	switch action {
	case usecase.ActionApproveDeployment, usecase.ActionRejectDeployment:
		return true
	default:
		return false
	}
}

func (a App) loadDetail(run model.Run) App {
	a.clearRouteError(RouteDetail)
	// The skeleton paints immediately from cached run data; jobs fold
	// in when the resolver returns. The repo breadcrumb must stay
	// truthful even while loading.
	a.detail = DetailModelForRun(run).WithRepo(a.runs.Context.Repo)
	if a.detailResolver == nil {
		return a.startArtifactsFetch(run)
	}
	resolver := a.detailResolver
	a = a.startLoad(loadKindDetail, "fetching jobs", func(ctx context.Context) func(App) App {
		resolved, err := resolver(ctx, run)
		return func(app App) App {
			if app.detail.Run.ID != run.ID {
				// The user opened a different run meanwhile; this
				// result no longer has a home.
				return app
			}
			if err != nil {
				app.setRouteError(RouteDetail, "detail unavailable: "+err.Error())
				return app
			}
			// Artifacts race jobs: the async artifacts fetch may have
			// landed on the skeleton already — carry it over.
			if len(app.detail.Artifacts) > 0 && len(resolved.Artifacts) == 0 {
				resolved = resolved.WithArtifacts(app.detail.Artifacts)
			}
			app.detail = resolved
			return app
		}
	})
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
	resolver := a.runsResolver
	// The result belongs to the scope that REQUESTED it: a local scope
	// toggle mid-flight must not receive another scope's rows.
	requestScope := a.runs.Context.Scope
	return a.startLoad(loadKindRuns, runsLoadLabel(query), func(ctx context.Context) func(App) App {
		resolved, err := resolver(ctx, filter)
		return func(app App) App {
			if err != nil {
				return app.handleRunsError(RouteRuns, "runs-filter", "runs filter failed: "+err.Error(), err)
			}
			switch requestScope {
			case usecase.LaunchScopeBranch:
				app.runs.Context.BranchRuns = resolved
			case usecase.LaunchScopeRepo:
				app.runs.Context.RepoRuns = resolved
			}
			if app.runs.Context.Scope != requestScope {
				// The user moved to another scope while this was in
				// flight: park the result in its cache slot only.
				return app
			}
			app.runs.Context.Runs = resolved
			app.runs.Context.Page = 1
			app.runs.Context.PerPage = perPageFor(app.runs.Context, app.config.PerPage)
			app.runs.Context.HasMore = len(resolved) >= app.runs.Context.PerPage
			app.runs.Selected = 0
			return app
		}
	})
}

// runsLoadLabel keeps the loading line hound-voiced per query.
func runsLoadLabel(query string) string {
	switch strings.TrimSpace(query) {
	case "":
		return "fetching the pack"
	case "failing", "failed", "red":
		return "sniffing out failing runs"
	case "running", "live":
		return "sniffing out running runs"
	case "passed", "passing", "green":
		return "sniffing out passing runs"
	default:
		return "sniffing out /" + strings.TrimSpace(query)
	}
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
	resolver := a.runsResolver
	requestScope := a.runs.Context.Scope
	return a.startLoad(loadKindRuns, "fetching more runs", func(ctx context.Context) func(App) App {
		resolved, err := resolver(ctx, filter)
		return func(app App) App {
			if err != nil {
				return app.handleRunsError(RouteRuns, "runs-page", "next page failed: "+err.Error(), err)
			}
			if app.runs.Context.Scope != requestScope {
				// The page belongs to a scope the user already left;
				// appending it would corrupt the active listing. Drop
				// it — G in the new scope fetches its own pages.
				return app
			}
			app.runs.Context.Page = nextPage
			app.runs.Context.PerPage = perPage
			_ = app.appendActiveRuns(resolved)
			// A full page that deduped to nothing still means deeper pages
			// exist (high-velocity repos shift runs between pages); latching
			// HasMore=false there froze pagination on openclaw-sized repos.
			app.runs.Context.HasMore = len(resolved) >= perPage
			return app
		}
	})
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
	resolved, err := a.runsResolver(context.Background(), filter)
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
	resolved, err := a.watchResolver(context.Background(), a.watch.State.Run)
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

// openPackWatch is the w entry point: watch the selected run's whole
// event group as one board. A single-run group degrades to the
// classic single-run watch — zero regression. The board paints
// immediately from the rows already on screen; a pack tick starts off
// the keypress path (Task 220 invariant) so out-of-band state changes
// fold in right away.
func (a App) openPackWatch(run model.Run) App {
	pack := usecase.PackForRun(a.runs.Context.Runs, run, a.config.WatchGroupMax)
	if len(pack) <= 1 {
		if a.loadBlocked(loadKindWatch) {
			return a
		}
		a = a.loadWatch(run)
		a.PushRoute(RouteWatch)
		return a
	}
	if a.loadBlocked(loadKindPack) {
		return a
	}
	a.clearRouteError(RouteWatchBoard)
	a.board = watch.NewBoard(a.runs.Context.Repo, a.runs.Context.Branch, run, pack)
	a.PushRoute(RouteWatchBoard)
	if a.packResolver == nil {
		return a
	}
	resolver := a.packResolver
	state := a.packState()
	return a.startLoad(loadKindPack, "rounding up the hunt", func(ctx context.Context) func(App) App {
		next, err := resolver(ctx, state)
		return func(app App) App {
			if err != nil {
				app.pushErrorToast("pack-fetch-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
				return app
			}
			app.board = app.board.WithRuns(next.Runs)
			return app
		}
	})
}

func (a App) packState() usecase.PackState {
	return usecase.PackState{
		Repo:    a.board.Repo,
		HeadSHA: a.board.HeadSHA,
		Event:   a.board.Event,
		Branch:  a.board.Branch,
		Max:     a.config.WatchGroupMax,
		Runs:    a.board.Runs,
	}
}

func (a App) updateWatchBoard(msg KeyMsg) (App, bool) {
	before := a.board
	a.board = a.board.Update(watch.KeyMsg{Key: msg.Key})
	switch a.board.Intent.Kind {
	case watch.BoardIntentDrill:
		if a.loadBlocked(loadKindWatch) {
			break
		}
		if run, ok := a.board.SelectedRun(); ok {
			a = a.loadWatch(run)
			a.PushRoute(RouteWatch)
		}
	case watch.BoardIntentCancel:
		if run, ok := a.board.SelectedRun(); ok {
			a = a.handleAction(RouteWatchBoard, ActionRequest{Action: usecase.ActionCancelRun, Run: run})
		}
	case watch.BoardIntentBack:
		a.PopRoute()
	}
	return a, watchBoardHandled(msg.Key) || before.Selected != a.board.Selected || before.Follow != a.board.Follow || a.board.Intent.Kind != watch.BoardIntentNone
}

func watchBoardHandled(key string) bool {
	switch key {
	case "j", "k", "down", "up", "enter", "x", "f", "esc":
		return true
	default:
		return false
	}
}

// refreshPack polls the board on the shared tick: one runs-list call
// covers every member (PackWatchService budget). Settling pushes the
// hound's verdict toast exactly once, at the transition.
func (a App) refreshPack() (App, bool) {
	if a.packResolver == nil || len(a.board.Runs) == 0 {
		return a, false
	}
	before := a.board.Summary()
	next, err := a.packResolver(context.Background(), a.packState())
	if err != nil {
		a.setRouteError(RouteWatchBoard, "pack watch refresh failed: "+err.Error())
		a.refreshCount++
		return a, true
	}
	a.clearRouteError(RouteWatchBoard)
	a.refreshCount++
	a.board = a.board.WithRuns(next.Runs)
	after := a.board.Summary()
	if before.Running > 0 && after.Settled() {
		a.pushPackSettledToast(after)
	}
	a.pollInterval = nextPollIntervalForRuns(next.Runs, a.pollInterval, a.config)
	return a, true
}

// pushPackSettledToast announces settlement in the hound voice. The
// body carries the counts so the title never echoes into it.
func (a *App) pushPackSettledToast(summary usecase.PackSummary) {
	if summary.Lost == 0 {
		a.pushToast("pack-settled", usecase.Resilience{
			Severity: usecase.SeverityOK,
			Title:    "pack's home.",
			Message:  summary.String(),
		})
		return
	}
	noun := "runs"
	if summary.Lost == 1 {
		noun = "run"
	}
	a.pushToast("pack-settled", usecase.Resilience{
		Severity: usecase.SeverityWarn,
		Title:    fmt.Sprintf("the hunt's home — %d %s lost.", summary.Lost, noun),
		Message:  summary.String(),
	})
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
	resolver := a.failureResolver
	return a.startLoad(loadKindFailure, "fetching the failure", func(ctx context.Context) func(App) App {
		resolved, fullLog, err := resolver(ctx, run, job)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteFailure, "failure unavailable: "+err.Error())
				return app
			}
			app.failure = resolved
			app.log = fullLog
			app.clearRouteError(RouteLog)
			app.pushLogRefetchToast(job.ID)
			return app
		}
	})
}

func (a App) loadLog(run model.Run, job model.Job) App {
	a.clearRouteError(RouteLog)
	if a.logResolver == nil {
		a.setRouteError(RouteLog, "log unavailable: live log loader is not configured")
		return a
	}
	resolver := a.logResolver
	return a.startLoadProgress(loadKindLog, "fetching log", func(ctx context.Context, progress func(read, total int64)) func(App) App {
		resolved, err := resolver(ctx, run, job, progress)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteLog, "log unavailable: "+err.Error())
				return app
			}
			app.log = resolved
			app.pushLogRefetchToast(job.ID)
			return app
		}
	})
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
	resolver := a.watchResolver
	return a.startLoad(loadKindWatch, "chasing down the run", func(ctx context.Context) func(App) App {
		resolved, err := resolver(ctx, run)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteWatch, "watch unavailable: "+err.Error())
				return app
			}
			app.watch = resolved
			return app
		}
	})
}

func (a App) openDispatch() App {
	// Cached choices decide instantly; only the fetch goes async. The
	// route is pushed when the decision is known so esc-from-palette
	// navigation matches the old synchronous behavior exactly.
	if len(a.dispatchWorkflows) > 0 {
		// States can change out from under the cache (a toggle in
		// another terminal): refresh them with ONE list call so the
		// picker badges and refusals stay truthful — the expensive
		// per-file dispatchability probes stay cached.
		if a.workflowsResolver != nil {
			resolver := a.workflowsResolver
			cached := append([]dispatch.Workflow(nil), a.dispatchWorkflows...)
			return a.startLoad(loadKindDispatch, "fetching workflows", func(ctx context.Context) func(App) App {
				roster, err := resolver(ctx)
				return func(app App) App {
					if err == nil {
						// Roster entries identify by file path OR
						// numeric ID (workflowToggleIdentifier
						// convention) — index fresh states under both.
						states := make(map[string]string, len(roster)*2)
						for _, workflow := range roster {
							states[strconv.FormatInt(workflow.ID, 10)] = workflow.State
							if path := strings.TrimSpace(workflow.Path); path != "" {
								states[path] = workflow.State
							}
						}
						for i := range cached {
							if state, ok := states[cached[i].ID]; ok {
								cached[i].State = state
							}
						}
					}
					app.dispatchWorkflows = cached
					return app.applyDispatchChoices(cached)
				}
			})
		}
		return a.applyDispatchChoices(a.dispatchWorkflows)
	}
	if a.dispatchWorkflowsResolver == nil {
		a = a.loadDispatch()
		a.PushRoute(RouteDispatch)
		return a
	}
	resolver := a.dispatchWorkflowsResolver
	return a.startLoad(loadKindDispatch, "fetching workflows", func(ctx context.Context) func(App) App {
		workflows, err := resolver(ctx)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteDispatch, "dispatch unavailable: "+err.Error())
				app.PushRoute(RouteDispatch)
				return app
			}
			app.dispatchWorkflows = append([]dispatch.Workflow(nil), workflows...)
			return app.applyDispatchChoices(workflows)
		}
	})
}

// applyDispatchChoices routes per the resolved workflow count: none →
// the single-workflow resolver path, one → straight to the form, many
// → the chooser palette over the current route.
func (a App) applyDispatchChoices(workflows []dispatch.Workflow) App {
	switch len(workflows) {
	case 0:
		// The workflows fetch already came back empty — go straight to
		// the single-form resolver instead of re-fetching the list.
		a = a.loadDispatchFallback()
		a.PushRoute(RouteDispatch)
	case 1:
		if refused, refusal := dispatchOffDutyRefusal(workflows[0]); refused {
			a.pushToast("dispatch-workflow-offduty", refusal)
			return a
		}
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
	// Already-fetched workflows open the form instantly; only the
	// network path goes async.
	if len(a.dispatchWorkflows) > 0 {
		a.dispatch = dispatch.NewModel(a.dispatchWorkflows[0])
		return a
	}
	workflowsResolver := a.dispatchWorkflowsResolver
	dispatchResolver := a.dispatchResolver
	if workflowsResolver == nil && dispatchResolver == nil {
		a.setRouteError(RouteDispatch, "dispatch unavailable: live workflow loader is not configured")
		return a
	}
	return a.startLoad(loadKindDispatch, "fetching workflows", func(ctx context.Context) func(App) App {
		if workflowsResolver != nil {
			if workflows, err := workflowsResolver(ctx); err == nil && len(workflows) > 0 {
				return func(app App) App {
					app.dispatchWorkflows = append([]dispatch.Workflow(nil), workflows...)
					app.dispatch = dispatch.NewModel(workflows[0])
					return app
				}
			}
		}
		return dispatchFallbackApply(ctx, dispatchResolver)
	})
}

// loadDispatchFallback fetches the single dispatch form without
// retrying the workflows list (callers know it already came back
// empty).
func (a App) loadDispatchFallback() App {
	a.clearRouteError(RouteDispatch)
	dispatchResolver := a.dispatchResolver
	if dispatchResolver == nil {
		a.setRouteError(RouteDispatch, "dispatch unavailable: live workflow loader is not configured")
		return a
	}
	return a.startLoad(loadKindDispatch, "fetching workflows", func(ctx context.Context) func(App) App {
		return dispatchFallbackApply(ctx, dispatchResolver)
	})
}

func dispatchFallbackApply(ctx context.Context, dispatchResolver func(context.Context) (dispatch.Model, error)) func(App) App {
	if dispatchResolver == nil {
		return func(app App) App {
			app.setRouteError(RouteDispatch, "dispatch unavailable: live workflow loader is not configured")
			return app
		}
	}
	resolved, err := dispatchResolver(ctx)
	return func(app App) App {
		if err != nil {
			app.setRouteError(RouteDispatch, "dispatch unavailable: "+err.Error())
			return app
		}
		app.dispatch = resolved
		return app
	}
}

func (a *App) dispatchWorkflowByValue(value string) (dispatch.Workflow, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return dispatch.Workflow{}, false
	}
	// Workflow-specific palette items only exist once the cache is
	// populated, so the cached list is authoritative here.
	for _, workflow := range a.dispatchWorkflows {
		if workflowValue(workflow) == value {
			return workflow, true
		}
	}
	return dispatch.Workflow{}, false
}

// dispatchOffDutyRefusal refuses dispatching a non-active workflow: a
// doomed 422 is not a dispatch. The toast points at the pack roster.
func dispatchOffDutyRefusal(workflow dispatch.Workflow) (bool, usecase.Resilience) {
	if workflow.State == "" || workflow.State == model.WorkflowStateActive {
		return false, usecase.Resilience{}
	}
	name := strings.TrimSpace(workflow.Name)
	if name == "" {
		name = workflow.ID
	}
	return true, usecase.Resilience{
		Severity: usecase.SeverityWarn,
		Title:    name + " is " + workflowsscreen.StateLabel(workflow.State),
		Message:  "wake it in :workflows before dispatching",
	}
}

func (a App) handleAction(route Route, request ActionRequest) App {
	if actionRequiresConfirmation(request.Action) {
		return a.openConfirm(route, request)
	}
	a, _ = a.executeAction(route, request)
	return a
}

// executeAction runs the mutation and reports whether it was accepted,
// so callers can keep state (like the approvals overlay) alive on a
// refusal and let the user retry.
func (a App) executeAction(route Route, request ActionRequest) (App, bool) {
	a.clearRouteError(route)
	if a.actionHandler == nil {
		a.setRouteError(route, "action unavailable: live GitHub mutation handler is not configured")
		return a, false
	}
	// Captured BEFORE the mutation: the dispatched run's created_at is
	// stamped by GitHub the moment the POST lands, so the discovery
	// fence must predate it.
	started := time.Now()
	result, err := a.actionHandler(request)
	if err != nil {
		a.pushErrorToast("action-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
		return a, false
	}
	resilience := usecase.ResilienceForSuccess(result)
	if resilience.Message == "" && request.Workflow.Name != "" {
		// Run-less actions target a workflow: say WHICH one went back
		// on duty (QA round 13).
		resilience.Message = "workflow " + request.Workflow.Name
	}
	a.pushToast("action-ok", resilience)
	a = a.attachHandoffWatch(request, result, started)
	return a, true
}

// attachHandoffWatch drops straight into watch after a dispatch or a
// run-level rerun, honoring config auto_watch (off by default — the
// toast alone is today's behavior).
func (a App) attachHandoffWatch(request ActionRequest, result usecase.ActionResult, started time.Time) App {
	if !a.config.AutoWatch || a.watchResolver == nil {
		return a
	}
	switch request.Action {
	case usecase.ActionDispatch:
		return a.attachDispatchWatch(request, result, started)
	case usecase.ActionRerunRun, usecase.ActionRerunFailedJobs:
		return a.attachRerunWatch(request.Run)
	default:
		return a
	}
}

// handoffClockSkew widens the discovery fence: created_at comes from
// GitHub's clock, the fence from ours.
const handoffClockSkew = 5 * time.Second

// attachDispatchWatch attaches the watch to the dispatched run. The
// 200 body carries the run id directly (API v2026-03-10); ONLY a 204
// host (no id) earns the bounded discovery poll.
func (a App) attachDispatchWatch(request ActionRequest, result usecase.ActionResult, started time.Time) App {
	if a.loadBlocked(loadKindWatch) {
		return a
	}
	if result.WorkflowRunID != 0 {
		a = a.loadWatch(model.Run{ID: result.WorkflowRunID, Name: request.Workflow.Name, HTMLURL: result.HTMLURL})
		return a.routeToWatchFromDispatch()
	}
	if a.dispatchAttachResolver == nil {
		return a
	}
	resolver := a.dispatchAttachResolver
	watchResolver := a.watchResolver
	workflowID := request.Workflow.ID
	ref := request.Dispatch.Ref
	since := started.Add(-handoffClockSkew)
	a = a.startLoad(loadKindWatch, "picking up the scent", func(ctx context.Context) func(App) App {
		run, err := resolver(ctx, workflowID, ref, since)
		if err != nil {
			return func(app App) App { return app.dropDispatchScent(err) }
		}
		resolved, err := watchResolver(ctx, run)
		return func(app App) App {
			if err != nil {
				return app.dropDispatchScent(err)
			}
			app.watch = resolved
			return app
		}
	})
	return a.routeToWatchFromDispatch()
}

func (a App) routeToWatchFromDispatch() App {
	if a.Route() == RouteDispatch {
		a.PopRoute()
	}
	if a.Route() != RouteWatch {
		a.PushRoute(RouteWatch)
	}
	return a
}

// dropDispatchScent returns the user to the runs list gracefully when
// the dispatched run never surfaced (or the first tick failed).
func (a App) dropDispatchScent(err error) App {
	if a.Route() == RouteWatch {
		a.PopRoute()
	}
	if errors.Is(err, usecase.ErrScentLost) {
		a.pushToast("dispatch-scent-lost", usecase.Resilience{
			Severity: usecase.SeverityWarn,
			Title:    "couldn't pick up the scent.",
			Message:  "the dispatched run hasn't surfaced yet — it'll show in the runs list when it lands",
		})
		return a
	}
	a.pushErrorToast("dispatch-watch-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
	return a
}

// attachRerunWatch reattaches the watch to the rerun's EXISTING run
// id — no discovery — waiting (bounded) for the attempt counter to
// advance so the board never shows the stale completed attempt.
func (a App) attachRerunWatch(run model.Run) App {
	if run.ID == 0 || a.loadBlocked(loadKindWatch) {
		return a
	}
	attach := a.rerunAttachResolver
	watchResolver := a.watchResolver
	a = a.startLoad(loadKindWatch, "back on the trail", func(ctx context.Context) func(App) App {
		fresh := run
		if attach != nil {
			updated, err := attach(ctx, run)
			if err != nil {
				return func(app App) App {
					app.setRouteError(RouteWatch, "watch unavailable: "+err.Error())
					return app
				}
			}
			fresh = updated
		}
		resolved, err := watchResolver(ctx, fresh)
		return func(app App) App {
			if err != nil {
				app.setRouteError(RouteWatch, "watch unavailable: "+err.Error())
				return app
			}
			app.watch = resolved
			return app
		}
	})
	if a.Route() != RouteWatch {
		a.PushRoute(RouteWatch)
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
	a.pendingCacheDelete = nil
	a.confirm = confirm.Model{}
	return a
}

func actionRequiresConfirmation(action usecase.Action) bool {
	switch action {
	case usecase.ActionRerunRun,
		usecase.ActionRerunFailedJobs,
		usecase.ActionRerunJob,
		usecase.ActionCancelRun,
		usecase.ActionForceCancelRun,
		usecase.ActionApproveDeployment,
		usecase.ActionRejectDeployment,
		usecase.ActionEnableWorkflow,
		usecase.ActionDisableWorkflow:
		return true
	default:
		return false
	}
}

func confirmMessage(request ActionRequest) string {
	switch request.Action {
	case usecase.ActionRerunRun:
		return "rerun " + runTarget(request.Run) + debugNose(request.Debug)
	case usecase.ActionRerunFailedJobs:
		return "rerun failed jobs for " + runTarget(request.Run) + debugNose(request.Debug)
	case usecase.ActionRerunJob:
		return "rerun " + jobTarget(request.Job) + debugNose(request.Debug)
	case usecase.ActionCancelRun:
		return "cancel " + runTarget(request.Run)
	case usecase.ActionForceCancelRun:
		return "force-cancel " + runTarget(request.Run)
	case usecase.ActionApproveDeployment:
		return "open the gate for " + environmentTarget(request.Environments) + "?"
	case usecase.ActionRejectDeployment:
		return "keep the gate shut for " + environmentTarget(request.Environments) + "?"
	case usecase.ActionEnableWorkflow:
		return "wake " + workflowConfirmTarget(request.Workflow) + "? it goes back on duty"
	case usecase.ActionDisableWorkflow:
		return "muzzle " + workflowConfirmTarget(request.Workflow) + "? no runs until it is woken"
	default:
		return string(request.Action)
	}
}

// workflowConfirmTarget names exactly one workflow — toggles are
// always singular, so the prompt is too.
func workflowConfirmTarget(workflow dispatch.Workflow) string {
	name := strings.TrimSpace(workflow.Name)
	if name == "" {
		name = strings.TrimSpace(workflow.ID)
	}
	if name == "" {
		return "the selected workflow"
	}
	return "workflow " + name
}

func environmentTarget(environments []string) string {
	if len(environments) == 0 {
		return "this deployment"
	}
	return strings.Join(environments, ", ")
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
	case RouteWatchBoard:
		return keys.ScreenWatchBoard
	case RouteDispatch:
		return keys.ScreenDispatch
	case RouteDiff:
		return keys.ScreenDiff
	case RouteCaches:
		return keys.ScreenCaches
	case RouteWorkflows:
		return keys.ScreenWorkflows
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
	case OverlayApprovals:
		return "deploy gate"
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
	case OverlayApprovals:
		return approvals.View(a.approvals, width-20)
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
		if load := a.load; load != nil && load.kind == loadKindFailure {
			// Stale counts from prior content mislead during a fetch.
			return "hound", "failed step", "fetching…"
		}
		return "hound", "failed step", failureRight(a.failure)
	case RouteLog:
		if load := a.load; load != nil && load.kind == loadKindLog {
			return "hound", "full log", "fetching…"
		}
		return "hound", "full log", logRight(a.log)
	case RouteWatch:
		if load := a.load; load != nil && load.kind == loadKindWatch {
			return "hound", "watch", "chasing…"
		}
		run := a.watch.State.Run
		return "hound", runChromeTitle(run), "streaming · follow ●"
	case RouteWatchBoard:
		return "hound", packContext(a.board), a.board.Summary().String()
	case RouteDispatch:
		return "hound", "workflow_dispatch", firstNonEmpty(a.dispatch.Workflow.Name, a.dispatch.Workflow.ID)
	case RouteDiff:
		if load := a.load; load != nil && load.kind == loadKindDiff {
			return "hound", "the trail", "sniffing…"
		}
		return "hound", diffContext(a.diff.Verdict), diffRight(a.diff.Verdict)
	case RouteCaches:
		if load := a.load; load != nil && load.kind == loadKindCaches {
			return "hound", "the kennel · caches", "fetching…"
		}
		return "hound", "the kennel · caches", caches.UsageLine(a.caches.Usage, a.caches.Cap)
	case RouteWorkflows:
		if load := a.load; load != nil && load.kind == loadKindWorkflows {
			return "hound", "the pack · workflows", "counting…"
		}
		return "hound", "the pack · workflows", workflowsRight(a.workflows)
	default:
		if a.runs.AllGreen() {
			return "hound", branchContext(a.runs.Context.Scope, a.runs.Context.Branch, a.runs.Context.Actor), runsRight(a.runs, a.refreshCount, a.runsMeta)
		}
		return "hound", branchContext(a.runs.Context.Scope, a.runs.Context.Branch, a.runs.Context.Actor), runsRight(a.runs, a.refreshCount, a.runsMeta)
	}
}

// packContext names the board's scent in the chrome: the shared sha
// and event the pack is running on.
func packContext(b watch.Board) string {
	context := "the hunt"
	if strings.TrimSpace(b.HeadSHA) != "" {
		context += " " + icons.Breadcrumb + " " + shortSHA(b.HeadSHA)
		if strings.TrimSpace(b.Event) != "" {
			context += " " + strings.TrimSpace(b.Event)
		}
	}
	return context
}

func diffContext(verdict usecase.RegressionVerdict) string {
	if strings.TrimSpace(verdict.Workflow) == "" {
		return "the trail"
	}
	context := "the trail " + icons.Breadcrumb + " " + strings.TrimSpace(verdict.Workflow)
	if strings.TrimSpace(verdict.Branch) != "" {
		context += " · " + strings.TrimSpace(verdict.Branch)
	}
	return context
}

func diffRight(verdict usecase.RegressionVerdict) string {
	switch verdict.Status {
	case usecase.RegressionLocated:
		return fmt.Sprintf("#%d %s → #%d %s", verdict.LastGood.RunNumber, icons.Success, verdict.FirstBad.RunNumber, icons.Failure)
	case usecase.RegressionGreen:
		return "all clean"
	case usecase.RegressionInconclusive:
		return "trail cold"
	default:
		return ""
	}
}

func workflowsRight(m workflowsscreen.Model) string {
	total := len(m.Workflows)
	if total == 0 {
		return ""
	}
	noun := "workflows"
	if total == 1 {
		noun = "workflow"
	}
	offDuty := 0
	for _, workflow := range m.Workflows {
		if workflow.State != model.WorkflowStateActive {
			offDuty++
		}
	}
	if offDuty == 0 {
		return fmt.Sprintf("%d %s · all on duty", total, noun)
	}
	return fmt.Sprintf("%d %s · %d off duty", total, noun, offDuty)
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
		{Name: "watch", Description: "the hunt · watch the run's event group", Route: string(RouteWatch)},
		{Name: "artifacts", Description: "selected run's artifacts", Route: "artifacts"},
		{Name: "approvals", Description: "review the deploy gate", Route: "approvals"},
		{Name: "diff", Description: "who broke main? · the trail", Route: string(RouteDiff)},
		{Name: "caches", Description: "the kennel · cache usage & eviction", Route: string(RouteCaches)},
		{Name: "workflows", Description: "the pack · states, wake & muzzle", Route: string(RouteWorkflows)},
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
		description := "workflow_dispatch · " + workflowValue(workflow)
		if workflow.State != "" && workflow.State != model.WorkflowStateActive {
			// Non-active workflows stay visible — the badge answers
			// "where did my workflow go" — but selection is refused.
			// The badge replaces the redundant trigger prefix so it
			// survives the 80-column overlay untruncated.
			description = workflowValue(workflow) + " · " + workflowsscreen.BadgeText(workflow.State)
		}
		items = append(items, palette.Item{
			Name:        "dispatch: " + name,
			Description: description,
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
		runsModel := a.runs
		if load := a.load; load != nil && (load.kind == loadKindRuns || load.kind == loadKindDispatch) {
			runsModel.Loading = true
			runsModel.LoadingLine = loadingLine(a.theme, load, bodyWidth, time.Now())
		}
		return runs.ViewSize(runsModel, bodyWidth, bodyHeight(height), time.Now())
	case RouteDetail:
		detailModel := a.detail
		if load := a.load; load != nil && load.kind == loadKindDetail {
			detailModel.Loading = true
			detailModel.LoadingLine = loadingLine(a.theme, load, bodyWidth/2, time.Now())
		}
		return detail.ViewSize(detailModel, bodyWidth, bodyHeight(height))
	case RouteFailure:
		if load := a.load; load != nil && load.kind == loadKindFailure {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		return failure.View(a.failure, bodyWidth)
	case RouteLog:
		if load := a.load; load != nil && load.kind == loadKindLog {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		logModel := a.log
		if rows := bodyHeight(height) - 1; rows > 0 {
			logModel.Height = rows
		}
		return logscreen.View(logModel, bodyWidth)
	case RouteWatch:
		if load := a.load; load != nil && load.kind == loadKindWatch {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		return watch.View(a.watch, bodyWidth)
	case RouteWatchBoard:
		boardModel := a.board
		if load := a.load; load != nil && load.kind == loadKindPack {
			boardModel.Loading = true
			boardModel.LoadingLine = loadingLine(a.theme, load, bodyWidth, time.Now())
		}
		return watch.BoardViewSize(boardModel, bodyWidth, bodyHeight(height), time.Now())
	case RouteDispatch:
		if load := a.load; load != nil && load.kind == loadKindDispatch {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		return dispatch.View(a.dispatch, bodyWidth)
	case RouteDiff:
		if load := a.load; load != nil && load.kind == loadKindDiff {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		return diffscreen.ViewSize(a.diff, bodyWidth, bodyHeight(height))
	case RouteCaches:
		if load := a.load; load != nil && load.kind == loadKindCaches {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		return caches.ViewSize(a.caches, bodyWidth, bodyHeight(height), time.Now())
	case RouteWorkflows:
		if load := a.load; load != nil && load.kind == loadKindWorkflows {
			return loadingBody(a.theme, load, bodyWidth, time.Now())
		}
		return workflowsscreen.ViewSize(a.workflows, bodyWidth, bodyHeight(height))
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
		pendingWatch := a.load != nil && a.load.kind == loadKindWatch
		if a.watch.State.Run.ID == 0 && !pendingWatch {
			message = "watch unavailable: select a live run first"
		}
	case RouteDispatch:
		if a.dispatch.Workflow.ID == "" && a.dispatch.Workflow.Name == "" {
			message = "dispatch unavailable: no workflow has been loaded"
		}
	case RouteDiff:
		pendingDiff := a.load != nil && a.load.kind == loadKindDiff
		if a.diff.Verdict.Status == "" && !pendingDiff {
			message = "the trail is empty: select a run and jump with :diff"
		}
	case RouteWorkflows:
		pendingWorkflows := a.load != nil && a.load.kind == loadKindWorkflows
		if len(a.workflows.Workflows) == 0 && !pendingWorkflows {
			message = "the pack is empty: no workflows loaded"
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

// sampleWaitingRunsModel mirrors the fake adapter's waiting scenario:
// the newest run holds at a deployment gate.
func sampleWaitingRunsModel() runs.Model {
	now := time.Now().UTC().Truncate(time.Second)
	return runs.NewModel(usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Actor:  "indrasvat",
		Scope:  usecase.LaunchScopeBranch,
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{
			{ID: 30433655, Name: "Deploy", DisplayTitle: "production rollout", Event: "push", Status: model.StatusWaiting, Conclusion: model.ConclusionNone, RunNumber: 572, Actor: "indrasvat", HeadBranch: "main", HeadSHA: "a1b2c3d", UpdatedAt: now.Add(-90 * time.Second), RunStartedAt: now.Add(-2 * time.Minute)},
			{ID: 30433571, Name: "CI", DisplayTitle: "fix parser", Event: "pull_request", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 571, Actor: "indrasvat", HeadBranch: "main", HeadSHA: "b4c5d6e", UpdatedAt: now.Add(-12 * time.Minute), RunStartedAt: now.Add(-14 * time.Minute)},
			{ID: 30433570, Name: "Docs", DisplayTitle: "docs refresh", Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 570, Actor: "indrasvat", HeadBranch: "main", HeadSHA: "c7d8e9f", UpdatedAt: now.Add(-3 * time.Hour), RunStartedAt: now.Add(-181 * time.Minute)},
		},
	})
}

func samplePendingDeployments() []model.PendingDeployment {
	return []model.PendingDeployment{
		{
			EnvironmentID:         7301,
			EnvironmentName:       "production",
			WaitTimer:             0,
			CurrentUserCanApprove: true,
			Reviewers:             []model.DeploymentReviewer{{Type: "User", Name: "indrasvat"}},
		},
		{
			EnvironmentID:         7302,
			EnvironmentName:       "staging",
			WaitTimer:             1800,
			CurrentUserCanApprove: false,
			Reviewers:             []model.DeploymentReviewer{{Type: "Team", Name: "deploy-keys"}},
		},
	}
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

// sampleCachesModel is a calm kennel (~3.1 GiB of 10) for fixtures;
// last-used offsets are now-relative so ages render stably.
func sampleCachesModel() caches.Model {
	now := time.Now().UTC()
	rows := []model.Cache{
		{ID: 9001, Key: "setup-go-Linux-x64-ubuntu24-go-1.26.4-d93f4ea308b07f7c7339055a38006c84c478b6cb448d9d34672d1a6fb9324780", Ref: "refs/heads/main", SizeInBytes: 1610612736, LastAccessedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-9 * 24 * time.Hour)},
		{ID: 9002, Key: "go-build-Linux-x64-main", Ref: "refs/heads/main", SizeInBytes: 858993459, LastAccessedAt: now.Add(-26 * time.Hour), CreatedAt: now.Add(-8 * 24 * time.Hour)},
		{ID: 9003, Key: "go-mod-Linux-x64-1f2e3d", Ref: "refs/heads/main", SizeInBytes: 536870912, LastAccessedAt: now.Add(-4 * 24 * time.Hour), CreatedAt: now.Add(-7 * 24 * time.Hour)},
		{ID: 9004, Key: "go-mod-Linux-x64-stale99", Ref: "refs/pull/7/merge", SizeInBytes: 209715200, LastAccessedAt: now.Add(-12 * 24 * time.Hour), CreatedAt: now.Add(-13 * 24 * time.Hour)},
		{ID: 9005, Key: "node-modules-pages-build", Ref: "refs/heads/main", SizeInBytes: 104857600, LastAccessedAt: now.Add(-21 * 24 * time.Hour), CreatedAt: now.Add(-21 * 24 * time.Hour)},
	}
	var total int64
	for _, row := range rows {
		total += row.SizeInBytes
	}
	return caches.NewModel("indrasvat/gh-hound", caches.Data{
		Usage:  model.CacheUsage{ActiveSizeInBytes: total, ActiveCount: len(rows)},
		Caches: rows,
	})
}

// sampleCachesPressureModel sits past the 90% eviction threshold so
// the warning state is pinned in fixtures.
func sampleCachesPressureModel() caches.Model {
	m := sampleCachesModel()
	sizes := []int64{3758096384, 3221225472, 2147483648, 858993459, 429496730}
	var total int64
	for i := range m.Caches {
		m.Caches[i].SizeInBytes = sizes[i%len(sizes)]
		total += m.Caches[i].SizeInBytes
	}
	m.Usage = model.CacheUsage{ActiveSizeInBytes: total, ActiveCount: len(m.Caches)}
	return m
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

// sampleBoardModel is the pack board fixture: one running, one home,
// one lost, follow-worst engaged so the cursor pins the lost row.
// Elapsed offsets are now-relative so the clock column renders stably.
func sampleBoardModel() watch.Board {
	now := time.Now().UTC().Truncate(time.Second)
	sha := "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432"
	runs := []model.Run{
		{ID: 30433701, Name: "CI", DisplayTitle: "feat: tighten the leash", Event: "push", HeadSHA: sha, HeadBranch: "main", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, RunNumber: 601, RunStartedAt: now.Add(-4 * time.Minute), UpdatedAt: now.Add(-94 * time.Second)},
		{ID: 30433702, Name: "Release", DisplayTitle: "feat: tighten the leash", Event: "push", HeadSHA: sha, HeadBranch: "main", Status: model.StatusInProgress, Conclusion: model.ConclusionNone, RunNumber: 602, RunStartedAt: now.Add(-48 * time.Second), UpdatedAt: now},
		{ID: 30433703, Name: "Docs", DisplayTitle: "feat: tighten the leash", Event: "push", HeadSHA: sha, HeadBranch: "main", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunNumber: 603, RunStartedAt: now.Add(-3 * time.Minute), UpdatedAt: now.Add(-108 * time.Second)},
	}
	board := watch.NewBoard("indrasvat/gh-hound", "main", runs[0], runs)
	return board.Update(watch.KeyMsg{Key: "f"})
}

func sampleDiffVerdict() usecase.RegressionVerdict {
	return usecase.RegressionVerdict{
		Repo:     "indrasvat/gh-hound",
		Workflow: "CI",
		Branch:   "main",
		Status:   usecase.RegressionLocated,
		LastGood: model.Run{ID: 30433572, Name: "CI", RunNumber: 572, RunAttempt: 2, HeadSHA: "c2b3a49", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
		FirstBad: model.Run{ID: 30433573, Name: "CI", RunNumber: 573, RunAttempt: 1, HeadSHA: "d3c4b5a", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure},
		SuspectCommits: []model.Commit{
			{SHA: "d3c4b5a9f0e1d2c3b4a5968778695a4b3c2d1e0f", Author: "indrasvat", Message: "feat: sharpen the lexer"},
			{SHA: "cc99aa1b2c3d4e5f60718293a4b5c6d7e8f90a1b", Author: "dependabot[bot]", Message: "chore(deps): bump charmbracelet/x/ansi from 0.11.6 to 0.11.7 with a subject long enough to need the ellipsis"},
			{SHA: "ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12", Author: "web-flow", Message: "docs: refresh the controls table"},
		},
		TotalSuspects: 3,
		CompareURL:    "https://github.com/indrasvat/gh-hound/compare/c2b3a49...d3c4b5a",
		RunsScanned:   4,
		Verdict:       "scent picked up: #572 was clean, #573 wasn't.",
	}
}

func sampleColdTrailVerdict() usecase.RegressionVerdict {
	return usecase.RegressionVerdict{
		Repo:        "openclaw/openclaw",
		Workflow:    "CI",
		Branch:      "main",
		Status:      usecase.RegressionInconclusive,
		RunsScanned: 1000,
		Verdict:     "trail went cold after 1,000 runs.",
	}
}

// sampleWorkflows covers every documented state plus an unknown one,
// mirroring the fake adapter, for fixtures and scenario apps.
func sampleWorkflows() []model.Workflow {
	return []model.Workflow{
		{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/ci.yml"},
		{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/nightly.yml"},
		{ID: 125, Name: "Stale Patrol", Path: ".github/workflows/stale.yml", State: model.WorkflowStateDisabledManually, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/stale.yml"},
		{ID: 126, Name: "Fork Gate", Path: ".github/workflows/fork-gate.yml", State: model.WorkflowStateDisabledFork, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/fork-gate.yml"},
		{ID: 127, Name: "Old Patrol", Path: ".github/workflows/old-patrol.yml", State: model.WorkflowStateDeleted, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/old-patrol.yml"},
		{ID: 128, Name: "Mystery Cron", Path: ".github/workflows/mystery.yml", State: "disabled_quarantine", HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/mystery.yml"},
	}
}

// sampleDispatchPickerWorkflows seeds the badged dispatch chooser:
// two on duty, one asleep, one muzzled.
func sampleDispatchPickerWorkflows() []dispatch.Workflow {
	return []dispatch.Workflow{
		{Name: "CI", ID: "ci.yml", Ref: "main", State: model.WorkflowStateActive},
		{Name: "Release", ID: "release.yml", Ref: "main", State: model.WorkflowStateActive},
		{Name: "Nightly Sweep", ID: "nightly.yml", Ref: "main", State: model.WorkflowStateDisabledInactivity},
		{Name: "Stale Patrol", ID: "stale.yml", Ref: "main", State: model.WorkflowStateDisabledManually},
	}
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
