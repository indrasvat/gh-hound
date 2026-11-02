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
	runspkg "github.com/indrasvat/gh-hound/internal/tui/screens/runs"
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
const maxRunRefreshes = 3

// paceMinRefresh is the least age at which a new green run refreshes a
// baseline: a 20-run median barely moves with one more run, and busy
// workflows go green every few seconds.
const paceMinRefresh = 10 * time.Minute

// paceRowSpan bounds the runs-list rows Pace works for to those within
// this many rows of the selection: what is on screen, not every page
// loaded with G.
const paceRowSpan = 30

type paceEntry struct {
	baseline  usecase.PaceBaseline
	fetchedAt time.Time
	// covered are the green runs of this key already accounted for:
	// on screen when it was fetched, or in its sample. A green run not
	// in it is new history, and refreshes the baseline once.
	covered map[int64]bool
}

// paceJobEntry is a judged run's job and step baseline, and the run
// baseline it was built on: a refreshed run baseline rebuilds it.
type paceJobEntry struct {
	baseline usecase.PaceJobBaseline
	runAt    time.Time
}

// paceFetchState carries one baseline fetch. Pointer-held so the
// goroutine's result survives App value copies; drained on poll ticks
// and keypresses (flakes precedent). One rides at a time.
type paceFetchState struct {
	mu       sync.Mutex
	key      string
	covered  map[int64]bool
	baseline usecase.PaceBaseline
	// jobKey marks a job baseline fetch (stored under key, which is
	// the same), built on the run baseline of runAt.
	jobKey string
	runAt  time.Time
	jobs   usecase.PaceJobBaseline
	err    error
	done   bool
	cancel context.CancelFunc
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
	return workflowKey(repo, run)
}

// workflowKey is paceKey whatever the run's outcome: a failed run's
// green jobs and steps are still judged against its workflow's history.
func workflowKey(repo string, run model.Run) string {
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
		rows, selected := nearSelection(a.runs)
		return a.runs.Context.Repo, rows, selected
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
	if a.Route() == RouteDetail {
		return a.startDetailPaceFetch()
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
		covered := map[int64]bool{}
		for _, id := range greenRuns(key, repo, runs) {
			covered[id] = true
		}
		a = a.launchRunBaseline(key, covered, run)
	}
	return a
}

