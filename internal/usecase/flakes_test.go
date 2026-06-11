package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

// flakeFake seeds a paginated workflow history plus attempt-level job
// conclusions and job logs, counting every call so the API budget is
// pinned. It deliberately implements ONLY the narrow capabilities the
// flake scan is allowed to touch: run history, attempt jobs, and job
// logs. Annotations are not reachable — the API only serves them for
// the latest attempt (community #103026), so flake evidence must come
// from attempt conclusions and logs, never annotations. That caveat is
// enforced at compile time by FlakesService's dependency surface.
type flakeFake struct {
	pages        map[int][]model.Run
	attemptJobs  map[string][]model.Job
	logs         map[int64]string
	listErr      error
	attemptErr   error
	listCalls    int
	attemptCalls int
	logCalls     int
}

func (f *flakeFake) ListWorkflowRuns(_ context.Context, _ string, _ string, filter RunFilter) ([]model.Run, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.pages[filter.Page], nil
}

func (f *flakeFake) ListJobsForAttempt(_ context.Context, _ string, runID int64, attempt int) ([]model.Job, error) {
	f.attemptCalls++
	if f.attemptErr != nil {
		return nil, f.attemptErr
	}
	return f.attemptJobs[fmt.Sprintf("%d/%d", runID, attempt)], nil
}

func (f *flakeFake) FetchJobLog(_ context.Context, _ string, jobID int64) (string, error) {
	f.logCalls++
	return f.logs[jobID], nil
}

func flakeRun(number, attempt int, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:         int64(2000 + number),
		Name:       "CI",
		Status:     model.StatusCompleted,
		Conclusion: conclusion,
		RunNumber:  number,
		RunAttempt: attempt,
		HeadBranch: "main",
		HeadSHA:    fmt.Sprintf("sha%04d", number),
	}
}

func flakeJob(id int64, name string, conclusion model.Conclusion) model.Job {
	return model.Job{ID: id, Name: name, Status: model.StatusCompleted, Conclusion: conclusion}
}

// seedFlip wires one run as a genuine attempt flip for the given job:
// failed on attempt 1, succeeded on attempt 2.
func seedFlip(fake *flakeFake, run model.Run, jobName string, jobID int64) {
	fake.attemptJobs[fmt.Sprintf("%d/1", run.ID)] = []model.Job{flakeJob(jobID, jobName, model.ConclusionFailure)}
	fake.attemptJobs[fmt.Sprintf("%d/2", run.ID)] = []model.Job{flakeJob(jobID+1, jobName, model.ConclusionSuccess)}
}

func newFlakeFake(runs ...model.Run) *flakeFake {
	return &flakeFake{
		pages:       map[int][]model.Run{1: runs},
		attemptJobs: map[string][]model.Job{},
		logs:        map[int64]string{},
	}
}

func scanFlakes(t *testing.T, fake *flakeFake, window int) FlakeReport {
	t.Helper()
	service := FlakesService{History: fake, Attempts: fake, Logs: fake, Window: window, PerPage: 100}
	report, err := service.Scan(context.Background(), "indrasvat/gh-hound", "ci.yml", "main")
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	return report
}

func TestFlakeScanSingleAttemptFlipIsSuspect(t *testing.T) {
	flip := flakeRun(105, 2, model.ConclusionSuccess)
	fake := newFlakeFake(flip,
		flakeRun(104, 1, model.ConclusionSuccess),
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionSuccess),
	)
	seedFlip(fake, flip, "build", 9100)

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusSuspect {
		t.Fatalf("status = %q, want suspect", report.Status)
	}
	if len(report.Jobs) != 1 || report.Jobs[0].Job != "build" {
		t.Fatalf("jobs = %+v, want one entry for build", report.Jobs)
	}
	job := report.Jobs[0]
	if job.Flips != 1 || job.Verdict != FlakeStatusSuspect {
		t.Fatalf("job = %+v, want 1 flip and suspect verdict", job)
	}
	if job.Score != 0.45 {
		t.Fatalf("score = %v, want 0.45", job.Score)
	}
	if len(job.Evidence) != 1 || job.Evidence[0].Kind != SignalAttemptFlip {
		t.Fatalf("evidence = %+v, want one attempt_flip", job.Evidence)
	}
	if job.Evidence[0].Run.ID != flip.ID || job.Evidence[0].Attempt != 1 {
		t.Fatalf("evidence run/attempt = %d/%d, want %d/1", job.Evidence[0].Run.ID, job.Evidence[0].Attempt, flip.ID)
	}
}

