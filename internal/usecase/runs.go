package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

type RunsService struct {
	GitHub GitHub
}

func (s RunsService) LoadRuns(ctx context.Context, filter RunFilter) ([]model.Run, error) {
	return s.GitHub.ListRuns(ctx, filter)
}

type RunDiagnosticKind string

const (
	RunDiagnosticTargetMismatch RunDiagnosticKind = "target_mismatch"
)

type RunDiagnostic struct {
	Kind         RunDiagnosticKind
	Scope        string
	Message      string
	NextCommands []string
}

func EmptyBranchRunDiagnostic(repo, branch string) (RunDiagnostic, bool) {
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if repo == "" || branch == "" {
		return RunDiagnostic{}, false
	}
	return RunDiagnostic{
		Kind:    RunDiagnosticTargetMismatch,
		Scope:   "branch",
		Message: fmt.Sprintf("No runs matched branch %q. Release, tag, or branchless workflows may live outside branch scope; try repo-wide discovery or target a run id.", branch),
		NextCommands: []string{
			fmt.Sprintf("gh hound runs -R %s --all --no-tui --json", repo),
			fmt.Sprintf("gh hound runs -R %s --branch <ref> --no-tui --json", repo),
			fmt.Sprintf("gh hound runs -R %s --run <id> --no-tui --json", repo),
			fmt.Sprintf("gh hound watch -R %s --group --run <run-id> --no-tui --timeout 10m", repo),
		},
	}, true
}

// MeActor is the actor filter value that stands for the authenticated
// user.
const MeActor = "@me"

// ViewerError explains why @me could not resolve. It is distinct from
// "no matching runs": callers must not fall back to an unfiltered list.
type ViewerError struct {
	Err error
}

func (e ViewerError) Error() string {
	return "can't resolve @me: " + e.Reason()
}

// Reason is the human cause without the @me prefix.
func (e ViewerError) Reason() string {
	var apiErr APIError
	if errors.As(e.Err, &apiErr) {
		switch apiErr.Kind {
		case APIErrorAuth:
			return "not signed in to GitHub (set GH_TOKEN or run gh auth login)"
		case APIErrorPermission:
			return "this token can't read its own user (GitHub App and Actions tokens can't)"
		case APIErrorRateLimit:
			return "GitHub rate limit reached"
		case APIErrorNetwork:
			return "GitHub is unreachable"
		}
	}
	if e.Err == nil {
		return "this GitHub client can't report the signed-in login"
	}
	return e.Err.Error()
}

func (e ViewerError) Unwrap() error {
	return e.Err
}

// ResolveActor turns an actor filter into the login GitHub filters by:
// a leading @ is dropped and @me becomes the authenticated login.
func ResolveActor(ctx context.Context, github GitHub, actor string) (string, error) {
	actor = strings.TrimSpace(actor)
	if !strings.EqualFold(actor, MeActor) {
		return strings.TrimPrefix(actor, "@"), nil
	}
	viewer, ok := github.(ViewerProvider)
	if !ok {
		return "", ViewerError{}
	}
	login, err := viewer.AuthenticatedLogin(ctx)
	if err != nil {
		return "", ViewerError{Err: err}
	}
	return login, nil
}
