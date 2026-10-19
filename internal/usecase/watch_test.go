package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestWatchServicePollsAndAppendsCompletedJobLogs(t *testing.T) {
	gh := &watchGitHub{
		runs: []model.Run{
			{ID: 570, Status: model.StatusInProgress, Conclusion: model.ConclusionNone},
			{ID: 570, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure},
		},
		jobs: []model.Job{{
			ID:         100,
			Name:       "build",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
			Steps: []model.Step{{
				Number:     6,
				Name:       "go test ./...",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}},
		}},
		logs: "17:43:02Z go test ./...\nok internal/api 0.214s\n",
	}
	service := usecase.WatchService{GitHub: gh, MinPoll: time.Second, MaxPoll: 30 * time.Second}
	state := usecase.WatchState{Repo: "indrasvat/gh-hound", RunID: 570}

	next, err := service.Tick(context.Background(), state)
	if err != nil {
		t.Fatalf("first tick returned error: %v", err)
	}
	if next.NextPoll != time.Second || next.Terminal || len(next.Lines) != 2 {
		t.Fatalf("first tick = %#v", next)
	}

	next, err = service.Tick(context.Background(), next)
	if err != nil {
		t.Fatalf("second tick returned error: %v", err)
	}
	if !next.Terminal || next.NextPoll != 30*time.Second || len(next.Lines) != 2 {
		t.Fatalf("second tick = %#v", next)
	}
	if gh.fetches != 1 {
		t.Fatalf("fetches = %d, want completed job log once", gh.fetches)
	}
}

func watchJob(id int64, status model.Status, steps int) model.Job {
	job := model.Job{ID: id, Name: fmt.Sprintf("job-%d", id), Status: status}
	for n := range steps {
		job.Steps = append(job.Steps, model.Step{Number: n + 1, Name: "step", Status: status})
	}
	return job
}

func lineTexts(lines []logs.Line) []string {
	texts := make([]string, 0, len(lines))
	for _, line := range lines {
		texts = append(texts, line.Text)
	}
	return texts
}

// Issue #60: the TUI watch carries its state across ticks through a
// WatchSession, so a completed job's log is downloaded once, not on
// every poll.
func TestWatchSessionFetchesEachCompletedJobLogOnce(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{
		watchJob(1, model.StatusCompleted, 2),
		watchJob(2, model.StatusInProgress, 1),
		watchJob(3, model.StatusCompleted, 1),
	}}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: gh}}
	run := model.Run{ID: 570}
	for range 5 {
		if _, err := session.Tick(context.Background(), "o/r", run); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	gh.setJobs([]model.Job{
		watchJob(1, model.StatusCompleted, 2),
		watchJob(2, model.StatusCompleted, 1),
		watchJob(3, model.StatusCompleted, 1),
	})
	var state usecase.WatchState
	for range 5 {
		var err error
		if state, err = session.Tick(context.Background(), "o/r", run); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	total, perJob := gh.fetchCounts()
	if total != 3 || perJob[1] != 1 || perJob[2] != 1 || perJob[3] != 1 {
		t.Fatalf("fetches = %d %v, want each completed job's log once", total, perJob)
	}
	want := []string{"job 1 line 1", "job 1 line 2", "job 2 line 1", "job 2 line 2", "job 3 line 1", "job 3 line 2"}
	if got := lineTexts(state.Lines); !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want job order %q", got, want)
	}
}

// A rerun reuses the run id with new job ids: the old attempt's lines
// leave the watch and its logs leave the cache.
func TestWatchSessionDropsJobsNoLongerListed(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{watchJob(1, model.StatusCompleted, 1)}}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: gh}}
	run := model.Run{ID: 570}
	if _, err := session.Tick(context.Background(), "o/r", run); err != nil {
		t.Fatalf("tick: %v", err)
	}
	gh.setJobs([]model.Job{watchJob(9, model.StatusCompleted, 1)})
	state, err := session.Tick(context.Background(), "o/r", run)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if got := lineTexts(state.Lines); !slices.Equal(got, []string{"job 9 line 1", "job 9 line 2"}) {
		t.Fatalf("lines = %q, want only the listed job", got)
	}
	if _, ok := state.JobLogs[1]; ok || len(state.JobLogs) != 1 {
		t.Fatalf("job logs = %v, want only job 9", state.JobLogs)
	}
}