func TestFlakeScanRepeatedAttemptFlipsAreFlaky(t *testing.T) {
	flipA := flakeRun(110, 2, model.ConclusionSuccess)
	flipB := flakeRun(106, 3, model.ConclusionSuccess)
	fake := newFlakeFake(flipA,
		flakeRun(109, 1, model.ConclusionSuccess),
		flakeRun(108, 1, model.ConclusionSuccess),
		flakeRun(107, 1, model.ConclusionSuccess),
		flipB,
	)
	seedFlip(fake, flipA, "build", 9200)
	fake.attemptJobs[fmt.Sprintf("%d/1", flipB.ID)] = []model.Job{flakeJob(9300, "build", model.ConclusionFailure)}
	fake.attemptJobs[fmt.Sprintf("%d/2", flipB.ID)] = []model.Job{flakeJob(9301, "build", model.ConclusionFailure)}
	fake.attemptJobs[fmt.Sprintf("%d/3", flipB.ID)] = []model.Job{flakeJob(9302, "build", model.ConclusionSuccess)}

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusFlaky {
		t.Fatalf("status = %q, want flaky", report.Status)
	}
	job := report.Jobs[0]
	if job.Flips != 2 || job.Score != 0.9 || job.Verdict != FlakeStatusFlaky {
		t.Fatalf("job = %+v, want 2 flips, score 0.9, flaky", job)
	}
	if job.FlakedRuns != 2 {
		t.Fatalf("flaked runs = %d, want 2", job.FlakedRuns)
	}
	if !strings.Contains(report.Verdict, "squirrel") {
		t.Fatalf("verdict %q must carry the squirrel call", report.Verdict)
	}
	if !strings.Contains(report.Verdict, "build") {
		t.Fatalf("verdict %q must name the flaky job", report.Verdict)
	}
}

// Live-verified caveat (run 27317503616): run_attempt > 1 does NOT
// imply an earlier attempt failed — reruns of green runs exist. Only
// an actual failed→succeeded job conclusion is a flip.
func TestFlakeScanRerunOfGreenIsNotAFlip(t *testing.T) {
	rerun := flakeRun(105, 2, model.ConclusionSuccess)
	fake := newFlakeFake(rerun,
		flakeRun(104, 1, model.ConclusionSuccess),
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionSuccess),
	)
	fake.attemptJobs[fmt.Sprintf("%d/1", rerun.ID)] = []model.Job{flakeJob(9400, "build", model.ConclusionSuccess)}
	fake.attemptJobs[fmt.Sprintf("%d/2", rerun.ID)] = []model.Job{flakeJob(9401, "build", model.ConclusionSuccess)}

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusClean {
		t.Fatalf("status = %q, want clean (rerun of green is not a flip)", report.Status)
	}
	if len(report.Jobs) != 0 {
		t.Fatalf("jobs = %+v, want none", report.Jobs)
	}
}

