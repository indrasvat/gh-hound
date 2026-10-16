package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

// EmptyBranchRunDiagnostic explains an empty branch-scoped listing.
// With an actor filter the next commands keep it, so following them
// never silently broadens to everyone's runs.
func EmptyBranchRunDiagnostic(repo, branch, actor string) (RunDiagnostic, bool) {
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	actor = strings.TrimSpace(actor)
	if repo == "" || branch == "" {
		return RunDiagnostic{}, false
	}
	subject, actorFlag := "No runs", ""
	if actor != "" {
		subject = "No runs by @" + strings.TrimPrefix(actor, "@")
		if strings.EqualFold(actor, MeActor) {
			subject = "None of your runs"
		}
		actorFlag = " --actor " + shellWord(actor)
	}
	return RunDiagnostic{
		Kind:    RunDiagnosticTargetMismatch,
		Scope:   "branch",
		Message: fmt.Sprintf("%s matched branch %q. Release, tag, or branchless workflows may live outside branch scope; try repo-wide discovery or target a run id.", subject, branch),
		NextCommands: []string{
			fmt.Sprintf("gh hound runs -R %s --all%s --no-tui --json", repo, actorFlag),
			fmt.Sprintf("gh hound runs -R %s --branch <ref>%s --no-tui --json", repo, actorFlag),
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
			return "no valid GitHub credentials (check GH_TOKEN or run gh auth login)"
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

// githubLogin accepts user, bot and Enterprise Managed User logins
// (alice_acme); it exists to reject typos like spaces, not to police
// GitHub's exact rules.
var githubLogin = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}(\[bot\])?$`)

// NormalizeActor validates an actor filter value: @me (any case) or a
// GitHub login with an optional leading @. Bots keep their [bot] suffix.
func NormalizeActor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, MeActor) {
		return MeActor, nil
	}
	login := strings.TrimPrefix(value, "@")
	if !githubLogin.MatchString(login) {
		return "", fmt.Errorf("%q isn't a GitHub login", value)
	}
	return login, nil
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

// KeepActorRuns narrows runs to those login started. GitHub's actor
// parameter is looser: it also returns runs tied to the login's pushes
// (scheduled and bot runs on their commits) whose actor is someone
// else, which would contradict the Actor column.
func KeepActorRuns(runs []model.Run, login string) []model.Run {
	login = strings.TrimPrefix(strings.TrimSpace(login), "@")
	if login == "" {
		return runs
	}
	kept := make([]model.Run, 0, len(runs))
	for _, run := range runs {
		if strings.EqualFold(run.Actor, login) {
			kept = append(kept, run)
		}
	}
	return kept
}

// ActorPageBudget bounds how many list-runs pages an actor listing
// reads to fill one page of that actor's runs.
const ActorPageBudget = 5

// ListActorRuns fills one page (filter.PerPage) with runs filter.Actor
// started. GitHub's actor parameter is loose, so a page can narrow to
// few or none of the actor's runs; following pages are read until the
// page fills, GitHub runs out, or ActorPageBudget pages were read.
func ListActorRuns(ctx context.Context, github GitHub, filter RunFilter) ([]model.Run, error) {
	if filter.Actor == "" || filter.PerPage <= 0 {
		return github.ListRuns(ctx, filter)
	}
	kept := []model.Run{}
	first := max(filter.Page, 1)
	for page := first; page < first+ActorPageBudget; page++ {
		filter.Page = page
		runs, err := github.ListRuns(ctx, filter)
		if err != nil {
			return nil, err
		}
		kept = append(kept, KeepActorRuns(runs, filter.Actor)...)
		if len(kept) >= filter.PerPage || len(runs) < filter.PerPage {
			break
		}
	}
	return kept[:min(len(kept), filter.PerPage)], nil
}

// shellWord quotes a value for a copy-pasteable command when it holds
// anything a shell would expand, like the brackets in dependabot[bot].
func shellWord(value string) string {
	if strings.Trim(value, "-@_.0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return "'" + value + "'"
	}
	return value
}
