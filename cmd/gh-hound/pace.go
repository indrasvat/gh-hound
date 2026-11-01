package main

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/usecase"
	"github.com/spf13/cobra"
)

func newPaceCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var window int
	cmd := &cobra.Command{
		Use:   "pace",
		Short: "Is this run on pace? ETA and slow verdict against recent green runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			var override *int
			if cmd.Flags().Changed("window") {
				override = &window
			}
			return writePaceResult(cmd.Context(), runtime.Stdout, *options, runtime, override)
		},
	}
	cmd.Flags().StringVar(&options.Workflow, "workflow", "", "workflow name, file (ci.yml), or numeric ID (default: the latest run's workflow)")
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "run ID to judge (default: the newest run of the workflow on the branch)")
	cmd.Flags().IntVar(&window, "window", 0, "green runs to sample, 5-100 (default pace_window, 20; env HOUND_PACE_WINDOW)")
	return cmd
}

// writePaceResult judges one run against its workflow's recent green
// runs and emits the pace envelope. Exit codes: 1 slow or overrun, 3
// running within the baseline, 2 typed error, 0 otherwise (JSON status
// is the source of truth).
func writePaceResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime, window *int) error {
	format := render.Format(options.Format)
	result := render.PaceResult{Workflow: options.Workflow, Branch: options.Branch, Statistic: "p50_p90"}
	refuse := func(kind, message string) error {
		result.Status = "error"
		result.Error = &render.PaceError{Kind: kind, Message: message}
		if err := render.WritePace(w, format, result); err != nil {
			return err
		}
		return outcomeError{code: render.ExitError}
	}

	env := runtime.Env
	if env == nil {
		env = func(string) (string, bool) { return "", false }
	}
	cfg, err := config.Load(config.LoadOptions{Path: configPath(env), LookupEnv: env, Overrides: config.Overrides{PaceWindow: window}})
	if err != nil {
		return refuse("validation", err.Error())
	}
	if options.RunID != 0 && strings.TrimSpace(options.Workflow) != "" {
		return refuse("validation", "pass --run or --workflow, not both: a run names its own workflow")
	}

	var githubClient usecase.GitHub
	var repo, branch string
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			result.Repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
			return refuse("network", "github api unavailable")
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
		branch = firstNonEmpty(options.Branch, "main")
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return refuse(diffErrorKind(err), err.Error())
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		repo, branch = target.repo, target.branch
	}
	result.Repo = repo

	history, hasHistory := githubClient.(usecase.WorkflowRunHistory)
	if !hasHistory {
		return refuse("validation", "this adapter cannot walk run history; the trail is closed")
	}
	// A foreign -R target without --branch reads the target's default
	// branch, as flakes does.
	if options.RunID == 0 && strings.TrimSpace(branch) == "" {
		if provider, ok := githubClient.(usecase.RepoInfoProvider); ok {
			if defaultBranch, defErr := provider.DefaultBranch(ctx, repo); defErr == nil {
				branch = defaultBranch
			}
		}
	}

	run, workflow, err := paceTarget(ctx, githubClient, history, repo, branch, options)
	if err != nil {
		return refuse(diffErrorKind(err), err.Error())
	}
	result.Workflow, result.Branch, result.BaselineEvent = workflow, run.HeadBranch, run.Event

	service := usecase.PaceService{History: history, Window: cfg.PaceWindow, PerPage: usecase.DiffPerPage}
	baseline, err := service.Baseline(ctx, repo, workflow, run.HeadBranch, run.Event)
	if err != nil {
		return refuse(diffErrorKind(err), err.Error())
	}
	assessed := usecase.AssessPace(run, baseline, time.Now())

	result.Status = string(assessed.Status)
	result.Window = baseline.Window
	result.SampleSize = assessed.SampleSize
	result.RunsScanned = baseline.RunsScanned
	if baseline.BaselineBranch != "" {
		result.BaselineBranch = &baseline.BaselineBranch
	}
	result.Run = paceRunResult(run, assessed)
	result.Verdict = usecase.PaceVerdictLine(assessed)
	if err := render.WritePace(w, format, result); err != nil {
		return err
	}
	if code := render.PaceExitCode(result); code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

// paceTarget is --run, or the newest run of the workflow on the
// branch; with no --workflow, the latest run on the branch.
func paceTarget(ctx context.Context, githubClient usecase.GitHub, history usecase.WorkflowRunHistory, repo, branch string, options cliOptions) (model.Run, string, error) {
	if options.RunID != 0 {
		run, err := githubClient.GetRun(ctx, repo, options.RunID)
		if err != nil {
			return model.Run{}, "", err
		}
		if workflowSelectorForRun(run) == "" {
			return model.Run{}, "", usecase.ActionError{Kind: usecase.ActionErrorValidation, Message: "the run names no workflow to read history from"}
		}
		return run, workflowSelectorForRun(run), nil
	}
	if strings.TrimSpace(options.Workflow) == "" {
		runs, err := githubClient.ListRuns(ctx, usecase.RunFilter{Repo: repo, Branch: branch, PerPage: 1})
		if err != nil {
			return model.Run{}, "", err
		}
		if len(runs) == 0 || workflowSelectorForRun(runs[0]) == "" {
			return model.Run{}, "", usecase.ActionError{Kind: usecase.ActionErrorValidation, Message: "no recent run to judge — pass --run <id> or --workflow <name|file|id>"}
		}
		return runs[0], workflowSelectorForRun(runs[0]), nil
	}
	workflow, err := resolveWorkflowSelector(ctx, githubClient, repo, options.Workflow)
	if err != nil {
		return model.Run{}, "", err
	}
	runs, err := history.ListWorkflowRuns(ctx, repo, workflow, usecase.RunFilter{Repo: repo, Branch: branch, PerPage: 1})
	if err != nil {
		return model.Run{}, "", err
	}
	if len(runs) == 0 {
		return model.Run{}, "", usecase.ActionError{Kind: usecase.ActionErrorValidation, Message: "no run of " + workflow + " on " + branch + " to judge — pass --run <id>"}
	}
	return runs[0], workflow, nil
}

func paceRunResult(run model.Run, a usecase.PaceAssessment) *render.PaceRun {
	out := &render.PaceRun{
		ID:             run.ID,
		Number:         run.RunNumber,
		Attempt:        run.RunAttempt,
		Event:          run.Event,
		Status:         string(run.Status),
		Conclusion:     string(run.Conclusion),
		ElapsedSeconds: wholeSeconds(a.Elapsed),
		QueuedSeconds:  wholeSeconds(a.Queued),
		Slow:           a.Slow,
		Reason:         a.Reason,
	}
	if a.HasETA {
		out.ETASeconds = new(wholeSeconds(a.ETA))
	}
	if a.Status != usecase.PaceInsufficient && a.Status != usecase.PaceNotComparable {
		out.P50Seconds = new(wholeSeconds(a.P50))
		out.P90Seconds = new(wholeSeconds(a.P90))
		out.Ratio = new(a.Ratio)
	}
	return out
}

func wholeSeconds(d time.Duration) int64 {
	return int64(d.Round(time.Second) / time.Second)
}
