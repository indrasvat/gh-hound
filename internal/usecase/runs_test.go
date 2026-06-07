package usecase_test

import (
	"context"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestRunsServiceLoadsFromGitHubPort(t *testing.T) {
	service := usecase.RunsService{GitHub: fake.New(fake.ScenarioFailing)}

	runs, err := service.LoadRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound"})
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	if len(runs) != 1 || runs[0].Name != "CI" {
		t.Fatalf("unexpected runs: %#v", runs)
	}
}
