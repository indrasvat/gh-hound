package usecase

import (
	"context"
	"io"

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

type RequestMeta struct {
	Resource      string
	Status        int
	Cache         string
	RateRemaining string
}

type LogRefetchNotice struct {
	JobID         int64
	Attempts      int
	ExpiredStatus int
	Message       string
}

type GitHub interface {
	ListRuns(context.Context, RunFilter) ([]model.Run, error)
	GetRun(context.Context, string, int64) (model.Run, error)
	GetRunAttempt(context.Context, string, int64, int) (model.Run, error)
	ListJobsForAttempt(context.Context, string, int64, int) ([]model.Job, error)
	ListJobs(context.Context, string, int64) ([]model.Job, error)
	GetJob(context.Context, string, int64) (model.Job, error)
	ListWorkflows(context.Context, string) ([]model.Workflow, error)
	FetchWorkflowFile(context.Context, string, string) (string, error)
	ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error)
	FetchJobLog(context.Context, string, int64) (string, error)
	ListArtifacts(context.Context, string, int64) ([]model.Artifact, error)
	DownloadArtifact(context.Context, string, int64) (io.ReadCloser, error)
	RerunRun(context.Context, string, int64, bool) (ActionResult, error)
	RerunFailedJobs(context.Context, string, int64) (ActionResult, error)
	RerunJob(context.Context, string, int64) (ActionResult, error)
	CancelRun(context.Context, string, int64) (ActionResult, error)
	ForceCancelRun(context.Context, string, int64) (ActionResult, error)
	DispatchWorkflow(context.Context, string, string, DispatchRequest) (ActionResult, error)
}

type GitHubDiagnostics interface {
	LastRequestMeta(resource string) (RequestMeta, bool)
}

type GitHubLogDiagnostics interface {
	LastLogRefetch(jobID int64) (LogRefetchNotice, bool)
}
