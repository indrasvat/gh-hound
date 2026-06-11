package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

const waitingFixtureRunID int64 = 30433655

func TestApprovalsListReturnsPendingEnvironments(t *testing.T) {
	service := usecase.ApprovalsService{GitHub: fake.New(fake.ScenarioWaiting)}

	pending, err := service.List(context.Background(), "indrasvat/gh-hound", waitingFixtureRunID)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	if pending[0].EnvironmentName != "production" || pending[0].EnvironmentID == 0 {
		t.Fatalf("first pending = %#v", pending[0])
	}
	if !pending[0].CurrentUserCanApprove || pending[1].CurrentUserCanApprove {
		t.Fatalf("approvability mismatch: %#v", pending)
	}
}

func TestApprovalsReviewValidatesEnvironmentNames(t *testing.T) {
	service := usecase.ApprovalsService{GitHub: fake.New(fake.ScenarioWaiting)}

	_, err := service.Review(context.Background(), "indrasvat/gh-hound", waitingFixtureRunID, usecase.DeploymentReviewRequest{
		Environments: []string{"mars"},
		Approve:      true,
	})
	actionErr, ok := usecase.AsActionError(err)
	if !ok || actionErr.Kind != usecase.ActionErrorValidation {
		t.Fatalf("unknown environment error = %#v, want validation", err)
	}
}

func TestApprovalsReviewRefusesNotApprovableEnvironment(t *testing.T) {
	github := &recordingApprovalsGitHub{Adapter: fake.New(fake.ScenarioWaiting)}
	service := usecase.ApprovalsService{GitHub: github}

	_, err := service.Review(context.Background(), "indrasvat/gh-hound", waitingFixtureRunID, usecase.DeploymentReviewRequest{
		Environments: []string{"staging"},
		Approve:      true,
	})
	actionErr, ok := usecase.AsActionError(err)
	if !ok || actionErr.Kind != usecase.ActionErrorPermission {
		t.Fatalf("not-approvable error = %#v, want typed permission", err)
	}
	if len(github.reviews) != 0 {
		t.Fatalf("refused review must not reach the API, got %d calls", len(github.reviews))
	}
}

func TestApprovalsReviewDefaultsToAllApprovableEnvironments(t *testing.T) {
	github := &recordingApprovalsGitHub{Adapter: fake.New(fake.ScenarioWaiting)}
	service := usecase.ApprovalsService{GitHub: github}

	outcome, err := service.Review(context.Background(), "indrasvat/gh-hound", waitingFixtureRunID, usecase.DeploymentReviewRequest{
		Approve: true,
	})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if len(github.reviews) != 1 {
		t.Fatalf("review calls = %d, want 1", len(github.reviews))
	}
	review := github.reviews[0]
	if len(review.EnvironmentIDs) != 1 || review.EnvironmentIDs[0] != 7301 {
		t.Fatalf("review must target only approvable environment ids, got %#v", review.EnvironmentIDs)
	}
	if review.State != usecase.DeploymentApproved {
		t.Fatalf("state = %s", review.State)
	}
	if review.Comment != usecase.DefaultReviewComment {
		t.Fatalf("comment = %q, want default", review.Comment)
	}
	if outcome.State != usecase.DeploymentApproved || len(outcome.Environments) != 1 || outcome.Environments[0] != "production" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if outcome.Result.Message != "gate's open." {
		t.Fatalf("approve message = %q", outcome.Result.Message)
	}
}

func TestApprovalsReviewRejectCarriesCommentAndVoice(t *testing.T) {
	github := &recordingApprovalsGitHub{Adapter: fake.New(fake.ScenarioWaiting)}
	service := usecase.ApprovalsService{GitHub: github}

	outcome, err := service.Review(context.Background(), "indrasvat/gh-hound", waitingFixtureRunID, usecase.DeploymentReviewRequest{
		Environments: []string{"production"},
		Approve:      false,
		Comment:      "not on a friday",
	})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if github.reviews[0].State != usecase.DeploymentRejected || github.reviews[0].Comment != "not on a friday" {
		t.Fatalf("review = %#v", github.reviews[0])
	}
	if outcome.Result.Message != "gate stays shut." {
		t.Fatalf("reject message = %q", outcome.Result.Message)
	}
	if outcome.Comment != "not on a friday" {
		t.Fatalf("outcome comment = %q", outcome.Comment)
	}
}

func TestApprovalsReviewWithNothingPendingIsTypedValidation(t *testing.T) {
	service := usecase.ApprovalsService{GitHub: fake.New(fake.ScenarioGreen)}

	_, err := service.Review(context.Background(), "indrasvat/gh-hound", 30433571, usecase.DeploymentReviewRequest{Approve: true})
	actionErr, ok := usecase.AsActionError(err)
	if !ok || actionErr.Kind != usecase.ActionErrorValidation {
		t.Fatalf("nothing-pending error = %#v, want validation", err)
	}
}

func TestApprovalsReviewAppliesMutationPacing(t *testing.T) {
	clock := &approvalsClock{now: time.Unix(1000, 0)}
	service := usecase.ApprovalsService{
		GitHub:  fake.New(fake.ScenarioWaiting),
		Limiter: &usecase.MutationLimiter{MinSpacing: time.Second, Clock: clock},
	}
	for range 2 {
		if _, err := service.Review(context.Background(), "indrasvat/gh-hound", waitingFixtureRunID, usecase.DeploymentReviewRequest{Approve: true}); err != nil {
			t.Fatalf("Review returned error: %v", err)
		}
	}
	if clock.slept == 0 {
		t.Fatal("second review must wait for the mutation limiter")
	}
}

type recordingApprovalsGitHub struct {
	*fake.Adapter
	reviews []usecase.DeploymentReview
}

func (g *recordingApprovalsGitHub) ReviewPendingDeployments(ctx context.Context, repo string, runID int64, review usecase.DeploymentReview) (usecase.ActionResult, error) {
	g.reviews = append(g.reviews, review)
	return g.Adapter.ReviewPendingDeployments(ctx, repo, runID, review)
}

type approvalsClock struct {
	now   time.Time
	slept time.Duration
}

func (c *approvalsClock) Now() time.Time {
	return c.now
}

func (c *approvalsClock) Sleep(duration time.Duration) {
	c.slept += duration
	c.now = c.now.Add(duration)
}