// Switching runs (or repos) starts from an empty cache: another run's
// jobs never leak into this one.
func TestWatchSessionStartsFreshForAnotherRun(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{watchJob(1, model.StatusCompleted, 1)}}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: gh}}
	for _, target := range []struct {
		repo string
		run  int64
	}{{"o/r", 570}, {"o/r", 571}, {"o/other", 571}, {"o/other", 571}} {
		if _, err := session.Tick(context.Background(), target.repo, model.Run{ID: target.run}); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	if total, _ := gh.fetchCounts(); total != 3 {
		t.Fatalf("fetches = %d, want one per distinct run (3)", total)
	}
}

// A job that completed without running a step (skipped, or cancelled
// before it started) has no log: GitHub answers 404, which used to
// fail every tick of the watch.
func TestWatchSessionSkipsJobsThatNeverRan(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{
		watchJob(1, model.StatusCompleted, 1),
		watchJob(2, model.StatusCompleted, 0),
	}}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: gh}}
	var state usecase.WatchState
	for range 3 {
		var err error
		if state, err = session.Tick(context.Background(), "o/r", model.Run{ID: 570}); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	if _, perJob := gh.fetchCounts(); perJob[2] != 0 || perJob[1] != 1 {
		t.Fatalf("fetched %v, want job 2 (no steps) never fetched", perJob)
	}
	if got := lineTexts(state.Lines); !slices.Equal(got, []string{"job 1 line 1", "job 1 line 2"}) {
		t.Fatalf("lines = %q", got)
	}
}

// A failed fetch fails the tick, as before, but keeps what the session
// already has: the next tick fetches only what is still missing.
func TestWatchSessionKeepsCacheAcrossAFailedTick(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{watchJob(1, model.StatusCompleted, 1)}}
	flaky := &failingLogs{watchGitHub: gh, failFor: 2}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: flaky}}
	run := model.Run{ID: 570}
	if _, err := session.Tick(context.Background(), "o/r", run); err != nil {
		t.Fatalf("tick: %v", err)
	}
	gh.setJobs([]model.Job{watchJob(1, model.StatusCompleted, 1), watchJob(2, model.StatusCompleted, 1)})
	if _, err := session.Tick(context.Background(), "o/r", run); err == nil {
		t.Fatal("tick with a failing log fetch must report the error")
	}
	flaky.failFor = 0
	state, err := session.Tick(context.Background(), "o/r", run)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if _, perJob := gh.fetchCounts(); perJob[1] != 1 || perJob[2] != 1 {
		t.Fatalf("fetched %v, want job 1 once (cached) and job 2 once (after the failure)", perJob)
	}
	if len(state.Lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(state.Lines))
	}
}

// A log that keeps failing (expired, say) must not cost the jobs
// listed before it a download on every tick.
func TestWatchSessionKeepsLogsFetchedBeforeAFailure(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{
		watchJob(1, model.StatusCompleted, 1),
		watchJob(2, model.StatusCompleted, 1),
		watchJob(3, model.StatusCompleted, 1),
	}}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: &failingLogs{watchGitHub: gh, failFor: 3}}}
	for range 4 {
		if _, err := session.Tick(context.Background(), "o/r", model.Run{ID: 570}); err == nil {
			t.Fatal("tick with a failing log fetch must report the error")
		}
	}
	if _, perJob := gh.fetchCounts(); perJob[1] != 1 || perJob[2] != 1 {
		t.Fatalf("fetched %v, want jobs 1 and 2 once each despite job 3 failing", perJob)
	}
}

