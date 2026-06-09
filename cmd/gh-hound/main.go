package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/github"
	"github.com/indrasvat/gh-hound/internal/adapter/repository"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logging"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/tui"
	tuibanner "github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
	failurescreen "github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/usecase"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type buildInfo struct {
	Version string
	Commit  string
	Date    string
}

func main() {
	code, err := executeCommand(newRootCommand(os.Stdout, os.Stderr, buildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
	if err != nil && !isOutcome(err) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

func newRootCommand(stdout, stderr io.Writer, info buildInfo) *cobra.Command {
	return newRootCommandWithRuntime(commandRuntime{
		Stdout: stdout,
		Stderr: stderr,
		Env:    os.LookupEnv,
		IsTTY:  defaultIsTTY(stdout),
	}, info)
}

type commandRuntime struct {
	Stdout    io.Writer
	Stderr    io.Writer
	Stdin     io.Reader
	Env       func(string) (string, bool)
	IsTTY     bool
	StateHome string
	GHToken   func() string
	GitHub    usecase.GitHub
	Repo      usecase.RepositoryContextProvider
	OpenURL   func(string) error
	CopyText  func(string) error
}

type cliOptions struct {
	Repo      string
	Branch    string
	Status    string
	Format    render.Format
	NoTUI     bool
	JSON      bool
	LogLevel  string
	TraceHTTP bool
	All       bool
	Watch     bool
	Fake      string
}

func newRootCommandWithRuntime(runtime commandRuntime, info buildInfo) *cobra.Command {
	if runtime.Stdout == nil {
		runtime.Stdout = io.Discard
	}
	if runtime.Stderr == nil {
		runtime.Stderr = io.Discard
	}
	if runtime.Stdin == nil {
		runtime.Stdin = os.Stdin
	}
	if runtime.Env == nil {
		runtime.Env = os.LookupEnv
	}

	options := cliOptions{Format: render.FormatJSON, LogLevel: "info"}
	cmd := &cobra.Command{
		Use:           "gh-hound",
		Short:         "Hunt down your GitHub Actions CI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			showVersion, err := cmd.Flags().GetBool("version")
			if err != nil {
				return err
			}
			if showVersion {
				return printVersion(runtime.Stdout, info)
			}
			applyEnv(&options, runtime.Env)
			if structuredOutput(options, runtime) {
				return writeResult(cmd.Context(), runtime.Stdout, options, runtime)
			}
			return runTUI(cmd.Context(), runtime, info, options)
		},
	}

	cmd.SetOut(runtime.Stdout)
	cmd.SetErr(runtime.Stderr)
	cmd.Flags().BoolP("version", "v", false, "print version information")
	cmd.PersistentFlags().BoolVar(&options.NoTUI, "no-tui", false, "disable the TUI and write structured output (env HOUND_NO_TUI)")
	cmd.PersistentFlags().BoolVar(&options.JSON, "json", false, "write JSON output and disable the TUI (env HOUND_JSON)")
	cmd.PersistentFlags().StringVarP(&options.Repo, "repo", "R", "", "GitHub repository owner/name (env GH_REPO or HOUND_REPO)")
	cmd.PersistentFlags().StringVar(&options.Branch, "branch", "", "Git branch/ref to inspect (env HOUND_BRANCH)")
	cmd.PersistentFlags().BoolVarP(&options.All, "all", "A", false, "show all branches instead of the current branch (env HOUND_ALL)")
	cmd.PersistentFlags().StringVar((*string)(&options.Format), "format", "json", "pipe output format: json, md, xml (env HOUND_FORMAT)")
	cmd.PersistentFlags().StringVar(&options.LogLevel, "log-level", "info", "log level: off, error, warn, info, debug (env HOUND_LOG_LEVEL)")
	cmd.PersistentFlags().BoolVar(&options.TraceHTTP, "trace-http", false, "trace GitHub API calls to the JSON log (env HOUND_TRACE_HTTP)")
	cmd.PersistentFlags().StringVar(&options.Fake, "fake-scenario", "", "deterministic fake scenario: green, failure, pending, empty, api_error (env HOUND_FAKE_SCENARIO)")
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		applyEnv(&options, runtime.Env)
		return nil
	}

	cmd.AddCommand(newVersionCommand(runtime.Stdout, info))
	cmd.AddCommand(newScreenCommand(runtime.Stdout))
	cmd.AddCommand(newInteractCommand(runtime.Stdout))
	cmd.AddCommand(newRunsCommand(runtime, &options))
	cmd.AddCommand(newWatchCommand(runtime, &options))
	cmd.AddCommand(newDispatchCommand(runtime, &options))
	return cmd
}

func newScreenCommand(stdout io.Writer) *cobra.Command {
	var screen string
	var width int
	var height int
	cmd := &cobra.Command{
		Use:    "__screen",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(stdout, tui.RenderFixtureSize(screen, width, height))
			return err
		},
	}
	cmd.Flags().StringVar(&screen, "screen", "runs", "fixture screen")
	cmd.Flags().IntVar(&width, "width", 80, "fixture width")
	cmd.Flags().IntVar(&height, "height", 24, "fixture height")
	return cmd
}

