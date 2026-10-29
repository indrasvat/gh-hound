package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/adapter/github"
	"github.com/indrasvat/gh-hound/internal/adapter/repository"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logging"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/tui"
	tuibanner "github.com/indrasvat/gh-hound/internal/tui/banner"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/screens/caches"
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
		Stdin:  os.Stdin,
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
	Repo   string
	Branch string
	Status string
	Actor  string
	// ActorFromEnv marks an actor that came from HOUND_ACTOR: an
	// explicit --run ignores it instead of refusing.
	ActorFromEnv  bool
	Workflow      string
	Format        render.Format
	NoTUI         bool
	JSON          bool
	LogLevel      string
	TraceHTTP     bool
	All           bool
	Watch         bool
	Fake          string
	WithArtifacts bool
	WithApprovals bool
	RunID         int64
	Attempt       int
	Download      string
	Dir           string
	Force         bool
	LaunchRoute   usecase.LaunchRoute
	Approve       bool
	Reject        bool
	Envs          []string
	Comment       string
	WatchTimeout  time.Duration
	// perPage is config's per_page, set where a listing is built.
	perPage int
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

	options := defaultCLIOptions()
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
			applyEnv(&options, runtime.Env, cmd.Flags().Changed)
			if structuredOutput(options, runtime) {
				return writeResult(cmd.Context(), runtime.Stdout, options, runtime)
			}
			return runTUI(cmd.Context(), runtime, info, options)
		},
	}

	cmd.SetOut(runtime.Stdout)
	cmd.SetErr(runtime.Stderr)
	cmd.Flags().BoolP("version", "v", false, "print version information")
	cmd.Flags().StringVar(&options.Actor, "actor", "", actorFlagUsage)
	cmd.PersistentFlags().BoolVar(&options.NoTUI, "no-tui", false, "disable the TUI and write structured output (env HOUND_NO_TUI)")
	cmd.PersistentFlags().BoolVar(&options.JSON, "json", false, "write JSON output and disable the TUI (env HOUND_JSON)")
	cmd.PersistentFlags().StringVarP(&options.Repo, "repo", "R", "", "GitHub repository owner/name (env GH_REPO or HOUND_REPO)")
	cmd.PersistentFlags().StringVar(&options.Branch, "branch", "", "Git branch/ref to inspect (env HOUND_BRANCH)")
	cmd.PersistentFlags().BoolVarP(&options.All, "all", "A", false, "show all branches instead of the current branch (env HOUND_ALL)")
	cmd.PersistentFlags().StringVar((*string)(&options.Format), "format", "json", "pipe output format: json, md, xml (env HOUND_FORMAT)")
	cmd.PersistentFlags().StringVar(&options.LogLevel, "log-level", "info", "log level: off, error, warn, info, debug (env HOUND_LOG_LEVEL)")
	cmd.PersistentFlags().BoolVar(&options.TraceHTTP, "trace-http", false, "trace GitHub API calls to the JSON log (env HOUND_TRACE_HTTP)")
	cmd.PersistentFlags().StringVar(&options.Fake, "fake-scenario", "", "deterministic fake scenario: green, failure, pending, empty, api_error, conflict, permission, waiting, regression, pack, flaky, queue, noisy (env HOUND_FAKE_SCENARIO)")
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		applyEnv(&options, runtime.Env, cmd.Flags().Changed)
		return nil
	}

	cmd.AddCommand(newVersionCommand(runtime.Stdout, info))
	cmd.AddCommand(newScreenCommand(runtime.Stdout))
	cmd.AddCommand(newInteractCommand(runtime.Stdout))
	cmd.AddCommand(newVQATUICommand(runtime, info))
	cmd.AddCommand(newRunsCommand(runtime, &options))
	cmd.AddCommand(newWatchCommand(runtime, &options))
	cmd.AddCommand(newDispatchCommand(runtime, info, &options))
	cmd.AddCommand(newArtifactsCommand(runtime, &options))
	cmd.AddCommand(newApprovalsCommand(runtime, &options))
	cmd.AddCommand(newCachesCommand(runtime, &options))
	cmd.AddCommand(newRerunCommand(runtime, &options))
	cmd.AddCommand(newCancelCommand(runtime, &options))
	cmd.AddCommand(newDiffCommand(runtime, &options))
	cmd.AddCommand(newFlakesCommand(runtime, &options))
	cmd.AddCommand(newQueueCommand(runtime, &options))
	cmd.AddCommand(newWorkflowsCommand(runtime, &options))
	return cmd
}

func newRerunCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var failedOnly, debug bool
	var jobID int64
	cmd := &cobra.Command{
		Use:   "rerun",
		Short: "Send a run (or job) back out, optionally with debug logging",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			return writeMutationResult(cmd.Context(), runtime.Stdout, *options, runtime, mutationRequest{
				runID:      options.RunID,
				jobID:      jobID,
				failedOnly: failedOnly,
				debug:      debug,
			})
		},
	}
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "run ID to rerun")
	cmd.Flags().Int64Var(&jobID, "job", 0, "rerun a single job by ID")
	cmd.Flags().BoolVar(&failedOnly, "failed-only", false, "rerun only the failed jobs")
	cmd.Flags().BoolVar(&debug, "debug", false, "enable runner diagnostic and step debug logging")
	return cmd
}

func newCancelCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Call a run off",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			return writeMutationResult(cmd.Context(), runtime.Stdout, *options, runtime, mutationRequest{
				runID:  options.RunID,
				cancel: true,
				force:  force,
			})
		},
	}
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "run ID to cancel")
	cmd.Flags().BoolVar(&force, "force", false, "force-cancel a run stuck in cancellation")
	return cmd
}

// mutationRequest is the resolved intent of a rerun/cancel invocation.
type mutationRequest struct {
	runID      int64
	jobID      int64
	failedOnly bool
	debug      bool
	cancel     bool
	force      bool
}

// writeMutationResult performs exactly one mutation API call and emits
// the schema-stable envelope. Exit codes follow the global contract:
// 0 accepted, 2 anything else (agents branch on error.kind upstream).
// mutationAction names the request's path in the result enum.
func mutationAction(request mutationRequest) string {
	switch {
	case request.cancel && request.force:
		return "force_cancel"
	case request.cancel:
		return "cancel"
	case request.jobID != 0:
		return "rerun_job"
	case request.failedOnly:
		return "rerun_failed"
	default:
		return "rerun"
	}
}

func writeMutationResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime, request mutationRequest) error {
	format := render.Format(options.Format)
	result := render.MutationResult{
		Repo:    firstNonEmpty(options.Repo, ""),
		RunID:   request.runID,
		JobID:   request.jobID,
		Action:  mutationAction(request),
		HTMLURL: fmt.Sprintf("https://github.com/%s/actions/runs/%d", firstNonEmpty(options.Repo, ""), request.runID),
	}
	// Every refusal writes the envelope: agents branch on error.kind
	// on stdout and exit 2 is never a bare stderr message (the silent
	// outcomeError suppresses duplicate printing in main).
	refuse := func(err error) error {
		kind, message, field := "unknown", err.Error(), ""
		if actionErr, ok := usecase.AsActionError(err); ok {
			kind, message = string(actionErr.Kind), actionErr.Message
			field = actionErr.Field
		}
		result.Accepted = false
		result.Error = &render.MutationError{Kind: kind, Field: field, Message: message}
		if writeErr := render.WriteMutation(w, format, result); writeErr != nil {
			return writeErr
		}
		return outcomeError{code: render.ExitError}
	}
	if request.runID <= 0 {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "run", Message: "--run <run-id> (a positive ID) is required"})
	}
	if request.jobID < 0 {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "job", Message: "--job must be a positive job ID"})
	}
	if request.jobID != 0 && request.failedOnly {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "job", Message: "--job and --failed-only are mutually exclusive"})
	}

	var githubClient usecase.GitHub
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			kind := usecase.ActionErrorNetwork
			if strings.Contains(strings.ToLower(options.Fake), "rate") {
				kind = usecase.ActionErrorRateLimit
			}
			return refuse(usecase.ActionError{Kind: kind, Message: "github api unavailable"})
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		result.Repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return refuse(err)
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		result.Repo = target.repo
	}
	result.HTMLURL = fmt.Sprintf("https://github.com/%s/actions/runs/%d", result.Repo, request.runID)

	service := usecase.ActionService{
		GitHub:  githubClient,
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second},
	}
	var err error
	switch result.Action {
	case "force_cancel":
		_, err = service.ForceCancelRun(ctx, result.Repo, request.runID)
	case "cancel":
		_, err = service.CancelRun(ctx, result.Repo, request.runID)
	case "rerun_job":
		_, err = service.RerunJob(ctx, result.Repo, request.jobID, request.debug)
	case "rerun_failed":
		_, err = service.RerunFailedJobs(ctx, result.Repo, request.runID, request.debug)
	default:
		_, err = service.RerunRun(ctx, result.Repo, request.runID, request.debug)
	}
	if err != nil {
		return refuse(err)
	}
	result.Accepted = true
	return render.WriteMutation(w, format, result)
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

func newVQATUICommand(runtime commandRuntime, info buildInfo) *cobra.Command {
	var scenario, actor, configHome string
	var latency time.Duration
	cmd := &cobra.Command{
		Use:    "__vqa-tui",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.Stdin == nil {
				runtime.Stdin = os.Stdin
			}
			// A fixture ignores the user's config.toml; --config-home
			// hands it one on purpose.
			runtime.Env = fixtureConfigEnv(runtime.Env, configHome)
			adapter := fake.New(fake.Scenario(scenario))
			runtime.GitHub = adapter
			if latency > 0 {
				runtime.GitHub = slowGitHub{Adapter: adapter, latency: latency}
			}
			runtime.Repo = staticRepositoryContext{
				context: usecase.RepositoryContext{
					Repo:   "indrasvat/gh-hound",
					Branch: "main",
					Actor:  "indrasvat",
				},
			}
			return runTUI(cmd.Context(), runtime, info, cliOptions{Repo: "indrasvat/gh-hound", Branch: "main", Actor: actor})
		},
	}
	cmd.Flags().StringVar(&scenario, "scenario", string(fake.ScenarioFailing), "fake adapter scenario for PTY VQA")
	cmd.Flags().StringVar(&actor, "actor", "", "launch actor filter (login or @me)")
	cmd.Flags().DurationVar(&latency, "latency", 0, "delay every runs listing and identity lookup, to rehearse loading and stale results")
	cmd.Flags().StringVar(&configHome, "config-home", "", "read <dir>/gh-hound/config.toml instead of no config file")
	return cmd
}

