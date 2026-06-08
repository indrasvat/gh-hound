package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/github"
	"github.com/indrasvat/gh-hound/internal/adapter/repository"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/tui"
	tuibanner "github.com/indrasvat/gh-hound/internal/tui/banner"
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
	GitHub    usecase.GitHub
	Repo      usecase.RepositoryContextProvider
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
			return runTUI(runtime, info, options)
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

func runTUI(runtime commandRuntime, info buildInfo, options cliOptions) error {
	build := tui.BuildInfo{
		Version: info.Version,
		Commit:  info.Commit,
		Date:    info.Date,
	}
	cfg := tui.NewApp(tui.Options{Config: defaultConfig(), Build: build})
	if options.Fake != "" {
		cfg = tui.NewScenarioApp(options.Fake, build)
	}
	width, height := terminalSize(runtime.Stdout)
	restore, err := rawInput(runtime.Stdin, runtime.IsTTY)
	if err != nil {
		return err
	}
	defer restore()

	render := func() error {
		if runtime.IsTTY {
			if _, err := io.WriteString(runtime.Stdout, "\x1b[2J\x1b[H"); err != nil {
				return err
			}
			_, err := io.WriteString(runtime.Stdout, ttyView(cfg.ViewSize(width, height)))
			return err
		}
		_, err := fmt.Fprintln(runtime.Stdout, cfg.ViewSize(width, height))
		return err
	}
	if err := render(); err != nil {
		return err
	}

	buf := make([]byte, 1)
	for !cfg.ShouldQuit() {
		n, err := runtime.Stdin.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if n == 0 {
			continue
		}
		key := keyName(buf[0], runtime.Stdin)
		var handled bool
		cfg, handled = cfg.Update(tui.KeyMsg{Key: key})
		if handled {
			if err := render(); err != nil {
				return err
			}
		}
	}
	return nil
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

func keyName(first byte, reader io.Reader) string {
	switch first {
	case 0x03:
		return "ctrl+c"
	case '\r', '\n':
		return "enter"
	case '\t':
		return "tab"
	case 0x7f, '\b':
		return "backspace"
	case 0x1b:
		return "esc"
	default:
		return string(rune(first))
	}
}

func defaultConfig() config.Config {
	cfg := config.Default()
	return cfg
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
	githubClient := runtime.GitHub
	if githubClient == nil {
		githubClient = github.NewClient("https://api.github.com", authenticatedHTTPClient(runtime.Env))
	}
	repoProvider := runtime.Repo
	if repoProvider == nil {
		repoProvider = repository.Detector{LookupEnv: runtime.Env}
	}

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

func parseStatusFilter(raw string) (model.Status, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case "failure", "failed", "failing":
		return model.StatusCompleted, nil
	case "success", "passed", "green":
		return model.StatusCompleted, nil
	default:
		return model.ParseStatus(raw)
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

func authenticatedHTTPClient(lookup func(string) (string, bool)) *http.Client {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	token := firstLookup(lookup, "GH_TOKEN", "GITHUB_TOKEN")
	if token == "" {
		return http.DefaultClient
	}
	return &http.Client{Transport: authTransport{
		token: token,
		base:  http.DefaultTransport,
	}}
}

type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
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