func TestFlakeScanCrossRunFlapSameSHA(t *testing.T) {
	redRun := flakeRun(105, 1, model.ConclusionFailure)
	greenRerun := flakeRun(106, 1, model.ConclusionSuccess)
	greenRerun.HeadSHA = redRun.HeadSHA
	fake := newFlakeFake(greenRerun, redRun,
		flakeRun(104, 1, model.ConclusionSuccess),
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
	)

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusSuspect {
		t.Fatalf("status = %q, want suspect", report.Status)
	}
	job := report.Jobs[0]
	if job.Flaps != 1 || job.Score != 0.3 {
		t.Fatalf("job = %+v, want 1 flap at score 0.3", job)
	}
	if job.Job != "CI" {
		t.Fatalf("flap without attempt data must land on the workflow entry, got %q", job.Job)
	}
	if len(job.Evidence) != 1 || job.Evidence[0].Kind != SignalCrossRunFlap {
		t.Fatalf("evidence = %+v, want one cross_run_flap", job.Evidence)
	}
	if !strings.Contains(job.Evidence[0].Detail, "#105") || !strings.Contains(job.Evidence[0].Detail, "#106") {
		t.Fatalf("flap detail %q must name both runs", job.Evidence[0].Detail)
	}
}

func TestFlakeScanTwoFlapsHitTheFlakyThreshold(t *testing.T) {
	redA := flakeRun(108, 1, model.ConclusionFailure)
	greenA := flakeRun(109, 1, model.ConclusionSuccess)
	greenA.HeadSHA = redA.HeadSHA
	redB := flakeRun(105, 1, model.ConclusionFailure)
	greenB := flakeRun(106, 1, model.ConclusionSuccess)
	greenB.HeadSHA = redB.HeadSHA
	fake := newFlakeFake(greenA, redA, flakeRun(107, 1, model.ConclusionSuccess), greenB, redB)

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusFlaky {
		t.Fatalf("status = %q, want flaky at the 0.6 boundary", report.Status)
	}
	job := report.Jobs[0]
	if job.Flaps != 2 || job.Score != 0.6 {
		t.Fatalf("job = %+v, want 2 flaps at score 0.6", job)
	}
}

// Adjacent-commit alternation (red → green → red across consecutive
// signal runs on different commits) is the weaker flap form; the
// commit range between the two red runs is SHOWN as evidence, never
// interpreted (no job-to-path inference).
func TestFlakeScanAdjacentAlternationShowsCompareRange(t *testing.T) {
	newestRed := flakeRun(105, 1, model.ConclusionFailure)
	middleGreen := flakeRun(104, 1, model.ConclusionSuccess)
	olderRed := flakeRun(103, 1, model.ConclusionFailure)
	fake := newFlakeFake(newestRed, middleGreen, olderRed,
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionSuccess),
	)

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusSuspect {
		t.Fatalf("status = %q, want suspect", report.Status)
	}
	job := report.Jobs[0]
	if job.Flaps != 1 {
		t.Fatalf("job = %+v, want 1 alternation flap", job)
	}
	detail := job.Evidence[0].Detail
	wantRange := "https://github.com/indrasvat/gh-hound/compare/sha0103...sha0105"
	if !strings.Contains(detail, wantRange) {
		t.Fatalf("alternation detail %q must show the commit range %q", detail, wantRange)
	}
}

func TestFlakeScanRetryMaskRaisesTheScore(t *testing.T) {
	flip := flakeRun(105, 2, model.ConclusionSuccess)
	fake := newFlakeFake(flip,
		flakeRun(104, 1, model.ConclusionSuccess),
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionSuccess),
	)
	seedFlip(fake, flip, "build", 9500)
	fake.logs[9501] = "##[group]Run nick-fields/retry@v3\nAttempt 1 failed. Retrying in 5 seconds...\nok"

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusFlaky {
		t.Fatalf("status = %q, want flaky (flip + mask = 0.65)", report.Status)
	}
	job := report.Jobs[0]
	if job.Masks != 1 || job.Score != 0.65 {
		t.Fatalf("job = %+v, want 1 mask at score 0.65", job)
	}
	var mask *FlakeEvidence
	for i := range job.Evidence {
		if job.Evidence[i].Kind == SignalRetryMask {
			mask = &job.Evidence[i]
		}
	}
	if mask == nil {
		t.Fatalf("evidence = %+v, want a retry_mask entry", job.Evidence)
	}
	if !strings.Contains(mask.Detail, "nick-fields/retry") {
		t.Fatalf("mask detail %q must name the matched wrapper", mask.Detail)
	}
}

