package tui

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

// approveState carries the approvals in flight. It is not a load: Esc
// cancels a load, but an approval GitHub accepted cannot be taken back,
// so its outcome must always reach the screen. Pointer-held, drained on
// ticks and keypresses.
type approveState struct {
	mu     sync.Mutex
	done   bool
	result usecase.ActionResult
	err    error
	route  Route
	runID  int64
}

// startApproveRuns approves the confirmed runs off the key path: one
// paced call per run, so a frozen screen would grow with the count.
func (a App) startApproveRuns(route Route, request ActionRequest) App {
	if a.actionHandler == nil {
		a.setRouteError(route, "action unavailable: live GitHub mutation handler is not configured")
		return a
	}
	if a.approving != nil {
		a.pushToast("approve-run", usecase.Resilience{Severity: usecase.SeverityInfo, Title: "an approval is still running"})
		return a
	}
	state := &approveState{route: route, runID: request.Run.ID}
	a.approving = state
	label := "approving the run…"
	if n := len(request.Held); n > 1 {
		label = fmt.Sprintf("approving %d runs…", n)
	}
	a.pushToast("approve-run", usecase.Resilience{Severity: usecase.SeverityInfo, Title: label})
	handler := a.actionHandler
	go func() {
		result, err := handler(request)
		state.mu.Lock()
		state.result, state.err, state.done = result, err, true
		state.mu.Unlock()
	}()
	return a
}

// drainApprove reports the approvals' outcome. From the detail of the
// approved run it returns to the list; the list reloads, so approved
// runs (all, or the ones before a refusal) show queued.
func (a App) drainApprove() (App, bool) {
	state := a.approving
	if state == nil {
		return a, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.done {
		return a, false
	}
	a.approving = nil
	a.toasts = a.toasts.Dismiss("approve-run")
	if state.err != nil {
		a.pushErrorToast("action-failed", usecase.ResilienceFor(state.err, usecase.ErrorContext{}))
	} else {
		a.pushToast("action-ok", usecase.ResilienceForSuccess(state.result))
	}
	if state.route == RouteDetail && a.Route() == RouteDetail && a.detail.Run.ID == state.runID {
		a.PopRoute()
	}
	if a.loadBlocked(loadKindRuns) {
		// Another load owns the slot; the next poll shows the queued runs.
		return a, true
	}
	return a.reloadRuns(), true
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