// Concurrent ticks (the initial load and a poll can overlap) share the
// cache without racing; once settled, a tick fetches nothing.
func TestWatchSessionConcurrentTicks(t *testing.T) {
	jobs := make([]model.Job, 0, 8)
	for id := range int64(8) {
		jobs = append(jobs, watchJob(id+1, model.StatusCompleted, 1))
	}
	gh := &watchGitHub{jobs: jobs}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: gh}}
	run := model.Run{ID: 570}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			state, err := session.Tick(context.Background(), "o/r", run)
			if err != nil || len(state.Lines) != 16 {
				t.Errorf("tick = %d lines, %v", len(state.Lines), err)
			}
		})
	}
	wg.Wait()
	settled, _ := gh.fetchCounts()
	if _, err := session.Tick(context.Background(), "o/r", run); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if total, _ := gh.fetchCounts(); total != settled {
		t.Fatalf("settled tick fetched %d logs, want 0", total-settled)
	}
}

// A tick for the run the user left that lands after the current run's
// tick must not evict the current run's logs.
func TestWatchSessionIgnoresALateTickForAnotherRun(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{watchJob(7, model.StatusCompleted, 1)}}
	held := &heldRun{watchGitHub: gh, run: 1, entered: make(chan struct{}), release: make(chan struct{})}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: held}}
	late := make(chan error)
	go func() {
		_, err := session.Tick(context.Background(), "o/r", model.Run{ID: 1})
		late <- err
	}()
	<-held.entered
	if _, err := session.Tick(context.Background(), "o/r", model.Run{ID: 2}); err != nil {
		t.Fatalf("tick: %v", err)
	}
	close(held.release)
	if err := <-late; err != nil {
		t.Fatalf("late tick: %v", err)
	}
	if _, err := session.Tick(context.Background(), "o/r", model.Run{ID: 2}); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if _, perJob := gh.fetchCounts(); perJob[7] != 2 {
		t.Fatalf("job 7 fetched %d times, want 2 (once per run)", perJob[7])
	}
}

// heldRun blocks ListJobs for one run until released.
type heldRun struct {
	*watchGitHub
	run     int64
	entered chan struct{}
	release chan struct{}
}

func (h *heldRun) ListJobs(ctx context.Context, repo string, id int64) ([]model.Job, error) {
	if id == h.run {
		close(h.entered)
		<-h.release
	}
	return h.watchGitHub.ListJobs(ctx, repo, id)
}

// A log past retention (410) shows as expired, never fails the tick
// and is asked for once; a log not there yet (404) shows as missing and
// is asked for again next tick.
func TestWatchSessionShowsMissingLogsWithoutFailing(t *testing.T) {
	gh := &watchGitHub{jobs: []model.Job{
		watchJob(1, model.StatusCompleted, 1),
		watchJob(2, model.StatusCompleted, 1),
		watchJob(3, model.StatusCompleted, 1),
	}}
	missing := &missingLogs{watchGitHub: gh, status: map[int64]int{2: http.StatusGone, 3: http.StatusNotFound}}
	session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: missing}}
	var state usecase.WatchState
	for range 3 {
		var err error
		if state, err = session.Tick(context.Background(), "o/r", model.Run{ID: 570}); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	want := []usecase.MissingLog{{Job: gh.jobs[1], Expired: true}, {Job: gh.jobs[2]}}
	if !slices.EqualFunc(state.NoLog, want, func(a, b usecase.MissingLog) bool { return a.Job.ID == b.Job.ID && a.Expired == b.Expired }) {
		t.Fatalf("no-log = %+v, want job 2 expired and job 3 not there yet", state.NoLog)
	}
	if got := lineTexts(state.Lines); !slices.Equal(got, []string{"job 1 line 1", "job 1 line 2"}) {
		t.Fatalf("lines = %q", got)
	}
	if _, perJob := gh.fetchCounts(); perJob[1] != 1 || perJob[2] != 1 || perJob[3] != 3 {
		t.Fatalf("fetched %v, want job 1 and the expired job 2 once, job 3 every tick", perJob)
	}

	delete(missing.status, 3) // the log lands
	if state, _ = session.Tick(context.Background(), "o/r", model.Run{ID: 570}); len(state.NoLog) != 1 || len(state.Lines) != 4 {
		t.Fatalf("after job 3's log lands: no-log %+v, %d lines", state.NoLog, len(state.Lines))
	}
}

