package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

// SuspectCommitCap bounds the rendered suspect list; TotalSuspects
// still reports the full range.
const SuspectCommitCap = 50

// DefaultDiffMaxPages bounds the history walk (pages × DiffPerPage
// runs). Hitting the cap yields an inconclusive verdict, never a hang.
const (
	DefaultDiffMaxPages = 10
	DiffPerPage         = 100
)

type RegressionStatus string

const (
	RegressionLocated      RegressionStatus = "located"
	RegressionGreen        RegressionStatus = "green"
	RegressionInconclusive RegressionStatus = "inconclusive"
)

// RegressionVerdict answers "who broke main?": the last-green →
// first-red boundary of a workflow's run history plus the commit range
// between them. Verdict carries the hound-voiced one-liner shared by
// the pipe surface and the TUI trail screen.
type RegressionVerdict struct {
	Repo           string
	Workflow       string
	Branch         string
	Status         RegressionStatus
	LastGood       model.Run
	FirstBad       model.Run
	SuspectCommits []model.Commit
	TotalSuspects  int
	CompareURL     string
	RunsScanned    int
	Verdict        string
}

type DiffService struct {
	History  WorkflowRunHistory
	Compare  CommitComparer
	MaxPages int
	PerPage  int
}

// LocateRegression walks the workflow's run history newest-first to
// the most recent completed-success run before the current failure
// streak.
//
// Signal rules (pinned by tests):
//   - completed + success → green; completed + failure/timed_out/
//     action_required → red.
//   - cancelled, skipped, neutral, stale, and anything not completed
//     carry no signal and are stepped over.
//   - a rerun-flipped run counts by its LATEST attempt conclusion (the
//     list endpoint already reports it; Task 300 owns earlier-attempt
//     forensics).
func (s DiffService) LocateRegression(ctx context.Context, repo, workflow, branch string) (RegressionVerdict, error) {
	repo = strings.TrimSpace(repo)
	workflow = strings.TrimSpace(workflow)
	if repo == "" {
		return RegressionVerdict{}, fmt.Errorf("repo is required")
	}
	if workflow == "" {
		return RegressionVerdict{}, fmt.Errorf("workflow is required")
	}
	if s.History == nil || s.Compare == nil {
		return RegressionVerdict{}, fmt.Errorf("regression scan is unavailable for this adapter")
	}

	verdict := RegressionVerdict{Repo: repo, Workflow: workflow, Branch: branch}
	walker := newRunHistoryWalker(s.History, repo, workflow, branch, s.maxPages(), s.perPage())

	var firstBad *model.Run
	for {
		run, ok, err := walker.Next(ctx)
		if err != nil {
			return RegressionVerdict{}, err
		}
		if !ok {
			break
		}
		verdict.RunsScanned++
		good, bad := regressionSignal(run)
		switch {
		case bad:
			failed := run
			firstBad = &failed
		case good && firstBad == nil:
			verdict.Status = RegressionGreen
			verdict.LastGood = run
			verdict.Verdict = fmt.Sprintf("nothing to chase: #%d came back clean.", run.RunNumber)
			return verdict, nil
		case good:
			verdict.Status = RegressionLocated
			verdict.LastGood = run
			verdict.FirstBad = *firstBad
			return s.resolveSuspects(ctx, verdict)
		}
	}

	verdict.Status = RegressionInconclusive
	switch {
	case walker.capped:
		verdict.Verdict = coldTrailLine(verdict.RunsScanned)
	case firstBad != nil:
		verdict.FirstBad = *firstBad
		verdict.Verdict = "trail went cold: history ran out before a clean run."
	default:
		verdict.Verdict = "nothing to read: no completed runs on this trail."
	}
	return verdict, nil
}