func fixtureConfigEnv(env func(string) (string, bool), configHome string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		switch key {
		case "XDG_CONFIG_HOME":
			return configHome, configHome != ""
		case "HOME":
			return "", false
		}
		return env(key)
	}
}

// slowGitHub delays the calls a filter edit makes so PTY rehearsals can
// see loading states and out-of-order results. Cancellation still wins.
type slowGitHub struct {
	*fake.Adapter
	latency time.Duration
}

func (g slowGitHub) wait(ctx context.Context) error {
	select {
	case <-time.After(g.latency):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g slowGitHub) ListRuns(ctx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	if err := g.wait(ctx); err != nil {
		return nil, err
	}
	return g.Adapter.ListRuns(ctx, filter)
}

func (g slowGitHub) AuthenticatedLogin(ctx context.Context) (string, error) {
	if err := g.wait(ctx); err != nil {
		return "", err
	}
	return g.Adapter.AuthenticatedLogin(ctx)
}

type staticRepositoryContext struct {
	context usecase.RepositoryContext
}

func (s staticRepositoryContext) Current(context.Context) (usecase.RepositoryContext, error) {
	return s.context, nil
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
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			return writeResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().StringVar(&options.Status, "status", "", "filter runs by status or conclusion (env HOUND_STATUS)")
	cmd.Flags().StringVar(&options.Actor, "actor", "", actorFlagUsage)
	cmd.Flags().BoolVar(&options.WithArtifacts, "artifacts", false, "include artifact metadata per run (paginated artifact-list calls per run)")
	cmd.Flags().BoolVar(&options.WithApprovals, "approvals", false, "include pending_environments for waiting runs (one pending-deployments call per waiting run)")
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "inspect a single run by ID (any branch)")
	cmd.Flags().IntVar(&options.Attempt, "attempt", 0, "target a specific run attempt for failure triage (requires --run)")
	return cmd
}

const actorFlagUsage = "show runs started by a GitHub login, or @me for the authenticated user (env HOUND_ACTOR)"

func newArtifactsCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifacts",
		Short: "List or download a run's artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			return writeArtifactsResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "run ID to inspect (defaults to the latest run on the selected branch)")
	cmd.Flags().StringVar(&options.Download, "download", "", "artifact name or ID to download and extract")
	cmd.Flags().StringVar(&options.Dir, "dir", ".", "destination directory for downloads")
	cmd.Flags().BoolVar(&options.Force, "force", false, "overwrite an existing extraction destination")
	return cmd
}

func newApprovalsCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approvals",
		Short: "List or review a waiting run's deployment gates",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			return writeApprovalsResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "run ID whose pending deployments to inspect")
	cmd.Flags().BoolVar(&options.Approve, "approve", false, "approve the pending deployment gates")
	cmd.Flags().BoolVar(&options.Reject, "reject", false, "reject the pending deployment gates")
	cmd.Flags().StringArrayVar(&options.Envs, "env", nil, "environment name to review (repeatable; default: all you can approve)")
	cmd.Flags().StringVar(&options.Comment, "comment", "", "review comment (default: \"reviewed from gh-hound\")")
	return cmd
}

// writeApprovalsResult renders the approvals envelope. Exit codes
// follow the global contract: 0 review accepted or nothing pending,
// 1 pending gates exist awaiting review (list form), 2 anything else
// with a typed error.kind refusal on stdout.
func writeApprovalsResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime) error {
	format := render.Format(options.Format)
	result := render.ApprovalsResult{
		Repo:    firstNonEmpty(options.Repo, ""),
		RunID:   options.RunID,
		Pending: []render.PendingDeployment{},
	}
	reviewRequested := options.Approve || options.Reject
	refuse := func(err error) error {
		kind, message := approvalsErrorKind(err)
		accepted := false
		result.Accepted = &accepted
		result.Error = &render.MutationError{Kind: kind, Message: message}
		if writeErr := render.WriteApprovals(w, format, result); writeErr != nil {
			return writeErr
		}
		return outcomeError{code: render.ExitError}
	}
	if options.RunID <= 0 {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Message: "--run <run-id> (a positive ID) is required"})
	}
	if options.Approve && options.Reject {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Message: "--approve and --reject are mutually exclusive"})
	}
	if !reviewRequested && (len(options.Envs) > 0 || options.Comment != "") {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Message: "--env and --comment require --approve or --reject"})
	}

	var githubClient usecase.GitHub
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			return refuse(usecase.ActionError{Kind: usecase.ActionErrorNetwork, Message: "github api unavailable"})
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		result.Repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return refuse(err)
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		result.Repo = target.repo
	}

	service := usecase.ApprovalsService{
		GitHub:  githubClient,
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second},
	}
	if !reviewRequested {
		pending, err := service.List(ctx, result.Repo, options.RunID)
		if err != nil {
			return refuse(err)
		}
		result.Pending = mapRenderPendingDeployments(pending)
		if err := render.WriteApprovals(w, format, result); err != nil {
			return err
		}
		if len(result.Pending) > 0 {
			// The actionable state: gates exist and await review.
			return outcomeError{code: render.ExitActionNeeded}
		}
		return nil
	}

	outcome, err := service.Review(ctx, result.Repo, options.RunID, usecase.DeploymentReviewRequest{
		Environments: options.Envs,
		Approve:      options.Approve,
		Comment:      options.Comment,
	})
	if err != nil {
		return refuse(err)
	}
	accepted := true
	result.Accepted = &accepted
	result.Reviewed = &render.DeploymentReview{
		State:        string(outcome.State),
		Environments: outcome.Environments,
		Comment:      outcome.Comment,
	}
	return render.WriteApprovals(w, format, result)
}

// approvalsErrorKind maps any error to the typed refusal taxonomy.
func approvalsErrorKind(err error) (string, string) {
	if actionErr, ok := usecase.AsActionError(err); ok {
		return string(actionErr.Kind), actionErr.Error()
	}
	var apiErr usecase.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Kind {
		case usecase.APIErrorAuth, usecase.APIErrorPermission:
			return string(usecase.ActionErrorPermission), apiErr.Error()
		case usecase.APIErrorRateLimit:
			return string(usecase.ActionErrorRateLimit), apiErr.Error()
		case usecase.APIErrorNetwork:
			return string(usecase.ActionErrorNetwork), apiErr.Error()
		}
		return string(usecase.ActionErrorUnknown), apiErr.Error()
	}
	return string(usecase.ActionErrorUnknown), err.Error()
}

// cachesErrorKind maps any error to the typed refusal taxonomy —
// list/usage failures arrive as usecase.APIError, deletes as
// ActionError, and both must keep the envelope contract typed.
func cachesErrorKind(err error) (string, string) {
	if actionErr, ok := usecase.AsActionError(err); ok {
		return string(actionErr.Kind), actionErr.Message
	}
	var apiErr usecase.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Kind {
		case usecase.APIErrorAuth, usecase.APIErrorPermission:
			return string(usecase.ActionErrorPermission), apiErr.Error()
		case usecase.APIErrorRateLimit:
			return string(usecase.ActionErrorRateLimit), apiErr.Error()
		case usecase.APIErrorNetwork:
			return string(usecase.ActionErrorNetwork), apiErr.Error()
		case usecase.APIErrorNotFound:
			return string(usecase.ActionErrorNotFound), apiErr.Error()
		}
		return string(usecase.ActionErrorUnknown), apiErr.Error()
	}
	return string(usecase.ActionErrorUnknown), err.Error()
}

func mapRenderPendingDeployments(pending []model.PendingDeployment) []render.PendingDeployment {
	out := make([]render.PendingDeployment, 0, len(pending))
	for _, gate := range pending {
		reviewers := make([]render.DeploymentReviewer, 0, len(gate.Reviewers))
		for _, reviewer := range gate.Reviewers {
			reviewers = append(reviewers, render.DeploymentReviewer{Type: reviewer.Type, Name: reviewer.Name})
		}
		out = append(out, render.PendingDeployment{
			EnvironmentID:         gate.EnvironmentID,
			Environment:           gate.EnvironmentName,
			WaitTimer:             gate.WaitTimer,
			CurrentUserCanApprove: gate.CurrentUserCanApprove,
			Reviewers:             reviewers,
		})
	}
	return out
}

// newCachesCommand is the kennel's pipe surface. Deletion goes
// through unambiguous flags — numeric cache keys are legal, so a
// shared positional operand would be a foot-gun.
func newCachesCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var request cacheDeleteRequest
	cmd := &cobra.Command{
		Use:   "caches",
		Short: "List the Actions cache kennel or dig up stale entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			return writeCachesResult(cmd.Context(), runtime.Stdout, *options, runtime, request)
		},
	}
	cmd.Flags().Int64Var(&request.id, "delete-id", 0, "delete one cache by its numeric ID")
	cmd.Flags().StringVar(&request.key, "delete-key", "", "delete every cache with exactly this key (all refs unless --ref narrows)")
	cmd.Flags().StringVar(&request.ref, "ref", "", "limit --delete-key to one git ref (refs/heads/main)")
	return cmd
}

// cacheDeleteRequest is the resolved intent of a caches invocation's
// delete flags; the zero value means list-only.
type cacheDeleteRequest struct {
	id  int64
	key string
	ref string
}

func writeCachesResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime, request cacheDeleteRequest) error {
	format := render.Format(options.Format)
	result := render.CachesResult{Repo: firstNonEmpty(options.Repo, "")}

	// Every refusal writes the envelope so agents branch on error.kind
	// on stdout; exit 2 is never a bare stderr message.
	refuse := func(deleted *render.CacheDeletion, err error) error {
		kind, message := cachesErrorKind(err)
		field := ""
		if actionErr, ok := usecase.AsActionError(err); ok {
			field = actionErr.Field
		}
		result.Deleted = deleted
		result.Error = &render.MutationError{Kind: kind, Field: field, Message: message}
		if writeErr := render.WriteCaches(w, format, result); writeErr != nil {
			return writeErr
		}
		return outcomeError{code: render.ExitError}
	}

	deletion := requestedCacheDeletion(request)
	if request.id != 0 && request.key != "" {
		return refuse(deletion, usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "delete", Message: "--delete-id and --delete-key are mutually exclusive"})
	}
	if request.id < 0 {
		return refuse(deletion, usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "id", Message: "--delete-id must be a positive cache ID"})
	}
	if request.ref != "" && request.key == "" {
		return refuse(deletion, usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "ref", Message: "--ref only narrows --delete-key"})
	}

	var githubClient usecase.GitHub
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			return refuse(deletion, usecase.ActionError{Kind: usecase.ActionErrorNetwork, Message: "github api unavailable"})
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		result.Repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return refuse(deletion, err)
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		result.Repo = target.repo
	}

	cachesClient, ok := githubClient.(usecase.GitHubCaches)
	if !ok {
		return refuse(deletion, usecase.ActionError{Kind: usecase.ActionErrorUnknown, Message: "this adapter cannot reach the cache kennel"})
	}
	service := usecase.CachesService{
		GitHub:  cachesClient,
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second},
	}

	if deletion != nil {
		var count int
		var err error
		if request.id != 0 {
			count, err = service.DeleteByID(ctx, result.Repo, request.id)
		} else {
			count, err = service.DeleteByKey(ctx, result.Repo, request.key, request.ref)
		}
		if err != nil {
			return refuse(deletion, err)
		}
		deletion.Accepted = true
		deletion.DeletedCount = count
		result.Deleted = deletion
		return render.WriteCaches(w, format, result)
	}

	usage, err := service.Usage(ctx, result.Repo)
	if err != nil {
		return refuse(nil, err)
	}
	caches, err := service.List(ctx, result.Repo, usecase.CacheFilter{})
	if err != nil {
		return refuse(nil, err)
	}
	result.Usage = &render.CacheUsage{
		ActiveSizeInBytes: usage.ActiveSizeInBytes,
		ActiveCount:       usage.ActiveCount,
		CapBytes:          service.Cap(ctx, result.Repo),
	}
	result.Caches = mapRenderCaches(caches)
	return render.WriteCaches(w, format, result)
}

// requestedCacheDeletion names the delete intent for the envelope, or
// nil when the invocation is list-only.
func requestedCacheDeletion(request cacheDeleteRequest) *render.CacheDeletion {
	switch {
	case request.id != 0:
		return &render.CacheDeletion{Action: "delete_id", ID: request.id}
	case request.key != "":
		return &render.CacheDeletion{Action: "delete_key", Key: request.key, Ref: request.ref}
	case request.ref != "":
		return &render.CacheDeletion{Action: "delete_key", Ref: request.ref}
	default:
		return nil
	}
}

func mapRenderCaches(caches []model.Cache) []render.Cache {
	out := make([]render.Cache, 0, len(caches))
	for _, cache := range caches {
		out = append(out, render.Cache{
			ID:             cache.ID,
			Key:            cache.Key,
			Ref:            cache.Ref,
			SizeInBytes:    cache.SizeInBytes,
			LastAccessedAt: cache.LastAccessedAt,
			CreatedAt:      cache.CreatedAt,
		})
	}
	return out
}

func newWatchCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var group bool
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch the current or selected run",
		Long: "Watch the current or selected run.\n\n" +
			"--group is the blocking/await mode: it streams the whole event group " +
			"as NDJSON and blocks until the hunt settles (exit 0 home, 1 lost). " +
			"Plain `watch --json` only snapshots the active run (exit 3 while pending). " +
			"For unattended agents, bound the block with --timeout <duration>: on " +
			"expiry the stream closes with a `timed_out:true` summary and exits 3 " +
			"if runs are still in flight, or 1 if a member has already been lost " +
			"(worst-outcome, same as a clean settle). The deadline bounds an " +
			"in-flight poll too, so a hung GitHub call can't outlast it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			// watch follows the active run whoever started it; an actor
			// from HOUND_ACTOR is for runs listings only.
			options.Actor, options.ActorFromEnv = "", false
			options.NoTUI = true
			options.Watch = true
			if options.WatchTimeout < 0 {
				return errors.New("--timeout must be a non-negative duration")
			}
			if group {
				return writeGroupWatchResult(cmd.Context(), runtime.Stdout, *options, runtime)
			}
			if options.WatchTimeout > 0 {
				return errors.New("--timeout bounds the blocking --group hunt; plain watch only snapshots — pass --group")
			}
			options.Status = "in_progress"
			return writeResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().BoolVar(&group, "group", false, "watch the whole event group (same head_sha + event) as NDJSON, blocking until the hunt settles")
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "anchor the group on a specific run ID (default: the newest run in scope)")
	cmd.Flags().DurationVar(&options.WatchTimeout, "timeout", 0, "bound the --group block (e.g. 10m); on expiry close with a timed_out summary and exit 3 in-flight / 1 if lost (0 = unbounded)")
	return cmd
}

// writeGroupWatchResult is the agent-facing pack watch: NDJSON state
// transitions per run until the group settles, then one terminal
// summary object. Exit code = worst outcome (existing semantics:
// 1 any lost, 0 all home).
func writeGroupWatchResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime) error {
	format := render.Format(options.Format)
	if format != "" && format != render.FormatJSON {
		return fmt.Errorf("watch --group emits NDJSON only; --format %s is not supported", format)
	}
	cfg, err := defaultConfig(runtime.Env)
	if err != nil {
		return err
	}

	var githubClient usecase.GitHub
	var repo, branch string
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			return errors.New("github api unavailable")
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
		branch = firstNonEmpty(options.Branch, "main")
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return err
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		repo, branch = target.repo, target.branch
	}

	listed, err := githubClient.ListRuns(ctx, usecase.RunFilter{Repo: repo, Branch: branch, PerPage: cfg.PerPage})
	if err != nil {
		return err
	}
	var anchor model.Run
	if options.RunID != 0 {
		for _, run := range listed {
			if run.ID == options.RunID {
				anchor = run
				break
			}
		}
		if anchor.ID == 0 {
			anchor, err = githubClient.GetRun(ctx, repo, options.RunID)
			if err != nil {
				return err
			}
		}
		if anchor.ID == 0 {
			return fmt.Errorf("run %d not found", options.RunID)
		}
	} else if len(listed) == 0 {
		return errors.New("no runs found to watch; pass --run <run-id> or push something")
	} else {
		// Anchor on the newest still-live run (the pack worth watching);
		// when everything has settled, fall back to the newest run.
		anchor = listed[0]
		for _, run := range listed {
			if run.Status != model.StatusCompleted {
				anchor = run
				break
			}
		}
	}

	service := usecase.PackWatchService{Runs: githubClient, MinPoll: cfg.PollMin, MaxPoll: cfg.PollMax}
	packBranch := firstNonEmptyString(anchor.HeadBranch, branch)
	if options.RunID != 0 {
		packBranch = strings.TrimSpace(anchor.HeadBranch)
	}
	state := usecase.PackState{
		Repo:    repo,
		HeadSHA: anchor.HeadSHA,
		Event:   anchor.Event,
		// The anchor's own branch scopes the ticks: a --run anchor can
		// live on a branch the launch never resolved, and listing the
		// wrong branch would never refresh it (ghent Codex P2).
		Branch: packBranch,
		Max:    cfg.WatchGroupMax,
		Runs:   usecase.PackForRun(listed, anchor, cfg.WatchGroupMax),
	}

	// Emit a transition line only when a run's state actually moved —
	// agents tail the stream, repeats are noise.
	seen := map[int64]string{}
	emit := func(runs []model.Run) error {
		now := time.Now().UTC()
		for _, run := range runs {
			key := string(run.Status) + "/" + string(run.Conclusion)
			if seen[run.ID] == key {
				continue
			}
			seen[run.ID] = key
			event := render.GroupEvent{
				TS:         now,
				RunID:      run.ID,
				Workflow:   firstNonEmpty(run.Name, run.Path, run.DisplayTitle),
				Status:     string(run.Status),
				Conclusion: string(run.Conclusion),
			}
			if err := render.WriteGroupEvent(w, event); err != nil {
				return err
			}
		}
		return nil
	}
	if err := emit(state.Runs); err != nil {
		return err
	}

	// A bounded --timeout closes the hunt early so an unattended agent
	// never blocks forever on a queued/stuck run. The deadline is wired
	// into the polling context so it bounds an in-flight Tick (a hung
	// GitHub call) too, not just the sleep between polls. When the
	// timeout is unset, watchCtx == ctx and the loop blocks exactly as
	// before. The initial anchor listing above stays on the parent ctx:
	// it is a single fast call, and the bound is about the hunt.
	watchCtx := ctx
	if options.WatchTimeout > 0 {
		var cancel context.CancelFunc
		watchCtx, cancel = context.WithTimeout(ctx, options.WatchTimeout)
		defer cancel()
	}

	timedOut := false
	for !packSettled(state.Runs) {
		wait := state.NextPoll
		if wait <= 0 {
			wait = cfg.PollMin
		}
		pollTimer := time.NewTimer(wait)
		select {
		case <-watchCtx.Done():
			pollTimer.Stop()
			// watchCtx is derived from ctx, so a parent cancel (Ctrl-C)
			// trips it too — only our own --timeout deadline is a graceful
			// close; a parent cancel is a hard stop.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			timedOut = true
		case <-pollTimer.C:
		}
		if timedOut {
			break
		}
		state, err = service.Tick(watchCtx, state)
		if err != nil {
			// A Tick the --timeout deadline cancelled is a graceful close,
			// not an API error — fall through to the timed_out summary
			// instead of surfacing context.DeadlineExceeded as exit 2.
			if options.WatchTimeout > 0 && watchCtx.Err() != nil && ctx.Err() == nil {
				timedOut = true
				break
			}
			return err
		}
		if err := emit(state.Runs); err != nil {
			return err
		}
	}

	summary := usecase.SummarizePack(state.Runs)
	if err := render.WriteGroupSummary(w, render.GroupSummary{
		TS:       time.Now().UTC(),
		Repo:     repo,
		HeadSHA:  state.HeadSHA,
		Event:    state.Event,
		Runs:     len(state.Runs),
		Running:  summary.Running,
		Home:     summary.Home,
		Lost:     summary.Lost,
		TimedOut: timedOut,
	}); err != nil {
		return err
	}
	// Exit follows the same worst-outcome contract as a clean settle: a
	// lost member is a lost hunt (exit 1) whether or not its siblings
	// finished, so a --timeout that fires with a run already lost still
	// exits 1. Only an in-flight-but-nothing-lost timeout downgrades to
	// the pending code (3) — the "still hunting" signal the snapshot path
	// uses. Either way the timed_out marker on the summary tells agents
	// the hunt was cut short rather than settled.
	if timedOut {
		if summary.Lost > 0 {
			return outcomeError{code: render.ExitActionNeeded}
		}
		return outcomeError{code: render.ExitPending}
	}
	if code := render.ExitCode(render.Result{Runs: mapRenderRuns(state.Runs)}, nil); code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

