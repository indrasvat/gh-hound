package usecase_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestLaunchServiceHandlesPRD82Fallbacks(t *testing.T) {
	tests := []struct {
		name       string
		repository fakeRepository
		github     *launchGitHub
		request    usecase.LaunchRequest
		wantScope  usecase.LaunchScope
		wantState  usecase.LaunchState
		wantNotice string
		wantError  string
		wantCalls  []usecase.RunFilter
	}{
		{
			name:       "branch zero runs widens to repo-wide",
			repository: fakeRepository{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "fix/parser", Actor: "indrasvat"}},
			github: &launchGitHub{
				runsByBranch: map[string][]model.Run{
					"fix/parser": {},
					"":           {greenRun(569, "main")},
				},
				workflows: []model.Workflow{{ID: 1, Name: "CI", State: "active"}},
			},
			wantScope:  usecase.LaunchScopeRepo,
			wantState:  usecase.LaunchStateAllGreen,
			wantNotice: "no runs on fix/parser — showing all branches",
			wantCalls: []usecase.RunFilter{
				{Repo: "indrasvat/gh-hound", Branch: "fix/parser", PerPage: 30, Page: 1},
				{Repo: "indrasvat/gh-hound", PerPage: 30, Page: 1},
			},
		},
		{
			name:       "repo has no workflows",
			repository: fakeRepository{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
			github: &launchGitHub{
				runsByBranch: map[string][]model.Run{"main": {}, "": {}},
				workflows:    []model.Workflow{},
			},
			wantScope:  usecase.LaunchScopeRepo,
			wantState:  usecase.LaunchStateEmpty,
			wantNotice: "Actions is not configured for indrasvat/gh-hound",
		},
		{
			name:       "not a git repo suggests explicit repo",
			repository: fakeRepository{err: usecase.ErrRepositoryContext{Reason: "not a git repo"}},
			github:     &launchGitHub{},
			wantState:  usecase.LaunchStateError,
			wantError:  "suggest gh hound -R owner/repo",
		},
		{
			name:       "github access denied is categorized",
			repository: fakeRepository{context: usecase.RepositoryContext{Repo: "openclaw/openclaw", Branch: "integration"}},
			github: &launchGitHub{
				err: usecase.APIError{Kind: usecase.APIErrorPermission, Status: 403, Message: "Resource not accessible by personal access token"},
			},
			wantScope: usecase.LaunchScopeBranch,
			wantState: usecase.LaunchStateError,
			wantError: "GitHub access denied: Resource not accessible by personal access token",
		},
		{
			name:       "detached head falls back to repo-wide",
			repository: fakeRepository{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", HeadSHA: "a1b2c3d", Detached: true}},
			github: &launchGitHub{
				runsByBranch: map[string][]model.Run{"": {greenRun(571, "main")}},
				workflows:    []model.Workflow{{ID: 1, Name: "CI", State: "active"}},
			},
			wantScope:  usecase.LaunchScopeRepo,
			wantState:  usecase.LaunchStateAllGreen,
			wantNotice: "detached HEAD — showing all branches",
		},
		{
			name:       "running run remains home unless watch requested",
			repository: fakeRepository{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
			github: &launchGitHub{
				runsByBranch: map[string][]model.Run{"main": {runningRun(571, "main")}},
				workflows:    []model.Workflow{{ID: 1, Name: "CI", State: "active"}},
			},
			wantScope: usecase.LaunchScopeBranch,
			wantState: usecase.LaunchStateRuns,
		},
		{
			name:       "watch command attaches to in-progress run",
			repository: fakeRepository{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
			github: &launchGitHub{
				runsByBranch: map[string][]model.Run{"main": {runningRun(571, "main")}},
				workflows:    []model.Workflow{{ID: 1, Name: "CI", State: "active"}},
			},
			request:   usecase.LaunchRequest{Route: usecase.LaunchRouteWatch},
			wantScope: usecase.LaunchScopeBranch,
			wantState: usecase.LaunchStateWatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := usecase.LaunchService{
				Config:     config.Default(),
				GitHub:     tt.github,
				Repository: tt.repository,
			}

			got := service.Resolve(context.Background(), tt.request)
			if got.Scope != tt.wantScope {
				t.Fatalf("scope = %s, want %s", got.Scope, tt.wantScope)
			}
			if got.State != tt.wantState {
				t.Fatalf("state = %s, want %s\n%#v", got.State, tt.wantState, got)
			}
			if tt.wantNotice != "" && got.Notice != tt.wantNotice {
				t.Fatalf("notice = %q, want %q", got.Notice, tt.wantNotice)
			}
			if tt.wantError != "" && !strings.Contains(got.ErrorMessage, tt.wantError) {
				t.Fatalf("error = %q, want substring %q", got.ErrorMessage, tt.wantError)
			}
			if len(tt.wantCalls) > 0 && !slices.EqualFunc(tt.github.calls, tt.wantCalls, sameFilter) {
				t.Fatalf("run calls = %#v, want %#v", tt.github.calls, tt.wantCalls)
			}
		})
	}
}

func TestLaunchRequestOverridesRepoBranchAndAllScope(t *testing.T) {
	gh := &launchGitHub{
		runsByBranch: map[string][]model.Run{"": {greenRun(571, "release")}},
		workflows:    []model.Workflow{{ID: 1, Name: "CI", State: "active"}},
	}
	service := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     gh,
		Repository: fakeRepository{context: usecase.RepositoryContext{Repo: "ignored/repo", Branch: "ignored"}},
	}

	got := service.Resolve(context.Background(), usecase.LaunchRequest{
		Repo:   "indrasvat/other",
		Branch: "release",
		All:    true,
	})

	if got.Repo != "indrasvat/other" || got.Branch != "release" || got.Scope != usecase.LaunchScopeRepo {
		t.Fatalf("unexpected launch context: %#v", got)
	}
	if len(gh.calls) != 1 || gh.calls[0].Repo != "indrasvat/other" || gh.calls[0].Branch != "" {
		t.Fatalf("unexpected filter: %#v", gh.calls)
	}
}

func TestLaunchServiceSniffsRepoActivityWhileStayingBranchScoped(t *testing.T) {
	gh := &launchGitHub{
		runsByBranch: map[string][]model.Run{
			"main": {greenRun(101, "main")},
			"":     {runningRun(202, "release/2026.6.5"), greenRun(101, "main")},
		},
		workflows: []model.Workflow{{ID: 1, Name: "CI", State: "active"}},
	}
	service := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     gh,
		Repository: fakeRepository{context: usecase.RepositoryContext{Repo: "openclaw/openclaw", Branch: "main", Actor: "indrasvat"}},
	}

	got := service.Resolve(context.Background(), usecase.LaunchRequest{})

	if got.Scope != usecase.LaunchScopeBranch || got.State != usecase.LaunchStateAllGreen {
		t.Fatalf("launch scope/state = %s/%s, want branch/all_green: %#v", got.Scope, got.State, got)
	}
	if !strings.Contains(got.Notice, "repo activity") || !strings.Contains(got.Notice, "release/2026.6.5") {
		t.Fatalf("notice = %q, want repo activity with active branch", got.Notice)
	}
	if len(got.BranchRuns) != 1 || len(got.RepoRuns) != 2 {
		t.Fatalf("branch/repo runs = %d/%d, want 1/2", len(got.BranchRuns), len(got.RepoRuns))
	}
	wantCalls := []usecase.RunFilter{
		{Repo: "openclaw/openclaw", Branch: "main", PerPage: 30, Page: 1},
		{Repo: "openclaw/openclaw", PerPage: 30, Page: 1},
	}
	if !slices.EqualFunc(gh.calls, wantCalls, sameFilter) {
		t.Fatalf("run calls = %#v, want %#v", gh.calls, wantCalls)
	}
}

