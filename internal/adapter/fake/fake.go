package fake

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	ScenarioWaiting      Scenario = "waiting"
	ScenarioRegression   Scenario = "regression"
)

type Adapter struct {
	scenario Scenario
}

func New(scenario Scenario) *Adapter {
	return &Adapter{scenario: scenario}
}

func (a *Adapter) ListRuns(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	runs, err := a.listScenarioRuns()
	if err != nil {
		return nil, err
	}
	return filterFixtureRuns(runs, filter), nil
}

// filterFixtureRuns applies server-side status semantics to fixture
// data so the deterministic lens cannot claim matches the real API
// would not return.
func filterFixtureRuns(runs []model.Run, filter usecase.RunFilter) []model.Run {
	status := strings.TrimSpace(filter.Status)
	if status == "" {
		return runs
	}
	out := make([]model.Run, 0, len(runs))
	for _, run := range runs {
		if string(run.Status) == status || string(run.Conclusion) == status {
			out = append(out, run)
		}
	}
	return out
}

func (a *Adapter) listScenarioRuns() ([]model.Run, error) {
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
	case ScenarioWaiting:
		return []model.Run{waitingRun(), greenRun(571)}, nil
	case ScenarioEmpty:
		return []model.Run{}, nil
	case ScenarioRegression:
		return regressionHistory(), nil
	case ScenarioRateLimited:
		return nil, usecase.APIError{
			Kind:       usecase.APIErrorRateLimit,
			Status:     http.StatusForbidden,
			Message:    "API rate limit exceeded",
			RetryAfter: 42 * time.Second,
			ResetAt:    fakeRateLimitReset(),
		}
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

func (a *Adapter) GetRunAttempt(ctx context.Context, repo string, runID int64, attempt int) (model.Run, error) {
	return a.GetRun(ctx, repo, runID)
}

func (a *Adapter) ListJobsForAttempt(ctx context.Context, repo string, runID int64, _ int) ([]model.Job, error) {
	return a.ListJobs(ctx, repo, runID)
}

func (a *Adapter) GetJob(context.Context, string, int64) (model.Job, error) {
	return job(), nil
}

// ListWorkflows covers every documented workflow state so badge,
// toggle, and why-line paths are rehearsable deterministically. Only
// ci.yml carries workflow_dispatch (see FetchWorkflowFile), keeping
// the dispatch launch single-form.
func (a *Adapter) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return []model.Workflow{
		{
			ID:      123,
			Name:    "CI",
			Path:    ".github/workflows/ci.yml",
			State:   model.WorkflowStateActive,
			HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/ci.yml",
		},
		{
			ID:      124,
			Name:    "Nightly Sweep",
			Path:    ".github/workflows/nightly.yml",
			State:   model.WorkflowStateDisabledInactivity,
			HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/nightly.yml",
		},
		{
			ID:      125,
			Name:    "Stale Patrol",
			Path:    ".github/workflows/stale.yml",
			State:   model.WorkflowStateDisabledManually,
			HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/stale.yml",
		},
		{
			ID:      126,
			Name:    "Fork Gate",
			Path:    ".github/workflows/fork-gate.yml",
			State:   model.WorkflowStateDisabledFork,
			HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/fork-gate.yml",
		},
		{
			ID:      127,
			Name:    "Old Patrol",
			Path:    ".github/workflows/old-patrol.yml",
			State:   model.WorkflowStateDeleted,
			HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/old-patrol.yml",
		},
	}, nil
}

