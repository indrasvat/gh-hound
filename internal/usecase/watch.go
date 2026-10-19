package usecase

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

type WatchState struct {
	Repo  string
	RunID int64
	Run   model.Run
	// Lines holds the completed jobs' log lines, in job order.
	Lines []logs.Line
	// JobLogs maps each listed completed job with a log to its parsed
	// lines; a completed job's log never changes, so the next tick
	// reuses it.
	JobLogs  map[int64][]logs.Line
	Terminal bool
	NextPoll time.Duration
}

type WatchService struct {
	GitHub  GitHub
	MinPoll time.Duration
	MaxPoll time.Duration
}

// Tick never mutates state.JobLogs: the next state gets a new map, so
// concurrent ticks may share one. A failed log fetch still returns the
// logs held so far, fetched ones included, with the error.
func (s WatchService) Tick(ctx context.Context, state WatchState) (WatchState, error) {
	minPoll := s.MinPoll
	if minPoll <= 0 {
		minPoll = 2 * time.Second
	}
	maxPoll := s.MaxPoll
	if maxPoll <= 0 {
		maxPoll = 30 * time.Second
	}

	run, err := s.GitHub.GetRun(ctx, state.Repo, state.RunID)
	if err != nil {
		return WatchState{}, err
	}
	jobs, err := s.GitHub.ListJobs(ctx, state.Repo, state.RunID)
	if err != nil {
		return WatchState{}, err
	}
	state.Run = run
	state.Lines = nil
	jobLogs := make(map[int64][]logs.Line, len(jobs))
	for _, job := range jobs {
		// A completed job that ran no step (skipped, cancelled before it
		// started, stopped at an environment gate) has no log: GitHub 404s.
		if job.Status != model.StatusCompleted || len(job.Steps) == 0 {
			continue
		}
		lines, cached := state.JobLogs[job.ID]
		if !cached {
			raw, err := s.GitHub.FetchJobLog(ctx, state.Repo, job.ID)
			if err != nil {
				held := maps.Clone(state.JobLogs)
				if held == nil {
					held = jobLogs
				} else {
					maps.Copy(held, jobLogs)
				}
				return WatchState{Repo: state.Repo, RunID: state.RunID, JobLogs: held}, err
			}
			lines = logs.Parse(raw).Lines
		}
		jobLogs[job.ID] = lines
		state.Lines = append(state.Lines, lines...)
	}
	state.JobLogs = jobLogs
	state.Terminal = run.Status == model.StatusCompleted
	if state.Terminal {
		state.NextPoll = maxPoll
	} else {
		state.NextPoll = minPoll
	}
	return state, nil
}

// WatchSession carries one run's job logs from tick to tick, so each
// completed job's log is fetched once. Ticks may run concurrently; the
// state a tick returns shares the session's logs and is read-only.
type WatchSession struct {
	Service WatchService

	mu   sync.Mutex
	last WatchState
}

func (s *WatchSession) Tick(ctx context.Context, repo string, run model.Run) (WatchState, error) {
	state := WatchState{Repo: repo, RunID: run.ID, Run: run}
	s.mu.Lock()
	if s.last.Repo == repo && s.last.RunID == run.ID {
		state.JobLogs = s.last.JobLogs
	}
	s.mu.Unlock()
	next, err := s.Service.Tick(ctx, state)
	if next.JobLogs != nil {
		s.mu.Lock()
		s.last = next
		s.mu.Unlock()
	}
	if err != nil {
		return WatchState{}, err
	}
	return next, nil
}
