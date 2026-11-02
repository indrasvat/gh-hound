package tui

import (
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

// Launch fetches both scopes; s straight after must not call the repo
// rows stale.
func TestSummaryTrustsBothScopesFetchedAtLaunch(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	branchRun := paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)
	repoRun := paceRow(601, ".github/workflows/ci.yml", "feat/x", "push", model.StatusInProgress, model.ConclusionNone, 0)
	launch := usecase.LaunchContext{Repo: "o/r", Branch: "main", State: usecase.LaunchStateRuns, Scope: usecase.LaunchScopeRepo,
		Runs: []model.Run{repoRun, branchRun}, BranchRuns: []model.Run{branchRun}, RepoRuns: []model.Run{repoRun, branchRun}}
	app := NewApp(Options{Config: cfg, Launch: launch})
	if stale := app.staleRuns(app.runs.Context.RepoRuns, time.Now()); len(stale) != 0 {
		t.Fatalf("rows fetched at launch read stale: %v", stale)
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

// On a busy repo the next page repeats runs already listed. The newer
// copy replaces the listed one, so the row shows what the fetch saw and
// its read vouches for it (before, the old copy stayed and read stale).
func TestLoadMoreRefreshesRunsItRepeats(t *testing.T) {
	app := NewApp(Options{Config: config.Default()})
	listed := paceRow(600, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "o/r", Scope: usecase.LaunchScopeRepo, State: usecase.LaunchStateRuns,
		Runs: []model.Run{listed}, RepoRuns: []model.Run{listed}})
	finished := listed
	finished.Status, finished.Conclusion, finished.UpdatedAt = model.StatusCompleted, model.ConclusionSuccess, listed.UpdatedAt.Add(time.Minute)
	older := paceRow(590, ".github/workflows/ci.yml", "main", "push", model.StatusInProgress, model.ConclusionNone, 0)

	if added := app.appendActiveRuns([]model.Run{finished, older}); added != 1 {
		t.Fatalf("added = %d, want only the new run appended", added)
	}
	app.runsConfirmed = confirmedAt(app.runsConfirmed, []model.Run{finished, older}, time.Now())
	for _, rows := range [][]model.Run{app.runs.Context.Runs, app.runs.Context.RepoRuns} {
		if len(rows) != 2 || rows[0].Status != model.StatusCompleted {
			t.Fatalf("the repeated run must take the newer copy: %+v", rows)
		}
	}
	if stale := app.staleRuns(app.runs.Context.Runs, time.Now()); len(stale) != 0 {
		t.Fatalf("rows just read must not be stale: %v", stale)
	}
}
