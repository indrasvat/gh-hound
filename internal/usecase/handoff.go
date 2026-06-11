package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// ErrScentLost is the typed give-up for a dispatch whose run never
// surfaced inside the discovery budget. Callers toast it in the hound
// voice and return the user to the runs list.
var ErrScentLost = errors.New("couldn't pick up the scent")

// RunGetter is the single capability the rerun attach needs.
type RunGetter interface {
	GetRun(context.Context, string, int64) (model.Run, error)
}

// HandoffService connects a mutation to the watch that follows it.
//
//   - Dispatch on modern hosts returns the run id in the 200 body —
//     no discovery needed, attach directly.
//   - Dispatch on 204 hosts (older GHES) gets a bounded discovery
//     poll: the workflow's runs filtered by ref + workflow_dispatch +
//     created after the dispatch timestamp.
//   - Rerun never discovers: the run id is already known; poll until
//     the attempt counter advances or the status leaves completed.
type HandoffService struct {
	History WorkflowRunHistory
	Runs    RunGetter
	Clock   Clock
	// MaxPolls and Interval bound both polls: 10 polls / 3s apart
	// (≈30s) by default, then give up gracefully.
	MaxPolls int
	Interval time.Duration
}

const (
	defaultHandoffPolls    = 10
	defaultHandoffInterval = 3 * time.Second
	// handoffDiscoveryPerPage keeps each discovery poll to one tiny
	// page: the freshest dispatch lands at the top.
	handoffDiscoveryPerPage = 10
)

func (s HandoffService) maxPolls() int {
	if s.MaxPolls > 0 {
		return s.MaxPolls
	}
	return defaultHandoffPolls
}

func (s HandoffService) interval() time.Duration {
	if s.Interval > 0 {
		return s.Interval
	}
	return defaultHandoffInterval
}

func (s HandoffService) clock() Clock {
	if s.Clock != nil {
		return s.Clock
	}
	return realClock{}
}

// handoffQuerySkew widens only the SERVER query fence: created_at is
// stamped by GitHub's clock, which may trail the local one. The
// client-side freshness check stays strict at the dispatch instant —
// a pre-dispatch run selected through the skew window would be a
// false attach, which is worse than a lost scent.
const handoffQuerySkew = 5 * time.Second

// DiscoverDispatchedRun is the 204-fallback ONLY: find the run a
// dispatch created when the response body carried no run id. Filters
// server-side by workflow + ref + event=workflow_dispatch + created
// after the (skew-widened) dispatch timestamp; client-side the fence
// is strict; returns ErrScentLost once the budget is spent.
func (s HandoffService) DiscoverDispatchedRun(ctx context.Context, repo, workflow, ref string, since time.Time) (model.Run, error) {
	if s.History == nil {
		return model.Run{}, errors.New("run history is unavailable for this adapter")
	}
	clock := s.clock()
	for poll := 0; poll < s.maxPolls(); poll++ {
		if poll > 0 {
			clock.Sleep(s.interval())
		}
		if err := ctx.Err(); err != nil {
			return model.Run{}, err
		}
		listed, err := s.History.ListWorkflowRuns(ctx, repo, workflow, RunFilter{
			Branch:       ref,
			Event:        "workflow_dispatch",
			CreatedAfter: since.Add(-handoffQuerySkew),
			PerPage:      handoffDiscoveryPerPage,
		})
		if err != nil {
			return model.Run{}, err
		}
		if run, ok := newestRunSince(listed, since); ok {
			return run, nil
		}
	}
	return model.Run{}, ErrScentLost
}

// newestRunSince picks the freshest run created at or after the
// dispatch timestamp; the created_after filter is also asserted
// client-side so a host ignoring the qualifier cannot hand back a
// stale run.
func newestRunSince(runs []model.Run, since time.Time) (model.Run, bool) {
	var newest model.Run
	found := false
	for _, run := range runs {
		if run.CreatedAt.Before(since) {
			continue
		}
		if !found || run.CreatedAt.After(newest.CreatedAt) || (run.CreatedAt.Equal(newest.CreatedAt) && run.ID > newest.ID) {
			newest = run
			found = true
		}
	}
	return newest, found
}

// AwaitRerunStart polls the rerun's existing run id until the attempt
// counter advances past prevAttempt or the status leaves completed —
// whichever the host surfaces first. If neither happens inside the
// budget it attaches anyway: watching the known run is always safe.
func (s HandoffService) AwaitRerunStart(ctx context.Context, repo string, runID int64, prevAttempt int) (model.Run, error) {
	if s.Runs == nil {
		return model.Run{}, errors.New("run lookups are unavailable for this adapter")
	}
	clock := s.clock()
	var last model.Run
	for poll := 0; poll < s.maxPolls(); poll++ {
		if poll > 0 {
			clock.Sleep(s.interval())
		}
		if err := ctx.Err(); err != nil {
			return model.Run{}, err
		}
		run, err := s.Runs.GetRun(ctx, repo, runID)
		if err != nil {
			return model.Run{}, err
		}
		last = run
		if run.RunAttempt > prevAttempt || run.Status != model.StatusCompleted {
			return run, nil
		}
	}
	return last, nil
}
