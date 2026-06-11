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
	ListPendingDeployments(context.Context, string, int64) ([]model.PendingDeployment, error)
	ReviewPendingDeployments(context.Context, string, int64, DeploymentReview) (ActionResult, error)
	RerunRun(context.Context, string, int64, bool) (ActionResult, error)
	RerunFailedJobs(context.Context, string, int64, bool) (ActionResult, error)
	RerunJob(context.Context, string, int64, bool) (ActionResult, error)
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

// RepoInfoProvider is an optional adapter capability: the target
// repo's default branch, used to pre-fill dispatch refs for foreign
// repos where the local checkout branch would be a lie.
type RepoInfoProvider interface {
	DefaultBranch(ctx context.Context, repo string) (string, error)
}

// RefValidator is an optional adapter capability: whether a ref exists
// on the target as a branch or tag, checked before dispatching so a
// typo fails as validation instead of a confusing 422.
type RefValidator interface {
	RefExists(ctx context.Context, repo, ref string) (bool, error)
}

// WorkflowRunHistory is an optional adapter capability: a workflow's
// run history (newest first), scoped server-side so the regression
// scan never pays for other workflows' runs.
type WorkflowRunHistory interface {
	ListWorkflowRuns(ctx context.Context, repo, workflow string, filter RunFilter) ([]model.Run, error)
}

// CommitComparer is an optional adapter capability: the commit range
// between two SHAs via the compare API — the suspects between the last
// clean run and the first dirty one.
type CommitComparer interface {
	CompareCommits(ctx context.Context, repo, base, head string) (model.CommitRange, error)
}

// LogProgressFetcher is an optional adapter capability: log download
// with byte-progress reporting (read, total; total <= 0 when the size
// is unknown). Adapters without it fall back to plain FetchJobLog and
// callers render an indeterminate spinner.
type LogProgressFetcher interface {
	FetchJobLogWithProgress(ctx context.Context, repo string, jobID int64, progress func(read, total int64)) (string, error)
}
