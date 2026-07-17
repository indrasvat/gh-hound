package usecase_test

import (
	"context"
	"strings"
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

func TestEmptyBranchRunDiagnosticGuidesReleaseAndTagTargeting(t *testing.T) {
	diagnostic, ok := usecase.EmptyBranchRunDiagnostic("indrasvat/gh-hound", "release/v1")
	if !ok {
		t.Fatal("expected diagnostic for branch-scoped empty result")
	}
	if diagnostic.Kind != usecase.RunDiagnosticTargetMismatch || diagnostic.Scope != "branch" {
		t.Fatalf("diagnostic identity = %#v", diagnostic)
	}
	for _, want := range []string{
		`branch "release/v1"`,
		"Release, tag, or branchless workflows",
	} {
		if !strings.Contains(diagnostic.Message, want) {
			t.Fatalf("message missing %q: %s", want, diagnostic.Message)
		}
	}
	joined := strings.Join(diagnostic.NextCommands, "\n")
	for _, want := range []string{
		"gh hound runs -R indrasvat/gh-hound --all --no-tui --json",
		"gh hound runs -R indrasvat/gh-hound --branch <ref> --no-tui --json",
		"gh hound runs -R indrasvat/gh-hound --run <id> --no-tui --json",
		"gh hound watch -R indrasvat/gh-hound --group --run <run-id> --no-tui --timeout 10m",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("next commands missing %q:\n%s", want, joined)
		}
	}
}

func TestEmptyBranchRunDiagnosticRequiresRepoAndBranch(t *testing.T) {
	for _, input := range []struct {
		repo   string
		branch string
	}{
		{repo: "", branch: "main"},
		{repo: "indrasvat/gh-hound", branch: ""},
	} {
		if diagnostic, ok := usecase.EmptyBranchRunDiagnostic(input.repo, input.branch); ok {
			t.Fatalf("unexpected diagnostic for %#v: %#v", input, diagnostic)
		}
	}
}
