package usecase

import (
	"context"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

type FailureService struct {
	GitHub GitHub
}

type FailureReport struct {
	Job         model.Job
	Log         logs.Document
	Annotations []model.Annotation
}

func (s FailureService) LoadFailure(ctx context.Context, repo string, job model.Job) (FailureReport, error) {
	raw, err := s.GitHub.FetchJobLog(ctx, repo, job.ID)
	if err != nil {
		return FailureReport{}, err
	}
	annotations, err := s.GitHub.ListAnnotations(ctx, repo, job)
	if err != nil {
		return FailureReport{}, err
	}
	return FailureReport{
		Job:         job,
		Log:         logs.Parse(raw),
		Annotations: annotations,
	}, nil
}
