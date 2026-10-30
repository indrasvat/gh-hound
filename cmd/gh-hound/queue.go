package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/usecase"
	"github.com/spf13/cobra"
)

func newQueueCommand(runtime commandRuntime, options *cliOptions) *cobra.Command {
	var jobID int64
	var groupName string
	var groups bool
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Explain GitHub Actions concurrency queue blockers",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyEnv(options, runtime.Env, cmd.Flags().Changed)
			options.NoTUI = true
			request := queueRequest{
				runID:     options.RunID,
				jobID:     jobID,
				groupName: groupName,
				groups:    groups,
			}
			return writeQueueResult(cmd.Context(), runtime.Stdout, *options, runtime, request)
		},
	}
	cmd.Flags().Int64Var(&options.RunID, "run", 0, "run ID to inspect")
	cmd.Flags().Int64Var(&jobID, "job", 0, "job ID to inspect")
	cmd.Flags().StringVar(&groupName, "group", "", "concurrency group name to inspect")
	cmd.Flags().BoolVar(&groups, "groups", false, "list active concurrency group summaries")
	return cmd
}

type queueRequest struct {
	runID     int64
	jobID     int64
	groupName string
	groups    bool
}

func writeQueueResult(ctx context.Context, w io.Writer, options cliOptions, runtime commandRuntime, request queueRequest) error {
	format := render.Format(options.Format)
	result := render.QueueResult{Repo: firstNonEmpty(options.Repo, ""), Target: queueTargetFromRequest(request), FetchedAt: time.Now().UTC()}
	refuse := func(kind, field, message string) error {
		result.Status = string(model.ConcurrencyStatusError)
		result.Decision = render.QueueDecision{Action: string(model.ConcurrencyActionError), Reason: message}
		result.Error = &render.QueueError{Kind: kind, Field: field, Message: message}
		if err := render.WriteQueue(w, format, result); err != nil {
			return err
		}
		return outcomeError{code: render.ExitError}
	}
	if err := validateQueueRequest(request); err != nil {
		return refuse("validation", "target", err.Error())
	}
	if _, err := defaultConfig(runtime.Env); err != nil {
		return refuse("validation", "config", err.Error())
	}

	var githubClient any
	var repo string
	if options.Fake != "" {
		scenario := normalizedScenario(options)
		if scenario == "api_error" {
			result.Repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
			return refuse("network", "", "github api unavailable")
		}
		githubClient = fake.New(fakeScenarioFor(scenario))
		repo = firstNonEmpty(options.Repo, "indrasvat/gh-hound")
	} else {
		target, err := resolveTarget(ctx, options, runtime)
		if err != nil {
			return refuse("unknown", "", err.Error())
		}
		defer func() {
			_ = target.close()
		}()
		githubClient = target.github
		repo = target.repo
	}
	result.Repo = repo

	concurrency, ok := githubClient.(usecase.GitHubConcurrency)
	if !ok {
		result.Status = string(model.ConcurrencyStatusUnsupported)
		result.Decision = render.QueueDecision{
			Action:     string(model.ConcurrencyActionUnsupported),
			Confidence: "high",
			Reason:     "this GitHub adapter does not expose Actions concurrency groups",
		}
		if err := render.WriteQueue(w, format, result); err != nil {
			return err
		}
		return nil
	}
	service := usecase.QueueService{GitHub: concurrency}
	if jobs, ok := githubClient.(usecase.JobGetter); ok {
		service.Jobs = jobs
	}

	var report model.ConcurrencyReport
	var err error
	switch {
	case request.groups:
		groupResult, err := queueGroupsResult(ctx, repo, concurrency)
		if err != nil {
			if isQueueUnsupported(ctx, err, request, repo, githubClient) {
				return writeQueueUnsupported(w, format, result)
			}
			return refuse(queueErrorKind(err), "", err.Error())
		}
		if err := render.WriteQueue(w, format, groupResult); err != nil {
			return err
		}
		return nil
	case request.groupName != "":
		report, err = inspectQueueGroup(ctx, repo, request.groupName, concurrency)
		if isQueueInactive(err) {
			result.Status = string(model.ConcurrencyStatusInactive)
			result.Decision = render.QueueDecision{Action: string(model.ConcurrencyActionNormalWatch), Confidence: "high", Reason: "concurrency group is inactive or empty"}
			result.Summary = "concurrency group is inactive"
			if err := render.WriteQueue(w, format, result); err != nil {
				return err
			}
			return nil
		}
	case request.jobID > 0:
		report, err = service.InspectJob(ctx, repo, request.jobID)
	default:
		report, err = service.InspectRun(ctx, repo, request.runID)
	}
	if err != nil {
		if isQueueUnsupported(ctx, err, request, repo, githubClient) {
			return writeQueueUnsupported(w, format, result)
		}
		return refuse(queueErrorKind(err), "", err.Error())
	}
	result = render.QueueResultFromReport(report)
	if err := render.WriteQueue(w, format, result); err != nil {
		return err
	}
	if code := render.QueueExitCode(result); code != render.ExitOK {
		return outcomeError{code: code}
	}
	return nil
}

