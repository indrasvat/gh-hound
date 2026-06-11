package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// fakeTrail seeds a paginated workflow history plus a compare range.
type fakeTrail struct {
	pages        map[int][]model.Run
	listErr      error
	listCalls    int
	lastFilter   RunFilter
	gotWorkflow  string
	rangeInfo    model.CommitRange
	compareErr   error
	compareCalls int
	gotBase      string
	gotHead      string
}

func (f *fakeTrail) ListWorkflowRuns(_ context.Context, _ string, workflow string, filter RunFilter) ([]model.Run, error) {
	f.listCalls++
	f.gotWorkflow = workflow
	f.lastFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.pages[filter.Page], nil
}

func (f *fakeTrail) CompareCommits(_ context.Context, _ string, base, head string) (model.CommitRange, error) {
	f.compareCalls++
	f.gotBase = base
	f.gotHead = head
	if f.compareErr != nil {
		return model.CommitRange{}, f.compareErr
	}
	return f.rangeInfo, nil
}

func trailRun(number int, status model.Status, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:         int64(1000 + number),
		Name:       "CI",
		Status:     status,
		Conclusion: conclusion,
		RunNumber:  number,
		RunAttempt: 1,
		HeadBranch: "main",
		HeadSHA:    fmt.Sprintf("sha%04d", number),
	}
}

func red(number int) model.Run {
	return trailRun(number, model.StatusCompleted, model.ConclusionFailure)
}

func green(number int) model.Run {
	return trailRun(number, model.StatusCompleted, model.ConclusionSuccess)
}

func locate(t *testing.T, trail *fakeTrail, maxPages int) RegressionVerdict {
	t.Helper()
	service := DiffService{History: trail, Compare: trail, MaxPages: maxPages, PerPage: 5}
	verdict, err := service.LocateRegression(context.Background(), "indrasvat/gh-hound", "ci.yml", "main")
	if err != nil {
		t.Fatalf("LocateRegression returned error: %v", err)
	}
	return verdict
}

