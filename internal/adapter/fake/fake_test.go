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
