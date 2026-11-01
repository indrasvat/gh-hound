package tui

import (
	"context"
	"errors"
	"maps"
	"slices"
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

// paceConfirmWindow is how recently a fetch must have returned a running
// row for Pace to judge it: a row that slid past page 1 may have finished
// unseen, and an overrun mark on it would be a confident false claim.
const paceConfirmWindow = 2 * time.Minute

// maxRunRefreshes bounds the unfinished rows past page 1 that one runs
// poll re-reads (304s when unchanged cost no primary rate limit).
const maxRunRefreshes = 5

// paceMinRefresh is the least age at which a new green run refreshes a
// baseline: a busy bot workflow goes green every few seconds and must
// not keep the one fetch slot from the rows on screen.
const paceMinRefresh = 2 * time.Minute

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
	mu       sync.Mutex
	key      string
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

// paceScope is the repository, the runs on the current screen, and the
// index of the selected one.
func (a App) paceScope() (string, []model.Run, int) {
	switch a.Route() {
	case RouteRuns:
		selected := 0
		if run, ok := a.runs.SelectedRun(); ok {
			selected = max(slices.IndexFunc(a.runs.Context.Runs, func(r model.Run) bool { return r.ID == run.ID }), 0)
		}
		return a.runs.Context.Repo, a.runs.Context.Runs, selected
	case RouteWatch:
		if a.watch.State.Run.ID == 0 {
			return "", nil, 0
		}
		return a.watch.State.Repo, []model.Run{a.watch.State.Run}, 0
	case RouteWatchBoard:
		return a.board.Repo, a.board.Runs, a.board.Selected
	}
	return "", nil, 0
}

// paceStale: past its TTL, or a green run of its key that the baseline
// has not accounted for. Run membership, not timestamps: GitHub's clock
// and ours need not agree.
func paceStale(entry paceEntry, key, repo string, runs []model.Run, now time.Time) bool {
	age := now.Sub(entry.fetchedAt)
	if age > paceTTL {
		return true
	}
	if age < paceMinRefresh {
		return false
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

// startPaceFetch begins the most wanted missing or stale baseline for
// the screen's runs, on poll ticks, while no other fetch rides. Wanted
// first: running runs with no baseline, then other runs with none, then
// stale ones; within each, the nearest to the selection, so the rows in
// view are served before a long list's tail.
func (a App) startPaceFetch() App {
	if !a.paceActive() || a.paceFetch != nil {
		return a
	}
	repo, runs, selected := a.paceScope()
	now := time.Now()
	best, bestRank := -1, 0
	for i, run := range runs {
		key := paceKey(repo, run)
		if key == "" {
			continue
		}
		if failed, ok := a.paceFailed[key]; ok && now.Sub(failed) < paceTTL {
			continue
		}
		tier := 0
		if entry, ok := a.paceBaselines[key]; ok {
			if !paceStale(entry, key, repo, runs, now) {
				continue
			}
			tier = 2
		} else if run.Status != model.StatusInProgress {
			tier = 1
		}
		distance := i - selected
		if distance < 0 {
			distance = -distance
		}
		if rank := tier*len(runs) + distance; best < 0 || rank < bestRank {
			best, bestRank = i, rank
		}
	}
	if best >= 0 {
		run := runs[best]
		key := paceKey(repo, run)
		ctx, cancel := context.WithCancel(context.Background())
		covered := map[int64]bool{}
		for _, id := range greenRuns(key, repo, runs) {
			covered[id] = true
		}
		state := &paceFetchState{key: key, covered: covered, cancel: cancel}
		a.paceFetch = state
		resolver := a.paceResolver
		go func() {
			defer cancel()
			baseline, err := resolver(ctx, run)
			state.mu.Lock()
			state.baseline, state.err, state.done = baseline, err, true
			state.mu.Unlock()
		}()
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

// paceFor maps each run to its cached baseline; nil when Pace is off.
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
		a.pushToast("pace-toggle", usecase.Resilience{Severity: usecase.SeverityInfo, Title: "pace off · " + a.keymap.KeyFor("pace_toggle") + " brings it back"})
		return a
	}
	// The marks coming back say it; the off toast would only cover them.
	a.toasts = a.toasts.Dismiss("pace-toggle")
	return a
}

// paceForRuns is paceFor without the running rows no fetch has
// confirmed within paceConfirmWindow.
func (a App) paceForRuns(repo string, runs []model.Run) map[int64]usecase.PaceBaseline {
	out := a.paceFor(repo, runs)
	now := time.Now()
	for _, run := range runs {
		if run.Status != model.StatusCompleted && now.Sub(a.runsConfirmed[run.ID]) > paceConfirmWindow {
			delete(out, run.ID)
		}
	}
	return out
}

// confirmedNow records that a fetch just returned rows. A fresh map:
// App copies share the old one.
func confirmedNow(confirmed map[int64]time.Time, rows []model.Run) map[int64]time.Time {
	out := make(map[int64]time.Time, len(confirmed)+len(rows))
	maps.Copy(out, confirmed)
	now := time.Now()
	for _, run := range rows {
		out[run.ID] = now
	}
	return out
}

// leastConfirmedUnfinished is up to n unfinished rows, least recently
// confirmed first, so every one is re-read in turn.
func (a App) leastConfirmedUnfinished(n int) []int64 {
	var ids []int64
	for _, run := range a.runs.Context.Runs {
		if run.Status != model.StatusCompleted && run.ID != 0 {
			ids = append(ids, run.ID)
		}
	}
	slices.SortStableFunc(ids, func(x, y int64) int { return a.runsConfirmed[x].Compare(a.runsConfirmed[y]) })
	return ids[:min(n, len(ids))]
}

// refreshUnfinished re-reads the candidates the page did not return. A
// failed read leaves the row as it was and unconfirmed, so Pace stays
// silent on it rather than guessing.
func refreshUnfinished(ctx context.Context, refresher func(context.Context, int64) (model.Run, error), candidates []int64, page []model.Run) []model.Run {
	var out []model.Run
	for _, id := range candidates {
		if slices.ContainsFunc(page, func(r model.Run) bool { return r.ID == id }) {
			continue
		}
		if run, err := refresher(ctx, id); err == nil {
			out = append(out, run)
		}
	}
	return out
}