func packSettled(runs []model.Run) bool {
	if len(runs) == 0 {
		return false
	}
	for _, run := range runs {
		if run.Status != model.StatusCompleted {
			return false
		}
	}
	return true
}

func newDispatchCommand(runtime commandRuntime, info buildInfo, options *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "dispatch",
		Short: "Trigger a workflow_dispatch workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			if options.NoTUI || options.JSON || !runtime.IsTTY {
				// The dispatch form is interactive; a flag-driven pipe
				// dispatch verb is planned (spec 240 conventions) but
				// not built — refusing beats silently printing runs.
				return errors.New("dispatch is interactive; run it in a terminal TUI (a flag-driven dispatch verb is planned)")
			}
			options.LaunchRoute = usecase.LaunchRouteDispatch
			return runTUI(cmd.Context(), runtime, info, *options)
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
	cfg, keymap, err := tuiConfig(runtime.Env)
	if err != nil {
		return err
	}
	// Raw input and the resize watch come before anything slow: keys
	// typed while the launch resolves are neither echoed nor lost, and
	// a resize after the size is read still arrives.
	restore, err := rawInput(runtime.Stdin, runtime.IsTTY)
	if err != nil {
		return err
	}
	defer restore()
	resizeEvents, stopResize := resizeSignals()
	defer stopResize()
	// Raw mode and the alt screen are undone by deferred calls, so a
	// termination signal must end the loop, not the process. It cancels
	// the context everything runs on, so a request stuck inside the
	// loop (a confirmed action) returns and the loop can exit.
	quitSignals := make(chan os.Signal, 1)
	signal.Notify(quitSignals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(quitSignals)
	ctx, quit := context.WithCancelCause(ctx)
	defer quit(nil)
	go func() {
		select {
		case sig := <-quitSignals:
			quit(signalExit(sig))
		case <-ctx.Done():
		}
	}()
	width, height := terminalSize(runtime.Stdout)
	out := runtime.Stdout
	if runtime.IsTTY {
		out = tuiOutput(runtime.Stdout, runtime.Env)
		// Alt screen + hidden cursor for the app's lifetime: scrollback
		// stays clean and the cursor never strobes during repaints.
		if _, err := io.WriteString(out, enterAlt+hideCursor); err != nil {
			return err
		}
		defer func() {
			_, _ = io.WriteString(out, showCursor+leaveAlt)
		}()
	}

	renderer := newFrameRenderer(out)
	renderer.SetSize(width, height)
	startup := tui.NewStartup(cfg, options.Repo)
	var screen interface {
		ViewSize(width, height int) string
	} = startup
	render := func() error {
		if runtime.IsTTY {
			return renderer.Render(screen.ViewSize(width, height))
		}
		_, err := fmt.Fprintln(runtime.Stdout, screen.ViewSize(width, height))
		return err
	}
	resize := func() {
		width, height = terminalSize(runtime.Stdout)
		// The terminal reflowed: the previous frame's rows no longer
		// correspond, so the diff must start over.
		renderer.SetSize(width, height)
		renderer.Invalidate()
	}
	if err := render(); err != nil {
		return err
	}

	// Building the app resolves the launch over the network: the
	// startup frame keeps painting meanwhile, and keys typed before the
	// app exists are replayed into it.
	type built struct {
		app tui.App
		err error
	}
	appCtx, cancelApp := context.WithCancel(ctx)
	defer cancelApp()
	ready := make(chan built, 1)
	go func() {
		app, err := defaultTUIApp(appCtx, preparedRuntime, build, options)
		ready <- built{app, err}
	}()
	events := readKeys(runtime.Stdin)
	ticker := time.NewTicker(startup.PollInterval())
	defer ticker.Stop()
	var typed []string
	var app tui.App
	inputEnded := false
	for starting := true; starting; {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-resizeEvents:
			resize()
			if err := render(); err != nil {
				return err
			}
		case event := <-events:
			if event.err != nil {
				if !errors.Is(event.err, io.EOF) {
					return event.err
				}
				// The keys before EOF still apply once the app exists.
				inputEnded, events = true, nil
			} else if event.key == "ctrl+c" || (event.key == keymap.KeyFor("quit") && len(typed) == 0) {
				// A first q would quit on any launch route once replayed.
				return nil
			} else {
				typed = append(typed, event.key)
			}
		case <-ticker.C:
			// Only a terminal animates; elsewhere each frame is printed.
			if !runtime.IsTTY {
				continue
			}
			if err := render(); err != nil {
				return err
			}
		case result := <-ready:
			if result.err != nil {
				return result.err
			}
			app, starting = result.app, false
		}
	}
	// Cancel in-flight background fetches on every exit path (quit,
	// EOF, ctx cancellation) so their goroutines don't outlive the
	// loop. The closure reads the latest app value at return time.
	defer func() { app.Shutdown() }()
	app = app.WithViewport(width, height)
	screen = &app
	if err := render(); err != nil {
		return err
	}
	for _, key := range typed {
		if app.ShouldQuit() {
			return nil
		}
		var handled bool
		if app, handled = app.Update(tui.KeyMsg{Key: key}); handled {
			if err := render(); err != nil {
				return err
			}
		}
	}
	if inputEnded {
		return nil
	}

	ticker.Reset(app.PollInterval())
	for !app.ShouldQuit() {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-resizeEvents:
			resize()
			app = app.WithViewport(width, height)
			if err := render(); err != nil {
				return err
			}
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

// signalExit reports a signal the shell way: 128 plus its number.
func signalExit(sig os.Signal) error {
	if number, ok := sig.(syscall.Signal); ok {
		return outcomeError{code: 128 + int(number)}
	}
	return outcomeError{code: render.ExitError}
}

type keyEvent struct {
	key string
	err error
}

// escDelay is how long a lone ESC waits for the rest of an escape
// sequence before it counts as the esc key (vim's ttimeout, tmux's
// escape-time): how full a read was says nothing about what follows.
const escDelay = 50 * time.Millisecond

func readKeys(reader io.Reader) <-chan keyEvent {
	type chunk struct {
		data []byte
		err  error
	}
	chunks := make(chan chunk)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := reader.Read(buf)
			chunks <- chunk{bytes.Clone(buf[:n]), err}
			if err != nil {
				return
			}
		}
	}()
	events := make(chan keyEvent, 1)
	go func() {
		decoder := keyDecoder{}
		for {
			if key, ok := decoder.pop(); ok {
				events <- keyEvent{key: key}
				continue
			}
			var escWait <-chan time.Time
			if decoder.escPending() {
				escWait = time.After(escDelay)
			}
			select {
			case c := <-chunks:
				decoder.pending = append(decoder.pending, c.data...)
				if c.err != nil {
					for key, ok := decoder.flush(); ok; key, ok = decoder.flush() {
						events <- keyEvent{key: key}
					}
					events <- keyEvent{err: c.err}
					return
				}
			case <-escWait:
				key, _ := decoder.flush()
				events <- keyEvent{key: key}
			}
		}
	}()
	return events
}

type keyDecoder struct {
	pending []byte
	err     error
}

// Next decodes synchronously; when input ends, what can no longer
// complete is flushed (a lone ESC is the esc key).
func (d *keyDecoder) Next(reader io.Reader, scratch []byte) (string, error) {
	if len(scratch) == 0 {
		scratch = make([]byte, 8)
	}
	for {
		if key, ok := d.pop(); ok {
			return key, nil
		}
		if d.err != nil {
			if key, ok := d.flush(); ok {
				return key, nil
			}
			return "", d.err
		}
		n, err := reader.Read(scratch)
		d.pending = append(d.pending, scratch[:n]...)
		d.err = err
	}
}

// escPending reports an ESC waiting for the rest of its sequence.
func (d *keyDecoder) escPending() bool {
	return len(d.pending) > 0 && d.pending[0] == 0x1b
}

// flush gives up waiting: a pending ESC is the esc key, and the rest
// decodes on its own; a partial rune is dropped.
func (d *keyDecoder) flush() (string, bool) {
	if d.escPending() {
		d.pending = d.pending[1:]
		return "esc", true
	}
	if key, ok := d.pop(); ok {
		return key, true
	}
	d.pending = nil
	return "", false
}

func (d *keyDecoder) pop() (string, bool) {
	for len(d.pending) > 0 {
		if n := escapeSequence(d.pending); n < 0 || string(d.pending) == "\x1b" {
			return "", false
		} else if n > 0 {
			key := keyName(d.pending[:n])
			d.pending = d.pending[n:]
			if key != "" {
				return key, true
			}
			continue
		}
		if !utf8.FullRune(d.pending) {
			// The rest of the rune is still in flight.
			return "", false
		}
		r, size := utf8.DecodeRune(d.pending)
		key := keyName(d.pending[:size])
		d.pending = d.pending[size:]
		if key != "" && (r != utf8.RuneError || size > 1) {
			return key, true
		}
	}
	return "", false
}

// escapeSequence measures the CSI (ESC [) or SS3 (ESC O) sequence that
// starts b: 0 when none does, -1 while its final byte is in flight.
func escapeSequence(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b || (b[1] != '[' && b[1] != 'O') {
		return 0
	}
	if b[1] == 'O' {
		if len(b) < 3 {
			return -1
		}
		return 3
	}
	for i := 2; i < len(b); i++ {
		switch c := b[i]; {
		case c >= 0x40 && c <= 0x7e:
			return i + 1
		case c < 0x20 || c > 0x3f:
			return 0
		}
	}
	return -1
}