func isQueueInactive(err error) bool {
	var apiErr usecase.APIError
	return errors.As(err, &apiErr) && apiErr.Kind == usecase.APIErrorNotFound
}

type queueRunGetter interface {
	GetRun(context.Context, string, int64) (model.Run, error)
}

func isQueueUnsupported(ctx context.Context, err error, request queueRequest, repo string, githubClient any) bool {
	var apiErr usecase.APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != usecase.APIErrorNotFound {
		return false
	}
	switch {
	case request.groupName != "":
		return false
	case request.runID > 0:
		runs, ok := githubClient.(queueRunGetter)
		if !ok {
			return true
		}
		_, lookupErr := runs.GetRun(ctx, repo, request.runID)
		return lookupErr == nil
	default:
		return true
	}
}

func writeQueueUnsupported(w io.Writer, format render.Format, result render.QueueResult) error {
	result.Status = string(model.ConcurrencyStatusUnsupported)
	result.Decision = render.QueueDecision{
		Action:     string(model.ConcurrencyActionUnsupported),
		Confidence: "high",
		Reason:     "this GitHub host does not expose Actions concurrency groups",
	}
	result.Summary = "Actions concurrency groups are unavailable on this GitHub host"
	if err := render.WriteQueue(w, format, result); err != nil {
		return err
	}
	return nil
}

func validateQueueRequest(request queueRequest) error {
	modes := 0
	if request.runID > 0 {
		modes++
	}
	if request.jobID > 0 {
		modes++
	}
	if strings.TrimSpace(request.groupName) != "" {
		modes++
	}
	if request.groups {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("exactly one of --run, --job, --group, or --groups is required")
	}
	return nil
}

func queueTargetFromRequest(request queueRequest) render.QueueTarget {
	switch {
	case request.runID > 0:
		return render.QueueTarget{Kind: string(model.ConcurrencyTargetRun), RunID: request.runID}
	case request.jobID > 0:
		return render.QueueTarget{Kind: string(model.ConcurrencyTargetJob), JobID: request.jobID}
	case strings.TrimSpace(request.groupName) != "":
		return render.QueueTarget{Kind: string(model.ConcurrencyTargetGroup), GroupName: request.groupName}
	case request.groups:
		return render.QueueTarget{Kind: string(model.ConcurrencyTargetList)}
	default:
		return render.QueueTarget{Kind: string(model.ConcurrencyTargetUnknown)}
	}
}

func queueGroupsResult(ctx context.Context, repo string, github usecase.GitHubConcurrency) (render.QueueResult, error) {
	summaries, err := github.ListConcurrencyGroups(ctx, repo, usecase.ConcurrencyListOptions{})
	if err != nil {
		return render.QueueResult{}, err
	}
	groups := make([]render.QueueGroup, 0, len(summaries))
	for _, summary := range summaries {
		groups = append(groups, render.QueueGroup{
			GroupName:      summary.GroupName,
			GroupURL:       summary.GroupURL,
			LastAcquiredAt: summary.LastAcquiredAt,
			Scope:          string(model.ConcurrencyScopeUnknown),
			State:          string(model.ConcurrencyStatusListed),
		})
	}
	return render.QueueResult{
		Repo:                 repo,
		FetchedAt:            time.Now().UTC(),
		Status:               string(model.ConcurrencyStatusListed),
		Target:               render.QueueTarget{Kind: string(model.ConcurrencyTargetList)},
		Decision:             render.QueueDecision{Action: string(model.ConcurrencyActionListOnly), Confidence: "high", Reason: "listed active concurrency groups"},
		Summary:              fmt.Sprintf("%d active concurrency group(s)", len(groups)),
		ConfiguredGroupCount: len(groups),
		Groups:               groups,
	}, nil
}

func inspectQueueGroup(ctx context.Context, repo, groupName string, github usecase.GitHubConcurrency) (model.ConcurrencyReport, error) {
	group, err := github.GetConcurrencyGroup(ctx, repo, groupName, usecase.ConcurrencyAheadFilter{})
	if err != nil {
		return model.ConcurrencyReport{}, err
	}
	return model.ConcurrencyReport{
		Repo:      repo,
		Target:    model.ConcurrencyTarget{Kind: model.ConcurrencyTargetGroup, GroupName: groupName},
		FetchedAt: time.Now().UTC(),
		Status:    model.ConcurrencyStatusListed,
		Decision: model.ConcurrencyDecision{
			Action:     model.ConcurrencyActionListOnly,
			Confidence: "high",
			Reason:     "listed concurrency group members",
		},
		Summary:              fmt.Sprintf("%s has %d active member(s)", group.GroupName, len(group.Members)),
		ConfiguredGroupCount: 1,
		Groups:               []model.ConcurrencyGroup{group},
	}, nil
}

func queueErrorKind(err error) string {
	if actionErr, ok := usecase.AsActionError(err); ok {
		return string(actionErr.Kind)
	}
	var apiErr usecase.APIError
	if errors.As(err, &apiErr) {
		return string(apiErr.Kind)
	}
	return "unknown"
}
