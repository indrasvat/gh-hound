package fake

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type Scenario string

const (
	ScenarioGreen        Scenario = "green"
	ScenarioFailing      Scenario = "failing"
	ScenarioRunning      Scenario = "running"
	ScenarioEmpty        Scenario = "empty"
	ScenarioRateLimited  Scenario = "rate_limited"
	ScenarioNetworkError Scenario = "network_error"
	ScenarioPermission   Scenario = "permission"
	ScenarioConflict     Scenario = "conflict"
	ScenarioLogRender    Scenario = "log_render"
	ScenarioLogRefetch   Scenario = "log_refetch"
)

type Adapter struct {
	scenario Scenario
}

func New(scenario Scenario) *Adapter {
	return &Adapter{scenario: scenario}
}

func (a *Adapter) ListRuns(context.Context, usecase.RunFilter) ([]model.Run, error) {
	switch a.scenario {
	case ScenarioGreen:
		return []model.Run{greenRun(571), greenRun(570), greenRun(569)}, nil
	case ScenarioFailing, ScenarioLogRefetch:
		run := greenRun(571)
		run.Conclusion = model.ConclusionFailure
		return []model.Run{run}, nil
	case ScenarioRunning:
		run := greenRun(571)
		run.Status = model.StatusInProgress
		run.Conclusion = model.ConclusionNone
		return []model.Run{run}, nil
	case ScenarioEmpty:
		return []model.Run{}, nil
	case ScenarioRateLimited:
		return nil, errors.New("github api rate limited")
	case ScenarioNetworkError:
		return nil, errors.New("network unavailable")
	case ScenarioPermission:
		return nil, usecase.ActionError{Kind: usecase.ActionErrorPermission, Message: "permission denied", Status: http.StatusForbidden}
	case ScenarioConflict:
		return nil, usecase.ActionError{Kind: usecase.ActionErrorConflict, Message: "run already completed", Status: http.StatusConflict}
	default:
		return nil, errors.New("unknown fake scenario")
	}
}

func (a *Adapter) GetRun(ctx context.Context, repo string, runID int64) (model.Run, error) {
	runs, err := a.ListRuns(ctx, usecase.RunFilter{Repo: repo})
	if err != nil {
		return model.Run{}, err
	}
	for _, run := range runs {
		if run.ID == runID {
			return run, nil
		}
	}
	return model.Run{}, errors.New("run not found")
}

func (a *Adapter) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	return []model.Job{job()}, nil
}

func (a *Adapter) GetJob(context.Context, string, int64) (model.Job, error) {
	return job(), nil
}

func (a *Adapter) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return []model.Workflow{{
		ID:      123,
		Name:    "CI",
		Path:    ".github/workflows/ci.yml",
		State:   "active",
		HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/ci.yml",
	}}, nil
}

func (a *Adapter) FetchWorkflowFile(context.Context, string, string) (string, error) {
	return `name: CI
on:
  workflow_dispatch:
    inputs:
      version:
        required: true
        type: string
      channel:
        type: choice
        options:
          - stable
          - beta
`, nil
}

func (a *Adapter) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	return []model.Annotation{{
		Path:      "internal/parser/lexer.go",
		StartLine: 142,
		EndLine:   142,
		Level:     "failure",
		Message:   "identifier mismatch",
		Title:     "go test",
	}}, nil
}

func (a *Adapter) FetchJobLog(context.Context, string, int64) (string, error) {
	if a.scenario == ScenarioLogRender {
		return "", usecase.LogRenderError{Message: "link expired"}
	}
	return strings.Join([]string{
		"17:42:53.114Z go test ./... -race",
		"##[group] Run go test ./...",
		"ok    internal/api 0.214s",
		"##[group] test output",
		"=== RUN   TestLexIdent/trailing_underscore",
		"    internal/parser/lexer.go:142: got \"foo\" want \"foo_\"",
		"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"FAIL  github.com/indrasvat/gh-hound/internal/parser  0.412s",
		"##[error]Process completed with exit code 1",
		"##[endgroup]",
		"##[endgroup]",
	}, "\n"), nil
}

func (a *Adapter) LastLogRefetch(jobID int64) (usecase.LogRefetchNotice, bool) {
	if a.scenario != ScenarioLogRefetch || jobID == 0 {
		return usecase.LogRefetchNotice{}, false
	}
	return usecase.LogRefetchNotice{
		JobID:         jobID,
		Attempts:      2,
		ExpiredStatus: http.StatusGone,
		Message:       "link had expired; re-requested job log",
	}, true
}