func defaultTUIApp(ctx context.Context, runtime commandRuntime, build tui.BuildInfo, options cliOptions) (tui.App, error) {
	cfg, keymap, err := tuiConfig(runtime.Env)
	if err != nil {
		return tui.App{}, err
	}
	actor := ""
	if options.Actor != "" {
		if actor, err = usecase.NormalizeActor(options.Actor); err != nil {
			return tui.App{}, fmt.Errorf("--actor: %w", err)
		}
	}
	githubClient := githubClientForRuntime(runtime)
	repoProvider := repoProviderForRuntime(runtime)
	// One limiter paces every mutation the TUI can fire — reruns,
	// cancels, and deployment reviews share the same budget.
	mutationLimiter := &usecase.MutationLimiter{MinSpacing: time.Second}
	actionService := usecase.ActionService{
		GitHub:  githubClient,
		Limiter: mutationLimiter,
	}
	approvalsService := usecase.ApprovalsService{
		GitHub:  githubClient,
		Limiter: mutationLimiter,
	}
	workflowsService := usecase.WorkflowsService{
		GitHub:  githubClient,
		Limiter: mutationLimiter,
	}
	failureService := usecase.FailureService{GitHub: githubClient}
	watchSession := &usecase.WatchSession{Service: usecase.WatchService{GitHub: githubClient, MinPoll: cfg.PollMin, MaxPoll: cfg.PollMax}}
	launch := usecase.LaunchService{
		Config:     cfg,
		GitHub:     githubClient,
		Repository: repoProvider,
	}.Resolve(ctx, usecase.LaunchRequest{
		Repo:    options.Repo,
		Branch:  options.Branch,
		All:     options.All,
		Route:   options.LaunchRoute,
		PerPage: cfg.PerPage,
		Actor:   actor,
	})
	return tui.NewApp(tui.Options{
		Config: cfg,
		Keymap: keymap,
		Build:  build,
		Launch: launch,
		RunsResolver: func(loadCtx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
			if filter.PerPage == 0 {
				filter.PerPage = cfg.PerPage
			}
			return githubClient.ListRuns(loadCtx, filter)
		},
		ViewerResolver: viewerResolver(githubClient),
		RunsMetadata: func() (usecase.RequestMeta, bool) {
			diagnostics, ok := githubClient.(usecase.GitHubDiagnostics)
			if !ok {
				return usecase.RequestMeta{}, false
			}
			return diagnostics.LastRequestMeta(githubRunsResource(launch.Repo))
		},
		LogRefetchNotice: func(jobID int64) (usecase.LogRefetchNotice, bool) {
			diagnostics, ok := githubClient.(usecase.GitHubLogDiagnostics)
			if !ok {
				return usecase.LogRefetchNotice{}, false
			}
			return diagnostics.LastLogRefetch(jobID)
		},
		DetailResolver: func(loadCtx context.Context, run model.Run) (detail.Model, error) {
			if run.Status != model.StatusCompleted {
				// A live run moves on: its status (and whether it still
				// waits at a gate) is re-read, so the detail poll knows
				// when it ends. The jobs are the screen; a failed re-read
				// keeps the snapshot and the next poll tries again.
				if fresh, err := githubClient.GetRun(loadCtx, launch.Repo, run.ID); err == nil && fresh.ID == run.ID {
					run = fresh
				}
			}
			jobs, err := githubClient.ListJobs(loadCtx, launch.Repo, run.ID)
			if err != nil {
				return detail.Model{}, err
			}
			resolved := detail.NewModel(run, jobs).WithRepo(launch.Repo)
			if run.Status == model.StatusWaiting {
				// The gate panel is auxiliary: a pending-deployments
				// failure must not take the whole detail screen down.
				if pending, pendingErr := approvalsService.List(loadCtx, launch.Repo, run.ID); pendingErr == nil {
					resolved = resolved.WithPendingDeployments(pending)
				}
			}
			return resolved, nil
		},
		FailureResolver: func(loadCtx context.Context, run model.Run, selected model.Job) (failurescreen.Model, logscreen.Model, error) {
			job, err := resolveJobForRun(loadCtx, githubClient, launch.Repo, run, selected)
			if err != nil {
				return failurescreen.Model{}, logscreen.Model{}, err
			}
			report, err := failureService.LoadFailure(loadCtx, launch.Repo, job)
			if err != nil {
				return failurescreen.Model{}, logscreen.Model{}, err
			}
			fullLog := logscreen.Fold(loadCtx, logscreen.NewModel(report.Log, 1, 6))
			return failurescreen.NewModel(launch.Repo, run.ID, report), fullLog.JumpToLine(report.Log.Failure.AnchorLine), nil
		},
		LogResolver: func(loadCtx context.Context, run model.Run, selected model.Job, progress func(read, total int64)) (logscreen.Model, error) {
			job, err := resolveJobForRun(loadCtx, githubClient, launch.Repo, run, selected)
			if err != nil {
				return logscreen.Model{}, err
			}
			var raw string
			// Byte progress is a capability, mirroring GitHubLogDiagnostics:
			// adapters without it fall back to the plain fetch and the
			// indeterminate spinner.
			if fetcher, ok := githubClient.(usecase.LogProgressFetcher); ok && progress != nil {
				raw, err = fetcher.FetchJobLogWithProgress(loadCtx, launch.Repo, job.ID, progress)
			} else {
				raw, err = githubClient.FetchJobLog(loadCtx, launch.Repo, job.ID)
			}
			if err != nil {
				return logscreen.Model{}, err
			}
			return logscreen.Fold(loadCtx, logscreen.NewModel(logs.Parse(raw), 1, 6)), nil
		},
		WatchResolver: func(loadCtx context.Context, run model.Run) (watch.Model, error) {
			state, err := watchSession.Tick(loadCtx, launch.Repo, run)
			if err != nil {
				return watch.Model{}, err
			}
			if state.Run.ID == 0 {
				state.Run = run
			}
			noLog := make([]watch.NoLog, 0, len(state.NoLog))
			for _, missing := range state.NoLog {
				noLog = append(noLog, watch.NoLog{Job: missing.Job.Name, Expired: missing.Expired})
			}
			return watch.NewModel(watch.State{
				Repo:    launch.Repo,
				Branch:  firstNonEmptyString(launch.Branch, run.HeadBranch),
				Run:     state.Run,
				Jobs:    state.Jobs,
				Lines:   state.Lines,
				NoLog:   noLog,
				Elapsed: elapsedRun(state.Run),
			}), nil
		},
		PackResolver: func(loadCtx context.Context, state usecase.PackState) (usecase.PackState, error) {
			// One head_sha-filtered list call per tick covers the whole
			// board (usecase.PackWatchService budget).
			service := usecase.PackWatchService{Runs: githubClient, MinPoll: cfg.PollMin, MaxPoll: cfg.PollMax}
			return service.Tick(loadCtx, state)
		},
		DispatchAttachResolver: func(loadCtx context.Context, workflowID, ref string, since time.Time) (model.Run, error) {
			// 204-fallback ONLY: modern hosts hand the run id back in the
			// dispatch body and never reach this resolver.
			history, ok := githubClient.(usecase.WorkflowRunHistory)
			if !ok {
				return model.Run{}, fmt.Errorf("run discovery is unavailable for this adapter")
			}
			service := usecase.HandoffService{History: history}
			return service.DiscoverDispatchedRun(loadCtx, launch.Repo, workflowID, ref, since)
		},
		RerunAttachResolver: func(loadCtx context.Context, run model.Run) (model.Run, error) {
			service := usecase.HandoffService{Runs: githubClient}
			return service.AwaitRerunStart(loadCtx, launch.Repo, run.ID, run.RunAttempt)
		},
		DispatchResolver: func(loadCtx context.Context) (dispatch.Model, error) {
			workflows, err := dispatchWorkflowModels(loadCtx, githubClient, launch)
			if err != nil {
				return dispatch.Model{}, err
			}
			if len(workflows) == 0 {
				return dispatch.Model{}, fmt.Errorf("no workflow_dispatch workflows found")
			}
			return dispatch.NewModel(workflows[0]), nil
		},
		DiffResolver: func(loadCtx context.Context, run model.Run) (usecase.RegressionVerdict, error) {
			history, hasHistory := githubClient.(usecase.WorkflowRunHistory)
			comparer, hasComparer := githubClient.(usecase.CommitComparer)
			if !hasHistory || !hasComparer {
				return usecase.RegressionVerdict{}, fmt.Errorf("regression scan is unavailable for this adapter")
			}
			workflow := workflowSelectorForRun(run)
			if workflow == "" {
				return usecase.RegressionVerdict{}, fmt.Errorf("the selected run has no workflow identity to follow")
			}
			service := usecase.DiffService{
				History:  history,
				Compare:  comparer,
				MaxPages: cfg.DiffMaxPages,
				PerPage:  usecase.DiffPerPage,
			}
			// The trail is anchored to the SELECTED run: its branch wins
			// over the launch branch so a repo-wide or all-branches list
			// scans the right history (review-required hardening).
			return service.LocateRegression(loadCtx, launch.Repo, workflow, firstNonEmptyString(run.HeadBranch, launch.Branch))
		},
		FlakesResolver: func(loadCtx context.Context, run model.Run) (usecase.FlakeReport, error) {
			history, hasHistory := githubClient.(usecase.WorkflowRunHistory)
			if !hasHistory {
				return usecase.FlakeReport{}, fmt.Errorf("flake scan is unavailable for this adapter")
			}
			workflow := workflowSelectorForRun(run)
			if workflow == "" {
				return usecase.FlakeReport{}, fmt.Errorf("the selected run has no workflow identity to follow")
			}
			service := usecase.FlakesService{
				History:  history,
				Attempts: githubClient,
				Logs:     githubClient,
				Window:   cfg.FlakeWindow,
				PerPage:  usecase.DiffPerPage,
			}
			// The scent is anchored to the SELECTED run: its branch wins
			// over the launch branch (diff precedent).
			return service.Scan(loadCtx, launch.Repo, workflow, firstNonEmptyString(run.HeadBranch, launch.Branch))
		},
		QueueResolver: func(loadCtx context.Context, run model.Run) (model.ConcurrencyReport, error) {
			concurrency, ok := githubClient.(usecase.GitHubConcurrency)
			if !ok {
				return model.ConcurrencyReport{}, fmt.Errorf("this GitHub adapter does not expose Actions concurrency groups")
			}
			service := usecase.QueueService{GitHub: concurrency, Jobs: githubClient}
			return service.InspectRun(loadCtx, launch.Repo, run.ID)
		},
		DispatchWorkflowsResolver: func(loadCtx context.Context) ([]dispatch.Workflow, error) {
			workflows, err := dispatchWorkflowModels(loadCtx, githubClient, launch)
			if err != nil {
				return nil, err
			}
			return workflows, nil
		},
		WorkflowsResolver: func(loadCtx context.Context) ([]model.Workflow, error) {
			return workflowsService.List(loadCtx, launch.Repo)
		},
		ActionHandler: func(request tui.ActionRequest) (usecase.ActionResult, error) {
			switch request.Action {
			case usecase.ActionRerunRun:
				return actionService.RerunRun(ctx, launch.Repo, request.Run.ID, request.Debug)
			case usecase.ActionRerunFailedJobs:
				return actionService.RerunFailedJobs(ctx, launch.Repo, request.Run.ID, request.Debug)
			case usecase.ActionRerunJob:
				if request.Job.ID == 0 {
					return usecase.ActionResult{}, fmt.Errorf("job is not loaded")
				}
				return actionService.RerunJob(ctx, launch.Repo, request.Job.ID, request.Debug)
			case usecase.ActionCancelRun:
				return actionService.CancelRun(ctx, launch.Repo, request.Run.ID)
			case usecase.ActionForceCancelRun:
				return actionService.ForceCancelRun(ctx, launch.Repo, request.Run.ID)
			case usecase.ActionDispatch:
				if request.Workflow.ID == "" {
					return usecase.ActionResult{}, fmt.Errorf("workflow is not loaded")
				}
				validator, ok := githubClient.(usecase.RefValidator)
				if !ok {
					// Validation is part of the dispatch contract, not
					// an optional nicety: an adapter without it cannot
					// dispatch (review-required hardening).
					return usecase.ActionResult{}, usecase.ActionError{
						Kind:    usecase.ActionErrorValidation,
						Field:   "ref",
						Message: "ref validation is unavailable for this adapter; dispatch refused",
					}
				}
				exists, refErr := validator.RefExists(ctx, launch.Repo, request.Dispatch.Ref)
				if refErr != nil {
					return usecase.ActionResult{}, refErr
				}
				if !exists {
					return usecase.ActionResult{}, usecase.ActionError{
						Kind:    usecase.ActionErrorValidation,
						Field:   "ref",
						Message: fmt.Sprintf("ref %q isn't in this yard — pass an existing branch or tag", request.Dispatch.Ref),
					}
				}
				return actionService.DispatchWorkflow(ctx, launch.Repo, request.Workflow.ID, request.Dispatch)
			case usecase.ActionEnableWorkflow, usecase.ActionDisableWorkflow:
				if request.Workflow.ID == "" {
					return usecase.ActionResult{}, fmt.Errorf("workflow is not loaded")
				}
				if request.Action == usecase.ActionEnableWorkflow {
					return workflowsService.Enable(ctx, launch.Repo, request.Workflow.ID)
				}
				return workflowsService.Disable(ctx, launch.Repo, request.Workflow.ID)
			case usecase.ActionApproveDeployment, usecase.ActionRejectDeployment:
				outcome, err := approvalsService.Review(ctx, launch.Repo, request.Run.ID, usecase.DeploymentReviewRequest{
					Environments: request.Environments,
					Approve:      request.Action == usecase.ActionApproveDeployment,
					Comment:      request.Comment,
				})
				if err != nil {
					return usecase.ActionResult{}, err
				}
				return outcome.Result, nil
			default:
				return usecase.ActionResult{}, fmt.Errorf("unsupported action %q", request.Action)
			}
		},
		ApprovalsResolver: func(loadCtx context.Context, run model.Run) ([]model.PendingDeployment, error) {
			return approvalsService.List(loadCtx, launch.Repo, run.ID)
		},
		ArtifactsResolver: func(run model.Run) ([]model.Artifact, error) {
			return usecase.ArtifactsService{GitHub: githubClient}.List(ctx, launch.Repo, run.ID)
		},
		ArtifactDownloader: func(artifact model.Artifact, destDir string, force bool, onProgress func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
			return usecase.ArtifactsService{GitHub: githubClient}.Download(ctx, launch.Repo, artifact, destDir, force, onProgress)
		},
		ArtifactDir: artifactDownloadRoot(runtime.Env),
		CachesResolver: func(loadCtx context.Context) (caches.Data, error) {
			service, err := cachesServiceFor(githubClient, mutationLimiter)
			if err != nil {
				return caches.Data{}, err
			}
			usage, err := service.Usage(loadCtx, launch.Repo)
			if err != nil {
				return caches.Data{}, err
			}
			items, err := service.List(loadCtx, launch.Repo, usecase.CacheFilter{})
			if err != nil {
				return caches.Data{}, err
			}
			return caches.Data{Usage: usage, Caches: items, Cap: service.Cap(loadCtx, launch.Repo)}, nil
		},
		CacheDeleter: func(loadCtx context.Context, request tui.CacheDeleteRequest) (int, error) {
			service, err := cachesServiceFor(githubClient, mutationLimiter)
			if err != nil {
				return 0, err
			}
			if request.ID != 0 {
				return service.DeleteByID(loadCtx, launch.Repo, request.ID)
			}
			return service.DeleteByKey(loadCtx, launch.Repo, request.Key, "")
		},
		OpenURL:  openURLForRuntime(runtime),
		CopyText: copyTextForRuntime(runtime),
	}), nil
}

