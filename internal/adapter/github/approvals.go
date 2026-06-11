package github

import (
	"context"
	"strconv"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// Payload shapes pinned 2026-06-10 from the official REST reference
// (docs.github.com/en/rest/actions/workflow-runs, pending_deployments
// endpoints; see testdata/pending_deployments.json). The GET was
// live-verified the same day (200 + [] on a completed run). The POST
// body requires environment_ids, state, and comment — all three.
type pendingDeploymentDTO struct {
	Environment struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"environment"`
	WaitTimer             int                     `json:"wait_timer"`
	CurrentUserCanApprove bool                    `json:"current_user_can_approve"`
	Reviewers             []deploymentReviewerDTO `json:"reviewers"`
}

type deploymentReviewerDTO struct {
	Type     string `json:"type"`
	Reviewer struct {
		Login string `json:"login"`
		Slug  string `json:"slug"`
		Name  string `json:"name"`
	} `json:"reviewer"`
}

func (c *Client) ListPendingDeployments(ctx context.Context, repo string, runID int64) ([]model.PendingDeployment, error) {
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/pending_deployments")
	var decoded []pendingDeploymentDTO
	if err := c.getJSON(ctx, resource, nil, &decoded); err != nil {
		return nil, err
	}
	pending := make([]model.PendingDeployment, 0, len(decoded))
	for _, dto := range decoded {
		pending = append(pending, mapPendingDeployment(dto))
	}
	return pending, nil
}

func mapPendingDeployment(dto pendingDeploymentDTO) model.PendingDeployment {
	reviewers := make([]model.DeploymentReviewer, 0, len(dto.Reviewers))
	for _, reviewer := range dto.Reviewers {
		name := reviewer.Reviewer.Login
		if name == "" {
			// Teams have no login: prefer the stable slug over the
			// display name.
			name = firstNonBlank(reviewer.Reviewer.Slug, reviewer.Reviewer.Name)
		}
		reviewers = append(reviewers, model.DeploymentReviewer{Type: reviewer.Type, Name: name})
	}
	return model.PendingDeployment{
		EnvironmentID:         dto.Environment.ID,
		EnvironmentName:       dto.Environment.Name,
		WaitTimer:             dto.WaitTimer,
		CurrentUserCanApprove: dto.CurrentUserCanApprove,
		Reviewers:             reviewers,
	}
}

// reviewDeploymentsBody mirrors the documented POST body exactly. The
// comment field has no omitempty: the API requires it on every review.
type reviewDeploymentsBody struct {
	EnvironmentIDs []int64 `json:"environment_ids"`
	State          string  `json:"state"`
	Comment        string  `json:"comment"`
}

func (c *Client) ReviewPendingDeployments(ctx context.Context, repo string, runID int64, review usecase.DeploymentReview) (usecase.ActionResult, error) {
	action := usecase.ActionApproveDeployment
	if review.State == usecase.DeploymentRejected {
		action = usecase.ActionRejectDeployment
	}
	comment := strings.TrimSpace(review.Comment)
	if comment == "" {
		comment = usecase.DefaultReviewComment
	}
	result := usecase.ActionResult{Action: action, Repo: repo, RunID: runID, Message: "Deployment review submitted"}
	body := reviewDeploymentsBody{
		EnvironmentIDs: review.EnvironmentIDs,
		State:          string(review.State),
		Comment:        comment,
	}
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/pending_deployments")
	return result, c.postJSON(ctx, resource, body)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
