package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Action string

const (
	ActionRerunRun        Action = "rerun_run"
	ActionRerunFailedJobs Action = "rerun_failed_jobs"
	ActionRerunJob        Action = "rerun_job"
	ActionCancelRun       Action = "cancel_run"
	ActionForceCancelRun  Action = "force_cancel_run"
	ActionDispatch        Action = "dispatch"
)

type ActionResult struct {
	Action     Action
	Repo       string
	RunID      int64
	JobID      int64
	WorkflowID string
	Message    string
}

type DispatchRequest struct {
	Ref            string            `json:"ref"`
	Inputs         map[string]string `json:"inputs,omitempty"`
	RequiredInputs []string          `json:"-"`
}

type ActionErrorKind string

const (
	ActionErrorValidation ActionErrorKind = "validation"
	ActionErrorPermission ActionErrorKind = "permission"
	ActionErrorConflict   ActionErrorKind = "conflict"
	ActionErrorRateLimit  ActionErrorKind = "rate_limit"
	ActionErrorNetwork    ActionErrorKind = "network"
	ActionErrorUnknown    ActionErrorKind = "unknown"
)

type ActionError struct {
	Kind       ActionErrorKind
	Message    string
	Status     int
	RetryAfter time.Duration
	ResetAt    time.Time
}

func (e ActionError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func AsActionError(err error) (ActionError, bool) {
	var actionErr ActionError
	if errors.As(err, &actionErr) {
		return actionErr, true
	}
	return ActionError{}, false
}

type ActionService struct {
	GitHub  GitHub
	Limiter *MutationLimiter
}

func (s ActionService) RerunRun(ctx context.Context, repo string, runID int64, debug bool) (ActionResult, error) {
	return s.mutate(ctx, func() (ActionResult, error) {
		return s.GitHub.RerunRun(ctx, repo, runID, debug)
	})
}

func (s ActionService) RerunFailedJobs(ctx context.Context, repo string, runID int64) (ActionResult, error) {
	return s.mutate(ctx, func() (ActionResult, error) {
		return s.GitHub.RerunFailedJobs(ctx, repo, runID)
	})
}

func (s ActionService) RerunJob(ctx context.Context, repo string, jobID int64) (ActionResult, error) {
	return s.mutate(ctx, func() (ActionResult, error) {
		return s.GitHub.RerunJob(ctx, repo, jobID)
	})
}

func (s ActionService) CancelRun(ctx context.Context, repo string, runID int64) (ActionResult, error) {
	return s.mutate(ctx, func() (ActionResult, error) {
		return s.GitHub.CancelRun(ctx, repo, runID)
	})
}

func (s ActionService) ForceCancelRun(ctx context.Context, repo string, runID int64) (ActionResult, error) {
	return s.mutate(ctx, func() (ActionResult, error) {
		return s.GitHub.ForceCancelRun(ctx, repo, runID)
	})
}

func (s ActionService) DispatchWorkflow(ctx context.Context, repo, workflowID string, request DispatchRequest) (ActionResult, error) {
	if request.Ref == "" {
		return ActionResult{}, ActionError{Kind: ActionErrorValidation, Message: "dispatch ref is required"}
	}
	for _, input := range request.RequiredInputs {
		if request.Inputs[input] == "" {
			return ActionResult{}, ActionError{Kind: ActionErrorValidation, Message: fmt.Sprintf("dispatch input %q is required", input)}
		}
	}
	return s.mutate(ctx, func() (ActionResult, error) {
		return s.GitHub.DispatchWorkflow(ctx, repo, workflowID, request)
	})
}

func (s ActionService) mutate(ctx context.Context, fn func() (ActionResult, error)) (ActionResult, error) {
	if s.Limiter != nil {
		if err := s.Limiter.Wait(ctx); err != nil {
			return ActionResult{}, err
		}
	}
	return fn()
}

type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

type MutationLimiter struct {
	MinSpacing time.Duration
	Clock      Clock

	last time.Time
}

func (l *MutationLimiter) Wait(ctx context.Context) error {
	if l.MinSpacing <= 0 {
		return ctx.Err()
	}
	clock := l.Clock
	if clock == nil {
		clock = realClock{}
	}
	now := clock.Now()
	if !l.last.IsZero() {
		wait := l.MinSpacing - now.Sub(l.last)
		if wait > 0 {
			clock.Sleep(wait)
			now = clock.Now()
		}
	}
	l.last = now
	return ctx.Err()
}
