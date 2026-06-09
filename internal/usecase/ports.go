package usecase

import (
	"context"

	"github.com/indrasvat/gh-hound/internal/model"
)

type RunFilter struct {
	Repo    string
	Branch  string
	Status  string
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
	GetJob(context.Context, string, int64) (model.Job, error)
	ListWorkflows(context.Context, string) ([]model.Workflow, error)
	ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error)
	FetchJobLog(context.Context, string, int64) (string, error)
	RerunRun(context.Context, string, int64, bool) (ActionResult, error)
	RerunFailedJobs(context.Context, string, int64) (ActionResult, error)
	RerunJob(context.Context, string, int64) (ActionResult, error)
	CancelRun(context.Context, string, int64) (ActionResult, error)
	ForceCancelRun(context.Context, string, int64) (ActionResult, error)
	DispatchWorkflow(context.Context, string, string, DispatchRequest) (ActionResult, error)
}