func newInteractCommand(stdout io.Writer) *cobra.Command {
	var scenario string
	var width int
	var height int
	cmd := &cobra.Command{
		Use:    "__interact",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(stdout, tui.RenderInteractionFixtureSize(scenario, width, height))
			return err
		},
	}
	cmd.Flags().StringVar(&scenario, "scenario", "global-help", "interaction fixture scenario")
	cmd.Flags().IntVar(&width, "width", 80, "fixture width")
	cmd.Flags().IntVar(&height, "height", 24, "fixture height")
	return cmd
}

func newVersionCommand(stdout io.Writer, info buildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printVersion(stdout, info)
		},
	}
}

func newRunsCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List GitHub Actions runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env)
			return writeResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().StringVar(&options.Status, "status", "", "filter runs by status or conclusion (env HOUND_STATUS)")
	return cmd
}

func newWatchCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch the current or selected run",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Status = "in_progress"
			options.NoTUI = true
			options.Watch = true
			return writeResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
}

func newDispatchCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "dispatch",
		Short: "Trigger a workflow_dispatch workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.NoTUI = true
			return writeResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
}

func printVersion(w io.Writer, info buildInfo) error {
	_, err := io.WriteString(w, tuibanner.RenderVersion(tuibanner.BuildInfo{
		Version: info.Version,
		Commit:  info.Commit,
		Date:    info.Date,
	}))
	return err
}

