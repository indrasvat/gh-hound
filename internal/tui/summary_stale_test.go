package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// An unfinished row nothing has re-read lately (it slid past page 1 and
// far from the selection) may have finished unseen: the summary must not
// count it as running, and must say how many it cannot vouch for.
func TestSummaryCountsOnlyRecentlyReadRowsAsRunning(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg})
	fresh := paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)
	old := paceRow(500, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)
	queued := paceRow(499, ".github/workflows/ci.yml", "main", "push", model.StatusQueued, model.ConclusionNone, 0)
	done := paceRow(498, ".github/workflows/ci.yml", "main", "push", model.StatusCompleted, model.ConclusionSuccess, time.Minute)
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Runs: []model.Run{fresh, old, queued, done}})
	app.routes = []Route{RouteRuns}
	now := time.Now()
	app.runsConfirmed = confirmedAt(nil, []model.Run{old, queued, done}, now.Add(-time.Hour))
	app.runsConfirmed = confirmedAt(app.runsConfirmed, []model.Run{fresh}, now)

	view := ansi.Strip(app.ViewSize(120, 30))
	if !strings.Contains(view, "0 failing · 1 running · 2 stale · 1 passed") {
		t.Fatalf("summary must count only the fresh row as running:\n%s", view)
	}

	// Read again, the same rows count as running and the note goes.
	app.runsConfirmed = confirmedAt(app.runsConfirmed, []model.Run{old, queued}, now)
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "0 failing · 3 running · 1 passed") || strings.Contains(view, "stale") {
		t.Fatalf("re-read rows are running again:\n%s", view)
	}

	// A slow poll (rate-limit backoff) does not make page 1 stale.
	app.pollInterval = 10 * time.Minute
	app.runsConfirmed = confirmedAt(nil, []model.Run{fresh, old, queued, done}, now.Add(-20*time.Minute))
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "stale") {
		t.Fatalf("rows read within three polls are not stale:\n%s", view)
	}
}

// Launch reads the branch, then the repo. Both are fresh reads, and the
// later one wins: s straight after launch shows no stale repo rows, and
// a branch row the repo read saw finish is not counted as running.
func TestSummaryTrustsBothScopesFetchedAtLaunch(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	branchRun := paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusQueued, model.ConclusionNone, 0)
	repoRun := paceRow(601, ".github/workflows/ci.yml", "feat/x", "push", model.StatusInProgress, model.ConclusionNone, 0)
	finished := branchRun
	finished.Status, finished.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	launch := usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Scope: usecase.LaunchScopeBranch,
		Runs: []model.Run{branchRun}, BranchRuns: []model.Run{branchRun}, RepoRuns: []model.Run{repoRun, finished}}
	app := NewApp(Options{Config: cfg, Launch: launch})
	now := time.Now()
	if stale := app.staleRuns(launch.RepoRuns, now); len(stale) != 0 {
		t.Fatalf("repo rows fetched at launch read stale: %v", stale)
	}
	if !app.staleRuns(launch.BranchRuns, now)[600] {
		t.Fatal("the branch copy of a run the later repo read saw finish must not count as running")
	}
}

// A fresh read of a run that finished must not vouch for another
// scope's older copy that still says running.
func TestSummaryDoesNotTrustAnotherSnapshotsStatus(t *testing.T) {
	app := NewApp(Options{Config: config.Default()})
	old := paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)
	finished := old
	finished.Status, finished.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	app.runsConfirmed = confirmedAt(nil, []model.Run{finished}, time.Now())
	if !app.staleRuns([]model.Run{old}, time.Now())[old.ID] {
		t.Fatal("an in_progress copy vouched for by a completed read must read stale")
	}
}

// On a busy repo the next page repeats listed runs, possibly newer. The
// listed copy stays, so the page must not vouch for it: recording the
// page's newer status made a just-read row read stale at once.
func TestLoadMoreVouchesOnlyForTheRowsItAdds(t *testing.T) {
	rows := []model.Run{}
	for i := range 30 {
		run := actorRun(int64(900-i), "alice", "trunk", model.ConclusionNone)
		run.Status = model.StatusInProgress
		rows = append(rows, run)
	}
	repeated := rows[29]
	repeated.Status, repeated.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	next := actorRun(800, "alice", "trunk", model.ConclusionSuccess)
	app := newActorApp(&actorRuns{rows: rows}, nil)
	app.runsResolver = func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
		if filter.Page > 1 {
			return []model.Run{repeated, next}, nil
		}
		return rows, nil
	}
	app = settleApp(t, typeQuery(t, app, "actor:alice"))
	app = settleApp(t, typeKeys(t, app, "G"))
	if len(app.runs.Context.Runs) != 31 {
		t.Fatalf("rows = %d, want the new run appended", len(app.runs.Context.Runs))
	}
	if stale := app.staleRuns(app.runs.Context.Runs, time.Now()); len(stale) != 0 {
		t.Fatalf("rows just read read stale: %v", stale)
	}
}

// A re-read that a status filter drops still read the run: the other
// scope's copy, still running, must not count as running.
func TestDroppedRefreshStillCountsAsRead(t *testing.T) {
	app := NewApp(Options{Config: config.Default()})
	cached := paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)
	app.runsConfirmed = confirmedAt(nil, []model.Run{cached}, time.Now())
	finished := cached
	finished.Status, finished.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	_, dropped, _ := refreshUnfinished(context.Background(), func(context.Context, int64) (model.Run, error) { return finished, nil },
		[]int64{600}, nil, string(model.StatusInProgress))
	app = app.applyRunsRefresh(fetchedRuns{dropped: dropped, at: time.Now(), refreshedAt: time.Now()}, nil, "")
	if !app.staleRuns([]model.Run{cached}, time.Now())[600] {
		t.Fatal("a run read as finished must not count as running in another scope")
	}
}
