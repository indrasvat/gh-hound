package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// heldOnCommit is run and every loaded run from the same fork and commit
// still held for approval: GitHub's "Approve and run" lets all of a
// pull request's workflows start, and so does A. Two forks can push the
// same commit, so the head repository must match too.
func (a App) heldOnCommit(run model.Run) []model.Run {
	held := []model.Run{run}
	for _, other := range a.runs.Context.Runs {
		if other.ID == run.ID || other.HeadSHA != run.HeadSHA || !other.FromFork || other.HeadRepository != run.HeadRepository {
			continue
		}
		if verdict, _ := model.ClassifyRun(other, nil); verdict.WaitingOnPerson() {
			held = append(held, other)
		}
	}
	return held
}

// openApproveRun asks before letting a held fork run start: approving
// runs someone else's code, so the confirm defaults to No. Write access
// is checked first, off the key path, as the gate overlay checks the
// gate. A held bot or agent run is approved on the web; GitHub's
// endpoint covers forks.
func (a App) openApproveRun(route Route, run model.Run) App {
	if !run.FromFork {
		a.pushToast("approve-in-browser", usecase.Resilience{
			Severity: usecase.SeverityInfo,
			Title:    "approve this one on GitHub",
			Message:  "hound approves fork runs only · approve it on the run's page",
		})
		return a
	}
	if a.pushAccessResolver == nil {
		a.pushErrorToast("approve-unavailable", usecase.ResilienceFor(errors.New("approving runs is not configured"), usecase.ErrorContext{}))
		return a
	}
	request := ActionRequest{Action: usecase.ActionApproveRun, Run: run, Held: a.heldOnCommit(run)}
	resolver := a.pushAccessResolver
	return a.startLoad(loadKindApprovals, "checking your access", func(ctx context.Context) func(App) App {
		canPush, err := resolver(ctx)
		return func(app App) App {
			switch {
			case err != nil:
				app.pushErrorToast("approve-access-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
			case !canPush:
				app.pushToast("approve-no-access", usecase.Resilience{
					Severity: usecase.SeverityInfo,
					Title:    "a maintainer has to approve this",
					Message:  "approving a fork's run needs write access to the repo",
				})
			default:
				app = app.openConfirm(route, request)
			}
			return app
		}
	})
}

// startApproveRuns approves the confirmed runs off the key path: one
// paced call per run, so a frozen screen would grow with the count.
// From the detail view it returns to the list, where the next poll
// shows the runs queued.
func (a App) startApproveRuns(route Route, request ActionRequest) App {
	if a.actionHandler == nil {
		a.setRouteError(route, "action unavailable: live GitHub mutation handler is not configured")
		return a
	}
	handler := a.actionHandler
	label := "approving the run"
	if n := len(request.Held); n > 1 {
		label = fmt.Sprintf("approving %d runs", n)
	}
	return a.startLoad(loadKindApprovals, label, func(context.Context) func(App) App {
		result, err := handler(request)
		return func(app App) App {
			if err != nil {
				app.pushErrorToast("action-failed", usecase.ResilienceFor(err, usecase.ErrorContext{}))
			} else {
				app.pushToast("action-ok", usecase.ResilienceForSuccess(result))
			}
			if app.Route() == RouteDetail && route == RouteDetail {
				app.PopRoute()
			}
			// Approved runs turn queued; partial success too.
			return app.reloadRuns()
		}
	})
}

func approveRunMessage(request ActionRequest) string {
	runs := "this run"
	if n := len(request.Held); n > 1 {
		runs = fmt.Sprintf("%d runs", n)
	}
	from := "its fork"
	if request.Run.Actor != "" {
		from = "@" + request.Run.Actor + "'s fork"
	}
	return fmt.Sprintf("approve %s from %s? their code will run", runs, from)
}
