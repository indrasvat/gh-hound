package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
)

type LaunchScope string

const (
	LaunchScopeBranch LaunchScope = "branch"
	LaunchScopeRepo   LaunchScope = "repo"
)

type LaunchRoute string

const (
	LaunchRouteHome     LaunchRoute = "home"
	LaunchRouteRuns     LaunchRoute = "runs"
	LaunchRouteWatch    LaunchRoute = "watch"
	LaunchRouteDispatch LaunchRoute = "dispatch"
)

type LaunchState string

const (
	LaunchStateRuns     LaunchState = "runs"
	LaunchStateAllGreen LaunchState = "all_green"
	LaunchStateEmpty    LaunchState = "empty"
	LaunchStateError    LaunchState = "error"
	LaunchStateWatch    LaunchState = "watch"
	LaunchStateDispatch LaunchState = "dispatch"
)

type LaunchRequest struct {
	Repo    string
	Branch  string
	All     bool
	Route   LaunchRoute
	PerPage int
}

type RepositoryContext struct {
	Repo     string
	Branch   string
	HeadSHA  string
	Actor    string
	Detached bool
}

type RepositoryContextProvider interface {
	Current(context.Context) (RepositoryContext, error)
}

type ErrRepositoryContext struct {
	Reason string
}

func (e ErrRepositoryContext) Error() string {
	if e.Reason == "" {
		return "repository context could not be resolved"
	}
	return e.Reason
}

type LaunchContext struct {
	Repo         string
	Branch       string
	HeadSHA      string
	Actor        string
	Scope        LaunchScope
	State        LaunchState
	Notice       string
	ErrorMessage string
	Runs         []model.Run
	Workflows    []model.Workflow
}

type LaunchService struct {
	Config     config.Config
	GitHub     GitHub
	Repository RepositoryContextProvider
}

func (s LaunchService) Resolve(ctx context.Context, request LaunchRequest) LaunchContext {
	cfg := s.Config
	if cfg.PerPage == 0 {
		cfg = config.Default()
	}

	repoCtx, repoErr := s.currentRepository(ctx)
	if repoErr != nil && request.Repo == "" {
		return LaunchContext{
			State:        LaunchStateError,
			ErrorMessage: fmt.Sprintf("%s; suggest gh hound -R owner/repo", repoErr),
		}
	}

	result := LaunchContext{
		Repo:    first(request.Repo, repoCtx.Repo),
		Branch:  first(request.Branch, repoCtx.Branch),
		HeadSHA: repoCtx.HeadSHA,
		Actor:   repoCtx.Actor,
		Scope:   LaunchScopeBranch,
	}
	if result.Repo == "" {
		return LaunchContext{
			State:        LaunchStateError,
			ErrorMessage: "repository context could not be resolved; suggest gh hound -R owner/repo",
		}
	}

	perPage := request.PerPage
	if perPage == 0 {
		perPage = cfg.PerPage
	}

	if request.Route == LaunchRouteDispatch {
		result.Scope = LaunchScopeRepo
		result.State = LaunchStateDispatch
		return result
	}

	if request.All || cfg.DefaultScope == config.ScopeRepo || result.Branch == "" {
		result.Scope = LaunchScopeRepo
	}
	if repoCtx.Detached && result.Branch == "" {
		result.Scope = LaunchScopeRepo
		result.Notice = "detached HEAD — showing all branches"
	}

	filter := RunFilter{Repo: result.Repo, PerPage: perPage}
	if result.Scope == LaunchScopeBranch {
		filter.Branch = result.Branch
	}
	runs, err := s.GitHub.ListRuns(ctx, filter)
	if err != nil {
		result.State = LaunchStateError
		result.ErrorMessage = err.Error()
		return result
	}

	if len(runs) == 0 && result.Scope == LaunchScopeBranch {
		result.Scope = LaunchScopeRepo
		result.Notice = fmt.Sprintf("no runs on %s — showing all branches", result.Branch)
		runs, err = s.GitHub.ListRuns(ctx, RunFilter{Repo: result.Repo, PerPage: perPage})
		if err != nil {
			result.State = LaunchStateError
			result.ErrorMessage = err.Error()
			return result
		}
	}

	result.Runs = runs
	if len(runs) == 0 {
		workflows, err := s.GitHub.ListWorkflows(ctx, result.Repo)
		if err != nil {
			result.State = LaunchStateError
			result.ErrorMessage = err.Error()
			return result
		}
		result.Workflows = workflows
		result.State = LaunchStateEmpty
		if len(workflows) == 0 {
			result.Notice = fmt.Sprintf("Actions is not configured for %s", result.Repo)
		} else if result.Notice == "" {
			result.Notice = fmt.Sprintf("no workflow runs yet for %s", result.Repo)
		}
		return result
	}

	if (request.Route == LaunchRouteWatch || cfg.AutoWatch) && hasInProgress(runs) {
		result.State = LaunchStateWatch
		return result
	}
	if allGreen(runs) {
		result.State = LaunchStateAllGreen
		return result
	}
	result.State = LaunchStateRuns
	return result
}

func (s LaunchService) currentRepository(ctx context.Context) (RepositoryContext, error) {
	if s.Repository == nil {
		return RepositoryContext{}, ErrRepositoryContext{Reason: "repository detector is not configured"}
	}
	repoCtx, err := s.Repository.Current(ctx)
	if err != nil {
		var repoErr ErrRepositoryContext
		if errors.As(err, &repoErr) {
			return RepositoryContext{}, repoErr
		}
		return RepositoryContext{}, ErrRepositoryContext{Reason: err.Error()}
	}
	return repoCtx, nil
}

func allGreen(runs []model.Run) bool {
	for _, run := range runs {
		if run.Status != model.StatusCompleted {
			return false
		}
		switch run.Conclusion {
		case model.ConclusionSuccess, model.ConclusionSkipped, model.ConclusionNeutral:
		default:
			return false
		}
	}
	return len(runs) > 0
}

func hasInProgress(runs []model.Run) bool {
	for _, run := range runs {
		if run.Status == model.StatusInProgress || run.Status == model.StatusQueued || run.Status == model.StatusWaiting || run.Status == model.StatusPending || run.Status == model.StatusRequested {
			return true
		}
	}
	return false
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
