package fake

import (
	"context"
	"strings"
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

func TestFakeCachesCapabilityIsDeterministicAndNearCap(t *testing.T) {
	adapter := New(ScenarioGreen)
	var _ usecase.GitHubCaches = adapter

	caches, err := adapter.ListCaches(context.Background(), "indrasvat/gh-hound", usecase.CacheFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(caches) != 5 {
		t.Fatalf("caches = %d, want 5", len(caches))
	}
	usage, err := adapter.CacheUsage(context.Background(), "indrasvat/gh-hound")
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, cache := range caches {
		total += cache.SizeInBytes
	}
	if usage.ActiveSizeInBytes != total || usage.ActiveCount != len(caches) {
		t.Fatalf("usage must mirror the fixture set: %#v vs %d/%d", usage, total, len(caches))
	}
	// The e2e scenario must sit past the 90% eviction warning of the
	// 10 GiB fallback cap.
	if cap := int64(10) << 30; float64(usage.ActiveSizeInBytes)/float64(cap) <= 0.9 {
		t.Fatalf("fixture usage %d must exceed 90%% of the 10 GiB cap", usage.ActiveSizeInBytes)
	}

	filtered, err := adapter.ListCaches(context.Background(), "indrasvat/gh-hound", usecase.CacheFilter{Key: "go-mod", Ref: "refs/pull/7/merge"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].ID != 9004 {
		t.Fatalf("key prefix + ref filter wrong: %#v", filtered)
	}
}

func TestFakeCacheDeletesReportCountsAndNotFound(t *testing.T) {
	adapter := New(ScenarioGreen)
	// Delete-by-key matches the COMPLETE key (live API semantics): a
	// prefix must not delete, the exact key digs up every ref's copy.
	if _, err := adapter.DeleteCachesByKey(context.Background(), "indrasvat/gh-hound", "go-mod", ""); err == nil {
		t.Fatal("key prefix must not delete — the live API matches complete keys")
	}
	count, err := adapter.DeleteCachesByKey(context.Background(), "indrasvat/gh-hound", "go-mod-Linux-x64-1f2e3d", "")
	if err != nil || count != 2 {
		t.Fatalf("delete by exact key = %d, %v; want 2 (both refs), nil", count, err)
	}
	count, err = adapter.DeleteCachesByKey(context.Background(), "indrasvat/gh-hound", "go-mod-Linux-x64-1f2e3d", "refs/pull/7/merge")
	if err != nil || count != 1 {
		t.Fatalf("ref-narrowed delete = %d, %v; want 1, nil", count, err)
	}
	count, err = adapter.DeleteCacheByID(context.Background(), "indrasvat/gh-hound", 9005)
	if err != nil || count != 1 {
		t.Fatalf("delete by id = %d, %v; want 1, nil", count, err)
	}
	_, err = adapter.DeleteCachesByKey(context.Background(), "indrasvat/gh-hound", "ghost", "")
	if actionErr, ok := usecase.AsActionError(err); !ok || actionErr.Kind != usecase.ActionErrorNotFound {
		t.Fatalf("ghost key must be typed not_found, got %v", err)
	}
	_, err = New(ScenarioPermission).DeleteCacheByID(context.Background(), "indrasvat/gh-hound", 9001)
	if actionErr, ok := usecase.AsActionError(err); !ok || actionErr.Kind != usecase.ActionErrorPermission {
		t.Fatalf("permission scenario must refuse deletes, got %v", err)
	}
	if _, err := New(ScenarioEmpty).ListCaches(context.Background(), "indrasvat/gh-hound", usecase.CacheFilter{}); err != nil {
		t.Fatalf("empty scenario lists an empty kennel, got %v", err)
	}
}

func TestFakeWorkflowsCoverAllDocumentedStates(t *testing.T) {
	adapter := New(ScenarioGreen)
	workflows, err := adapter.ListWorkflows(context.Background(), "indrasvat/gh-hound")
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	states := map[string]string{}
	for _, workflow := range workflows {
		states[workflow.State] = workflow.Path
	}
	for _, state := range []string{
		model.WorkflowStateActive,
		model.WorkflowStateDisabledManually,
		model.WorkflowStateDisabledInactivity,
		model.WorkflowStateDisabledFork,
		model.WorkflowStateDeleted,
	} {
		if states[state] == "" {
			t.Fatalf("fake workflows missing state %q (have %#v)", state, states)
		}
	}
	// The dispatch path must keep exactly one dispatchable workflow so
	// the single-form launch behavior stays deterministic: only ci.yml
	// carries a workflow_dispatch trigger.
	for _, workflow := range workflows {
		raw, err := adapter.FetchWorkflowFile(context.Background(), "indrasvat/gh-hound", workflow.Path)
		if err != nil {
			t.Fatalf("FetchWorkflowFile(%s): %v", workflow.Path, err)
		}
		hasDispatch := strings.Contains(raw, "workflow_dispatch")
		if workflow.Path == ".github/workflows/ci.yml" && !hasDispatch {
			t.Fatalf("ci.yml lost its workflow_dispatch trigger")
		}
		if workflow.Path != ".github/workflows/ci.yml" && hasDispatch {
			t.Fatalf("%s should not be dispatchable in the fake", workflow.Path)
		}
	}
}

func TestFakeWorkflowToggleHonorsScenarioErrors(t *testing.T) {
	adapter := New(ScenarioGreen)
	result, err := adapter.EnableWorkflow(context.Background(), "indrasvat/gh-hound", "nightly.yml")
	if err != nil {
		t.Fatalf("EnableWorkflow: %v", err)
	}
	if result.Action != usecase.ActionEnableWorkflow || result.WorkflowID != "nightly.yml" {
		t.Fatalf("enable result = %#v", result)
	}
	if _, err := adapter.DisableWorkflow(context.Background(), "indrasvat/gh-hound", "ci.yml"); err != nil {
		t.Fatalf("DisableWorkflow: %v", err)
	}
	permission := New(ScenarioPermission)
	_, err = permission.DisableWorkflow(context.Background(), "indrasvat/gh-hound", "ci.yml")
	actionErr, ok := usecase.AsActionError(err)
	if !ok || actionErr.Kind != usecase.ActionErrorPermission {
		t.Fatalf("permission scenario error = %v", err)
	}
}

func TestPackScenarioStaggersCompletionAcrossTicks(t *testing.T) {
	adapter := New(ScenarioPack)
	statuses := func() []string {
		runs, err := adapter.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound"})
		if err != nil {
			t.Fatalf("pack list returned error: %v", err)
		}
		if len(runs) != 4 {
			t.Fatalf("pack scenario runs = %d, want 4 (3 pack + 1 chained)", len(runs))
		}
		out := make([]string, 0, len(runs))
		for _, run := range runs {
			out = append(out, run.Name+":"+string(run.Status)+"/"+string(run.Conclusion))
		}
		return out
	}

	tick1 := statuses()
	if tick1[3] != "CI:in_progress/" || tick1[2] != "Release:queued/" || tick1[1] != "Docs:queued/" {
		t.Fatalf("tick 1 = %#v", tick1)
	}
	tick2 := statuses()
	if tick2[3] != "CI:completed/success" || tick2[2] != "Release:in_progress/" {
		t.Fatalf("tick 2 = %#v", tick2)
	}
	tick3 := statuses()
	if tick3[2] != "Release:completed/success" || tick3[1] != "Docs:in_progress/" {
		t.Fatalf("tick 3 = %#v", tick3)
	}
	tick4 := statuses()
	if tick4[1] != "Docs:completed/failure" {
		t.Fatalf("tick 4 = %#v", tick4)
	}
	// The settled state holds on later ticks.
	tick5 := statuses()
	if tick5[1] != "Docs:completed/failure" || tick5[3] != "CI:completed/success" {
		t.Fatalf("tick 5 = %#v", tick5)
	}
	// All four share the sha; the chained run rides a different event.
	runs, _ := adapter.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound"})
	for _, run := range runs {
		if run.HeadSHA != packSHA {
			t.Fatalf("pack run %s sha = %q, want shared %q", run.Name, run.HeadSHA, packSHA)
		}
	}
	if runs[0].Event != "workflow_run" {
		t.Fatalf("chained run event = %q, want workflow_run", runs[0].Event)
	}
}