func TestFlakeScanCleanWindowIsFreshScent(t *testing.T) {
	fake := newFlakeFake(
		flakeRun(105, 1, model.ConclusionSuccess),
		flakeRun(104, 1, model.ConclusionFailure),
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionSuccess),
	)
	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusClean {
		t.Fatalf("status = %q, want clean", report.Status)
	}
	if !strings.Contains(report.Verdict, "fresh scent") {
		t.Fatalf("verdict %q must read fresh scent", report.Verdict)
	}
	if report.SampleSize != 5 {
		t.Fatalf("sample size = %d, want 5", report.SampleSize)
	}
}

func TestFlakeScanThinHistoryIsInsufficientData(t *testing.T) {
	fake := newFlakeFake(
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionCancelled),
	)
	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusInsufficient {
		t.Fatalf("status = %q, want insufficient_data", report.Status)
	}
	if report.SampleSize != 2 {
		t.Fatalf("sample size = %d, want 2 (cancelled runs carry no signal)", report.SampleSize)
	}
	if !strings.Contains(report.Verdict, "trail") {
		t.Fatalf("verdict %q must say the trail is thin", report.Verdict)
	}
}

// Pinned by the spec: insufficient_data means ONLY that no non-clean
// verdict can be supported — evidence is never discarded. A two-run
// window with two attempt flips is flaky, not insufficient.
func TestFlakeScanUnderfilledButFlakyStaysFlaky(t *testing.T) {
	flipA := flakeRun(102, 2, model.ConclusionSuccess)
	flipB := flakeRun(101, 2, model.ConclusionSuccess)
	fake := newFlakeFake(flipA, flipB)
	seedFlip(fake, flipA, "build", 9600)
	seedFlip(fake, flipB, "build", 9700)

	report := scanFlakes(t, fake, 50)
	if report.Status != FlakeStatusFlaky {
		t.Fatalf("status = %q, want flaky despite the underfilled window", report.Status)
	}
	if report.SampleSize >= FlakeMinSample {
		t.Fatalf("sample size = %d, test wants an underfilled window", report.SampleSize)
	}
}

func TestFlakeScanScoreCapsAtOne(t *testing.T) {
	runs := make([]model.Run, 0, 6)
	fake := newFlakeFake()
	for number := 106; number >= 101; number-- {
		run := flakeRun(number, 2, model.ConclusionSuccess)
		runs = append(runs, run)
		seedFlip(fake, run, "build", int64(number*100))
	}
	fake.pages[1] = runs

	report := scanFlakes(t, fake, 50)
	if report.Jobs[0].Score != 1 {
		t.Fatalf("score = %v, want capped at 1", report.Jobs[0].Score)
	}
}

// The API budget, pinned: list calls stay within ceil(window/per_page),
// attempt calls are spent ONLY on runs inside the window that had more
// than one attempt, and log calls only on jobs that flipped. A run
// beyond the window must cost nothing even if it has attempts.
func TestFlakeScanPinsTheCallBudget(t *testing.T) {
	page1 := make([]model.Run, 0, 25)
	for number := 150; number > 125; number-- {
		page1 = append(page1, flakeRun(number, 1, model.ConclusionSuccess))
	}
	page2 := make([]model.Run, 0, 25)
	for number := 125; number > 100; number-- {
		page2 = append(page2, flakeRun(number, 1, model.ConclusionSuccess))
	}
	fake := &flakeFake{
		pages:       map[int][]model.Run{1: page1, 2: page2},
		attemptJobs: map[string][]model.Job{},
		logs:        map[int64]string{},
	}
	// One flip inside the window.
	flip := flakeRun(140, 2, model.ConclusionSuccess)
	page1[10] = flip
	seedFlip(fake, flip, "build", 9800)
	// A multi-attempt run BEYOND the window: page 3 must never load.
	fake.pages[3] = []model.Run{flakeRun(99, 4, model.ConclusionSuccess)}

	service := FlakesService{History: fake, Attempts: fake, Logs: fake, Window: 50, PerPage: 25}
	report, err := service.Scan(context.Background(), "indrasvat/gh-hound", "ci.yml", "main")
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if report.RunsScanned != 50 {
		t.Fatalf("runs scanned = %d, want exactly the window", report.RunsScanned)
	}
	if fake.listCalls != 2 {
		t.Fatalf("list calls = %d, want 2 (ceil(50/25))", fake.listCalls)
	}
	if fake.attemptCalls != 2 {
		t.Fatalf("attempt calls = %d, want 2 (one multi-attempt run, two attempts)", fake.attemptCalls)
	}
	if fake.logCalls != 1 {
		t.Fatalf("log calls = %d, want 1 (one flipped job)", fake.logCalls)
	}
}

