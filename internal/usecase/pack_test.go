package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func packRun(id int64, name, sha, event string, status model.Status, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:         id,
		Name:       name,
		HeadSHA:    sha,
		Event:      event,
		Status:     status,
		Conclusion: conclusion,
		RunNumber:  int(id % 1000),
	}
}

func TestPackForRunGroupsByHeadSHAAndEvent(t *testing.T) {
	anchor := packRun(102, "CI", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone)
	rows := []model.Run{
		packRun(104, "Release", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
		// Same sha, DIFFERENT event: the workflow_run chain is its own
		// hunt, not part of this pack.
		packRun(105, "Deploy Pages", "a1b2c3d", "workflow_run", model.StatusInProgress, model.ConclusionNone),
		anchor,
		// Different sha entirely.
		packRun(99, "CI", "f6e5d4c", "push", model.StatusCompleted, model.ConclusionSuccess),
		packRun(103, "Docs", "a1b2c3d", "push", model.StatusCompleted, model.ConclusionSuccess),
		// Duplicate of the anchor must not double-count.
		anchor,
	}

	pack := usecase.PackForRun(rows, anchor, 10)
	if len(pack) != 3 {
		t.Fatalf("pack size = %d, want 3 (%#v)", len(pack), pack)
	}
	// Stable board order: run ID ascending.
	if pack[0].ID != 102 || pack[1].ID != 103 || pack[2].ID != 104 {
		t.Fatalf("pack order = [%d %d %d], want [102 103 104]", pack[0].ID, pack[1].ID, pack[2].ID)
	}
}

func TestPackForRunSingleRunGroupDegradesToAnchor(t *testing.T) {
	anchor := packRun(102, "CI", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone)
	pack := usecase.PackForRun([]model.Run{packRun(99, "CI", "f6e5d4c", "push", model.StatusCompleted, model.ConclusionSuccess)}, anchor, 10)
	if len(pack) != 1 || pack[0].ID != anchor.ID {
		t.Fatalf("single-run pack = %#v, want just the anchor", pack)
	}
}

func TestPackForRunWithoutSHAStaysSingle(t *testing.T) {
	anchor := model.Run{ID: 7, Event: "push"}
	rows := []model.Run{{ID: 8, Event: "push"}, {ID: 9, Event: "push"}}
	pack := usecase.PackForRun(rows, anchor, 10)
	if len(pack) != 1 || pack[0].ID != 7 {
		t.Fatalf("sha-less pack = %#v, want just the anchor", pack)
	}
}

func TestPackForRunCapsGroupSizeAndKeepsAnchor(t *testing.T) {
	anchor := packRun(150, "CI", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone)
	rows := make([]model.Run, 0, 14)
	for i := range int64(14) {
		rows = append(rows, packRun(100+i, "wf", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone))
	}
	pack := usecase.PackForRun(rows, anchor, 10)
	if len(pack) != 10 {
		t.Fatalf("capped pack size = %d, want 10", len(pack))
	}
	found := false
	for _, run := range pack {
		if run.ID == anchor.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("capped pack dropped the anchor: %#v", pack)
	}
}

// recordingRunLister counts list calls and pages through scripted
// responses, one per tick.
type recordingRunLister struct {
	calls   int
	filters []usecase.RunFilter
	pages   [][]model.Run
}

func (l *recordingRunLister) ListRuns(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	l.calls++
	l.filters = append(l.filters, filter)
	page := l.pages[0]
	if len(l.pages) > 1 {
		l.pages = l.pages[1:]
	}
	return page, nil
}

func TestPackWatchTickSpendsOneListCallRegardlessOfPackSize(t *testing.T) {
	members := []model.Run{
		packRun(101, "CI", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(102, "Release", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(103, "Docs", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(104, "Security", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(105, "Lint", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
	}
	lister := &recordingRunLister{pages: [][]model.Run{members}}
	service := usecase.PackWatchService{Runs: lister, MinPoll: time.Second, MaxPoll: 30 * time.Second}
	state := usecase.PackState{Repo: "indrasvat/gh-hound", HeadSHA: "a1b2c3d", Event: "push", Max: 10, Runs: members}

	for tick := range 3 {
		var err error
		state, err = service.Tick(context.Background(), state)
		if err != nil {
			t.Fatalf("tick %d returned error: %v", tick, err)
		}
	}
	if lister.calls != 3 {
		t.Fatalf("list calls = %d for 3 ticks of a 5-run pack, want exactly 3 (one per tick)", lister.calls)
	}
	for _, filter := range lister.filters {
		if filter.HeadSHA != "a1b2c3d" {
			t.Fatalf("tick filter must be server-side by head_sha, got %#v", filter)
		}
	}
}

func TestPackWatchTickMergesCompletionsAndJoiners(t *testing.T) {
	ci := packRun(101, "CI", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone)
	release := packRun(102, "Release", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone)

	ciDone := ci
	ciDone.Status = model.StatusCompleted
	ciDone.Conclusion = model.ConclusionSuccess
	releaseDone := release
	releaseDone.Status = model.StatusCompleted
	releaseDone.Conclusion = model.ConclusionFailure
	// A third workflow lands mid-watch (queued behind runner capacity).
	docs := packRun(103, "Docs", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone)
	docsDone := docs
	docsDone.Status = model.StatusCompleted
	docsDone.Conclusion = model.ConclusionSuccess
	// Same sha, foreign event: must never join.
	pages := [][]model.Run{
		{ciDone, release, docs, packRun(900, "Deploy Pages", "a1b2c3d", "workflow_run", model.StatusInProgress, model.ConclusionNone)},
		{ciDone, releaseDone, docsDone},
	}
	lister := &recordingRunLister{pages: pages}
	service := usecase.PackWatchService{Runs: lister, MinPoll: 2 * time.Second, MaxPoll: 30 * time.Second}
	state := usecase.PackState{Repo: "indrasvat/gh-hound", HeadSHA: "a1b2c3d", Event: "push", Max: 10, Runs: []model.Run{ci, release}}

	state, err := service.Tick(context.Background(), state)
	if err != nil {
		t.Fatalf("first tick returned error: %v", err)
	}
	if len(state.Runs) != 3 {
		t.Fatalf("after joiner tick pack size = %d, want 3 (%#v)", len(state.Runs), state.Runs)
	}
	if state.Runs[0].Conclusion != model.ConclusionSuccess {
		t.Fatalf("CI completion did not merge: %#v", state.Runs[0])
	}
	if state.Terminal || state.NextPoll != 2*time.Second {
		t.Fatalf("live pack state = terminal=%t poll=%s, want live at min poll", state.Terminal, state.NextPoll)
	}

	state, err = service.Tick(context.Background(), state)
	if err != nil {
		t.Fatalf("second tick returned error: %v", err)
	}
	if !state.Terminal || state.NextPoll != 30*time.Second {
		t.Fatalf("settled pack state = terminal=%t poll=%s, want terminal at max poll", state.Terminal, state.NextPoll)
	}
	summary := usecase.SummarizePack(state.Runs)
	if summary.Running != 0 || summary.Home != 2 || summary.Lost != 1 {
		t.Fatalf("settled summary = %#v, want 0 running · 2 home · 1 lost", summary)
	}
}

func TestPackWatchTickRespectsCapOnJoiners(t *testing.T) {
	members := []model.Run{
		packRun(101, "CI", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(102, "Release", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone),
	}
	joiners := make([]model.Run, 0, 12)
	joiners = append(joiners, members...)
	for i := range int64(12) {
		joiners = append(joiners, packRun(200+i, "wf", "a1b2c3d", "push", model.StatusInProgress, model.ConclusionNone))
	}
	lister := &recordingRunLister{pages: [][]model.Run{joiners}}
	service := usecase.PackWatchService{Runs: lister}
	state := usecase.PackState{Repo: "indrasvat/gh-hound", HeadSHA: "a1b2c3d", Event: "push", Max: 4, Runs: members}

	state, err := service.Tick(context.Background(), state)
	if err != nil {
		t.Fatalf("tick returned error: %v", err)
	}
	if len(state.Runs) != 4 {
		t.Fatalf("capped pack size = %d, want 4", len(state.Runs))
	}
	if state.Runs[0].ID != 101 || state.Runs[1].ID != 102 {
		t.Fatalf("existing members must survive the cap: %#v", state.Runs)
	}
}

func TestSummarizePackCountsAndHeader(t *testing.T) {
	runs := []model.Run{
		packRun(1, "a", "s", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(2, "b", "s", "push", model.StatusQueued, model.ConclusionNone),
		packRun(3, "c", "s", "push", model.StatusWaiting, model.ConclusionNone),
		packRun(4, "d", "s", "push", model.StatusCompleted, model.ConclusionSuccess),
		packRun(5, "e", "s", "push", model.StatusCompleted, model.ConclusionFailure),
		packRun(6, "f", "s", "push", model.StatusCompleted, model.ConclusionCancelled),
	}
	summary := usecase.SummarizePack(runs)
	if summary.Running != 3 || summary.Home != 1 || summary.Lost != 2 {
		t.Fatalf("summary = %#v, want 3 running · 1 home · 2 lost", summary)
	}
	if got := summary.String(); got != "3 running · 1 home · 2 lost" {
		t.Fatalf("summary line = %q", got)
	}
}

func TestWorstRunIndexPrefersLostThenRunning(t *testing.T) {
	runs := []model.Run{
		packRun(1, "a", "s", "push", model.StatusCompleted, model.ConclusionSuccess),
		packRun(2, "b", "s", "push", model.StatusInProgress, model.ConclusionNone),
		packRun(3, "c", "s", "push", model.StatusCompleted, model.ConclusionFailure),
		packRun(4, "d", "s", "push", model.StatusCompleted, model.ConclusionFailure),
	}
	if got := usecase.WorstRunIndex(runs); got != 2 {
		t.Fatalf("worst index = %d, want the first lost run (2)", got)
	}
	if got := usecase.WorstRunIndex(runs[:2]); got != 1 {
		t.Fatalf("worst index without lost = %d, want the running run (1)", got)
	}
	if got := usecase.WorstRunIndex(runs[:1]); got != 0 {
		t.Fatalf("worst index all home = %d, want 0", got)
	}
	if got := usecase.WorstRunIndex(nil); got != 0 {
		t.Fatalf("worst index empty = %d, want 0", got)
	}
}
