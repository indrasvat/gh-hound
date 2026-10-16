package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type launchViewerGitHub struct {
	*launchGitHub
	login string
	err   error
}

func (g launchViewerGitHub) AuthenticatedLogin(context.Context) (string, error) {
	return g.login, g.err
}

func TestLaunchActorFilterReachesEveryListing(t *testing.T) {
	github := launchViewerGitHub{launchGitHub: &launchGitHub{runsByBranch: map[string][]model.Run{
		"fix/parser": {},
		"":           {greenRun(569, "main")},
	}}, login: "alice"}
	launch := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     github,
		Repository: fakeRepository{context: usecase.RepositoryContext{Repo: "cli/cli", Branch: "fix/parser", Actor: "Git Author"}},
	}.Resolve(context.Background(), usecase.LaunchRequest{Actor: usecase.MeActor})

	if launch.Query != "actor:@me" || launch.Viewer != "alice" {
		t.Fatalf("query=%q viewer=%q", launch.Query, launch.Viewer)
	}
	for _, call := range github.calls {
		if call.Actor != "alice" {
			t.Fatalf("listing without the actor filter: %+v", call)
		}
	}
	if launch.Scope != usecase.LaunchScopeRepo || !strings.HasPrefix(launch.Notice, `None of your runs matched branch "fix/parser".`) {
		t.Fatalf("fallback scope=%s notice=%q", launch.Scope, launch.Notice)
	}
}

func TestLaunchActorFilteredEmptyStaysOnClearableRunsList(t *testing.T) {
	github := &launchGitHub{runsByBranch: map[string][]model.Run{"main": {}, "": {}}}
	launch := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     github,
		Repository: fakeRepository{context: usecase.RepositoryContext{Repo: "cli/cli", Branch: "main"}},
	}.Resolve(context.Background(), usecase.LaunchRequest{Actor: "carol"})
	if launch.State != usecase.LaunchStateRuns || launch.Query != "actor:carol" || launch.Viewer != "" {
		t.Fatalf("state=%s query=%q viewer=%q", launch.State, launch.Query, launch.Viewer)
	}
	if strings.Contains(launch.Notice, "no workflow runs yet") || strings.Contains(launch.Notice, "not configured") {
		t.Fatalf("a filtered miss must not claim the repo has no runs: %q", launch.Notice)
	}
}

func TestLaunchActorMeFailureIsExplainedNotBroadened(t *testing.T) {
	github := launchViewerGitHub{
		launchGitHub: &launchGitHub{runsByBranch: map[string][]model.Run{"main": {greenRun(1, "main")}}},
		err:          usecase.APIError{Kind: usecase.APIErrorAuth, Status: 401},
	}
	launch := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     github,
		Repository: fakeRepository{context: usecase.RepositoryContext{Repo: "cli/cli", Branch: "main"}},
	}.Resolve(context.Background(), usecase.LaunchRequest{Actor: usecase.MeActor})
	if launch.State != usecase.LaunchStateError || !strings.Contains(launch.ErrorMessage, "can't resolve @me: not signed in") {
		t.Fatalf("state=%s error=%q", launch.State, launch.ErrorMessage)
	}
	if len(github.calls) != 0 || len(launch.Runs) != 0 {
		t.Fatalf("no listing may run without the actor: %d calls", len(github.calls))
	}
}

func TestLaunchActorNarrowsLooseMatchesAndKeepsHasMore(t *testing.T) {
	page := []model.Run{}
	for i := range 30 {
		run := greenRun(i+1, "main")
		run.Actor = "cli-triage[bot]"
		if i == 0 {
			run.Actor = "alice"
		}
		page = append(page, run)
	}
	github := &launchGitHub{runsByBranch: map[string][]model.Run{"main": page, "": page}}
	launch := usecase.LaunchService{
		Config:     config.Default(),
		GitHub:     github,
		Repository: fakeRepository{context: usecase.RepositoryContext{Repo: "cli/cli", Branch: "main"}},
	}.Resolve(context.Background(), usecase.LaunchRequest{Actor: "alice"})
	if len(launch.Runs) != 1 || launch.Runs[0].Actor != "alice" || len(launch.RepoRuns) != 1 || !launch.HasMore {
		t.Fatalf("runs=%d repo=%d hasMore=%v", len(launch.Runs), len(launch.RepoRuns), launch.HasMore)
	}
}