func runTUI(ctx context.Context, runtime commandRuntime, info buildInfo, options cliOptions) error {
	if options.Fake != "" {
		return fmt.Errorf("--fake-scenario is not available for the interactive TUI; use --no-tui for fixture output")
	}
	build := tui.BuildInfo{
		Version: info.Version,
		Commit:  info.Commit,
		Date:    info.Date,
	}
	preparedRuntime, closeTrace, err := runtimeWithGitHubClient(runtime, options)
	if err != nil {
		return err
	}
	defer func() {
		_ = closeTrace()
	}()
	app, err := defaultTUIApp(ctx, preparedRuntime, build, options)
	if err != nil {
		return err
	}
	width, height := terminalSize(runtime.Stdout)
	restore, err := rawInput(runtime.Stdin, runtime.IsTTY)
	if err != nil {
		return err
	}
	defer restore()
	if runtime.IsTTY {
		if _, err := io.WriteString(runtime.Stdout, "\x1b[?25l"); err != nil {
			return err
		}
		defer func() {
			_, _ = io.WriteString(runtime.Stdout, "\x1b[?25h")
		}()
	}

	render := func() error {
		if runtime.IsTTY {
			if _, err := io.WriteString(runtime.Stdout, "\x1b[?25l\x1b[2J\x1b[H"); err != nil {
				return err
			}
			_, err := io.WriteString(runtime.Stdout, ttyView(app.ViewSize(width, height)))
			return err
		}
		_, err := fmt.Fprintln(runtime.Stdout, app.ViewSize(width, height))
		return err
	}
	if err := render(); err != nil {
		return err
	}

	events := readKeys(runtime.Stdin)
	ticker := time.NewTicker(app.PollInterval())
	defer ticker.Stop()
	for !app.ShouldQuit() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-events:
			if event.err != nil {
				if errors.Is(event.err, io.EOF) {
					return nil
				}
				return event.err
			}
			var handled bool
			app, handled = app.Update(tui.KeyMsg{Key: event.key})
			if handled {
				ticker.Reset(app.PollInterval())
				if err := render(); err != nil {
					return err
				}
			}
		case <-ticker.C:
			var changed bool
			app, changed = app.Refresh()
			ticker.Reset(app.PollInterval())
			if changed {
				if err := render(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type keyEvent struct {
	key string
	err error
}

func readKeys(reader io.Reader) <-chan keyEvent {
	events := make(chan keyEvent, 1)
	go func() {
		buf := make([]byte, 8)
		decoder := keyDecoder{}
		for {
			key, err := decoder.Next(reader, buf)
			events <- keyEvent{key: key, err: err}
			if err != nil {
				return
			}
		}
	}()
	return events
}

type keyDecoder struct {
	pending []byte
}

func (d *keyDecoder) Next(reader io.Reader, scratch []byte) (string, error) {
	if len(scratch) == 0 {
		scratch = make([]byte, 8)
	}
	for {
		if key, ok := d.pop(); ok {
			return key, nil
		}
		n, err := reader.Read(scratch)
		if err != nil {
			return "", err
		}
		if n > 0 {
			d.pending = append(d.pending, scratch[:n]...)
		}
	}
}

func (d *keyDecoder) pop() (string, bool) {
	if len(d.pending) == 0 {
		return "", false
	}
	if len(d.pending) >= 3 && d.pending[0] == 0x1b && d.pending[1] == '[' {
		if key := keyName(d.pending[:3]); key != "esc" {
			d.pending = d.pending[3:]
			return key, true
		}
	}
	key := keyName(d.pending[:1])
	d.pending = d.pending[1:]
	return key, true
}

func defaultTUIApp(ctx context.Context, runtime commandRuntime, build tui.BuildInfo, options cliOptions) (tui.App, error) {
	cfg, err := defaultConfig(runtime.Env)
	if err != nil {
		return tui.App{}, err
	}
	githubClient := githubClientForRuntime(runtime)
	repoProvider := repoProviderForRuntime(runtime)
	actionService := usecase.ActionService{
		GitHub:  githubClient,
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second},
	}
	failureService := usecase.FailureService{GitHub: githubClient}
	watchService := usecase.WatchService{GitHub: githubClient, MinPoll: cfg.PollMin, MaxPoll: cfg.PollMax}
	launch := usecase.LaunchService{
		Config:     cfg,
		GitHub:     githubClient,
		Repository: repoProvider,
	}.Resolve(ctx, usecase.LaunchRequest{
		Repo:    options.Repo,
		Branch:  options.Branch,
		All:     options.All,
		PerPage: cfg.PerPage,
	})
	return tui.NewApp(tui.Options{
		Config: cfg,
		Build:  build,
		Launch: launch,
		RunsResolver: func(filter usecase.RunFilter) ([]model.Run, error) {
			if filter.PerPage == 0 {
				filter.PerPage = cfg.PerPage
			}
			return githubClient.ListRuns(ctx, filter)
		},
		DetailResolver: func(run model.Run) (detail.Model, error) {
			jobs, err := githubClient.ListJobs(ctx, launch.Repo, run.ID)
			if err != nil {
				return detail.Model{}, err
			}
			return detail.NewModel(run, jobs).WithRepo(launch.Repo), nil
		},
		FailureResolver: func(run model.Run, selected model.Job) (failurescreen.Model, logscreen.Model, error) {
			job, err := resolveJobForRun(ctx, githubClient, launch.Repo, run, selected)
			if err != nil {
				return failurescreen.Model{}, logscreen.Model{}, err
			}
			report, err := failureService.LoadFailure(ctx, launch.Repo, job)
			if err != nil {
				return failurescreen.Model{}, logscreen.Model{}, err
			}
			return failurescreen.NewModel(launch.Repo, run.ID, report), logscreen.NewModel(report.Log, report.Log.Failure.AnchorLine, 6), nil
		},
		LogResolver: func(run model.Run, selected model.Job) (logscreen.Model, error) {
			job, err := resolveJobForRun(ctx, githubClient, launch.Repo, run, selected)
			if err != nil {
				return logscreen.Model{}, err
			}
			raw, err := githubClient.FetchJobLog(ctx, launch.Repo, job.ID)
			if err != nil {
				return logscreen.Model{}, err
			}
			return logscreen.NewModel(logs.Parse(raw), 1, 6), nil
		},
		WatchResolver: func(run model.Run) (watch.Model, error) {
			state, err := watchService.Tick(ctx, usecase.WatchState{Repo: launch.Repo, RunID: run.ID, Run: run})
			if err != nil {
				return watch.Model{}, err
			}
			if state.Run.ID == 0 {
				state.Run = run
			}
			return watch.NewModel(watch.State{
				Repo:    launch.Repo,
				Branch:  firstNonEmptyString(launch.Branch, run.HeadBranch),
				Run:     state.Run,
				Lines:   state.Appended,
				Elapsed: elapsedRun(state.Run),
			}), nil
		},
		DispatchResolver: func() (dispatch.Model, error) {
			workflows := launch.Workflows
			var err error
			if len(workflows) == 0 {
				workflows, err = githubClient.ListWorkflows(ctx, launch.Repo)
				if err != nil {
					return dispatch.Model{}, err
				}
			}
			workflow, err := chooseDispatchWorkflow(ctx, githubClient, launch.Repo, workflows)
			if err != nil {
				return dispatch.Model{}, err
			}
			workflowName := workflowDisplayName(workflow)
			workflowID := workflowIdentifier(workflow)
			if workflowName == "" || workflowID == "" {
				return dispatch.Model{}, fmt.Errorf("workflow metadata is incomplete")
			}
			ref, err := dispatchRef(launch)
			if err != nil {
				return dispatch.Model{}, err
			}
			return dispatch.NewModel(dispatch.Workflow{
				Name:   workflowName,
				ID:     workflowID,
				Ref:    ref,
				Inputs: dispatchInputs(workflow.Inputs),
			}), nil
		},
		ActionHandler: func(request tui.ActionRequest) (usecase.ActionResult, error) {
			switch request.Action {
			case usecase.ActionRerunRun:
				return actionService.RerunRun(ctx, launch.Repo, request.Run.ID, request.Debug)
			case usecase.ActionRerunFailedJobs:
				return actionService.RerunFailedJobs(ctx, launch.Repo, request.Run.ID)
			case usecase.ActionRerunJob:
				if request.Job.ID == 0 {
					return usecase.ActionResult{}, fmt.Errorf("job is not loaded")
				}
				return actionService.RerunJob(ctx, launch.Repo, request.Job.ID)
			case usecase.ActionCancelRun:
				return actionService.CancelRun(ctx, launch.Repo, request.Run.ID)
			case usecase.ActionForceCancelRun:
				return actionService.ForceCancelRun(ctx, launch.Repo, request.Run.ID)
			case usecase.ActionDispatch:
				if request.Workflow.ID == "" {
					return usecase.ActionResult{}, fmt.Errorf("workflow is not loaded")
				}
				return actionService.DispatchWorkflow(ctx, launch.Repo, request.Workflow.ID, request.Dispatch)
			default:
				return usecase.ActionResult{}, fmt.Errorf("unsupported action %q", request.Action)
			}
		},
		OpenURL:  openURLForRuntime(runtime),
		CopyText: copyTextForRuntime(runtime),
	}), nil
}

func openURLForRuntime(runtime commandRuntime) func(string) error {
	if runtime.OpenURL != nil {
		return runtime.OpenURL
	}
	return openBrowser
}

func copyTextForRuntime(runtime commandRuntime) func(string) error {
	if runtime.CopyText != nil {
		return runtime.CopyText
	}
	return copyToClipboard
}

func openBrowser(rawURL string) error {
	name, args, err := browserCommand(goruntime.GOOS, rawURL)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func copyToClipboard(value string) error {
	name, args, err := clipboardCommand(goruntime.GOOS)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(value)
	return cmd.Run()
}

func browserCommand(goos, rawURL string) (string, []string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil, fmt.Errorf("url is empty")
	}
	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "xdg-open", []string{rawURL}, nil
	}
}

func clipboardCommand(goos string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "pbcopy", nil, nil
	case "windows":
		return "clip", nil, nil
	default:
		for _, candidate := range []struct {
			name string
			args []string
		}{
			{name: "wl-copy"},
			{name: "xclip", args: []string{"-selection", "clipboard"}},
			{name: "xsel", args: []string{"--clipboard", "--input"}},
		} {
			if _, err := exec.LookPath(candidate.name); err == nil {
				return candidate.name, candidate.args, nil
			}
		}
		return "", nil, fmt.Errorf("no clipboard command found; install wl-copy, xclip, or xsel")
	}
}