func (a *Adapter) RerunRun(ctx context.Context, repo string, runID int64, debug bool) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionRerunRun, repo, runID, 0, "")
}

func (a *Adapter) RerunFailedJobs(ctx context.Context, repo string, runID int64) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionRerunFailedJobs, repo, runID, 0, "")
}

func (a *Adapter) RerunJob(ctx context.Context, repo string, jobID int64) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionRerunJob, repo, 0, jobID, "")
}

func (a *Adapter) CancelRun(ctx context.Context, repo string, runID int64) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionCancelRun, repo, runID, 0, "")
}

func (a *Adapter) ForceCancelRun(ctx context.Context, repo string, runID int64) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionForceCancelRun, repo, runID, 0, "")
}

func (a *Adapter) DispatchWorkflow(ctx context.Context, repo, workflowID string, request usecase.DispatchRequest) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionDispatch, repo, 0, 0, workflowID)
}

func (a *Adapter) actionResult(action usecase.Action, repo string, runID, jobID int64, workflowID string) (usecase.ActionResult, error) {
	switch a.scenario {
	case ScenarioRateLimited:
		return usecase.ActionResult{}, usecase.ActionError{Kind: usecase.ActionErrorRateLimit, Message: "rate limited", Status: http.StatusTooManyRequests}
	case ScenarioNetworkError:
		return usecase.ActionResult{}, usecase.ActionError{Kind: usecase.ActionErrorNetwork, Message: "network unavailable"}
	case ScenarioPermission:
		return usecase.ActionResult{}, usecase.ActionError{Kind: usecase.ActionErrorPermission, Message: "permission denied", Status: http.StatusForbidden}
	case ScenarioConflict:
		return usecase.ActionResult{}, usecase.ActionError{Kind: usecase.ActionErrorConflict, Message: "run already completed", Status: http.StatusConflict}
	default:
		return usecase.ActionResult{
			Action:     action,
			Repo:       repo,
			RunID:      runID,
			JobID:      jobID,
			WorkflowID: workflowID,
			Message:    "accepted",
		}, nil
	}
}

func greenRun(number int) model.Run {
	return model.Run{
		ID:              int64(30433000 + number),
		Name:            "CI",
		DisplayTitle:    "fix parser",
		Status:          model.StatusCompleted,
		Conclusion:      model.ConclusionSuccess,
		Event:           "pull_request",
		HeadBranch:      "main",
		HeadSHA:         "a1b2c3d",
		Path:            ".github/workflows/ci.yml",
		RunNumber:       number,
		RunAttempt:      1,
		WorkflowID:      123,
		Actor:           "indrasvat",
		TriggeringActor: "indrasvat",
		CreatedAt:       time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 6, 7, 17, 44, 0, 0, time.UTC),
		RunStartedAt:    time.Date(2026, 6, 7, 17, 42, 10, 0, time.UTC),
		HTMLURL:         "https://github.com/indrasvat/gh-hound/actions/runs/30433642",
		JobsURL:         "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/30433642/jobs",
		LogsURL:         "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/30433642/logs",
		PullRequests:    []int{7},
	}
}

func job() model.Job {
	return model.Job{
		ID:              399444496,
		RunID:           30433642,
		Status:          model.StatusCompleted,
		Conclusion:      model.ConclusionFailure,
		StartedAt:       time.Date(2026, 6, 7, 17, 42, 40, 0, time.UTC),
		CompletedAt:     time.Date(2026, 6, 7, 17, 44, 39, 0, time.UTC),
		Name:            "build",
		Labels:          []string{"ubuntu-latest"},
		RunnerName:      "GitHub Actions 1",
		RunnerGroupName: "GitHub Actions",
		WorkflowName:    "CI",
		HeadBranch:      "main",
		HTMLURL:         "https://github.com/indrasvat/gh-hound/actions/runs/30433642/job/399444496",
		CheckRunURL:     "https://api.github.com/repos/indrasvat/gh-hound/check-runs/399444496",
		Steps: []model.Step{{
			Name:        "go test ./...",
			Status:      model.StatusCompleted,
			Conclusion:  model.ConclusionFailure,
			Number:      6,
			StartedAt:   time.Date(2026, 6, 7, 17, 43, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 6, 7, 17, 44, 0, 0, time.UTC),
		}},
	}
}
