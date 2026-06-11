package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/usecase"
	"github.com/spf13/cobra"
)

func newWorkflowsCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var enable, disable string
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "List the pack's workflows with their state — and wake or muzzle one",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env)
			options.NoTUI = true
			return writeWorkflowsResult(cmd.Context(), runtime.Stdout, *options, runtime, enable, disable)
		},
	}
	cmd.Flags().StringVar(&enable, "enable", "", "enable a workflow by numeric ID or workflow file path")
	cmd.Flags().StringVar(&disable, "disable", "", "disable a workflow by numeric ID or workflow file path")
	return cmd
}

// writeWorkflowsResult renders the workflows envelope. The list form
// is one API call; a toggle is exactly one API call (no list lookup —
// the selector goes to the API as given). Exit codes: 0 ok, 2 anything
// else with a typed error.kind refusal on stdout — exit 1 and 3 are
// never returned by this verb.
func writeWorkflowsResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime, enable, disable string) error {
	format := render.Format(options.Format)
	result := render.WorkflowsResult{
		Repo:      firstNonEmpty(options.Repo, ""),
		Workflows: []render.WorkflowInfo{},
	}
	enable = strings.TrimSpace(enable)
	disable = strings.TrimSpace(disable)
	toggleRequested := enable != "" || disable != ""
	// Every refusal writes the envelope: agents branch on error.kind on
	// stdout, and exit 2 is never a bare stderr message (GET failures
	// stay typed too — the list form keeps the same contract).
	refuse := func(err error) error {
		kind, field, message := workflowsErrorKind(err)
		if toggleRequested {
			accepted := false
			result.Accepted = &accepted
		}
		result.Error = &render.MutationError{Kind: kind, Field: field, Message: message}
		if writeErr := render.WriteWorkflows(w, format, result); writeErr != nil {
			return writeErr
		}
		return outcomeError{code: render.ExitError}
	}
	if enable != "" && disable != "" {
		return refuse(usecase.ActionError{Kind: usecase.ActionErrorValidation, Field: "workflow", Message: "--enable and --disable are mutually exclusive — one leash at a time"})
	}

	var githubClient usecase.GitHub
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			result.Repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
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

	service := usecase.WorkflowsService{
		GitHub:  githubClient,
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second},
	}
	if !toggleRequested {
		workflows, err := service.List(ctx, result.Repo)
		if err != nil {
			return refuse(err)
		}
		result.Workflows = mapRenderWorkflows(workflows)
		return render.WriteWorkflows(w, format, result)
	}

	target, action, landing := enable, "enable", model.WorkflowStateActive
	toggle := service.Enable
	if disable != "" {
		target, action, landing = disable, "disable", model.WorkflowStateDisabledManually
		toggle = service.Disable
	}
	if _, err := toggle(ctx, result.Repo, target); err != nil {
		return refuse(err)
	}
	accepted := true
	result.Accepted = &accepted
	result.Toggled = &render.WorkflowToggle{Target: target, Action: action, State: landing}
	return render.WriteWorkflows(w, format, result)
}

func mapRenderWorkflows(workflows []model.Workflow) []render.WorkflowInfo {
	out := make([]render.WorkflowInfo, 0, len(workflows))
	for _, workflow := range workflows {
		out = append(out, render.WorkflowInfo{
			ID:    workflow.ID,
			Name:  workflow.Name,
			Path:  workflow.Path,
			State: workflow.State,
		})
	}
	return out
}

// workflowsErrorKind maps any error to the typed refusal taxonomy,
// carrying the offending field for validation refusals.
func workflowsErrorKind(err error) (string, string, string) {
	if actionErr, ok := usecase.AsActionError(err); ok {
		return string(actionErr.Kind), actionErr.Field, actionErr.Error()
	}
	var apiErr usecase.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Kind {
		case usecase.APIErrorAuth, usecase.APIErrorPermission:
			return string(usecase.ActionErrorPermission), "", apiErr.Error()
		case usecase.APIErrorRateLimit:
			return string(usecase.ActionErrorRateLimit), "", apiErr.Error()
		case usecase.APIErrorNetwork:
			return string(usecase.ActionErrorNetwork), "", apiErr.Error()
		case usecase.APIErrorNotFound:
			// A missing repo or workflow resource is actionable for
			// agents — it must not collapse into unknown.
			return string(usecase.ActionErrorNotFound), "", apiErr.Error()
		}
		return string(usecase.ActionErrorUnknown), "", apiErr.Error()
	}
	return string(usecase.ActionErrorUnknown), "", err.Error()
}