// launchRunBaseline starts the one fetch in flight: run's baseline,
// stored under key with the green runs it already accounts for.
func (a App) launchRunBaseline(key string, covered map[int64]bool, run model.Run) App {
	ctx, cancel := context.WithCancel(context.Background())
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
	if fetch.jobKey != "" {
		jobs := make(map[string]paceJobEntry, len(a.paceJobs)+1)
		maps.Copy(jobs, a.paceJobs)
		jobs[fetch.key] = paceJobEntry{baseline: fetch.jobs, runAt: fetch.runAt}
		a.paceJobs = jobs
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
	case keys.ScreenRunsList, keys.ScreenAllGreen, keys.ScreenDetail, keys.ScreenWatch, keys.ScreenWatchDone, keys.ScreenWatchBoard:
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

// runConfirmation is when a fetch returned a run, and the snapshot it
// returned: a cache holding an older snapshot of the same run (the
// other scope's list) is not vouched for.
type runConfirmation struct {
	at      time.Time
	status  model.Status
	updated time.Time
}

// nearSelection is the loaded runs within paceRowSpan of the selected
// one, and the selection's index among them.
func nearSelection(m runspkg.Model) ([]model.Run, int) {
	rows := m.Context.Runs
	selected := 0
	if run, ok := m.SelectedRun(); ok {
		selected = max(slices.IndexFunc(rows, func(r model.Run) bool { return r.ID == run.ID }), 0)
	}
	lo, hi := max(selected-paceRowSpan, 0), min(selected+paceRowSpan+1, len(rows))
	return rows[lo:hi], selected - lo
}

// paceForRuns is paceFor for the runs list: only rows near the
// selection, and a running row only while a fetch confirmed its
// snapshot within paceConfirmWindow. A running row is judged as of
// that confirmation (asOf), so a run that finished unseen cannot grow
// into an overrun or a false "due".
func (a App) paceForRuns(m runspkg.Model) (map[int64]usecase.PaceBaseline, map[int64]time.Time) {
	rows, _ := nearSelection(m)
	out := a.paceFor(m.Context.Repo, rows)
	asOf := map[int64]time.Time{}
	now := time.Now()
	for _, run := range rows {
		if _, ok := out[run.ID]; !ok || run.Status == model.StatusCompleted {
			continue
		}
		c, ok := a.runsConfirmed[run.ID]
		if !ok || c.status != run.Status || !c.updated.Equal(run.UpdatedAt) || now.Sub(c.at) > paceConfirmWindow {
			delete(out, run.ID)
			continue
		}
		asOf[run.ID] = c.at
	}
	return out, asOf
}

// staleRuns is the unfinished rows no fetch has returned unfinished
// within three polls (five minutes at least): rows past page 1 and far
// from the selection are not re-read, and may have finished unseen. A
// read that saw the run finished never vouches for a copy that is not.
func (a App) staleRuns(rows []model.Run, now time.Time) map[int64]bool {
	limit := max(5*time.Minute, 3*a.pollDueInterval())
	out := map[int64]bool{}
	for _, run := range rows {
		if run.Status == model.StatusCompleted {
			continue
		}
		if c, ok := a.runsConfirmed[run.ID]; !ok || c.status == model.StatusCompleted || now.Sub(c.at) > limit {
			out[run.ID] = true
		}
	}
	return out
}

// confirmedAt records that a fetch returned rows at at (when the
// response arrived, not when it was applied). A fresh map: App copies
// share the old one.
func confirmedAt(confirmed map[int64]runConfirmation, rows []model.Run, at time.Time) map[int64]runConfirmation {
	out := make(map[int64]runConfirmation, len(confirmed)+len(rows))
	maps.Copy(out, confirmed)
	for _, run := range rows {
		out[run.ID] = runConfirmation{at: at, status: run.Status, updated: run.UpdatedAt}
	}
	return out
}

// refreshCandidates is up to n unfinished rows near the selection to
// re-read: the selected one first, then the least recently read (a
// confirmation or a failed attempt both count), so each is read in turn
// and a failing one cannot starve the rest.
func (a App) refreshCandidates(n int) []int64 {
	rows, selected := nearSelection(a.runs)
	var ids []int64
	last := map[int64]time.Time{}
	if selected < len(rows) && rows[selected].Status != model.StatusCompleted {
		last[rows[selected].ID] = time.Time{}.Add(-time.Hour) // always first
	}
	for _, run := range rows {
		if run.Status != model.StatusCompleted && run.ID != 0 {
			ids = append(ids, run.ID)
			if _, first := last[run.ID]; first {
				continue
			}
			last[run.ID] = a.runsConfirmed[run.ID].at
			if tried := a.runsTried[run.ID]; tried.After(last[run.ID]) {
				last[run.ID] = tried
			}
		}
	}
	slices.SortStableFunc(ids, func(x, y int64) int { return last[x].Compare(last[y]) })
	return ids[:min(n, len(ids))]
}

// refreshUnfinished re-reads the candidates the page did not return,
// keeping a row only while it still matches the server status filter
// (status is the one filtered field a run changes). It returns the rows
// to keep, the rows to drop (as read), and every id it asked about. A failed read
// leaves the row as it was and unconfirmed, so Pace stays silent on it.
func refreshUnfinished(ctx context.Context, refresher func(context.Context, int64) (model.Run, error), candidates []int64, page []model.Run, status string) (kept, dropped []model.Run, tried []int64) {
	for _, id := range candidates {
		if slices.ContainsFunc(page, func(r model.Run) bool { return r.ID == id }) {
			continue
		}
		tried = append(tried, id)
		run, err := refresher(ctx, id)
		if err != nil {
			continue
		}
		if status != "" && string(run.Status) != status && string(run.Conclusion) != status {
			dropped = append(dropped, run)
			continue
		}
		kept = append(kept, run)
	}
	return kept, dropped, tried
}

// detailPaceKey is the detail run's job baseline identity: its workflow
// key and the run itself, which its baseline leaves out.
func (a App) detailPaceKey() (string, string) {
	key := workflowKey(a.detail.Repo, a.detail.Run)
	if key == "" {
		return "", ""
	}
	return key, key + "#" + strconv.FormatInt(a.detail.Run.ID, 10)
}

// startDetailPaceFetch serves the open detail run: its run baseline
// first, then its job baseline built on it. Missing or past paceTTL, or
// built on an older run baseline; a failed key waits paceTTL.
func (a App) startDetailPaceFetch() App {
	key, jobKey := a.detailPaceKey()
	if key == "" || a.paceJobResolver == nil {
		return a
	}
	now := time.Now()
	recentlyFailed := func(k string) bool {
		failed, ok := a.paceFailed[k]
		return ok && now.Sub(failed) < paceTTL
	}
	run := a.detail.Run
	entry, ok := a.paceBaselines[key]
	if !ok || now.Sub(entry.fetchedAt) > paceTTL {
		if recentlyFailed(key) {
			return a
		}
		return a.launchRunBaseline(key, map[int64]bool{}, run)
	}
	// A job baseline lives as long as the run baseline it was built on.
	if jobs, ok := a.paceJobs[jobKey]; ok && jobs.runAt.Equal(entry.fetchedAt) || recentlyFailed(jobKey) {
		return a
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &paceFetchState{key: jobKey, jobKey: jobKey, runAt: entry.fetchedAt, cancel: cancel}
	a.paceFetch = state
	resolver, baseline := a.paceJobResolver, entry.baseline
	go func() {
		defer cancel()
		jobs, err := resolver(ctx, run, baseline)
		state.mu.Lock()
		state.jobs, state.err, state.done = jobs, err, true
		state.mu.Unlock()
	}()
	return a
}

// detailPace is the open detail run's job baseline; nil when Pace is
// off or it has not arrived.
func (a App) detailPace() *usecase.PaceJobBaseline {
	if !a.paceActive() {
		return nil
	}
	_, jobKey := a.detailPaceKey()
	entry, ok := a.paceJobs[jobKey]
	if !ok {
		return nil
	}
	return &entry.baseline
}
