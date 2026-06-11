package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	PerPage      int
	Page         int
	HasMore      bool
	Runs         []model.Run
	BranchRuns   []model.Run
	RepoRuns     []model.Run
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

	perPage := request.PerPage
	if perPage == 0 {
		perPage = cfg.PerPage
	}

	// Local checkout context (branch, head sha) only applies when the
	// target IS the local repo: a foreign -R target must not inherit a
	// branch that likely does not exist there (issue #15).
	sameRepo := request.Repo == "" || strings.EqualFold(strings.TrimSpace(request.Repo), strings.TrimSpace(repoCtx.Repo))
	branch := strings.TrimSpace(request.Branch)
	headSHA := ""
	if sameRepo {
		branch = first(request.Branch, repoCtx.Branch)
		headSHA = repoCtx.HeadSHA
	}
	result := LaunchContext{
		Repo:    first(request.Repo, repoCtx.Repo),
		Branch:  branch,
		HeadSHA: headSHA,
		Actor:   repoCtx.Actor,
		Scope:   LaunchScopeBranch,
		PerPage: perPage,
		Page:    1,
	}
	if result.Repo == "" {
		return LaunchContext{
			State:        LaunchStateError,
			ErrorMessage: "repository context could not be resolved; suggest gh hound -R owner/repo",
		}
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

	filter := RunFilter{Repo: result.Repo, PerPage: perPage, Page: result.Page}
	if result.Scope == LaunchScopeBranch {
		filter.Branch = result.Branch
	}
	runs, err := s.GitHub.ListRuns(ctx, filter)
	if err != nil {
		result.State = LaunchStateError
		result.ErrorMessage = launchErrorMessage(err)
		return result
	}
	if result.Scope == LaunchScopeBranch {
		result.BranchRuns = runs
	}

	if len(runs) == 0 && result.Scope == LaunchScopeBranch {
		result.Scope = LaunchScopeRepo
		result.Notice = fmt.Sprintf("no runs on %s — showing all branches", result.Branch)
		runs, err = s.GitHub.ListRuns(ctx, RunFilter{Repo: result.Repo, PerPage: perPage, Page: result.Page})
		if err != nil {
			result.State = LaunchStateError
			result.ErrorMessage = launchErrorMessage(err)
			return result
		}
		result.RepoRuns = runs
	} else if result.Scope == LaunchScopeBranch {
		result.RepoRuns = s.sniffRepoRuns(ctx, result.Repo, perPage)
		if notice := repoActivityNotice(result.Branch, runs, result.RepoRuns); notice != "" {
			result.Notice = notice
		}
	} else {
		result.RepoRuns = runs
	}

	result.Runs = runs
	result.HasMore = perPage > 0 && len(runs) >= perPage
	if result.Scope == LaunchScopeBranch && len(result.BranchRuns) == 0 {
		result.BranchRuns = runs
	}
	if len(runs) == 0 {
		workflows, err := s.GitHub.ListWorkflows(ctx, result.Repo)
		if err != nil {
			result.State = LaunchStateError
			result.ErrorMessage = launchErrorMessage(err)
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

func launchErrorMessage(err error) string {
	resilience := ResilienceFor(err, ErrorContext{})
	if resilience.Title == "" {
		return err.Error()
	}
	if resilience.Message == "" {
		return resilience.Title
	}
	return resilience.Title + ": " + resilience.Message
}

func (s LaunchService) sniffRepoRuns(ctx context.Context, repo string, perPage int) []model.Run {
	runs, err := s.GitHub.ListRuns(ctx, RunFilter{Repo: repo, PerPage: perPage, Page: 1})
	if err != nil {
		return nil
	}
	return runs
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

func repoActivityNotice(branch string, branchRuns []model.Run, repoRuns []model.Run) string {
	if len(repoRuns) == 0 {
		return ""
	}
	repoSummary := summarize(repoRuns)
	if repoSummary.Failing == 0 && repoSummary.Running == 0 {
		return ""
	}
	branchSummary := summarize(branchRuns)
	if branchSummary.Failing > 0 || branchSummary.Running > 0 {
		return ""
	}
	parts := []string{}
	if repoSummary.Failing > 0 {
		parts = append(parts, fmt.Sprintf("%d failing", repoSummary.Failing))
	}
	if repoSummary.Running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", repoSummary.Running))
	}
	branches := activeBranches(branch, repoRuns)
	if branches == "" {
		return "repo activity: " + strings.Join(parts, ", ") + " across all branches · s scope"
	}
	return "repo activity: " + strings.Join(parts, ", ") + " across " + branches + " · s scope"
}

type runSummary struct {
	Failing int
	Running int
}

func summarize(runs []model.Run) runSummary {
	var summary runSummary
	for _, run := range runs {
		if run.Status == model.StatusInProgress || run.Status == model.StatusQueued || run.Status == model.StatusWaiting || run.Status == model.StatusPending || run.Status == model.StatusRequested {
			summary.Running++
			continue
		}
		switch run.Conclusion {
		case model.ConclusionFailure, model.ConclusionActionRequired, model.ConclusionTimedOut:
			summary.Failing++
		}
	}
	return summary
}

func activeBranches(current string, runs []model.Run) string {
	seen := map[string]bool{}
	branches := []string{}
	for _, run := range runs {
		if run.HeadBranch == "" || run.HeadBranch == current || seen[run.HeadBranch] {
			continue
		}
		if !isAttentionRun(run) {
			continue
		}
		seen[run.HeadBranch] = true
		branches = append(branches, run.HeadBranch)
		if len(branches) == 2 {
			break
		}
	}
	return strings.Join(branches, ", ")
}

func isAttentionRun(run model.Run) bool {
	if run.Status == model.StatusInProgress || run.Status == model.StatusQueued || run.Status == model.StatusWaiting || run.Status == model.StatusPending || run.Status == model.StatusRequested {
		return true
	}
	switch run.Conclusion {
	case model.ConclusionFailure, model.ConclusionActionRequired, model.ConclusionTimedOut:
		return true
	default:
		return false
	}
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
