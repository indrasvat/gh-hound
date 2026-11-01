package tui

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// paceTTL bounds how long a baseline (or a failed fetch) stands before
// the next tick reads it again.
const paceTTL = 15 * time.Minute

type paceEntry struct {
	baseline  usecase.PaceBaseline
	fetchedAt time.Time
	// covered are the green runs of this key already accounted for:
	// on screen when it was fetched, or in its sample. A green run not
	// in it is new history, and refreshes the baseline once.
	covered map[int64]bool
}

// paceFetchState carries one baseline fetch. Pointer-held so the
// goroutine's result survives App value copies; drained on poll ticks
// and keypresses (flakes precedent). One rides at a time.
type paceFetchState struct {
	mu  sync.Mutex
	key string
	// gen is the cache generation the fetch started under; a reset
	// moves on, and its result is dropped even if it completed.
	gen      int
	covered  map[int64]bool
	baseline usecase.PaceBaseline
	err      error
	done     bool
	cancel   context.CancelFunc
}

// paceActive: on in config and not hidden with p this session. Off
// means no marks and no requests.
func (a App) paceActive() bool {
	return a.config.Pace && !a.paceHidden && a.paceResolver != nil
}

// paceKey is a baseline's identity: repository, workflow, branch and
// event. Empty for a run that has no green baseline to compare with.
func paceKey(repo string, run model.Run) string {
	if run.RunAttempt > 1 {
		return ""
	}
	running := run.Status == model.StatusInProgress
	green := run.Status == model.StatusCompleted && run.Conclusion == model.ConclusionSuccess
	if !running && !green {
		return ""
	}
	workflow := run.Path
	if workflow == "" && run.WorkflowID != 0 {
		workflow = strconv.FormatInt(run.WorkflowID, 10)
	}
	if workflow == "" {
		return ""
	}
	return repo + "|" + workflow + "|" + run.HeadBranch + "|" + run.Event
}

// paceScope is the repository and the runs on the current screen.
func (a App) paceScope() (string, []model.Run) {
	switch a.Route() {
	case RouteRuns:
		return a.runs.Context.Repo, a.runs.Context.Runs
	case RouteWatch:
		if a.watch.State.Run.ID == 0 {
			return "", nil
		}
		return a.watch.State.Repo, []model.Run{a.watch.State.Run}
	case RouteWatchBoard:
		return a.board.Repo, a.board.Runs
	}
	return "", nil
}

// paceStale: past its TTL, or a green run of its key that the baseline
// has not accounted for. Run membership, not timestamps: GitHub's clock
// and ours need not agree.
func paceStale(entry paceEntry, key, repo string, runs []model.Run, now time.Time) bool {
	if now.Sub(entry.fetchedAt) > paceTTL {
		return true
	}
	for _, run := range greenRuns(key, repo, runs) {
		if !entry.covered[run] {
			return true
		}
	}
	return false
}

func greenRuns(key, repo string, runs []model.Run) []int64 {
	var ids []int64
	for _, run := range runs {
		if run.Status == model.StatusCompleted && run.Conclusion == model.ConclusionSuccess && paceKey(repo, run) == key {
			ids = append(ids, run.ID)
		}
	}
	return ids
}