func resolveJobForRun(ctx context.Context, githubClient usecase.GitHub, repo string, run model.Run, selected model.Job) (model.Job, error) {
	if selected.ID != 0 {
		return selected, nil
	}
	jobs, err := githubClient.ListJobs(ctx, repo, run.ID)
	if err != nil {
		return model.Job{}, err
	}
	if len(jobs) == 0 {
		return model.Job{}, fmt.Errorf("no jobs found for run #%d", run.RunNumber)
	}
	for _, job := range jobs {
		if job.Conclusion == model.ConclusionFailure || job.Conclusion == model.ConclusionActionRequired || job.Conclusion == model.ConclusionTimedOut {
			return job, nil
		}
	}
	return jobs[0], nil
}

func chooseDispatchWorkflow(ctx context.Context, githubClient usecase.GitHub, repo string, workflows []model.Workflow) (model.Workflow, error) {
	for _, workflow := range workflows {
		if workflow.State != "" && workflow.State != "active" {
			continue
		}
		if strings.TrimSpace(workflow.Path) == "" {
			continue
		}
		raw, err := githubClient.FetchWorkflowFile(ctx, repo, workflow.Path)
		if err != nil {
			var apiErr usecase.APIError
			if errors.As(err, &apiErr) && apiErr.Kind == usecase.APIErrorNotFound {
				continue
			}
			return model.Workflow{}, err
		}
		inputs, ok, err := usecase.ParseWorkflowDispatchInputs(raw)
		if err != nil {
			return model.Workflow{}, err
		}
		if !ok {
			continue
		}
		workflow.Inputs = inputs
		return workflow, nil
	}
	return model.Workflow{}, fmt.Errorf("no workflow_dispatch workflows found")
}