// cachesServiceFor builds the kennel service when the adapter has the
// capability. The caller passes its mutation limiter so cache deletes
// share the same one-per-second budget as reruns, cancels, and
// deployment reviews — a fresh limiter here would reset the pacing.
func cachesServiceFor(githubClient usecase.GitHub, limiter *usecase.MutationLimiter) (usecase.CachesService, error) {
	cachesClient, ok := githubClient.(usecase.GitHubCaches)
	if !ok {
		return usecase.CachesService{}, errors.New("this adapter cannot reach the cache kennel")
	}
	return usecase.CachesService{
		GitHub:  cachesClient,
		Limiter: limiter,
	}, nil
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

// artifactDownloadRoot resolves where TUI artifact downloads extract:
// $GH_HOUND_ARTIFACT_DIR when set, else the working directory — always
// absolute, because the TUI shows this path in confirms and rows and a
// relative "." would hide the one fact the user needs. env is the
// runtime's injected lookup so tests stay hermetic.
func artifactDownloadRoot(env func(string) (string, bool)) string {
	root := ""
	if env != nil {
		if value, ok := env("GH_HOUND_ARTIFACT_DIR"); ok {
			root = strings.TrimSpace(value)
		}
	}
	if root == "" {
		root = "."
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
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

func chooseDispatchWorkflows(ctx context.Context, githubClient usecase.GitHub, repo string, workflows []model.Workflow) ([]model.Workflow, error) {
	dispatchable := []model.Workflow{}
	for _, workflow := range workflows {
		// Toggleable disabled workflows (asleep/muzzled) stay visible —
		// the picker badges them and refuses selection with a pointer
		// at :workflows. Fork-disabled, deleted, and unknown states can
		// be neither dispatched nor woken from here.
		if workflow.State != "" && !workflow.Toggleable() {
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
			return nil, err
		}
		inputs, ok, err := usecase.ParseWorkflowDispatchInputs(raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		workflow.Inputs = inputs
		dispatchable = append(dispatchable, workflow)
	}
	if len(dispatchable) == 0 {
		return nil, fmt.Errorf("no workflow_dispatch workflows found")
	}
	return dispatchable, nil
}

func dispatchWorkflowModels(ctx context.Context, githubClient usecase.GitHub, launch usecase.LaunchContext) ([]dispatch.Workflow, error) {
	workflows := launch.Workflows
	var err error
	if len(workflows) == 0 {
		workflows, err = githubClient.ListWorkflows(ctx, launch.Repo)
		if err != nil {
			return nil, err
		}
	}
	dispatchable, err := chooseDispatchWorkflows(ctx, githubClient, launch.Repo, workflows)
	if err != nil {
		return nil, err
	}
	ref, err := dispatchRef(launch)
	if err != nil {
		// Foreign target without --branch: the target's own default
		// branch is the honest pre-fill (issue #15). Capability-gated;
		// fakes without it keep the explicit error.
		provider, ok := githubClient.(usecase.RepoInfoProvider)
		if !ok {
			return nil, err
		}
		ref, err = provider.DefaultBranch(ctx, launch.Repo)
		if err != nil {
			return nil, fmt.Errorf("dispatch ref is unavailable: %w", err)
		}
	}
	out := make([]dispatch.Workflow, 0, len(dispatchable))
	for _, workflow := range dispatchable {
		workflowName := workflowDisplayName(workflow)
		workflowID := workflowIdentifier(workflow)
		if workflowName == "" || workflowID == "" {
			return nil, fmt.Errorf("workflow metadata is incomplete")
		}
		out = append(out, dispatch.Workflow{
			Name:   workflowName,
			ID:     workflowID,
			Ref:    ref,
			Inputs: dispatchInputs(workflow.Inputs),
			State:  workflow.State,
		})
	}
	return out, nil
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

func githubRunsResource(repo string) string {
	return "/repos/" + strings.Trim(strings.TrimSpace(repo), "/") + "/actions/runs"
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

// viewerResolver exposes the client's /user lookup to the TUI, or nil
// when the client cannot report who it is.
func viewerResolver(githubClient usecase.GitHub) func(context.Context) (string, error) {
	viewer, ok := githubClient.(usecase.ViewerProvider)
	if !ok {
		return nil
	}
	return viewer.AuthenticatedLogin
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
	case "\x1b[A", "\x1bOA":
		return "up"
	case "\x1b[B", "\x1bOB":
		return "down"
	case "\x1b[C", "\x1bOC":
		return "right"
	case "\x1b[D", "\x1bOD":
		return "left"
	case "\x1b[Z":
		return "shift+tab"
	}
	if len(input) > 1 && input[0] == 0x1b {
		// An escape sequence for a key the TUI doesn't bind.
		return ""
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
	case ' ':
		// The symbolic name every Update handler matches on; text
		// inputs append " " from their explicit space cases.
		return "space"
	case 0x1b:
		return "esc"
	default:
		r, _ := utf8.DecodeRune(input)
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			// Unbound: typed into an input it would reach the terminal.
			return ""
		}
		return string(r)
	}
}

func defaultConfig(lookup ...func(string) (string, bool)) (config.Config, error) {
	env := os.LookupEnv
	if len(lookup) > 0 && lookup[0] != nil {
		env = lookup[0]
	}
	return config.Load(config.LoadOptions{Path: configPath(env), LookupEnv: env})
}

// tuiConfig is defaultConfig plus its [keybindings], which only the TUI
// reads: a bad override stops the launch like any bad config value.
func tuiConfig(env func(string) (string, bool)) (config.Config, keys.Keymap, error) {
	cfg, err := defaultConfig(env)
	if err != nil {
		return config.Config{}, keys.Keymap{}, err
	}
	// Keybindings only ever come from the file.
	keymap, err := keys.NewKeymap(cfg.Keybindings)
	if err != nil {
		return config.Config{}, keys.Keymap{}, fmt.Errorf("config %s: %w", configPath(env), err)
	}
	return cfg, keymap, nil
}

// configPath is $XDG_CONFIG_HOME/gh-hound/config.toml, else
// ~/.config/gh-hound/config.toml (the documented path on every OS, not
// os.UserConfigDir's). Empty when neither variable is set.
func configPath(lookup func(string) (string, bool)) string {
	// The XDG spec ignores a relative XDG_CONFIG_HOME.
	if dir, ok := lookup("XDG_CONFIG_HOME"); ok && filepath.IsAbs(dir) {
		return filepath.Join(dir, "gh-hound", "config.toml")
	}
	if home, ok := lookup("HOME"); ok && home != "" {
		return filepath.Join(home, ".config", "gh-hound", "config.toml")
	}
	return ""
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
	if options.RunID != 0 && options.ActorFromEnv {
		// --run always shows that run: HOUND_ACTOR does not apply.
		options.Actor = ""
	}
	if options.Actor != "" {
		if options.RunID != 0 {
			return render.Result{}, errors.New("--actor cannot be combined with --run: --run always shows that run")
		}
		actor, err := usecase.NormalizeActor(options.Actor)
		if err != nil {
			return render.Result{}, fmt.Errorf("--actor: %w", err)
		}
		options.Actor = actor
	}
	cfg, err := defaultConfig(runtime.Env)
	if err != nil {
		return render.Result{}, err
	}
	options.perPage = cfg.PerPage
	if options.Fake == "" {
		return liveResult(ctx, options, runtime)
	}
	scenario := normalizedScenario(options)
	if scenario == "api_error" || scenario == "network_error" || scenario == "rate_limited" {
		return render.Result{}, errors.New("github api unavailable")
	}
	if scenario == "noisy" {
		runtime.GitHub = fake.New(fake.ScenarioNoisy)
		runtime.Repo = staticRepositoryContext{context: usecase.RepositoryContext{
			Repo: firstNonEmpty(options.Repo, "indrasvat/gh-hound"), Branch: firstNonEmpty(options.Branch, "main"), Actor: "indrasvat",
		}}
		options.Fake = ""
		return liveResult(ctx, options, runtime)
	}
	return fakeResult(options, scenario), nil
}

type resolvedTarget struct {
	github usecase.GitHub
	repo   string
	branch string
	close  func() error
}

func resolveTarget(ctx context.Context, options cliOptions, runtime commandRuntime) (resolvedTarget, error) {
	preparedRuntime, closeTrace, err := runtimeWithGitHubClient(runtime, options)
	if err != nil {
		return resolvedTarget{}, err
	}
	githubClient := githubClientForRuntime(preparedRuntime)
	repoProvider := repoProviderForRuntime(preparedRuntime)

	repoCtx, err := repoProvider.Current(ctx)
	if err != nil && options.Repo == "" {
		_ = closeTrace()
		return resolvedTarget{}, fmt.Errorf("%s; pass -R owner/repo or set GH_REPO", err)
	}
	repo := firstNonEmpty(options.Repo, repoCtx.Repo)
	if repo == "" {
		_ = closeTrace()
		return resolvedTarget{}, errors.New("repository context could not be resolved; pass -R owner/repo or set GH_REPO")
	}
	// The local checkout branch only applies to the local repo: a
	// foreign -R target must not be filtered by a branch that likely
	// does not exist there (issue #15, pipe path).
	branch := strings.TrimSpace(options.Branch)
	if branch == "" && (options.Repo == "" || strings.EqualFold(strings.TrimSpace(options.Repo), strings.TrimSpace(repoCtx.Repo))) {
		branch = repoCtx.Branch
	}
	if options.All {
		branch = ""
	}
	return resolvedTarget{github: githubClient, repo: repo, branch: branch, close: closeTrace}, nil
}

func liveResult(ctx context.Context, options cliOptions, runtime commandRuntime) (render.Result, error) {
	target, err := resolveTarget(ctx, options, runtime)
	if err != nil {
		return render.Result{}, err
	}
	defer func() {
		_ = target.close()
	}()
	githubClient := target.github
	repo, branch := target.repo, target.branch

	if options.Attempt > 0 && options.RunID == 0 {
		return render.Result{}, errors.New("--attempt requires --run <run-id>")
	}
	if options.RunID != 0 {
		var run model.Run
		if options.Attempt > 0 {
			run, err = githubClient.GetRunAttempt(ctx, repo, options.RunID, options.Attempt)
		} else {
			run, err = githubClient.GetRun(ctx, repo, options.RunID)
		}
		if err != nil {
			return render.Result{}, err
		}
		singleRuns := []model.Run{run}
		renderSingle := mapRenderRuns(singleRuns)
		triage := usecase.TriageService{GitHub: githubClient}
		if failures, triageErr := triage.LoadRunFailuresAttempt(ctx, repo, run, options.Attempt); triageErr == nil && len(failures) > 0 {
			renderSingle[0].Failed = mapRenderFailures(failures)
		}
		if options.WithArtifacts {
			attachArtifacts(ctx, usecase.ArtifactsService{GitHub: githubClient}, repo, singleRuns, renderSingle)
		}
		if options.WithApprovals {
			attachApprovals(ctx, usecase.ApprovalsService{GitHub: githubClient}, repo, singleRuns, renderSingle)
		}
		return render.Result{Repo: repo, Branch: branch, Runs: renderSingle}, nil
	}

	filter := usecase.RunFilter{Repo: repo, Branch: branch, PerPage: options.perPage}
	if options.Actor != "" {
		login, err := usecase.ResolveActor(ctx, githubClient, options.Actor)
		if err != nil {
			return render.Result{}, fmt.Errorf("%w; pass --actor <login>", err)
		}
		filter.Actor = login
	}
	if options.Status != "" {
		status, err := parseStatusFilter(options.Status)
		if err != nil {
			return render.Result{}, err
		}
		filter.Status = status
	}
	runs, err := usecase.ListActorRuns(ctx, githubClient, filter)
	if err != nil {
		return render.Result{}, err
	}
	runs = filterRunsByConclusion(runs, options.Status)
	renderRuns := mapRenderRuns(runs)
	attachFailures(ctx, usecase.TriageService{GitHub: githubClient}, repo, runs, renderRuns)
	if options.WithArtifacts {
		attachArtifacts(ctx, usecase.ArtifactsService{GitHub: githubClient}, repo, runs, renderRuns)
	}
	if options.WithApprovals {
		attachApprovals(ctx, usecase.ApprovalsService{GitHub: githubClient}, repo, runs, renderRuns)
	}
	diagnostic := runDiagnosticForOptions(ctx, githubClient, options, repo, branch, renderRuns)
	return render.Result{Repo: repo, Branch: branch, Runs: renderRuns, Diagnostic: diagnostic}, nil
}

func runDiagnosticForOptions(ctx context.Context, githubClient usecase.GitHub, options cliOptions, repo, branch string, runs []render.Run) *render.Diagnostic {
	if len(runs) > 0 || options.RunID != 0 || options.All || strings.TrimSpace(branch) == "" {
		return nil
	}
	// A filtered empty result does not mean the branch has no runs:
	// probe it unfiltered before calling the target a mismatch.
	if (strings.TrimSpace(options.Status) != "" || options.Actor != "") && githubClient != nil {
		branchRuns, err := githubClient.ListRuns(ctx, usecase.RunFilter{Repo: repo, Branch: branch, PerPage: 1})
		if err != nil {
			return nil
		}
		if len(branchRuns) > 0 {
			return nil
		}
	}
	return renderDiagnosticForEmptyBranch(repo, branch, options.Actor)
}

func renderDiagnosticForEmptyBranch(repo, branch, actor string) *render.Diagnostic {
	diagnostic, ok := usecase.EmptyBranchRunDiagnostic(repo, branch, actor)
	if !ok {
		return nil
	}
	return &render.Diagnostic{
		Kind:         string(diagnostic.Kind),
		Scope:        diagnostic.Scope,
		Message:      diagnostic.Message,
		NextCommands: append([]string{}, diagnostic.NextCommands...),
	}
}

// attachApprovals fills pending_environments for waiting runs in
// place. Opt-in via --approvals; runs that are not waiting cost zero
// calls even with the flag, and errors leave the run unenriched rather
// than failing the listing.
func attachApprovals(ctx context.Context, service usecase.ApprovalsService, repo string, runs []model.Run, out []render.Run) {
	semaphore := make(chan struct{}, triageWorkers)
	var wg sync.WaitGroup
	for i, run := range runs {
		if run.Status != model.StatusWaiting {
			continue
		}
		wg.Add(1)
		go func(i int, run model.Run) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			pending, err := service.List(ctx, repo, run.ID)
			if err != nil || len(pending) == 0 {
				return
			}
			names := make([]string, 0, len(pending))
			for _, gate := range pending {
				names = append(names, gate.EnvironmentName)
			}
			out[i].PendingEnvironments = names
		}(i, run)
	}
	wg.Wait()
}

// attachArtifacts fills artifacts[] for each run in place. Opt-in via
// --artifacts because it costs one API call per run; errors leave the
// run's artifacts empty rather than failing the listing.
func attachArtifacts(ctx context.Context, service usecase.ArtifactsService, repo string, runs []model.Run, out []render.Run) {
	semaphore := make(chan struct{}, triageWorkers)
	var wg sync.WaitGroup
	for i, run := range runs {
		wg.Add(1)
		go func(i int, run model.Run) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			artifacts, err := service.List(ctx, repo, run.ID)
			if err != nil || len(artifacts) == 0 {
				return
			}
			out[i].Artifacts = mapRenderArtifacts(artifacts)
		}(i, run)
	}
	wg.Wait()
}

func mapRenderArtifacts(artifacts []model.Artifact) []render.Artifact {
	out := make([]render.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, render.Artifact{
			ID:          artifact.ID,
			Name:        artifact.Name,
			SizeInBytes: artifact.SizeInBytes,
			Expired:     artifact.Expired,
			CreatedAt:   artifact.CreatedAt,
			ExpiresAt:   artifact.ExpiresAt,
			Digest:      artifact.Digest,
		})
	}
	return out
}

func writeArtifactsResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime) error {
	var githubClient usecase.GitHub
	var repo, branch string
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" || scenario == "network_error" || scenario == "rate_limited" {
			return errors.New("github api unavailable")
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
		branch = firstNonEmpty(options.Branch, "main")
		_ = branch
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return err
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		repo, branch = target.repo, target.branch
	}

	runID := options.RunID
	if runID == 0 {
		runs, err := githubClient.ListRuns(ctx, usecase.RunFilter{Repo: repo, Branch: branch, PerPage: 1})
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			return errors.New("no runs found to inspect; pass --run <run-id>")
		}
		runID = runs[0].ID
	}

	service := usecase.ArtifactsService{GitHub: githubClient}
	artifacts, err := service.List(ctx, repo, runID)
	if err != nil {
		return err
	}
	result := render.ArtifactsResult{Repo: repo, RunID: runID, Artifacts: mapRenderArtifacts(artifacts)}

	if options.Download != "" {
		artifact, ok := findArtifact(artifacts, options.Download)
		if !ok {
			return fmt.Errorf("artifact %q not found in run %d", options.Download, runID)
		}
		outcome, err := service.Download(ctx, repo, artifact, firstNonEmpty(options.Dir, "."), options.Force, nil)
		if err != nil {
			var destErr usecase.DestinationExistsError
			if errors.As(err, &destErr) {
				return fmt.Errorf("%s; pass --force to overwrite", destErr.Error())
			}
			return err
		}
		result.Downloaded = &render.Download{Name: artifact.Name, Path: outcome.Path, FileCount: outcome.FileCount}
	}
	return render.WriteArtifacts(w, options.Format, result)
}