// The default path is metadata-only: a window with no multi-attempt
// runs spends zero attempt calls and zero log calls.
func TestFlakeScanMetadataOnlyWhenNoAttempts(t *testing.T) {
	fake := newFlakeFake(
		flakeRun(105, 1, model.ConclusionSuccess),
		flakeRun(104, 1, model.ConclusionFailure),
		flakeRun(103, 1, model.ConclusionSuccess),
		flakeRun(102, 1, model.ConclusionSuccess),
		flakeRun(101, 1, model.ConclusionSuccess),
	)
	_ = scanFlakes(t, fake, 50)
	if fake.attemptCalls != 0 || fake.logCalls != 0 {
		t.Fatalf("attempt/log calls = %d/%d, want 0/0 on the metadata-only path", fake.attemptCalls, fake.logCalls)
	}
}

func TestFlakeScanHistoryErrorPropagates(t *testing.T) {
	fake := newFlakeFake()
	fake.listErr = errors.New("boom")
	service := FlakesService{History: fake, Attempts: fake, Logs: fake}
	if _, err := service.Scan(context.Background(), "o/r", "ci.yml", "main"); err == nil {
		t.Fatal("Scan must propagate history errors")
	}
}

func TestFlakeScanValidatesInputs(t *testing.T) {
	fake := newFlakeFake()
	service := FlakesService{History: fake, Attempts: fake, Logs: fake}
	if _, err := service.Scan(context.Background(), "", "ci.yml", "main"); err == nil {
		t.Fatal("Scan must require a repo")
	}
	if _, err := service.Scan(context.Background(), "o/r", "", "main"); err == nil {
		t.Fatal("Scan must require a workflow")
	}
	if _, err := (FlakesService{}).Scan(context.Background(), "o/r", "ci.yml", "main"); err == nil {
		t.Fatal("Scan must refuse when the adapter lacks history")
	}
}

// The matcher table is data-driven: one row per known retry wrapper.
func TestRetryMaskMatcherTable(t *testing.T) {
	cases := []struct {
		name    string
		log     string
		pattern string
		ok      bool
	}{
		{"nick-fields retry action", "Run nick-fields/retry@v3\nok", "nick-fields/retry", true},
		{"retrying-in countdown", "error: connect timeout\nRetrying in 9 seconds...", "retrying in", true},
		{"attempt counter", "Attempt 2 of 5 failed, waiting before retry", "attempt counter", true},
		{"attempt slash counter", "attempt 3/5 failed", "attempt counter", true},
		{"plain failure", "FAIL TestLexIdent", "", false},
		{"empty log", "", "", false},
		{"retry word alone is not a wrapper", "do not retry this manually", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match, ok := MatchRetryMask(tc.log)
			if ok != tc.ok {
				t.Fatalf("MatchRetryMask(%q) ok = %v, want %v", tc.log, ok, tc.ok)
			}
			if ok && match.Pattern != tc.pattern {
				t.Fatalf("pattern = %q, want %q", match.Pattern, tc.pattern)
			}
		})
	}
}
