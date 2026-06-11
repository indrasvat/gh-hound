package fake

import (
	"context"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestScenariosReturnDeterministicRuns(t *testing.T) {
	tests := []struct {
		scenario Scenario
		wantRuns int
		wantErr  bool
		want     model.Conclusion
		status   model.Status
	}{
		{ScenarioGreen, 3, false, model.ConclusionSuccess, model.StatusCompleted},
		{ScenarioFailing, 1, false, model.ConclusionFailure, model.StatusCompleted},
		{ScenarioRunning, 1, false, model.ConclusionNone, model.StatusInProgress},
		{ScenarioLogRefetch, 1, false, model.ConclusionFailure, model.StatusCompleted},
		{ScenarioEmpty, 0, false, model.ConclusionNone, ""},
		{ScenarioRateLimited, 0, true, model.ConclusionNone, ""},
		{ScenarioNetworkError, 0, true, model.ConclusionNone, ""},
	}

	for _, tt := range tests {
		adapter := New(tt.scenario)
		runs, err := adapter.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound"})
		if (err != nil) != tt.wantErr {
			t.Fatalf("%s err = %v, wantErr %v", tt.scenario, err, tt.wantErr)
		}
		if len(runs) != tt.wantRuns {
			t.Fatalf("%s runs = %d, want %d", tt.scenario, len(runs), tt.wantRuns)
		}
		if tt.wantRuns > 0 && (runs[0].Conclusion != tt.want || runs[0].Status != tt.status) {
			t.Fatalf("%s first run = %#v", tt.scenario, runs[0])
		}
	}
}

func TestLogRefetchScenarioExposesRecoveredLogNotice(t *testing.T) {
	adapter := New(ScenarioLogRefetch)
	raw, err := adapter.FetchJobLog(context.Background(), "indrasvat/gh-hound", 399444496)
	if err != nil {
		t.Fatalf("FetchJobLog error = %v", err)
	}
	if raw == "" {
		t.Fatal("FetchJobLog returned empty recovered log")
	}
	notice, ok := adapter.LastLogRefetch(399444496)
	if !ok {
		t.Fatal("missing log refetch notice")
	}
	if notice.ExpiredStatus != 410 || notice.Attempts != 2 {
		t.Fatalf("notice = %#v", notice)
	}
}

func TestScenariosReproduceErrorTaxonomy(t *testing.T) {
	tests := []struct {
		name     string
		scenario Scenario
		action   func(*Adapter) error
		want     usecase.ErrorClass
	}{
		{
			name:     "rate limit",
			scenario: ScenarioRateLimited,
			action: func(adapter *Adapter) error {
				_, err := adapter.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
				return err
			},
			want: usecase.ErrorClassRateLimit,
		},
		{
			name:     "network",
			scenario: ScenarioNetworkError,
			action: func(adapter *Adapter) error {
				_, err := adapter.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound"})
				return err
			},
			want: usecase.ErrorClassNetwork,
		},
		{
			name:     "log render",
			scenario: ScenarioLogRender,
			action: func(adapter *Adapter) error {
				_, err := adapter.FetchJobLog(context.Background(), "indrasvat/gh-hound", 399444496)
				return err
			},
			want: usecase.ErrorClassLogRender,
		},
		{
			name:     "mutation rejected",
			scenario: ScenarioConflict,
			action: func(adapter *Adapter) error {
				_, err := adapter.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
				return err
			},
			want: usecase.ErrorClassMutationRejected,
		},
	}
	for _, tt := range tests {
		err := tt.action(New(tt.scenario))
		got := usecase.ResilienceFor(err, usecase.ErrorContext{})
		if got.Class != tt.want {
			t.Fatalf("%s class = %s, want %s (err %v)", tt.name, got.Class, tt.want, err)
		}
	}
}

func TestWaitingScenarioSurfacesGatedRunAndPendingEnvironments(t *testing.T) {
	adapter := New(ScenarioWaiting)
	runs, err := adapter.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound"})
	if err != nil {
		t.Fatalf("ListRuns error = %v", err)
	}
	if len(runs) < 2 {
		t.Fatalf("waiting scenario runs = %d, want at least 2", len(runs))
	}
	if runs[0].Status != model.StatusWaiting {
		t.Fatalf("newest run status = %s, want waiting", runs[0].Status)
	}

	pending, err := adapter.ListPendingDeployments(context.Background(), "indrasvat/gh-hound", runs[0].ID)
	if err != nil {
		t.Fatalf("ListPendingDeployments error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d environments, want 2", len(pending))
	}
	if pending[0].EnvironmentName != "production" || !pending[0].CurrentUserCanApprove {
		t.Fatalf("first pending env = %#v, want approvable production", pending[0])
	}
	if pending[1].CurrentUserCanApprove {
		t.Fatalf("second pending env must not be approvable: %#v", pending[1])
	}
	if len(pending[0].Reviewers) == 0 || len(pending[1].Reviewers) == 0 {
		t.Fatal("pending environments must carry reviewers")
	}

	// Non-waiting runs have no gates.
	none, err := adapter.ListPendingDeployments(context.Background(), "indrasvat/gh-hound", runs[1].ID)
	if err != nil || len(none) != 0 {
		t.Fatalf("non-waiting run pending = %v, %v; want empty", none, err)
	}

	result, err := adapter.ReviewPendingDeployments(context.Background(), "indrasvat/gh-hound", runs[0].ID, usecase.DeploymentReview{
		EnvironmentIDs: []int64{pending[0].EnvironmentID},
		State:          usecase.DeploymentApproved,
		Comment:        usecase.DefaultReviewComment,
	})
	if err != nil {
		t.Fatalf("ReviewPendingDeployments error = %v", err)
	}
	if result.Action != usecase.ActionApproveDeployment {
		t.Fatalf("review result = %#v", result)
	}
}

func TestWaitingScenarioReviewHonorsErrorScenarios(t *testing.T) {
	adapter := New(ScenarioPermission)
	_, err := adapter.ReviewPendingDeployments(context.Background(), "indrasvat/gh-hound", 1, usecase.DeploymentReview{
		EnvironmentIDs: []int64{1},
		State:          usecase.DeploymentRejected,
	})
	actionErr, ok := usecase.AsActionError(err)
	if !ok || actionErr.Kind != usecase.ActionErrorPermission {
		t.Fatalf("permission scenario review error = %#v", err)
	}
}

func TestFakeListRunsHonorsStatusFilter(t *testing.T) {
	adapter := New(ScenarioFailing)
	runs, err := adapter.ListRuns(context.Background(), usecase.RunFilter{Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("failing fixture has no in_progress runs; got %d", len(runs))
	}
	runs, err = adapter.ListRuns(context.Background(), usecase.RunFilter{Status: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("failing fixture must match status=failure: got %d", len(runs))
	}
}