func findArtifact(artifacts []model.Artifact, selector string) (model.Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Name == selector || strconv.FormatInt(artifact.ID, 10) == selector {
			return artifact, true
		}
	}
	return model.Artifact{}, false
}

func fakeScenarioFor(scenario string) fake.Scenario {
	switch scenario {
	case "failure":
		return fake.ScenarioFailing
	case "pending":
		return fake.ScenarioRunning
	case "empty":
		return fake.ScenarioEmpty
	case "conflict":
		return fake.ScenarioConflict
	case "permission":
		return fake.ScenarioPermission
	case "waiting":
		return fake.ScenarioWaiting
	case "regression":
		return fake.ScenarioRegression
	case "pack":
		return fake.ScenarioPack
	case "flaky":
		return fake.ScenarioFlaky
	case "queue":
		return fake.ScenarioQueue
	case "noisy":
		return fake.ScenarioNoisy
	default:
		return fake.ScenarioGreen
	}
}

// triageWorkers bounds concurrent failure enrichment so a fully red
// --all listing stays fast without hammering the API.
const triageWorkers = 4

// attachFailures fills failed[] for actionable runs in place. A triage
// error leaves that run's failed[] empty instead of failing the whole
// listing; the run conclusion and exit code still signal the failure.
func attachFailures(ctx context.Context, triage usecase.TriageService, repo string, runs []model.Run, out []render.Run) {
	semaphore := make(chan struct{}, triageWorkers)
	var wg sync.WaitGroup
	for i, run := range runs {
		wg.Add(1)
		go func(i int, run model.Run) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			failures, err := triage.LoadRunFailures(ctx, repo, run)
			if err != nil || len(failures) == 0 {
				return
			}
			out[i].Failed = mapRenderFailures(failures)
		}(i, run)
	}
	wg.Wait()
}

