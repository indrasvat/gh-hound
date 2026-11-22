package usecase

import (
	"context"
	"fmt"

	"github.com/indrasvat/gh-hound/internal/model"
)

// VerdictService classifies a red run for screens that have not loaded
// its jobs. It reads jobs and failing jobs' annotations, never logs.
type VerdictService struct {
	GitHub GitHub
}

// Classify returns the run's verdict. Run fields settle action_required
// and startup_failure without a call; other red runs cost one jobs
// listing plus one annotations listing per failing job.
func (s VerdictService) Classify(ctx context.Context, repo string, run model.Run) (model.RunVerdict, error) {
	if verdict, known := model.ClassifyRun(run, nil); known {
		return verdict, nil
	}
	jobs, err := s.GitHub.ListJobsForAttempt(ctx, repo, run.ID, run.RunAttempt)
	if err != nil {
		return model.RunVerdict{}, fmt.Errorf("jobs of run %d: %w", run.ID, err)
	}
	evidence := &model.RunEvidence{Jobs: jobs, Annotations: map[int64][]model.Annotation{}}
	for _, job := range jobs {
		if !job.Conclusion.Failing() {
			continue
		}
		annotations, err := s.GitHub.ListAnnotations(ctx, repo, job)
		if err != nil {
			return model.RunVerdict{}, fmt.Errorf("annotations of job %d: %w", job.ID, err)
		}
		evidence.Annotations[job.ID] = annotations
	}
	verdict, _ := model.ClassifyRun(run, evidence)
	return verdict, nil
}

// RunApprover is an optional adapter capability: approving a held run
// from a public fork (POST …/actions/runs/{id}/approve).
type RunApprover interface {
	ApproveRun(ctx context.Context, repo string, runID int64) (ActionResult, error)
}

// PushAccess is an optional adapter capability: whether the credentials
// can write to repo, as approving a held fork run requires.
type PushAccess interface {
	CanPush(ctx context.Context, repo string) (bool, error)
}
