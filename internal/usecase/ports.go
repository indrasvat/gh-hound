package usecase

import (
	"context"

	"github.com/indrasvat/gh-hound/internal/model"
)

type RunFilter struct {
	Repo    string
	Branch  string
	Status  model.Status
	Event   string
	Actor   string
	HeadSHA string
	PerPage int
	Page    int
}

type GitHub interface {
	ListRuns(context.Context, RunFilter) ([]model.Run, error)
	GetRun(context.Context, string, int64) (model.Run, error)
	ListJobs(context.Context, string, int64) ([]model.Job, error)
	ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error)
}
