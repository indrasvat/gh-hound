package tui

import (
	"fmt"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// heldOnCommit is run and every loaded run from the same fork commit
// still held for approval: GitHub's "Approve and run" lets all of a
// pull request's workflows start, and so does A.
func (a App) heldOnCommit(run model.Run) []model.Run {
	held := []model.Run{run}
	for _, other := range a.runs.Context.Runs {
		if other.ID == run.ID || other.HeadSHA != run.HeadSHA || !other.FromFork {
			continue
		}
		if verdict, _ := model.ClassifyRun(other, nil); verdict.WaitingOnPerson() {
			held = append(held, other)
		}
	}
	return held
}

// openApproveRun asks before letting a held fork run start: approving
// runs someone else's code, so the confirm defaults to No. A held bot
// or agent run is approved on the web; GitHub's endpoint covers forks.
func (a App) openApproveRun(route Route, run model.Run) App {
	if !run.FromFork {
		a.pushToast("approve-in-browser", usecase.Resilience{
			Severity: usecase.SeverityInfo,
			Title:    "approve this one on GitHub",
			Message:  "hound approves fork runs only · approve it on the run's page",
		})
		return a
	}
	return a.openConfirm(route, ActionRequest{Action: usecase.ActionApproveRun, Run: run, Held: a.heldOnCommit(run)})
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