func dispatchInputs(inputs []model.WorkflowInput) []dispatch.Input {
	out := make([]dispatch.Input, 0, len(inputs))
	for _, input := range inputs {
		mapped := dispatch.Input{
			Name:     input.Name,
			Required: input.Required,
			Default:  input.Default,
			Options:  append([]string(nil), input.Options...),
		}
		switch input.Type {
		case "boolean":
			mapped.Type = dispatch.InputBool
			if len(mapped.Options) == 0 {
				mapped.Options = []string{"false", "true"}
			}
		case "choice":
			mapped.Type = dispatch.InputSelect
		default:
			mapped.Type = dispatch.InputText
		}
		out = append(out, mapped)
	}
	return out
}

func workflowIdentifier(workflow model.Workflow) string {
	if workflow.Path != "" {
		return workflow.Path
	}
	if workflow.ID != 0 {
		return strconv.FormatInt(workflow.ID, 10)
	}
	return ""
}

func workflowDisplayName(workflow model.Workflow) string {
	if strings.TrimSpace(workflow.Name) != "" {
		return strings.TrimSpace(workflow.Name)
	}
	if strings.TrimSpace(workflow.Path) != "" {
		return strings.TrimSpace(workflow.Path)
	}
	if workflow.ID != 0 {
		return strconv.FormatInt(workflow.ID, 10)
	}
	return ""
}