func (s DiffService) resolveSuspects(ctx context.Context, verdict RegressionVerdict) (RegressionVerdict, error) {
	rangeInfo, err := s.Compare.CompareCommits(ctx, verdict.Repo, verdict.LastGood.HeadSHA, verdict.FirstBad.HeadSHA)
	if err != nil {
		return RegressionVerdict{}, err
	}
	commits := rangeInfo.Commits
	if len(commits) > SuspectCommitCap {
		commits = commits[:SuspectCommitCap]
	}
	verdict.SuspectCommits = commits
	verdict.TotalSuspects = rangeInfo.TotalCommits
	if verdict.TotalSuspects == 0 {
		verdict.TotalSuspects = len(rangeInfo.Commits)
	}
	verdict.CompareURL = strings.TrimSpace(rangeInfo.HTMLURL)
	if verdict.CompareURL == "" {
		verdict.CompareURL = fmt.Sprintf("https://github.com/%s/compare/%s...%s", verdict.Repo, verdict.LastGood.HeadSHA, verdict.FirstBad.HeadSHA)
	}
	verdict.Verdict = fmt.Sprintf("scent picked up: #%d was clean, #%d wasn't.", verdict.LastGood.RunNumber, verdict.FirstBad.RunNumber)
	return verdict, nil
}

func (s DiffService) maxPages() int {
	if s.MaxPages > 0 {
		return s.MaxPages
	}
	return DefaultDiffMaxPages
}

func (s DiffService) perPage() int {
	if s.PerPage > 0 {
		return s.PerPage
	}
	return DiffPerPage
}

// regressionSignal classifies one run for the boundary scan.
func regressionSignal(run model.Run) (good, bad bool) {
	if run.Status != model.StatusCompleted {
		return false, false
	}
	switch run.Conclusion {
	case model.ConclusionSuccess:
		return true, false
	case model.ConclusionFailure, model.ConclusionTimedOut, model.ConclusionActionRequired:
		return false, true
	default:
		// cancelled, skipped, neutral, stale: no signal either way.
		return false, false
	}
}

// runHistoryWalker pages a workflow's run history newest-first under a
// hard page budget. Extracted as the reusable history walk for Task
// 300 (flake scoring) — same pagination, different classifier.
type runHistoryWalker struct {
	history   WorkflowRunHistory
	repo      string
	workflow  string
	filter    RunFilter
	maxPages  int
	page      int
	buf       []model.Run
	idx       int
	exhausted bool
	capped    bool
	seen      map[int64]bool
}

func newRunHistoryWalker(history WorkflowRunHistory, repo, workflow, branch string, maxPages, perPage int) *runHistoryWalker {
	return &runHistoryWalker{
		history:  history,
		repo:     repo,
		workflow: workflow,
		filter:   RunFilter{Repo: repo, Branch: branch, PerPage: perPage},
		maxPages: maxPages,
		seen:     map[int64]bool{},
	}
}

func (w *runHistoryWalker) Next(ctx context.Context) (model.Run, bool, error) {
	for {
		for w.idx >= len(w.buf) {
			if w.exhausted {
				return model.Run{}, false, nil
			}
			if w.page >= w.maxPages {
				w.capped = true
				return model.Run{}, false, nil
			}
			w.page++
			filter := w.filter
			filter.Page = w.page
			runs, err := w.history.ListWorkflowRuns(ctx, w.repo, w.workflow, filter)
			if err != nil {
				return model.Run{}, false, err
			}
			if len(runs) < filter.PerPage {
				// A short page is the end of recorded history.
				w.exhausted = true
			}
			if w.page == 1 && len(runs) > 0 && w.filter.CreatedBefore.IsZero() {
				// Anchor later pages to the newest run that existed when
				// the scan started: runs landing mid-walk would otherwise
				// shift page seams and re-serve (or skip) rows.
				w.filter.CreatedBefore = runs[0].CreatedAt
			}
			w.buf = runs
			w.idx = 0
			if len(runs) == 0 {
				return model.Run{}, false, nil
			}
		}
		run := w.buf[w.idx]
		w.idx++
		if w.seen[run.ID] {
			// Same-timestamp arrivals can still slip inside the anchor;
			// a duplicate row is noise, not history — skip it.
			continue
		}
		w.seen[run.ID] = true
		return run, true, nil
	}
}

// coldTrailLine voices the capped-scan verdict.
func coldTrailLine(runsScanned int) string {
	return fmt.Sprintf("trail went cold after %s runs.", groupThousands(runsScanned))
}

func groupThousands(value int) string {
	raw := fmt.Sprintf("%d", value)
	if len(raw) <= 3 {
		return raw
	}
	var out strings.Builder
	lead := len(raw) % 3
	if lead > 0 {
		out.WriteString(raw[:lead])
	}
	for i := lead; i < len(raw); i += 3 {
		if out.Len() > 0 {
			out.WriteString(",")
		}
		out.WriteString(raw[i : i+3])
	}
	return out.String()
}