type fakeRepository struct {
	context usecase.RepositoryContext
	err     error
}

func (f fakeRepository) Current(context.Context) (usecase.RepositoryContext, error) {
	if f.err != nil {
		return usecase.RepositoryContext{}, f.err
	}
	return f.context, nil
}

type launchGitHub struct {
	runsByBranch map[string][]model.Run
	workflows    []model.Workflow
	calls        []usecase.RunFilter
	err          error
}

func (g *launchGitHub) ListRuns(ctx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	g.calls = append(g.calls, filter)
	if g.err != nil {
		return nil, g.err
	}
	return slices.Clone(g.runsByBranch[filter.Branch]), nil
}

func (g *launchGitHub) GetRun(context.Context, string, int64) (model.Run, error) {
	return model.Run{}, errors.New("not implemented")
}

func (g *launchGitHub) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	return nil, errors.New("not implemented")
}

func (g *launchGitHub) GetJob(context.Context, string, int64) (model.Job, error) {
	return model.Job{}, errors.New("not implemented")
}

func (g *launchGitHub) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return slices.Clone(g.workflows), nil
}

func (g *launchGitHub) FetchWorkflowFile(context.Context, string, string) (string, error) {
	return "on:\n  workflow_dispatch:\n", nil
}

func (g *launchGitHub) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	return nil, errors.New("not implemented")
}