// startPaceFetch begins the next missing or stale baseline for the
// screen's runs. Runs on ticks only, never on render or the keypress
// path, and never while another fetch rides.
func (a App) startPaceFetch() App {
	if !a.paceActive() || a.paceFetch != nil {
		return a
	}
	repo, runs := a.paceScope()
	now := time.Now()
	for _, run := range runs {
		key := paceKey(repo, run)
		if key == "" {
			continue
		}
		if failed, ok := a.paceFailed[key]; ok && now.Sub(failed) < paceTTL {
			continue
		}
		if entry, ok := a.paceBaselines[key]; ok && !paceStale(entry, key, repo, runs, now) {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		covered := map[int64]bool{}
		for _, id := range greenRuns(key, repo, runs) {
			covered[id] = true
		}
		state := &paceFetchState{key: key, gen: a.paceGen, covered: covered, cancel: cancel}
		a.paceFetch = state
		resolver := a.paceResolver
		go func() {
			defer cancel()
			baseline, err := resolver(ctx, run)
			state.mu.Lock()
			state.baseline, state.err, state.done = baseline, err, true
			state.mu.Unlock()
		}()
		return a
	}
	return a
}

func (a App) drainPaceFetch() (App, bool) {
	fetch := a.paceFetch
	if fetch == nil {
		return a, false
	}
	fetch.mu.Lock()
	defer fetch.mu.Unlock()
	if !fetch.done {
		return a, false
	}
	a.paceFetch = nil
	if fetch.gen != a.paceGen {
		// Started before a reset: cancellation cannot recall a request
		// already answered, so its result is dropped here.
		return a, true
	}
	if errors.Is(fetch.err, context.Canceled) {
		// Abandoned on purpose: no toast. Still report the change, or
		// the caller keeps the finished fetch and no other ever starts.
		return a, true
	}
	if fetch.err != nil {
		failed := make(map[string]time.Time, len(a.paceFailed)+1)
		maps.Copy(failed, a.paceFailed)
		failed[fetch.key] = time.Now()
		a.paceFailed = failed
		if !a.paceWarned {
			// Once per session: a rate-limited hour must not stack a
			// toast per workflow.
			a.paceWarned = true
			a.pushToast("pace-unavailable", usecase.Resilience{
				Severity: usecase.SeverityWarn,
				Title:    "pace unavailable",
				Message:  "could not read recent green runs; durations show without estimates",
			})
		}
		return a, true
	}
	baselines := make(map[string]paceEntry, len(a.paceBaselines)+1)
	maps.Copy(baselines, a.paceBaselines)
	for _, sample := range fetch.baseline.Samples {
		fetch.covered[sample.RunID] = true
	}
	baselines[fetch.key] = paceEntry{baseline: fetch.baseline, fetchedAt: time.Now(), covered: fetch.covered}
	a.paceBaselines = baselines
	return a, true
}

// abandonPaceFetch cancels the fetch in flight; its result is dropped.
func (a *App) abandonPaceFetch() {
	if fetch := a.paceFetch; fetch != nil && fetch.cancel != nil {
		fetch.cancel()
	}
}

// resetPace drops every baseline: a rerun is about to add attempts.
func (a *App) resetPace() {
	a.abandonPaceFetch()
	a.paceGen++
	a.paceBaselines = map[string]paceEntry{}
	a.paceFailed = map[string]time.Time{}
}

// paceFor maps each run to its cached baseline; nil when Pace is off.
// Pure lookup: rendering never spends a request.
func (a App) paceFor(repo string, runs []model.Run) map[int64]usecase.PaceBaseline {
	if !a.paceActive() {
		return nil
	}
	out := map[int64]usecase.PaceBaseline{}
	for _, run := range runs {
		if entry, ok := a.paceBaselines[paceKey(repo, run)]; ok {
			out[run.ID] = entry.baseline
		}
	}
	return out
}

// paceScreen: the screens p acts on.
func (a App) paceScreen() bool {
	switch a.footerScreen() {
	case keys.ScreenRunsList, keys.ScreenWatch, keys.ScreenWatchDone, keys.ScreenWatchBoard:
		return true
	}
	return false
}

// togglePace hides or shows Pace for the session. Hidden, it spends
// nothing; shown again, cached baselines answer at once.
func (a App) togglePace() App {
	if !a.config.Pace {
		a.pushToast("pace-toggle", usecase.Resilience{Severity: usecase.SeverityInfo, Title: "pace is off in config", Message: "set pace = true (or HOUND_PACE=true) to see ETAs"})
		return a
	}
	a.paceHidden = !a.paceHidden
	if a.paceHidden {
		a.abandonPaceFetch()
		a.pushToast("pace-toggle", usecase.Resilience{Severity: usecase.SeverityInfo, Title: "pace off · p brings it back"})
		return a
	}
	// The marks coming back say it; the off toast would only cover them.
	a.toasts = a.toasts.Dismiss("pace-toggle")
	return a
}
