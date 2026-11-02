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
