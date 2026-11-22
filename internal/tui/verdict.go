package tui

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// verdictRetry is how long a run whose jobs could not be read waits
// before a tick asks again; until then it reads as failing.
const verdictRetry = 2 * time.Minute

// verdictBatch bounds the runs one fetch reads (one jobs listing each,
// plus one annotations listing per failing job that ran).
const verdictBatch = 10

// verdictKey is a run attempt: a finished attempt's jobs never change,
// so its verdict is kept for the session.
type verdictKey struct {
	run     int64
	attempt int
}

// verdictFetchState carries one batch of red runs being classified.
// Pointer-held so results survive App value copies; drained on ticks
// and keypresses, a run at a time as each lands (pace precedent).
type verdictFetchState struct {
	mu      sync.Mutex
	results map[verdictKey]model.RunVerdict
	failed  map[verdictKey]bool
	done    bool
	cancel  context.CancelFunc
}

// startVerdictFetch classifies the red rows near the selection whose
// run fields do not settle them, nearest first, off the key path.
func (a App) startVerdictFetch() App {
	if a.verdictResolver == nil || a.verdictFetch != nil || a.Route() != RouteRuns {
		return a
	}
	rows, selected := nearSelection(a.runs)
	now := time.Now()
	var wanted []model.Run
	for _, i := range nearestFirst(selected, len(rows)) {
		run := rows[i]
		key := verdictKey{run.ID, run.RunAttempt}
		if _, known := model.ClassifyRun(run, nil); known {
			continue
		}
		if _, ok := a.verdicts[key]; ok {
			continue
		}
		if at, ok := a.verdictFailed[key]; ok && now.Sub(at) < verdictRetry {
			continue
		}
		if wanted = append(wanted, run); len(wanted) == verdictBatch {
			break
		}
	}
	if len(wanted) == 0 {
		return a
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &verdictFetchState{results: map[verdictKey]model.RunVerdict{}, failed: map[verdictKey]bool{}, cancel: cancel}
	a.verdictFetch = state
	resolver := a.verdictResolver
	go func() {
		defer cancel()
		for _, run := range wanted {
			verdict, err := resolver(ctx, run)
			state.mu.Lock()
			if err == nil {
				state.results[verdictKey{run.ID, run.RunAttempt}] = verdict
			} else if !errors.Is(err, context.Canceled) {
				state.failed[verdictKey{run.ID, run.RunAttempt}] = true
			}
			state.mu.Unlock()
			if ctx.Err() != nil {
				break
			}
		}
		state.mu.Lock()
		state.done = true
		state.mu.Unlock()
	}()
	return a
}

// nearestFirst is 0..n-1 ordered by distance from selected, the row
// above before the row below.
func nearestFirst(selected, n int) []int {
	order := make([]int, 0, n)
	for d := 0; len(order) < n; d++ {
		if i := selected - d; i >= 0 && i < n {
			order = append(order, i)
		}
		if i := selected + d; d > 0 && i >= 0 && i < n {
			order = append(order, i)
		}
		if selected-d < 0 && selected+d >= n {
			break
		}
	}
	return order
}

// drainVerdictFetch folds in the verdicts that have landed; the slot
// frees once the batch is done.
func (a App) drainVerdictFetch() (App, bool) {
	fetch := a.verdictFetch
	if fetch == nil {
		return a, false
	}
	fetch.mu.Lock()
	defer fetch.mu.Unlock()
	changed := len(fetch.results) > 0
	if changed {
		verdicts := make(map[verdictKey]model.RunVerdict, len(a.verdicts)+len(fetch.results))
		maps.Copy(verdicts, a.verdicts)
		maps.Copy(verdicts, fetch.results)
		a.verdicts = verdicts
		clear(fetch.results)
	}
	if len(fetch.failed) > 0 {
		failed := make(map[verdictKey]time.Time, len(a.verdictFailed)+len(fetch.failed))
		maps.Copy(failed, a.verdictFailed)
		for key := range fetch.failed {
			failed[key] = time.Now()
		}
		a.verdictFailed = failed
		clear(fetch.failed)
		if !a.verdictWarned {
			// The row stays red as failing: a miss must not pass for a
			// verdict. Said once a session.
			a.verdictWarned = true
			a.pushToast("verdict-unavailable", usecase.Resilience{
				Severity: usecase.SeverityWarn,
				Title:    "could not read why a run is red",
				Message:  "it counts as failing until its jobs can be read",
			})
		}
		changed = true
	}
	if fetch.done {
		a.verdictFetch = nil
		changed = true
	}
	return a, changed
}

// verdictsFor maps the rows to the verdicts read for the attempt each
// row shows.
func (a App) verdictsFor(rows []model.Run) map[int64]model.RunVerdict {
	out := map[int64]model.RunVerdict{}
	for _, run := range rows {
		if verdict, ok := a.verdicts[verdictKey{run.ID, run.RunAttempt}]; ok {
			out[run.ID] = verdict
		}
	}
	return out
}

// detailNeverRan reports an open detail run in which no job ran.
func (a App) detailNeverRan() bool {
	_, never := a.detail.NeverRan()
	return never
}