func (a *Adapter) FetchWorkflowFile(_ context.Context, _ string, workflowPath string) (string, error) {
	if workflowPath != "" && workflowPath != ".github/workflows/ci.yml" {
		// Non-CI fixtures are schedule-only: the classic cron that
		// fell asleep. They must never enter the dispatch form.
		return `name: Nightly Sweep
on:
  schedule:
    - cron: "0 3 * * *"
`, nil
	}
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

func (a *Adapter) ListArtifacts(context.Context, string, int64) ([]model.Artifact, error) {
	return []model.Artifact{
		{
			ID:          901,
			Name:        "coverage",
			SizeInBytes: 1262848,
			Expired:     false,
			CreatedAt:   time.Date(2026, 6, 7, 17, 44, 30, 0, time.UTC),
			ExpiresAt:   time.Date(2026, 6, 14, 17, 44, 30, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 6, 7, 17, 44, 30, 0, time.UTC),
			Digest:      "sha256:9c4f3a2b1d0e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a",
			RunID:       30433642,
			HeadBranch:  "main",
			HeadSHA:     "a1b2c3d",
		},
		{
			ID:          902,
			Name:        "old-report",
			SizeInBytes: 52480,
			Expired:     true,
			CreatedAt:   time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
			ExpiresAt:   time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
			Digest:      "sha256:1f2e3d4c5b6a7980aabbccddeeff00112233445566778899aabbccddeeff0011",
			RunID:       30433001,
			HeadBranch:  "main",
			HeadSHA:     "d4e5f6a",
		},
	}, nil
}

func (a *Adapter) DownloadArtifact(context.Context, string, int64) (io.ReadCloser, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"coverage.out":        "mode: atomic\ngithub.com/indrasvat/gh-hound/internal/usecase/artifacts.go:42.2,44.16 2 1\n",
		"nested/summary.json": `{"total":"83.4%"}` + "\n",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

// waitingRunID is the fixture run gated on environment review in the
// waiting scenario.
const waitingRunID int64 = 30433655

func waitingRun() model.Run {
	run := greenRun(572)
	run.ID = waitingRunID
	run.Name = "Deploy"
	run.DisplayTitle = "production rollout"
	run.Event = "push"
	run.Status = model.StatusWaiting
	run.Conclusion = model.ConclusionNone
	run.HTMLURL = "https://github.com/indrasvat/gh-hound/actions/runs/30433655"
	return run
}

func (a *Adapter) ListPendingDeployments(_ context.Context, _ string, runID int64) ([]model.PendingDeployment, error) {
	switch a.scenario {
	case ScenarioRateLimited, ScenarioNetworkError, ScenarioPermission, ScenarioConflict:
		_, err := a.actionResult(usecase.ActionApproveDeployment, "", runID, 0, "")
		return nil, err
	case ScenarioWaiting:
		if runID != waitingRunID {
			return []model.PendingDeployment{}, nil
		}
		return []model.PendingDeployment{
			{
				EnvironmentID:         7301,
				EnvironmentName:       "production",
				WaitTimer:             0,
				CurrentUserCanApprove: true,
				Reviewers: []model.DeploymentReviewer{
					{Type: "User", Name: "indrasvat"},
				},
			},
			{
				EnvironmentID:         7302,
				EnvironmentName:       "staging",
				WaitTimer:             1800,
				CurrentUserCanApprove: false,
				Reviewers: []model.DeploymentReviewer{
					{Type: "Team", Name: "deploy-keys"},
				},
			},
		}, nil
	default:
		return []model.PendingDeployment{}, nil
	}
}

func (a *Adapter) ReviewPendingDeployments(_ context.Context, repo string, runID int64, review usecase.DeploymentReview) (usecase.ActionResult, error) {
	action := usecase.ActionApproveDeployment
	if review.State == usecase.DeploymentRejected {
		action = usecase.ActionRejectDeployment
	}
	return a.actionResult(action, repo, runID, 0, "")
}

func (a *Adapter) FetchJobLog(context.Context, string, int64) (string, error) {
	if a.scenario == ScenarioLogRender {
		return "", usecase.LogRenderError{Message: "link expired"}
	}
	// Timestamped like real runner logs (with a 51s gap before the
	// failing test) so the time-jump picker's clocks and gap entries
	// are exercisable in the deterministic lens.
	return strings.Join([]string{
		"17:42:53.114Z go test ./... -race",
		"17:42:53.500Z ##[group] Run go test ./...",
		"17:42:54.100Z ok    internal/api 0.214s",
		"17:43:45.000Z ##[group] test output",
		"17:43:45.200Z === RUN   TestLexIdent/trailing_underscore",
		"17:43:45.300Z     internal/parser/lexer.go:142: got \"foo\" want \"foo_\"",
		"17:43:46.000Z --- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"17:43:46.100Z FAIL  github.com/indrasvat/gh-hound/internal/parser  0.412s",
		"17:43:46.200Z ##[error]Process completed with exit code 1",
		"17:43:46.300Z ##[endgroup]",
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

func (a *Adapter) RerunFailedJobs(ctx context.Context, repo string, runID int64, _ bool) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionRerunFailedJobs, repo, runID, 0, "")
}

func (a *Adapter) RerunJob(ctx context.Context, repo string, jobID int64, _ bool) (usecase.ActionResult, error) {
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

func (a *Adapter) EnableWorkflow(_ context.Context, repo, workflowID string) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionEnableWorkflow, repo, 0, 0, workflowID)
}

func (a *Adapter) DisableWorkflow(_ context.Context, repo, workflowID string) (usecase.ActionResult, error) {
	return a.actionResult(usecase.ActionDisableWorkflow, repo, 0, 0, workflowID)
}

func (a *Adapter) actionResult(action usecase.Action, repo string, runID, jobID int64, workflowID string) (usecase.ActionResult, error) {
	switch a.scenario {
	case ScenarioRateLimited:
		return usecase.ActionResult{}, usecase.ActionError{
			Kind:       usecase.ActionErrorRateLimit,
			Message:    "rate limited",
			Status:     http.StatusTooManyRequests,
			RetryAfter: 42 * time.Second,
			ResetAt:    fakeRateLimitReset(),
		}
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

func fakeRateLimitReset() time.Time {
	return time.Date(2026, 6, 9, 20, 4, 0, 0, time.UTC)
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

// regressionHistory seeds the boundary the diff scan must locate:
// a red streak (#575, #573) with a cancelled run between (#574),
// broken by the rerun-flipped green #572 (attempt 2 — the latest-
// attempt rule), then plain green history.
func regressionHistory() []model.Run {
	streakHead := greenRun(575)
	streakHead.Conclusion = model.ConclusionFailure
	streakHead.HeadSHA = "f5e6d7c"
	streakHead.DisplayTitle = "feat: sharpen the lexer"

	cancelled := greenRun(574)
	cancelled.Conclusion = model.ConclusionCancelled
	cancelled.HeadSHA = "e4d5c6b"

	firstBad := greenRun(573)
	firstBad.Conclusion = model.ConclusionFailure
	firstBad.HeadSHA = "d3c4b5a"
	firstBad.DisplayTitle = "feat: sharpen the lexer"

	lastGood := greenRun(572)
	lastGood.RunAttempt = 2
	lastGood.HeadSHA = "c2b3a49"

	older := greenRun(571)
	older.HeadSHA = "b1a2938"

	return []model.Run{streakHead, cancelled, firstBad, lastGood, older}
}

// ListWorkflowRuns implements usecase.WorkflowRunHistory over the
// scenario fixtures: one page of history, scenario errors intact.
func (a *Adapter) ListWorkflowRuns(_ context.Context, _ string, _ string, filter usecase.RunFilter) ([]model.Run, error) {
	if filter.Page > 1 {
		return []model.Run{}, nil
	}
	runs, err := a.listScenarioRuns()
	if err != nil {
		return nil, err
	}
	return filterFixtureRuns(runs, filter), nil
}

// CompareCommits implements usecase.CommitComparer with deterministic
// suspects for the regression scenario.
func (a *Adapter) CompareCommits(_ context.Context, repo, base, head string) (model.CommitRange, error) {
	switch a.scenario {
	case ScenarioRateLimited:
		return model.CommitRange{}, usecase.APIError{Kind: usecase.APIErrorRateLimit, Status: http.StatusForbidden, Message: "API rate limit exceeded"}
	case ScenarioNetworkError:
		return model.CommitRange{}, errors.New("network unavailable")
	}
	return model.CommitRange{
		TotalCommits: 2,
		HTMLURL:      "https://github.com/" + repo + "/compare/" + base + "..." + head,
		Commits: []model.Commit{
			{SHA: "d3c4b5a9f0e1d2c3b4a5968778695a4b3c2d1e0f", Author: "indrasvat", Message: "feat: sharpen the lexer"},
			{SHA: "cc99aa1b2c3d4e5f60718293a4b5c6d7e8f90a1b", Author: "dependabot[bot]", Message: "chore(deps): bump charmbracelet/x/ansi"},
		},
	}, nil
}

// fakeCaches is the deterministic kennel: sizes sum to ~9.7 GiB so
// the usage gauge sits past the 90% eviction warning in e2e and PTY
// rehearsals without any live repo.
func fakeCaches() []model.Cache {
	return []model.Cache{
		{ID: 9001, Key: "setup-go-Linux-x64-ubuntu24-go-1.26.4-d93f4ea308b07f7c7339055a38006c84c478b6cb448d9d34672d1a6fb9324780", Ref: "refs/heads/main", SizeInBytes: 3758096384, LastAccessedAt: time.Date(2026, 6, 7, 17, 44, 30, 0, time.UTC), CreatedAt: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)},
		{ID: 9002, Key: "go-build-Linux-x64-main", Ref: "refs/heads/main", SizeInBytes: 3221225472, LastAccessedAt: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)},
		{ID: 9003, Key: "go-mod-Linux-x64-1f2e3d", Ref: "refs/heads/main", SizeInBytes: 2147483648, LastAccessedAt: time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)},
		// Same key as 9003 on a PR ref: the live API caches one key per
		// ref, and delete-by-key without --ref digs them up together.
		{ID: 9004, Key: "go-mod-Linux-x64-1f2e3d", Ref: "refs/pull/7/merge", SizeInBytes: 858993459, LastAccessedAt: time.Date(2026, 5, 30, 8, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 5, 29, 9, 0, 0, 0, time.UTC)},
		{ID: 9005, Key: "node-modules-pages-build", Ref: "refs/heads/main", SizeInBytes: 429496730, LastAccessedAt: time.Date(2026, 5, 28, 8, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 5, 28, 8, 0, 0, 0, time.UTC)},
	}
}

// ListCaches implements usecase.GitHubCaches with server-side key
// prefix and ref semantics so the deterministic lens cannot claim
// matches the real API would not return.
func (a *Adapter) ListCaches(_ context.Context, _ string, filter usecase.CacheFilter) ([]model.Cache, error) {
	if err := a.cachesScenarioError(); err != nil {
		return nil, err
	}
	if a.scenario == ScenarioEmpty {
		return []model.Cache{}, nil
	}
	out := make([]model.Cache, 0)
	for _, cache := range fakeCaches() {
		if filter.Key != "" && !strings.HasPrefix(cache.Key, filter.Key) {
			continue
		}
		if filter.Ref != "" && cache.Ref != filter.Ref {
			continue
		}
		out = append(out, cache)
	}
	return out, nil
}

func (a *Adapter) CacheUsage(context.Context, string) (model.CacheUsage, error) {
	if err := a.cachesScenarioError(); err != nil {
		return model.CacheUsage{}, err
	}
	if a.scenario == ScenarioEmpty {
		return model.CacheUsage{}, nil
	}
	var total int64
	caches := fakeCaches()
	for _, cache := range caches {
		total += cache.SizeInBytes
	}
	return model.CacheUsage{ActiveSizeInBytes: total, ActiveCount: len(caches)}, nil
}

// CacheStorageLimit mirrors github.com's storage-limit endpoint with
// the default 10 GB so fixtures and rehearsals exercise the provider
// path instead of the fallback.
func (a *Adapter) CacheStorageLimit(context.Context, string) (int64, error) {
	if err := a.cachesScenarioError(); err != nil {
		return 0, err
	}
	return int64(10) << 30, nil
}

func (a *Adapter) DeleteCacheByID(_ context.Context, _ string, id int64) (int, error) {
	if err := a.cachesScenarioError(); err != nil {
		return 0, err
	}
	for _, cache := range fakeCaches() {
		if cache.ID == id {
			return 1, nil
		}
	}
	return 0, usecase.ActionError{Kind: usecase.ActionErrorNotFound, Message: fmt.Sprintf("no cache with id %d in this kennel", id)}
}

func (a *Adapter) DeleteCachesByKey(_ context.Context, _ string, key, ref string) (int, error) {
	if err := a.cachesScenarioError(); err != nil {
		return 0, err
	}
	count := 0
	for _, cache := range fakeCaches() {
		// The live DELETE matches the COMPLETE key (only LIST's key
		// param prefix-matches) — the fake must not promise broader
		// deletes than production performs.
		if cache.Key != key {
			continue
		}
		if ref != "" && cache.Ref != ref {
			continue
		}
		count++
	}
	if count == 0 {
		return 0, usecase.ActionError{Kind: usecase.ActionErrorNotFound, Message: "no caches matched key " + strconv.Quote(key)}
	}
	return count, nil
}

// cachesScenarioError mirrors actionResult's refusal taxonomy for the
// cache surface.
func (a *Adapter) cachesScenarioError() error {
	switch a.scenario {
	case ScenarioRateLimited:
		return usecase.ActionError{
			Kind:       usecase.ActionErrorRateLimit,
			Message:    "rate limited",
			Status:     http.StatusTooManyRequests,
			RetryAfter: 42 * time.Second,
			ResetAt:    fakeRateLimitReset(),
		}
	case ScenarioNetworkError:
		return usecase.ActionError{Kind: usecase.ActionErrorNetwork, Message: "network unavailable"}
	case ScenarioPermission:
		return usecase.ActionError{Kind: usecase.ActionErrorPermission, Message: "permission denied", Status: http.StatusForbidden}
	case ScenarioConflict:
		return usecase.ActionError{Kind: usecase.ActionErrorConflict, Message: "cache is in use", Status: http.StatusConflict}
	default:
		return nil
	}
}

// RefExists implements usecase.RefValidator: fake refs always exist
// except the canonical typo "ghost", reserved for refusal rehearsals.
func (a *Adapter) RefExists(_ context.Context, _ string, ref string) (bool, error) {
	return ref != "" && ref != "ghost", nil
}

// DefaultBranch implements usecase.RepoInfoProvider for fakes.
func (a *Adapter) DefaultBranch(context.Context, string) (string, error) {
	return "main", nil
}