func mapRenderRepeats(repeats []usecase.Repeat) []render.Repeat {
	if len(repeats) == 0 {
		return nil
	}
	out := make([]render.Repeat, 0, len(repeats))
	for _, r := range repeats {
		out = append(out, render.Repeat{
			StartLine: r.StartLine, EndLine: r.EndLine, Cycles: r.Cycles, BlockLines: r.Period,
			HiddenLines: r.Hidden, From: r.From, To: r.To, Summary: r.Summary,
		})
	}
	return out
}

func mapRenderFailures(failures []usecase.RunFailure) []render.Failure {
	out := make([]render.Failure, 0, len(failures))
	for _, failure := range failures {
		annotations := make([]render.Annotation, 0, len(failure.Annotations))
		for _, annotation := range failure.Annotations {
			annotations = append(annotations, render.Annotation{
				Path:    annotation.Path,
				Line:    annotation.StartLine,
				Level:   annotation.Level,
				Message: annotation.Message,
			})
		}
		out = append(out, render.Failure{
			Job:         failure.Job.Name,
			Step:        failure.Step.Name,
			ExitCode:    failure.ExitCode,
			Annotations: annotations,
			LogExcerpt:  failure.LogExcerpt,
			Repeats:     mapRenderRepeats(failure.Repeats),
		})
	}
	return out
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
			ID:              run.ID,
			Workflow:        firstNonEmpty(run.Name, run.DisplayTitle, run.Path),
			RunNumber:       run.RunNumber,
			Event:           run.Event,
			HeadBranch:      run.HeadBranch,
			HeadSHA:         run.HeadSHA,
			Status:          string(run.Status),
			Attempt:         run.RunAttempt,
			Conclusion:      string(run.Conclusion),
			Actor:           run.Actor,
			TriggeringActor: run.TriggeringActor,
			CreatedAt:       run.CreatedAt,
			HTMLURL:         run.HTMLURL,
			Failed:          []render.Failure{},
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
	if scenario == "failure" || scenario == "flaky" {
		conclusion = "failure"
	}
	if scenario == "pending" {
		runStatus = "in_progress"
		conclusion = ""
	}
	if scenario == "empty" {
		runs := []render.Run{}
		diagnostic := runDiagnosticForOptions(context.Background(), nil, options, repo, branch, runs)
		return render.Result{Repo: repo, Branch: branch, Runs: runs, Diagnostic: diagnostic}
	}
	run := render.Run{
		ID:              30433642,
		Workflow:        "CI",
		RunNumber:       571,
		Event:           "pull_request",
		HeadBranch:      branch,
		HeadSHA:         "a1b2c3d",
		Status:          runStatus,
		Conclusion:      conclusion,
		Actor:           "indrasvat",
		TriggeringActor: "indrasvat",
		CreatedAt:       time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC),
		HTMLURL:         "https://github.com/" + repo + "/actions/runs/30433642",
		Failed:          []render.Failure{},
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
	if options.Actor != "" && options.Actor != usecase.MeActor && !strings.EqualFold(options.Actor, run.Actor) {
		// The fake credentials are run.Actor's: anyone else's filter is
		// a genuine miss, not a target mismatch.
		return render.Result{Repo: repo, Branch: branch, Runs: []render.Run{}}
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
	case "conflict", "permission":
		// Mutation-refusal scenarios: typed errors for agent harnesses.
		return raw
	case "waiting", "gated":
		// Deployment-approval scenario: a run holding at the gate.
		return "waiting"
	case "regression":
		// Seeded last-green → first-red boundary for the diff verb.
		return raw
	case "pack":
		// Multi-run watch scenario: 3 workflows off one push, staggered
		// completion, Docs lost at the end.
		return raw
	case "flaky", "flake", "flakes":
		// Seeded attempt flips + a retry-masked step for flake forensics.
		return "flaky"
	case "queue":
		// Seeded Actions concurrency-group blocker for queue diagnostics.
		return "queue"
	case "noisy":
		// A failing run whose log is mostly repeats, through real triage.
		return "noisy"
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

func defaultCLIOptions() cliOptions {
	return cliOptions{Format: render.FormatJSON, LogLevel: "info"}
}

// applyEnv layers HOUND_* variables under the flags: a variable applies
// only when its flag was not given (changed reports an explicit flag).
func applyEnv(options *cliOptions, lookup func(string) (string, bool), changed func(string) bool) {
	if !changed("repo") && options.Repo == "" {
		if value, ok := lookup("GH_REPO"); ok {
			options.Repo = value
		}
		if value, ok := lookup("HOUND_REPO"); ok {
			options.Repo = value
		}
	}
	if !changed("branch") && options.Branch == "" {
		if value, ok := lookup("HOUND_BRANCH"); ok {
			options.Branch = value
		}
	}
	if !changed("status") && options.Status == "" {
		if value, ok := lookup("HOUND_STATUS"); ok {
			options.Status = value
		}
	}
	if !changed("actor") && options.Actor == "" {
		if value, ok := lookup("HOUND_ACTOR"); ok && strings.TrimSpace(value) != "" {
			options.Actor = value
			options.ActorFromEnv = true
		}
	}
	if value, ok := lookup("HOUND_FORMAT"); ok && !changed("format") {
		options.Format = render.Format(value)
	}
	if value, ok := lookup("HOUND_NO_TUI"); ok && !changed("no-tui") {
		options.NoTUI = parseBool(value)
	}
	// An explicit --format names the output, so HOUND_JSON yields its
	// format to it (and everything to --json), but still means no TUI.
	if value, ok := lookup("HOUND_JSON"); ok && !changed("json") {
		if !changed("format") {
			options.JSON = parseBool(value)
		} else if parseBool(value) && !changed("no-tui") {
			options.NoTUI = true
		}
	}
	if value, ok := lookup("HOUND_LOG_LEVEL"); ok && !changed("log-level") {
		options.LogLevel = value
	}
	if value, ok := lookup("HOUND_TRACE_HTTP"); ok && !changed("trace-http") {
		options.TraceHTTP = parseBool(value)
	}
	if value, ok := lookup("HOUND_ALL"); ok && !changed("all") {
		options.All = parseBool(value)
	}
	if !changed("fake-scenario") && options.Fake == "" {
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
