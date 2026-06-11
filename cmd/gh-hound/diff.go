package main

import (
	"context"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/usecase"
	"github.com/spf13/cobra"
)

func newDiffCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Pick up the scent: last green vs first bad, with the suspect commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env)
			options.NoTUI = true
			return writeDiffResult(cmd.Context(), runtime.Stdout, *options, runtime)
		},
	}
	cmd.Flags().StringVar(&options.Workflow, "workflow", "", "workflow name, file (ci.yml), or numeric ID (required)")
	return cmd
}

// writeDiffResult runs the regression scan and emits the verdict
// envelope. Exit codes follow the global contract: 1 boundary located
// (action needed), 0 green or inconclusive (JSON status is the source
// of truth), 2 error — 3 is never used here.
func writeDiffResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime) error {
	format := render.Format(options.Format)
	result := render.DiffResult{
		Workflow:       options.Workflow,
		Branch:         options.Branch,
		SuspectCommits: []render.Commit{},
	}
	refuse := func(kind, message string) error {
		result.Status = "error"
		result.Error = &render.DiffError{Kind: kind, Message: message}
		if err := render.WriteDiff(w, format, result); err != nil {
			return err
		}
		return outcomeError{code: render.ExitError}
	}
	if strings.TrimSpace(options.Workflow) == "" {
		return refuse("validation", "--workflow <name|file|id> is required — the hound needs one trail to follow")
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
	comparer, hasComparer := githubClient.(usecase.CommitComparer)
	if !hasHistory || !hasComparer {
		return refuse("validation", "this adapter cannot walk run history; the trail is closed")
	}

	workflow, err := resolveWorkflowSelector(ctx, githubClient, repo, options.Workflow)
	if err != nil {
		return refuse(diffErrorKind(err), err.Error())
	}

	// A foreign -R target without --branch scans the target's default
	// branch — "who broke main?" should not require knowing main's name.
	if strings.TrimSpace(branch) == "" {
		if provider, ok := githubClient.(usecase.RepoInfoProvider); ok {
			if defaultBranch, defErr := provider.DefaultBranch(ctx, repo); defErr == nil {
				branch = defaultBranch
			}
		}
	}
	result.Branch = branch

	cfg, err := defaultConfig(runtime.Env)
	if err != nil {
		return refuse("validation", err.Error())
	}
	service := usecase.DiffService{
		History:  history,
		Compare:  comparer,
		MaxPages: cfg.DiffMaxPages,
		PerPage:  usecase.DiffPerPage,
	}
	verdict, err := service.LocateRegression(ctx, repo, workflow, branch)
	if err != nil {
		return refuse(diffErrorKind(err), err.Error())
	}

	result.Status = string(verdict.Status)
	result.RunsScanned = verdict.RunsScanned
	result.Verdict = verdict.Verdict
	result.TotalSuspects = verdict.TotalSuspects
	result.CompareURL = verdict.CompareURL
	if verdict.Status == usecase.RegressionLocated {
		lastGood := mapRenderRuns([]model.Run{verdict.LastGood})[0]
		firstBad := mapRenderRuns([]model.Run{verdict.FirstBad})[0]
		result.LastGood = &lastGood
		result.FirstBad = &firstBad
	}
	for _, commit := range verdict.SuspectCommits {
		result.SuspectCommits = append(result.SuspectCommits, render.Commit{
			SHA:     commit.SHA,
			Author:  commit.Author,
			Message: commit.Message,
		})
	}
	if err := render.WriteDiff(w, format, result); err != nil {
		return err
	}
	if code := render.DiffExitCode(result); code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

// workflowSelectorLiteral resolves selectors that need no API lookup:
// numeric IDs and workflow file names (the runs endpoint accepts
// both). Returns "" when the selector is a display name that must be
// resolved through ListWorkflows.
func workflowSelectorLiteral(selector string) string {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return ""
	}
	if _, err := strconv.ParseInt(selector, 10, 64); err == nil {
		return selector
	}
	base := path.Base(selector)
	if strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") {
		return base
	}
	return ""
}

func resolveWorkflowSelector(ctx context.Context, githubClient usecase.GitHub, repo, selector string) (string, error) {
	if literal := workflowSelectorLiteral(selector); literal != "" {
		return literal, nil
	}
	workflows, err := githubClient.ListWorkflows(ctx, repo)
	if err != nil {
		return "", err
	}
	want := strings.ToLower(strings.TrimSpace(selector))
	for _, workflow := range workflows {
		if strings.ToLower(strings.TrimSpace(workflow.Name)) != want {
			continue
		}
		if base := path.Base(strings.TrimSpace(workflow.Path)); strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") {
			return base, nil
		}
		if workflow.ID != 0 {
			return strconv.FormatInt(workflow.ID, 10), nil
		}
	}
	return "", usecase.APIError{
		Kind:    usecase.APIErrorNotFound,
		Message: "no workflow named \"" + strings.TrimSpace(selector) + "\" in this yard — try the file name (ci.yml) or numeric ID",
	}
}

// workflowSelectorForRun derives the history-endpoint selector from a
// run: the workflow file name when the run carries its path, else the
// numeric workflow ID.
func workflowSelectorForRun(run model.Run) string {
	if literal := workflowSelectorLiteral(run.Path); literal != "" {
		return literal
	}
	if run.WorkflowID != 0 {
		return strconv.FormatInt(run.WorkflowID, 10)
	}
	return ""
}

// diffErrorKind maps adapter/usecase errors onto the envelope's error
// taxonomy.
func diffErrorKind(err error) string {
	var apiErr usecase.APIError
	if errors.As(err, &apiErr) {
		return string(apiErr.Kind)
	}
	if actionErr, ok := usecase.AsActionError(err); ok {
		return string(actionErr.Kind)
	}
	return "unknown"
}
