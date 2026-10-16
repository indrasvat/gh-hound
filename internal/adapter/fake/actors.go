package fake

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// fakeViewer is the login the fake credentials authenticate as.
const fakeViewer = "indrasvat"

// AuthenticatedLogin implements usecase.ViewerProvider. The actors_anon
// scenario lists runs but cannot say who it is, so @me must fail with
// an explanation while explicit actor:login keeps working.
func (a *Adapter) AuthenticatedLogin(context.Context) (string, error) {
	if a.scenario == ScenarioActorsAnon {
		return "", usecase.APIError{Kind: usecase.APIErrorAuth, Status: http.StatusUnauthorized, Resource: "/user", Message: "Requires authentication"}
	}
	return fakeViewer, nil
}

// actorRuns seeds a busy team repo: several humans, bots, a 34-char
// login, a run whose actor is missing, and a re-run started by someone
// else — more rows than one page so actor filters must reach page 2.
func actorRuns() []model.Run {
	now := time.Now().UTC()
	type seed struct {
		name, title, branch, event, actor, triggering string
		status                                        model.Status
		conclusion                                    model.Conclusion
		attempt                                       int
	}
	seeds := []seed{
		{"CI", "fix: debounce the poller", "main", "push", fakeViewer, "", model.StatusInProgress, model.ConclusionNone, 1},
		{"CI", "feat: actor column", "feat/actors", "pull_request", "alice", "", model.StatusCompleted, model.ConclusionFailure, 1},
		{"Release", "chore(deps): bump x/term", "main", "push", "dependabot[bot]", "", model.StatusCompleted, model.ConclusionSuccess, 1},
		{"CI", "fix: flaky lexer test", "main", "push", "alice", fakeViewer, model.StatusCompleted, model.ConclusionFailure, 2},
		{"CodeQL", "nightly scan", "main", "schedule", "github-actions[bot]", "", model.StatusCompleted, model.ConclusionSuccess, 1},
		{"Docs", "docs: filter syntax", "docs/filters", "pull_request", "a-very-long-contributor-login-name", "", model.StatusCompleted, model.ConclusionSuccess, 1},
		{"Deploy", "deploy main", "main", "workflow_dispatch", fakeViewer, "", model.StatusQueued, model.ConclusionNone, 1},
		{"CI", "imported run", "main", "push", "", "", model.StatusCompleted, model.ConclusionCancelled, 1},
		{"CI", "refactor: scope caches", "feat/actors", "pull_request", fakeViewer, "", model.StatusCompleted, model.ConclusionFailure, 1},
		{"CI", "test: pagination", "main", "push", "BagToad", "", model.StatusCompleted, model.ConclusionSuccess, 1},
	}
	runs := make([]model.Run, 0, 40)
	for i := range 40 {
		s := seeds[i%len(seeds)]
		number := 700 - i
		run := greenRun(number)
		run.ID = int64(30434000 + number)
		run.Name = s.name
		run.DisplayTitle = s.title
		run.HeadBranch = s.branch
		run.Event = s.event
		run.Actor = s.actor
		run.TriggeringActor = s.actor
		if s.triggering != "" {
			run.TriggeringActor = s.triggering
		}
		run.Status = s.status
		run.Conclusion = s.conclusion
		run.RunAttempt = s.attempt
		run.HeadSHA = fmt.Sprintf("%07x", 0xa1b2c3d+number)
		started := now.Add(-time.Duration(i*17+3) * time.Minute)
		run.CreatedAt = started
		run.RunStartedAt = started
		run.UpdatedAt = started.Add(time.Duration(40+i*7) * time.Second)
		if s.status != model.StatusCompleted {
			run.UpdatedAt = now
		}
		run.HTMLURL = fmt.Sprintf("https://github.com/indrasvat/gh-hound/actions/runs/%d", run.ID)
		runs = append(runs, run)
	}
	return runs
}