func (g *launchGitHub) FetchJobLog(context.Context, string, int64) (string, error) {
	return "", errors.New("not implemented")
}

func (g *launchGitHub) RerunRun(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *launchGitHub) RerunFailedJobs(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *launchGitHub) RerunJob(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *launchGitHub) CancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *launchGitHub) ForceCancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *launchGitHub) DispatchWorkflow(context.Context, string, string, usecase.DispatchRequest) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func greenRun(number int, branch string) model.Run {
	return model.Run{
		ID:         int64(number),
		Name:       "CI",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionSuccess,
		HeadBranch: branch,
		RunNumber:  number,
	}
}

func runningRun(number int, branch string) model.Run {
	run := greenRun(number, branch)
	run.Status = model.StatusInProgress
	run.Conclusion = model.ConclusionNone
	return run
}

func sameFilter(a, b usecase.RunFilter) bool {
	return a.Repo == b.Repo && a.Branch == b.Branch && a.PerPage == b.PerPage && a.Page == b.Page
}

func (g *launchGitHub) ListArtifacts(context.Context, string, int64) ([]model.Artifact, error) {
	return nil, nil
}

func (g *launchGitHub) DownloadArtifact(context.Context, string, int64) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (g *launchGitHub) GetRunAttempt(context.Context, string, int64, int) (model.Run, error) {
	return model.Run{}, errors.New("not implemented")
}

func (g *launchGitHub) ListJobsForAttempt(context.Context, string, int64, int) ([]model.Job, error) {
	return nil, errors.New("not implemented")
}

func TestForeignRepoDoesNotInheritLocalCheckoutBranch(t *testing.T) {
	gh := &launchGitHub{}
	service := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     gh,
		Repository: &fakeRepository{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "fix/local-work", HeadSHA: "deadbee", Actor: "indrasvat"}},
	}
	result := service.Resolve(context.Background(), usecase.LaunchRequest{Repo: "openclaw/openclaw"})
	if result.Branch != "" {
		t.Fatalf("foreign repo inherited local branch %q", result.Branch)
	}
	if result.HeadSHA != "" {
		t.Fatalf("foreign repo inherited local head sha %q", result.HeadSHA)
	}
	if result.Scope != usecase.LaunchScopeRepo {
		t.Fatalf("foreign repo without branch should be repo-scoped, got %s", result.Scope)
	}

	same := service.Resolve(context.Background(), usecase.LaunchRequest{Repo: "indrasvat/gh-hound"})
	if same.Branch != "fix/local-work" {
		t.Fatalf("same repo lost local branch: %q", same.Branch)
	}
	explicit := service.Resolve(context.Background(), usecase.LaunchRequest{Repo: "openclaw/openclaw", Branch: "main"})
	if explicit.Branch != "main" {
		t.Fatalf("explicit --branch lost: %q", explicit.Branch)
	}
}