func dispatchRef(launch usecase.LaunchContext) (string, error) {
	if ref := strings.TrimSpace(launch.Branch); ref != "" {
		return ref, nil
	}
	return "", fmt.Errorf("dispatch ref is unavailable; pass --branch or run from a branch checkout")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func elapsedRun(run model.Run) string {
	start := run.RunStartedAt
	if start.IsZero() {
		start = run.CreatedAt
	}
	end := run.UpdatedAt
	if run.Status != model.StatusCompleted || end.IsZero() || end.Before(start) {
		end = time.Now()
	}
	if start.IsZero() || end.Before(start) {
		return ""
	}
	elapsed := end.Sub(start).Round(time.Second)
	if elapsed < time.Minute {
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
}

func githubClientForRuntime(runtime commandRuntime) usecase.GitHub {
	if runtime.GitHub != nil {
		return runtime.GitHub
	}
	return github.NewClient("https://api.github.com", authenticatedHTTPClient(runtime.Env, ghTokenLookup(runtime)))
}

func runtimeWithGitHubClient(runtime commandRuntime, options cliOptions) (commandRuntime, func() error, error) {
	if runtime.GitHub != nil {
		return runtime, func() error { return nil }, nil
	}
	closeTrace := func() error { return nil }
	var loggerOptions github.ClientOptions
	if options.TraceHTTP {
		configured, err := logging.Configure(logging.Options{StateHome: runtime.StateHome, Level: "debug"})
		if err != nil {
			return runtime, closeTrace, err
		}
		closeTrace = configured.Close
		loggerOptions = github.ClientOptions{TraceHTTP: true, Logger: configured.Logger}
	}
	runtime.GitHub = github.NewClientWithOptions("https://api.github.com", authenticatedHTTPClient(runtime.Env, ghTokenLookup(runtime)), loggerOptions)
	return runtime, closeTrace, nil
}

func repoProviderForRuntime(runtime commandRuntime) usecase.RepositoryContextProvider {
	if runtime.Repo != nil {
		return runtime.Repo
	}
	return repository.Detector{LookupEnv: runtime.Env}
}

func rawInput(reader io.Reader, enabled bool) (func(), error) {
	if !enabled {
		return func() {}, nil
	}
	file, ok := reader.(*os.File)
	if !ok {
		return func() {}, nil
	}
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return func() {}, nil
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() {
		_ = term.Restore(fd, state)
	}, nil
}

func terminalSize(writer io.Writer) (int, int) {
	file, ok := writer.(*os.File)
	if !ok {
		return 80, 24
	}
	width, height, err := term.GetSize(int(file.Fd()))
	if err != nil || width < 1 {
		return 80, 24
	}
	if height < 1 {
		height = 24
	}
	return width, height
}

func keyName(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	switch string(input) {
	case "\x1b[A":
		return "up"
	case "\x1b[B":
		return "down"
	case "\x1b[C":
		return "right"
	case "\x1b[D":
		return "left"
	case "\x1b[Z":
		return "shift+tab"
	}
	switch input[0] {
	case 0x03:
		return "ctrl+c"
	case 0x04:
		return "ctrl+d"
	case 0x15:
		return "ctrl+u"
	case '\r', '\n':
		return "enter"
	case '\t':
		return "tab"
	case 0x7f, '\b':
		return "backspace"
	case 0x1b:
		return "esc"
	default:
		return string(rune(input[0]))
	}
}

func defaultConfig(lookup ...func(string) (string, bool)) (config.Config, error) {
	var env func(string) (string, bool)
	if len(lookup) > 0 {
		env = lookup[0]
	}
	return config.Load(config.LoadOptions{LookupEnv: env})
}

func ttyView(view string) string {
	return strings.ReplaceAll(strings.TrimRight(view, "\n"), "\n", "\r\n")
}

func executeCommand(cmd *cobra.Command) (int, error) {
	err := cmd.Execute()
	if err == nil {
		return render.ExitOK, nil
	}
	var outcome outcomeError
	if errors.As(err, &outcome) {
		return outcome.code, err
	}
	return render.ExitError, err
}

type outcomeError struct {
	code int
}

func (e outcomeError) Error() string {
	return fmt.Sprintf("gh-hound exited with code %d", e.code)
}

func isOutcome(err error) bool {
	var outcome outcomeError
	return errors.As(err, &outcome)
}

func writeResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime) error {
	result, err := resultForOptions(ctx, options, runtime)
	if err != nil {
		return err
	}
	if err := render.Write(w, options.Format, result); err != nil {
		return err
	}
	code := render.ExitCode(result, nil)
	if code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

func resultForOptions(ctx context.Context, options cliOptions, runtime commandRuntime) (render.Result, error) {
	if options.Fake == "" {
		return liveResult(ctx, options, runtime)
	}
	scenario := normalizedScenario(options)
	if scenario == "api_error" || scenario == "network_error" || scenario == "rate_limited" {
		return render.Result{}, errors.New("github api unavailable")
	}
	return fakeResult(options, scenario), nil
}

func liveResult(ctx context.Context, options cliOptions, runtime commandRuntime) (render.Result, error) {
	preparedRuntime, closeTrace, err := runtimeWithGitHubClient(runtime, options)
	if err != nil {
		return render.Result{}, err
	}
	defer func() {
		_ = closeTrace()
	}()
	githubClient := githubClientForRuntime(preparedRuntime)
	repoProvider := repoProviderForRuntime(preparedRuntime)

	repoCtx, err := repoProvider.Current(ctx)
	if err != nil && options.Repo == "" {
		return render.Result{}, fmt.Errorf("%s; pass -R owner/repo or set GH_REPO", err)
	}
	repo := firstNonEmpty(options.Repo, repoCtx.Repo)
	if repo == "" {
		return render.Result{}, errors.New("repository context could not be resolved; pass -R owner/repo or set GH_REPO")
	}
	branch := firstNonEmpty(options.Branch, repoCtx.Branch)
	if options.All {
		branch = ""
	}

	filter := usecase.RunFilter{Repo: repo, Branch: branch, PerPage: 30}
	if options.Status != "" {
		status, err := parseStatusFilter(options.Status)
		if err != nil {
			return render.Result{}, err
		}
		filter.Status = status
	}
	runs, err := githubClient.ListRuns(ctx, filter)
	if err != nil {
		return render.Result{}, err
	}
	runs = filterRunsByConclusion(runs, options.Status)
	return render.Result{Repo: repo, Branch: branch, Runs: mapRenderRuns(runs)}, nil
}

func parseStatusFilter(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case "failure", "failed", "failing":
		return string(model.ConclusionFailure), nil
	case "success", "passed", "green":
		return string(model.ConclusionSuccess), nil
	default:
		if status, err := model.ParseStatus(raw); err == nil {
			return string(status), nil
		}
		conclusion, err := model.ParseConclusion(raw)
		if err != nil {
			return "", err
		}
		return string(conclusion), nil
	}
}

func mapRenderRuns(runs []model.Run) []render.Run {
	out := make([]render.Run, 0, len(runs))
	for _, run := range runs {
		out = append(out, render.Run{
			ID:         run.ID,
			Workflow:   firstNonEmpty(run.Name, run.DisplayTitle, run.Path),
			RunNumber:  run.RunNumber,
			Event:      run.Event,
			HeadBranch: run.HeadBranch,
			HeadSHA:    run.HeadSHA,
			Status:     string(run.Status),
			Conclusion: string(run.Conclusion),
			CreatedAt:  run.CreatedAt,
			HTMLURL:    run.HTMLURL,
			Failed:     []render.Failure{},
		})
	}
	return out
}

func filterRunsByConclusion(runs []model.Run, rawStatus string) []model.Run {
	var want model.Conclusion
	switch strings.ToLower(strings.TrimSpace(rawStatus)) {
	case "failure", "failed", "failing":
		want = model.ConclusionFailure
	case "success", "passed", "green":
		want = model.ConclusionSuccess
	default:
		return runs
	}
	filtered := make([]model.Run, 0, len(runs))
	for _, run := range runs {
		if run.Conclusion == want {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func authenticatedHTTPClient(lookup func(string) (string, bool), ghToken func() string) *http.Client {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	token := firstLookup(lookup, "GH_TOKEN", "GITHUB_TOKEN")
	if token == "" && ghToken != nil {
		token = strings.TrimSpace(ghToken())
	}
	if token == "" {
		return http.DefaultClient
	}
	return &http.Client{Transport: authTransport{
		token: token,
		base:  http.DefaultTransport,
	}}
}

func ghTokenLookup(runtime commandRuntime) func() string {
	if runtime.GHToken != nil {
		return runtime.GHToken
	}
	return ghAuthToken
}

func ghAuthToken() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if isGitHubHost(clone.URL.Hostname()) {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	return t.base.RoundTrip(clone)
}

func isGitHubHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "api.github.com" || host == "github.com" || strings.HasSuffix(host, ".github.com")
}

func firstLookup(lookup func(string) (string, bool), keys ...string) string {
	for _, key := range keys {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fakeResult(options cliOptions, scenario string) render.Result {
	repo := firstNonEmpty(options.Repo, "indrasvat/gh-hound")
	branch := firstNonEmpty(options.Branch, "main")
	if options.All {
		branch = ""
	}
	runStatus := "completed"
	conclusion := "success"
	if scenario == "failure" {
		conclusion = "failure"
	}
	if scenario == "pending" {
		runStatus = "in_progress"
		conclusion = ""
	}
	if scenario == "empty" {
		return render.Result{Repo: repo, Branch: branch, Runs: []render.Run{}}
	}
	run := render.Run{
		ID:         30433642,
		Workflow:   "CI",
		RunNumber:  571,
		Event:      "pull_request",
		HeadBranch: branch,
		HeadSHA:    "a1b2c3d",
		Status:     runStatus,
		Conclusion: conclusion,
		CreatedAt:  time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC),
		HTMLURL:    "https://github.com/" + repo + "/actions/runs/30433642",
		Failed:     []render.Failure{},
	}
	if conclusion == "failure" {
		run.Failed = []render.Failure{{
			Job:      "build",
			Step:     "go test ./...",
			ExitCode: 1,
			Annotations: []render.Annotation{{
				Path:    "internal/parser/lexer.go",
				Line:    142,
				Level:   "failure",
				Message: "identifier mismatch",
			}},
			LogExcerpt: "--- FAIL: TestLexIdent/trailing_underscore ...",
		}}
	}
	return render.Result{
		Repo:   repo,
		Branch: branch,
		Runs:   []render.Run{run},
	}
}

func normalizedScenario(options cliOptions) string {
	raw := strings.ToLower(strings.TrimSpace(options.Fake))
	switch raw {
	case "ok", "green", "success", "passed":
		return "green"
	case "failure", "failed", "failing":
		return "failure"
	case "pending", "running", "in_progress", "queued":
		return "pending"
	case "empty", "none":
		return "empty"
	case "api_error", "network_error", "rate_limited", "error":
		return "api_error"
	}
	status := strings.ToLower(strings.TrimSpace(options.Status))
	switch status {
	case "failure", "failed", "failing":
		return "failure"
	case "in_progress", "queued", "pending", "running":
		return "pending"
	case "empty":
		return "empty"
	default:
		return "green"
	}
}

func applyEnv(options *cliOptions, lookup func(string) (string, bool)) {
	if options.Repo == "" {
		if value, ok := lookup("GH_REPO"); ok {
			options.Repo = value
		}
		if value, ok := lookup("HOUND_REPO"); ok {
			options.Repo = value
		}
	}
	if options.Branch == "" {
		if value, ok := lookup("HOUND_BRANCH"); ok {
			options.Branch = value
		}
	}
	if options.Status == "" {
		if value, ok := lookup("HOUND_STATUS"); ok {
			options.Status = value
		}
	}
	if value, ok := lookup("HOUND_FORMAT"); ok {
		options.Format = render.Format(value)
	}
	if value, ok := lookup("HOUND_NO_TUI"); ok {
		options.NoTUI = parseBool(value)
	}
	if value, ok := lookup("HOUND_JSON"); ok {
		options.JSON = parseBool(value)
	}
	if value, ok := lookup("HOUND_LOG_LEVEL"); ok {
		options.LogLevel = value
	}
	if value, ok := lookup("HOUND_TRACE_HTTP"); ok {
		options.TraceHTTP = parseBool(value)
	}
	if value, ok := lookup("HOUND_ALL"); ok {
		options.All = parseBool(value)
	}
	if options.Fake == "" {
		if value, ok := lookup("HOUND_FAKE_SCENARIO"); ok {
			options.Fake = value
		}
	}
	if options.JSON {
		options.NoTUI = true
		options.Format = render.FormatJSON
	}
}

func structuredOutput(options cliOptions, runtime commandRuntime) bool {
	return options.NoTUI || options.JSON || !runtime.IsTTY
}

func defaultIsTTY(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func parseBool(raw string) bool {
	value, err := strconv.ParseBool(raw)
	return err == nil && value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
