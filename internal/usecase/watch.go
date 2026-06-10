package usecase

import (
	"context"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

type WatchState struct {
	Repo             string
	RunID            int64
	Run              model.Run
	Jobs             []model.Job
	Appended         []logs.Line
	CompletedJobLogs map[int64]bool
	Terminal         bool
	NextPoll         time.Duration
}

type WatchService struct {
	GitHub  GitHub
	MinPoll time.Duration
	MaxPoll time.Duration
}

func (s WatchService) Tick(ctx context.Context, state WatchState) (WatchState, error) {
	if state.CompletedJobLogs == nil {
		state.CompletedJobLogs = map[int64]bool{}
	}
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
	state.Jobs = jobs
	for _, job := range jobs {
		if job.Status != model.StatusCompleted || state.CompletedJobLogs[job.ID] {
			continue
		}
		raw, err := s.GitHub.FetchJobLog(ctx, state.Repo, job.ID)
		if err != nil {
			return WatchState{}, err
		}
		state.Appended = append(state.Appended, logs.Parse(raw).Lines...)
		state.CompletedJobLogs[job.ID] = true
	}
	state.Terminal = run.Status == model.StatusCompleted
	if state.Terminal {
		state.NextPoll = maxPoll
	} else {
		state.NextPoll = minPoll
	}
	return state, nil
}
