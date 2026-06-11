package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

// ApprovalsService surfaces a waiting run's deployment gates and
// reviews them. Review validates environment names against the live
// pending list before any mutation call, so refusals are typed and
// never burn an API write.
type ApprovalsService struct {
	GitHub  GitHub
	Limiter *MutationLimiter
}

// DeploymentReviewRequest is the caller-facing review intent. An empty
// Environments slice means "every environment the user can approve".
type DeploymentReviewRequest struct {
	Environments []string
	Approve      bool
	Comment      string
}

// DeploymentReviewOutcome reports what was actually reviewed: the
// resolved environment names, the state posted, and the comment that
// accompanied it (the documented default when the user left it blank).
type DeploymentReviewOutcome struct {
	Result       ActionResult
	State        DeploymentReviewState
	Environments []string
	Comment      string
}

func (s ApprovalsService) List(ctx context.Context, repo string, runID int64) ([]model.PendingDeployment, error) {
	return s.GitHub.ListPendingDeployments(ctx, repo, runID)
}

func (s ApprovalsService) Review(ctx context.Context, repo string, runID int64, request DeploymentReviewRequest) (DeploymentReviewOutcome, error) {
	pending, err := s.List(ctx, repo, runID)
	if err != nil {
		return DeploymentReviewOutcome{}, err
	}
	targets, err := resolveReviewTargets(pending, request.Environments)
	if err != nil {
		return DeploymentReviewOutcome{}, err
	}

	state := DeploymentRejected
	if request.Approve {
		state = DeploymentApproved
	}
	comment := strings.TrimSpace(request.Comment)
	if comment == "" {
		comment = DefaultReviewComment
	}
	ids := make([]int64, 0, len(targets))
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.EnvironmentID)
		names = append(names, target.EnvironmentName)
	}

	if s.Limiter != nil {
		if err := s.Limiter.Wait(ctx); err != nil {
			return DeploymentReviewOutcome{}, err
		}
	}
	result, err := s.GitHub.ReviewPendingDeployments(ctx, repo, runID, DeploymentReview{
		EnvironmentIDs: ids,
		State:          state,
		Comment:        comment,
	})
	if err != nil {
		return DeploymentReviewOutcome{}, err
	}
	// One source for the hound voice: every surface (TUI toast, pipe
	// docs) reads the verdict from here.
	if state == DeploymentApproved {
		result.Message = "gate's open."
	} else {
		result.Message = "gate stays shut."
	}
	return DeploymentReviewOutcome{
		Result:       result,
		State:        state,
		Environments: names,
		Comment:      comment,
	}, nil
}

// resolveReviewTargets maps requested environment names to pending
// gates, refusing unknown names (validation) and gates the user cannot
// review (permission). No names selects every approvable gate.
func resolveReviewTargets(pending []model.PendingDeployment, names []string) ([]model.PendingDeployment, error) {
	if len(pending) == 0 {
		return nil, ActionError{Kind: ActionErrorValidation, Message: "no pending deployments on this run — nothing to review"}
	}
	if len(names) == 0 {
		approvable := make([]model.PendingDeployment, 0, len(pending))
		for _, gate := range pending {
			if gate.CurrentUserCanApprove {
				approvable = append(approvable, gate)
			}
		}
		if len(approvable) == 0 {
			return nil, ActionError{Kind: ActionErrorPermission, Message: "not yours to open — you are not a required reviewer for any pending environment"}
		}
		return approvable, nil
	}
	byName := make(map[string]model.PendingDeployment, len(pending))
	for _, gate := range pending {
		byName[strings.ToLower(gate.EnvironmentName)] = gate
	}
	targets := make([]model.PendingDeployment, 0, len(names))
	for _, name := range names {
		gate, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return nil, ActionError{Kind: ActionErrorValidation, Message: fmt.Sprintf("environment %q is not waiting on this run", name)}
		}
		if !gate.CurrentUserCanApprove {
			return nil, ActionError{Kind: ActionErrorPermission, Message: fmt.Sprintf("environment %q: not yours to open — you are not a required reviewer", gate.EnvironmentName)}
		}
		targets = append(targets, gate)
	}
	return targets, nil
}
