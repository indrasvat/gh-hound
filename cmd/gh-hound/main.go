package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/indrasvat/gh-hound/internal/render"
	tuibanner "github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/spf13/cobra"
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
	Env       func(string) (string, bool)
	IsTTY     bool
	StateHome string
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
}

func newRootCommandWithRuntime(runtime commandRuntime, info buildInfo) *cobra.Command {
	if runtime.Stdout == nil {
		runtime.Stdout = io.Discard
	}
	if runtime.Stderr == nil {
		runtime.Stderr = io.Discard
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
				return writeResult(runtime.Stdout, options)
			}
			return printPlaceholder(runtime.Stdout)
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
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		applyEnv(&options, runtime.Env)
		return nil
	}

	cmd.AddCommand(newVersionCommand(runtime.Stdout, info))
	cmd.AddCommand(newRunsCommand(runtime, &options))
	cmd.AddCommand(newWatchCommand(runtime, &options))
	cmd.AddCommand(newDispatchCommand(runtime, &options))
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
			return writeResult(runtime.Stdout, *options)
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
			return writeResult(runtime.Stdout, *options)
		},
	}
}

func newDispatchCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "dispatch",
		Short: "Trigger a workflow_dispatch workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.NoTUI = true
			return writeResult(runtime.Stdout, *options)
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

func printPlaceholder(w io.Writer) error {
	_, err := fmt.Fprintln(w, "gh-hound TUI scaffold is ready; screen implementation starts in Task 080.")
	return err
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

func writeResult(w io.Writer, options cliOptions) error {
	result := fakeResult(options)
	if err := render.Write(w, options.Format, result); err != nil {
		return err
	}
	code := render.ExitCode(result, nil)
	if code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

func fakeResult(options cliOptions) render.Result {
	repo := firstNonEmpty(options.Repo, "indrasvat/gh-hound")
	branch := firstNonEmpty(options.Branch, "main")
	if options.All {
		branch = ""
	}
	status := firstNonEmpty(options.Status, "success")
	runStatus := "completed"
	conclusion := "success"
	if status == "failure" || status == "failed" {
		conclusion = "failure"
	}
	if status == "in_progress" || status == "queued" || status == "pending" {
		runStatus = status
		conclusion = ""
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
