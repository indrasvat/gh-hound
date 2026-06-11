package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// stubClock advances instantly so bounded polls test in microseconds.
type stubClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *stubClock) Now() time.Time { return c.now }

func (c *stubClock) Sleep(d time.Duration) {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
}

type scriptedHistory struct {
	calls   int
	filters []usecase.RunFilter
	pages   [][]model.Run
}

func (h *scriptedHistory) ListWorkflowRuns(_ context.Context, _ string, _ string, filter usecase.RunFilter) ([]model.Run, error) {
	h.calls++
	h.filters = append(h.filters, filter)
	page := h.pages[0]
	if len(h.pages) > 1 {
		h.pages = h.pages[1:]
	}
	return page, nil
}

type scriptedRunGetter struct {
	calls int
	runs  []model.Run
}

func (g *scriptedRunGetter) GetRun(context.Context, string, int64) (model.Run, error) {
	g.calls++
	run := g.runs[0]
	if len(g.runs) > 1 {
		g.runs = g.runs[1:]
	}
	return run, nil
}

func TestDiscoverDispatchedRunFindsTheFreshRun(t *testing.T) {
	since := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	fresh := model.Run{ID: 27318354797, Name: "Release", Event: "workflow_dispatch", CreatedAt: since.Add(4 * time.Second)}
	history := &scriptedHistory{pages: [][]model.Run{{}, {fresh}}}
	clock := &stubClock{now: since}
	service := usecase.HandoffService{History: history, Clock: clock, MaxPolls: 10, Interval: 3 * time.Second}

	run, err := service.DiscoverDispatchedRun(context.Background(), "indrasvat/gh-hound", "release.yml", "main", since)
	if err != nil {
		t.Fatalf("discover returned error: %v", err)
	}
	if run.ID != fresh.ID {
		t.Fatalf("discovered run = %#v, want %d", run, fresh.ID)
	}
	if history.calls != 2 {
		t.Fatalf("discovery polls = %d, want 2", history.calls)
	}
	filter := history.filters[0]
	if filter.Event != "workflow_dispatch" || filter.Branch != "main" || !filter.CreatedAfter.Equal(since) {
		t.Fatalf("discovery filter = %#v, want event=workflow_dispatch branch=main created_after=%s", filter, since)
	}
}

func TestDiscoverDispatchedRunIgnoresStaleRuns(t *testing.T) {
	since := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	stale := model.Run{ID: 1, Event: "workflow_dispatch", CreatedAt: since.Add(-time.Minute)}
	fresh := model.Run{ID: 2, Event: "workflow_dispatch", CreatedAt: since.Add(2 * time.Second)}
	history := &scriptedHistory{pages: [][]model.Run{{stale}, {fresh, stale}}}
	service := usecase.HandoffService{History: history, Clock: &stubClock{now: since}, MaxPolls: 10, Interval: 3 * time.Second}

	run, err := service.DiscoverDispatchedRun(context.Background(), "indrasvat/gh-hound", "release.yml", "main", since)
	if err != nil {
		t.Fatalf("discover returned error: %v", err)
	}
	if run.ID != fresh.ID {
		t.Fatalf("discovered run = %d, want the fresh run %d (stale must be skipped)", run.ID, fresh.ID)
	}
}

func TestDiscoverDispatchedRunLosesTheScentAfterBudget(t *testing.T) {
	since := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	history := &scriptedHistory{pages: [][]model.Run{{}}}
	clock := &stubClock{now: since}
	service := usecase.HandoffService{History: history, Clock: clock, MaxPolls: 10, Interval: 3 * time.Second}

	_, err := service.DiscoverDispatchedRun(context.Background(), "indrasvat/gh-hound", "release.yml", "main", since)
	if !errors.Is(err, usecase.ErrScentLost) {
		t.Fatalf("exhausted discovery error = %v, want ErrScentLost", err)
	}
	if history.calls != 10 {
		t.Fatalf("discovery polls = %d, want the documented 10-poll budget", history.calls)
	}
	// 9 sleeps between 10 polls keeps the budget inside ~30s.
	if len(clock.sleeps) != 9 {
		t.Fatalf("sleeps = %d, want 9 (between polls only)", len(clock.sleeps))
	}
}

func TestDiscoverDispatchedRunHonorsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := usecase.HandoffService{History: &scriptedHistory{pages: [][]model.Run{{}}}, Clock: &stubClock{}, MaxPolls: 10, Interval: 3 * time.Second}
	_, err := service.DiscoverDispatchedRun(ctx, "indrasvat/gh-hound", "release.yml", "main", time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled discovery error = %v, want context.Canceled", err)
	}
}

func TestAwaitRerunStartReturnsWhenAttemptAdvances(t *testing.T) {
	stale := model.Run{ID: 571, RunAttempt: 1, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	advanced := model.Run{ID: 571, RunAttempt: 2, Status: model.StatusQueued}
	getter := &scriptedRunGetter{runs: []model.Run{stale, stale, advanced}}
	service := usecase.HandoffService{Runs: getter, Clock: &stubClock{}, MaxPolls: 10, Interval: 3 * time.Second}

	run, err := service.AwaitRerunStart(context.Background(), "indrasvat/gh-hound", 571, 1)
	if err != nil {
		t.Fatalf("await rerun returned error: %v", err)
	}
	if run.RunAttempt != 2 || getter.calls != 3 {
		t.Fatalf("await rerun = attempt %d after %d polls, want attempt 2 after 3 polls", run.RunAttempt, getter.calls)
	}
}

func TestAwaitRerunStartReturnsOnStatusLeavingCompleted(t *testing.T) {
	// Some hosts surface the status flip before the attempt counter.
	requeued := model.Run{ID: 571, RunAttempt: 1, Status: model.StatusQueued}
	getter := &scriptedRunGetter{runs: []model.Run{requeued}}
	service := usecase.HandoffService{Runs: getter, Clock: &stubClock{}}

	run, err := service.AwaitRerunStart(context.Background(), "indrasvat/gh-hound", 571, 1)
	if err != nil {
		t.Fatalf("await rerun returned error: %v", err)
	}
	if run.Status != model.StatusQueued || getter.calls != 1 {
		t.Fatalf("await rerun = %#v after %d polls, want the requeued run after 1 poll", run, getter.calls)
	}
}

func TestAwaitRerunStartAttachesAnywayAfterBudget(t *testing.T) {
	stale := model.Run{ID: 571, RunAttempt: 1, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	getter := &scriptedRunGetter{runs: []model.Run{stale}}
	service := usecase.HandoffService{Runs: getter, Clock: &stubClock{}, MaxPolls: 10, Interval: 3 * time.Second}

	run, err := service.AwaitRerunStart(context.Background(), "indrasvat/gh-hound", 571, 1)
	if err != nil {
		t.Fatalf("await rerun must attach to the existing run even when the attempt never advances, got error: %v", err)
	}
	if run.ID != 571 || getter.calls != 10 {
		t.Fatalf("await rerun = run %d after %d polls, want run 571 after the 10-poll budget", run.ID, getter.calls)
	}
}