// TestLocateRegressionAnchorsPagesAndSkipsSeamDuplicates pins the
// pagination-stability contract: page 2+ requests carry the first
// page's newest created_at as CreatedBefore, and rows re-served across
// a drifted seam are deduped instead of re-scanned (codex blocker:
// without the anchor, runs landing mid-walk push the boundary past the
// page cap and a located regression degrades to inconclusive).
func TestLocateRegressionAnchorsPagesAndSkipsSeamDuplicates(t *testing.T) {
	newest := red(10)
	newest.CreatedAt = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	trail := &fakeTrail{
		pages: map[int][]model.Run{
			// Page 2 re-serves page 1's tail (seam drift) before the
			// real continuation that holds the clean boundary.
			1: {newest, red(9), red(8), red(7), red(6)},
			2: {red(7), red(6), red(5), green(4), red(3)},
		},
		rangeInfo: model.CommitRange{TotalCommits: 1},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionLocated {
		t.Fatalf("status = %s, want located\n%#v", verdict.Status, verdict)
	}
	if verdict.LastGood.RunNumber != 4 || verdict.FirstBad.RunNumber != 5 {
		t.Fatalf("boundary = #%d -> #%d, want #4 -> #5", verdict.LastGood.RunNumber, verdict.FirstBad.RunNumber)
	}
	if got := trail.lastFilter.CreatedBefore; !got.Equal(newest.CreatedAt) {
		t.Fatalf("page 2 filter CreatedBefore = %v, want the page-1 anchor %v", got, newest.CreatedAt)
	}
	if verdict.RunsScanned != 7 {
		t.Fatalf("runs scanned = %d, want 7 unique (duplicates must not count)", verdict.RunsScanned)
	}
}

func TestLocateRegressionFindsCleanBoundary(t *testing.T) {
	trail := &fakeTrail{
		pages: map[int][]model.Run{
			1: {red(104), red(103), red(102), green(101), green(100)},
		},
		rangeInfo: model.CommitRange{
			TotalCommits: 2,
			HTMLURL:      "https://github.com/indrasvat/gh-hound/compare/sha0101...sha0102",
			Commits: []model.Commit{
				{SHA: "aaa", Author: "indrasvat", Message: "feat: break it"},
				{SHA: "bbb", Author: "indrasvat", Message: "chore: tidy"},
			},
		},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionLocated {
		t.Fatalf("status = %q, want located", verdict.Status)
	}
	if verdict.LastGood.RunNumber != 101 || verdict.FirstBad.RunNumber != 102 {
		t.Fatalf("boundary = #%d → #%d, want #101 → #102", verdict.LastGood.RunNumber, verdict.FirstBad.RunNumber)
	}
	if trail.gotBase != "sha0101" || trail.gotHead != "sha0102" {
		t.Fatalf("compare called with %s...%s", trail.gotBase, trail.gotHead)
	}
	if verdict.TotalSuspects != 2 || len(verdict.SuspectCommits) != 2 {
		t.Fatalf("suspects = %d/%d", len(verdict.SuspectCommits), verdict.TotalSuspects)
	}
	if verdict.CompareURL != trail.rangeInfo.HTMLURL {
		t.Fatalf("compare url = %q", verdict.CompareURL)
	}
	if verdict.Verdict != "scent picked up: #101 was clean, #102 wasn't." {
		t.Fatalf("verdict line = %q", verdict.Verdict)
	}
	if trail.gotWorkflow != "ci.yml" || trail.lastFilter.Branch != "main" {
		t.Fatalf("history called with workflow=%q branch=%q", trail.gotWorkflow, trail.lastFilter.Branch)
	}
}

// Cancelled and skipped runs between the streak and the boundary carry
// no signal either way: the scan steps over them (documented skip-list:
// cancelled, skipped, neutral, stale, and anything not completed).
func TestLocateRegressionSkipsCancelledAndSkippedInsideStreak(t *testing.T) {
	cancelled := trailRun(103, model.StatusCompleted, model.ConclusionCancelled)
	skipped := trailRun(101, model.StatusCompleted, model.ConclusionSkipped)
	trail := &fakeTrail{
		pages: map[int][]model.Run{
			1: {red(104), cancelled, red(102), skipped, green(100)},
		},
		rangeInfo: model.CommitRange{TotalCommits: 1, Commits: []model.Commit{{SHA: "ccc"}}},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionLocated {
		t.Fatalf("status = %q, want located", verdict.Status)
	}
	if verdict.LastGood.RunNumber != 100 || verdict.FirstBad.RunNumber != 102 {
		t.Fatalf("boundary = #%d → #%d, want #100 → #102", verdict.LastGood.RunNumber, verdict.FirstBad.RunNumber)
	}
}

// A run that failed then succeeded on rerun counts by its LATEST
// attempt conclusion — the list endpoint already reports the latest
// attempt, and the scan must not second-guess it. (Interplay with Task
// 300: flake scoring will look at earlier attempts; the boundary scan
// never does.)
func TestLocateRegressionCountsRerunFlippedRunAsGreen(t *testing.T) {
	flipped := green(102)
	flipped.RunAttempt = 2
	trail := &fakeTrail{
		pages: map[int][]model.Run{
			1: {red(104), red(103), flipped, green(101)},
		},
		rangeInfo: model.CommitRange{TotalCommits: 1, Commits: []model.Commit{{SHA: "ddd"}}},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionLocated {
		t.Fatalf("status = %q, want located", verdict.Status)
	}
	if verdict.LastGood.RunNumber != 102 || verdict.LastGood.RunAttempt != 2 {
		t.Fatalf("last good = #%d attempt %d, want the rerun-flipped #102", verdict.LastGood.RunNumber, verdict.LastGood.RunAttempt)
	}
	if verdict.FirstBad.RunNumber != 103 {
		t.Fatalf("first bad = #%d, want #103", verdict.FirstBad.RunNumber)
	}
}

func TestLocateRegressionAllGreenVerdict(t *testing.T) {
	trail := &fakeTrail{
		pages: map[int][]model.Run{1: {green(104), green(103)}},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionGreen {
		t.Fatalf("status = %q, want green", verdict.Status)
	}
	if verdict.LastGood.RunNumber != 104 {
		t.Fatalf("last good = #%d, want the newest green", verdict.LastGood.RunNumber)
	}
	if trail.compareCalls != 0 {
		t.Fatalf("compare calls = %d, want 0", trail.compareCalls)
	}
	if verdict.Verdict != "nothing to chase: #104 came back clean." {
		t.Fatalf("verdict line = %q", verdict.Verdict)
	}
}

// A pending head does not flip the verdict: the scan reads the newest
// COMPLETED signal.
func TestLocateRegressionIgnoresPendingHead(t *testing.T) {
	running := trailRun(105, model.StatusInProgress, model.ConclusionNone)
	trail := &fakeTrail{
		pages: map[int][]model.Run{
			1: {running, red(104), green(103)},
		},
		rangeInfo: model.CommitRange{TotalCommits: 1, Commits: []model.Commit{{SHA: "eee"}}},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionLocated || verdict.FirstBad.RunNumber != 104 {
		t.Fatalf("verdict = %+v, want located at #104", verdict)
	}
}

// Hitting the page cap yields an inconclusive verdict — never a hang,
// never an error. The call count pins the API budget.
func TestLocateRegressionStopsAtPageCapInconclusive(t *testing.T) {
	pages := map[int][]model.Run{}
	number := 1000
	for page := 1; page <= 4; page++ {
		var runs []model.Run
		for range 5 {
			runs = append(runs, red(number))
			number--
		}
		pages[page] = runs
	}
	trail := &fakeTrail{pages: pages}
	verdict := locate(t, trail, 3)
	if verdict.Status != RegressionInconclusive {
		t.Fatalf("status = %q, want inconclusive", verdict.Status)
	}
	if trail.listCalls != 3 {
		t.Fatalf("list calls = %d, want exactly the 3-page budget", trail.listCalls)
	}
	if trail.compareCalls != 0 {
		t.Fatalf("compare calls = %d, want 0", trail.compareCalls)
	}
	if verdict.RunsScanned != 15 {
		t.Fatalf("runs scanned = %d, want 15", verdict.RunsScanned)
	}
	if verdict.Verdict != "trail went cold after 15 runs." {
		t.Fatalf("verdict line = %q", verdict.Verdict)
	}
}

// A boundary on page 2 costs exactly 2 list pages + 1 compare — the
// ≤3-round-trip budget for first-two-page boundaries.
func TestLocateRegressionCrossesPagesWithinBudget(t *testing.T) {
	trail := &fakeTrail{
		pages: map[int][]model.Run{
			1: {red(110), red(109), red(108), red(107), red(106)},
			2: {red(105), green(104), green(103)},
		},
		rangeInfo: model.CommitRange{TotalCommits: 3, Commits: []model.Commit{{SHA: "fff"}}},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionLocated {
		t.Fatalf("status = %q, want located", verdict.Status)
	}
	if verdict.LastGood.RunNumber != 104 || verdict.FirstBad.RunNumber != 105 {
		t.Fatalf("boundary = #%d → #%d", verdict.LastGood.RunNumber, verdict.FirstBad.RunNumber)
	}
	if trail.listCalls != 2 || trail.compareCalls != 1 {
		t.Fatalf("round trips = %d list + %d compare, want 2 + 1", trail.listCalls, trail.compareCalls)
	}
}

func TestLocateRegressionHistoryExhaustedWithoutGreen(t *testing.T) {
	trail := &fakeTrail{
		pages: map[int][]model.Run{1: {red(104), red(103)}},
	}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionInconclusive {
		t.Fatalf("status = %q, want inconclusive", verdict.Status)
	}
	if trail.listCalls != 1 {
		t.Fatalf("list calls = %d; a short page means history ended", trail.listCalls)
	}
	if verdict.Verdict != "trail went cold: history ran out before a clean run." {
		t.Fatalf("verdict line = %q", verdict.Verdict)
	}
}

func TestLocateRegressionNoCompletedRunsIsInconclusive(t *testing.T) {
	trail := &fakeTrail{pages: map[int][]model.Run{}}
	verdict := locate(t, trail, 10)
	if verdict.Status != RegressionInconclusive {
		t.Fatalf("status = %q, want inconclusive", verdict.Status)
	}
	if verdict.Verdict != "nothing to read: no completed runs on this trail." {
		t.Fatalf("verdict line = %q", verdict.Verdict)
	}
}

func TestLocateRegressionCapsSuspectsAtFifty(t *testing.T) {
	commits := make([]model.Commit, 80)
	for i := range commits {
		commits[i] = model.Commit{SHA: fmt.Sprintf("c%03d", i)}
	}
	trail := &fakeTrail{
		pages:     map[int][]model.Run{1: {red(104), green(103)}},
		rangeInfo: model.CommitRange{TotalCommits: 80, Commits: commits},
	}
	verdict := locate(t, trail, 10)
	if len(verdict.SuspectCommits) != SuspectCommitCap {
		t.Fatalf("suspects rendered = %d, want %d", len(verdict.SuspectCommits), SuspectCommitCap)
	}
	if verdict.TotalSuspects != 80 {
		t.Fatalf("total suspects = %d, want the uncapped count", verdict.TotalSuspects)
	}
}

func TestLocateRegressionFallsBackToConstructedCompareURL(t *testing.T) {
	trail := &fakeTrail{
		pages:     map[int][]model.Run{1: {red(104), green(103)}},
		rangeInfo: model.CommitRange{TotalCommits: 1, Commits: []model.Commit{{SHA: "abc"}}},
	}
	verdict := locate(t, trail, 10)
	want := "https://github.com/indrasvat/gh-hound/compare/sha0103...sha0104"
	if verdict.CompareURL != want {
		t.Fatalf("compare url = %q, want %q", verdict.CompareURL, want)
	}
}

func TestLocateRegressionPropagatesListAndCompareErrors(t *testing.T) {
	listBroken := &fakeTrail{listErr: APIError{Kind: APIErrorRateLimit, Message: "rate limited"}}
	service := DiffService{History: listBroken, Compare: listBroken, MaxPages: 2, PerPage: 5}
	if _, err := service.LocateRegression(context.Background(), "o/r", "ci.yml", "main"); err == nil {
		t.Fatal("expected list error to propagate")
	}

	compareBroken := &fakeTrail{
		pages:      map[int][]model.Run{1: {red(104), green(103)}},
		compareErr: errors.New("boom"),
	}
	service = DiffService{History: compareBroken, Compare: compareBroken, MaxPages: 2, PerPage: 5}
	if _, err := service.LocateRegression(context.Background(), "o/r", "ci.yml", "main"); err == nil {
		t.Fatal("expected compare error to propagate")
	}
}

func TestLocateRegressionValidatesInputs(t *testing.T) {
	trail := &fakeTrail{}
	service := DiffService{History: trail, Compare: trail}
	if _, err := service.LocateRegression(context.Background(), "o/r", "", "main"); err == nil {
		t.Fatal("expected error for missing workflow")
	}
	if _, err := service.LocateRegression(context.Background(), "", "ci.yml", "main"); err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestLocateRegressionVerdictLineFormatsThousands(t *testing.T) {
	if got := coldTrailLine(1000); got != "trail went cold after 1,000 runs." {
		t.Fatalf("cold trail line = %q", got)
	}
}