// missingLogs answers like GitHub for jobs without a log.
type missingLogs struct {
	*watchGitHub
	status map[int64]int
}

func (m *missingLogs) FetchJobLog(ctx context.Context, repo string, id int64) (string, error) {
	if status, ok := m.status[id]; ok {
		_, _ = m.watchGitHub.FetchJobLog(ctx, repo, id) // count the request
		return "", usecase.LogUnavailableError{JobID: id, Status: status}
	}
	return m.watchGitHub.FetchJobLog(ctx, repo, id)
}

type failingLogs struct {
	*watchGitHub
	failFor int64
}

func (f *failingLogs) FetchJobLog(ctx context.Context, repo string, id int64) (string, error) {
	if id == f.failFor {
		return "", errors.New("log download failed")
	}
	return f.watchGitHub.FetchJobLog(ctx, repo, id)
}

type watchGitHub struct {
	mu      sync.Mutex
	runs    []model.Run
	jobs    []model.Job
	logs    string
	fetches int
	fetched map[int64]int
}

func (g *watchGitHub) GetRun(_ context.Context, _ string, id int64) (model.Run, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.runs) == 0 {
		return model.Run{ID: id, Status: model.StatusInProgress}, nil
	}
	run := g.runs[0]
	if len(g.runs) > 1 {
		g.runs = g.runs[1:]
	}
	return run, nil
}

func (g *watchGitHub) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.jobs, nil
}

func (g *watchGitHub) setJobs(jobs []model.Job) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.jobs = jobs
}

func (g *watchGitHub) fetchCounts() (int, map[int64]int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.fetches, maps.Clone(g.fetched)
}

func (g *watchGitHub) GetJob(context.Context, string, int64) (model.Job, error) {
	return model.Job{}, errors.New("not implemented")
}

func (g *watchGitHub) FetchJobLog(_ context.Context, _ string, id int64) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fetches++
	if g.fetched == nil {
		g.fetched = map[int64]int{}
	}
	g.fetched[id]++
	if g.logs != "" {
		return g.logs, nil
	}
	return fmt.Sprintf("job %d line 1\njob %d line 2\n", id, id), nil
}

func (g *watchGitHub) ListRuns(context.Context, usecase.RunFilter) ([]model.Run, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) ListWorkflows(context.Context, string) ([]model.Workflow, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) FetchWorkflowFile(context.Context, string, string) (string, error) {
	return "", errors.New("not implemented")
}

func (g *watchGitHub) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) RerunRun(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) RerunFailedJobs(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) RerunJob(context.Context, string, int64, bool) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) CancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) ForceCancelRun(context.Context, string, int64) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) DispatchWorkflow(context.Context, string, string, usecase.DispatchRequest) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) EnableWorkflow(context.Context, string, string) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) DisableWorkflow(context.Context, string, string) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}

func (g *watchGitHub) ListArtifacts(context.Context, string, int64) ([]model.Artifact, error) {
	return nil, nil
}

func (g *watchGitHub) DownloadArtifact(context.Context, string, int64) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (g *watchGitHub) GetRunAttempt(ctx context.Context, repo string, runID int64, _ int) (model.Run, error) {
	return g.GetRun(ctx, repo, runID)
}

func (g *watchGitHub) ListJobsForAttempt(ctx context.Context, repo string, runID int64, _ int) ([]model.Job, error) {
	return g.ListJobs(ctx, repo, runID)
}

func (g *watchGitHub) ListPendingDeployments(context.Context, string, int64) ([]model.PendingDeployment, error) {
	return nil, nil
}

func (g *watchGitHub) ReviewPendingDeployments(context.Context, string, int64, usecase.DeploymentReview) (usecase.ActionResult, error) {
	return usecase.ActionResult{}, errors.New("not implemented")
}
