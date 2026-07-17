package usecase

import (
	"context"
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
