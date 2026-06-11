package main

import (
	"context"
	"io"
	"strings"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/usecase"
	"github.com/spf13/cobra"
)

func newFlakesCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flakes",
		Short: "Real failure or flake? Score the wobble across recent runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env)
			options.NoTUI = true
			return writeFlakesResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().StringVar(&options.Workflow, "workflow", "", "workflow name, file (ci.yml), or numeric ID (default: the latest run's workflow)")
	return cmd
}

// writeFlakesResult runs the flake scan and emits the verdict
// envelope. Exit codes follow the global contract: 1 flaky or suspect
// (action needed — JSON distinguishes rerun vs investigate), 0 clean
// or insufficient_data (JSON status is the source of truth), 2 typed
// error. 3 is never used here.
func writeFlakesResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime) error {
	format := render.Format(options.Format)
	result := render.FlakesResult{
		Workflow: options.Workflow,
		Branch:   options.Branch,
	}
	refuse := func(kind, message string) error {
		result.Status = "error"
		result.Error = &render.FlakeError{Kind: kind, Message: message}
		if err := render.WriteFlakes(w, format, result); err != nil {
			return err
		}
		return outcomeError{code: render.ExitError}
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

	// A foreign -R target without --branch scans the target's default
	// branch — "is main flaky?" should not require knowing main's name.
	if strings.TrimSpace(branch) == "" {
		if provider, ok := githubClient.(usecase.RepoInfoProvider); ok {
			if defaultBranch, defErr := provider.DefaultBranch(ctx, repo); defErr == nil {
				branch = defaultBranch
			}
		}
	}
	result.Branch = branch

	workflow := strings.TrimSpace(options.Workflow)
	if workflow == "" {
		// No --workflow: follow the latest run on the selected branch.
		runs, err := githubClient.ListRuns(ctx, usecase.RunFilter{Repo: repo, Branch: branch, PerPage: 1})
		if err != nil {
			return refuse(diffErrorKind(err), err.Error())
		}
		if len(runs) == 0 || workflowSelectorForRun(runs[0]) == "" {
			return refuse("validation", "no recent run to follow — pass --workflow <name|file|id>")
		}
		workflow = workflowSelectorForRun(runs[0])
	} else {
		resolved, err := resolveWorkflowSelector(ctx, githubClient, repo, workflow)
		if err != nil {
			return refuse(diffErrorKind(err), err.Error())
		}
		workflow = resolved
	}
	result.Workflow = workflow

	cfg, err := defaultConfig(runtime.Env)
	if err != nil {
		return refuse("validation", err.Error())
	}
	service := usecase.FlakesService{
		History:  history,
		Attempts: githubClient,
		Logs:     githubClient,
		Window:   cfg.FlakeWindow,
		PerPage:  usecase.DiffPerPage,
	}
	report, err := service.Scan(ctx, repo, workflow, branch)
	if err != nil {
		return refuse(diffErrorKind(err), err.Error())
	}

	result.Status = string(report.Status)
	result.SampleSize = report.SampleSize
	result.Window = report.Window
	result.RunsScanned = report.RunsScanned
	result.SignalsEvaluated = report.SignalsEvaluated
	result.Jobs = mapRenderFlakeJobs(report.Jobs)
	result.Verdict = report.Verdict
	if err := render.WriteFlakes(w, format, result); err != nil {
		return err
	}
	if code := render.FlakesExitCode(result); code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

func mapRenderFlakeJobs(jobs []usecase.JobFlake) []render.FlakeJob {
	out := make([]render.FlakeJob, 0, len(jobs))
	for _, job := range jobs {
		evidence := make([]render.FlakeEvidence, 0, len(job.Evidence))
		for _, item := range job.Evidence {
			evidence = append(evidence, render.FlakeEvidence{
				RunID:     item.Run.ID,
				RunNumber: item.Run.RunNumber,
				Attempt:   item.Attempt,
				Kind:      string(item.Kind),
				Detail:    item.Detail,
			})
		}
		out = append(out, render.FlakeJob{
			Job:        job.Job,
			FlakeScore: job.Score,
			Verdict:    string(job.Verdict),
			Flips:      job.Flips,
			Flaps:      job.Flaps,
			Masks:      job.Masks,
			FlakedRuns: job.FlakedRuns,
			Evidence:   evidence,
		})
	}
	return out
}
