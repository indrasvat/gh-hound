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
