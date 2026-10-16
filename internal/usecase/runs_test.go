package usecase_test

import (
	"context"
	"errors"
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

type viewerGitHub struct {
	usecase.GitHub
	login string
	err   error
	calls int
}

func (v *viewerGitHub) AuthenticatedLogin(context.Context) (string, error) {
	v.calls++
	return v.login, v.err
}

func TestResolveActor(t *testing.T) {
	ctx := context.Background()
	viewer := &viewerGitHub{login: "alice"}
	for raw, want := range map[string]string{"bob": "bob", "@bob": "bob", " @me ": "alice", "@ME": "alice"} {
		got, err := usecase.ResolveActor(ctx, viewer, raw)
		if err != nil || got != want {
			t.Fatalf("usecase.ResolveActor(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if viewer.calls != 2 {
		t.Fatalf("explicit logins must not call /user: %d calls", viewer.calls)
	}
}

func TestResolveActorExplainsFailures(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		github usecase.GitHub
		want   string
	}{
		{&viewerGitHub{err: usecase.APIError{Kind: usecase.APIErrorAuth, Status: 401}}, "not signed in to GitHub"},
		{&viewerGitHub{err: usecase.APIError{Kind: usecase.APIErrorPermission, Status: 403}}, "can't read its own user"},
		{&viewerGitHub{err: usecase.APIError{Kind: usecase.APIErrorNetwork}}, "GitHub is unreachable"},
		{struct{ usecase.GitHub }{}, "can't report the signed-in login"},
	}
	for _, tt := range tests {
		_, err := usecase.ResolveActor(ctx, tt.github, usecase.MeActor)
		var viewerErr usecase.ViewerError
		if !errors.As(err, &viewerErr) {
			t.Fatalf("error %v is not a usecase.ViewerError", err)
		}
		if !strings.HasPrefix(err.Error(), "can't resolve @me: ") || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("error = %q, want %q", err, tt.want)
		}
	}
}
