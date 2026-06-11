package usecase

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

// FlakeSignal names one kind of flake evidence.
type FlakeSignal string

const (
	// SignalAttemptFlip is the strongest signal: a job failed on
	// attempt N and succeeded on a later attempt of the SAME run.
	SignalAttemptFlip FlakeSignal = "attempt_flip"
	// SignalCrossRunFlap is the same commit (or adjacent commits)
	// alternating fail/pass across runs.
	SignalCrossRunFlap FlakeSignal = "cross_run_flap"
	// SignalRetryMask is a retry wrapper hiding instability inside a
	// "successful" step (nick-fields/retry, `Retrying in …` loops).
	SignalRetryMask FlakeSignal = "retry_mask"
)

// FlakeStatus is the verdict agents and the exit code branch on.
type FlakeStatus string

const (
	FlakeStatusFlaky   FlakeStatus = "flaky"
	FlakeStatusSuspect FlakeStatus = "suspect"
	FlakeStatusClean   FlakeStatus = "clean"
	// FlakeStatusInsufficient means ONLY that no non-clean verdict can
	// be supported because the sample is below FlakeMinSample — never
	// that evidence was discarded. Worst job verdict always wins: an
	// underfilled window with clear attempt flips is still flaky.
	FlakeStatusInsufficient FlakeStatus = "insufficient_data"
)

// Documented thresholds. Scores are per job over the window:
// 0.45 per attempt flip + 0.30 per cross-run flap + 0.20 per retry
// mask, capped at 1.0. A score at or past FlakeFlakyThreshold is
// flaky (two flips, two flaps, or one flip plus any second signal);
// any evidence at all is at least suspect.
const (
	DefaultFlakeWindow  = 50
	FlakeMinSample      = 5
	FlakeFlakyThreshold = 0.6

	flipWeight = 0.45
	flapWeight = 0.30
	maskWeight = 0.20
)

// AttemptJobs is the attempt-forensics capability (Task 210 plumbing):
// job conclusions for one attempt of a run.
type AttemptJobs interface {
	ListJobsForAttempt(ctx context.Context, repo string, runID int64, attempt int) ([]model.Job, error)
}

// JobLogs fetches one job's log. The flake scan spends log calls ONLY
// on jobs that flipped inside multi-attempt runs — never a blanket
// download across the window.
type JobLogs interface {
	FetchJobLog(ctx context.Context, repo string, jobID int64) (string, error)
}

// FlakeEvidence is one observed wobble: which run (and attempt), what
// kind of signal, and a human-readable detail line.
type FlakeEvidence struct {
	Run     model.Run
	Attempt int
	Kind    FlakeSignal
	Detail  string
}

// JobFlake scores one job name within the window.
type JobFlake struct {
	Job        string
	Score      float64
	Verdict    FlakeStatus
	Flips      int
	Flaps      int
	Masks      int
	FlakedRuns int
	Evidence   []FlakeEvidence
}

// FlakeReport is the scored verdict for one workflow+branch window.
//
// Evidence comes from attempt job conclusions and logs only. Check-run
// annotations are retrievable for the LATEST attempt only (community
// #103026), so they are deliberately unreachable from this service —
// the dependency surface (WorkflowRunHistory, AttemptJobs, JobLogs)
// pins that at compile time.
type FlakeReport struct {
	Repo             string
	Workflow         string
	Branch           string
	Status           FlakeStatus
	SampleSize       int
	Window           int
	RunsScanned      int
	SignalsEvaluated []string
	Jobs             []JobFlake
	Verdict          string
}

// WorstJob returns the highest-scoring job entry, if any.
func (r FlakeReport) WorstJob() (JobFlake, bool) {
	if len(r.Jobs) == 0 {
		return JobFlake{}, false
	}
	return r.Jobs[0], true
}

// FlakesService answers "real failure or flake?" by combining the
// Task 260 history walk with Task 210 attempt forensics.
type FlakesService struct {
	History WorkflowRunHistory
	// Attempts and Logs are optional capabilities; without them the
	// scan degrades to the metadata-only signals and says so in
	// SignalsEvaluated.
	Attempts AttemptJobs
	Logs     JobLogs
	Window   int
	PerPage  int
}

// Scan walks the workflow's recent history (newest first, bounded by
// Window) and scores every job that wobbled.
//
// API budget, pinned by tests: at most ceil(Window/PerPage) history
// list calls; attempt-jobs calls only for runs inside the window with
// run_attempt > 1; log calls only for jobs that flipped. Everything
// else is metadata already in hand.
func (s FlakesService) Scan(ctx context.Context, repo, workflow, branch string) (FlakeReport, error) {
	repo = strings.TrimSpace(repo)
	workflow = strings.TrimSpace(workflow)
	if repo == "" {
		return FlakeReport{}, fmt.Errorf("repo is required")
	}
	if workflow == "" {
		return FlakeReport{}, fmt.Errorf("workflow is required")
	}
	if s.History == nil {
		return FlakeReport{}, fmt.Errorf("flake scan is unavailable for this adapter")
	}

	window := s.Window
	if window <= 0 {
		window = DefaultFlakeWindow
	}
	perPage := s.PerPage
	if perPage <= 0 {
		perPage = DiffPerPage
	}
	maxPages := (window + perPage - 1) / perPage

	walker := newRunHistoryWalker(s.History, repo, workflow, branch, maxPages, perPage)
	runs := make([]model.Run, 0, window)
	for len(runs) < window {
		run, ok, err := walker.Next(ctx)
		if err != nil {
			return FlakeReport{}, err
		}
		if !ok {
			break
		}
		runs = append(runs, run)
	}

	report := FlakeReport{
		Repo:             repo,
		Workflow:         workflow,
		Branch:           branch,
		Window:           window,
		RunsScanned:      len(runs),
		SignalsEvaluated: s.signalsEvaluated(),
	}

	scorer := newFlakeScorer(workflow)

	// Signal 1 + 3: attempt flips and retry masks, paid for only on
	// runs that actually had more than one attempt.
	failedJobsByRun := map[int64][]string{}
	if s.Attempts != nil {
		for _, run := range runs {
			if run.RunAttempt <= 1 {
				continue
			}
			flips, failedJobs, err := s.attemptFlips(ctx, repo, run)
			if err != nil {
				return FlakeReport{}, err
			}
			failedJobsByRun[run.ID] = failedJobs
			for _, flip := range flips {
				scorer.addFlip(run, flip)
				if s.Logs == nil {
					continue
				}
				log, logErr := s.Logs.FetchJobLog(ctx, repo, flip.finalJobID)
				if logErr != nil {
					// The mask is auxiliary evidence; a missing log
					// must not sink the verdict the flip already made.
					continue
				}
				if mask, ok := MatchRetryMask(log); ok {
					scorer.addMask(run, flip.job, mask)
				}
			}
		}
	}

	// Signal 2: cross-run flapping over run metadata already in hand.
	signalRuns := filterSignalRuns(runs)
	report.SampleSize = len(signalRuns)
	scoreCrossRunFlaps(scorer, signalRuns, failedJobsByRun, repo)

	report.Jobs = scorer.finalize()
	report.Status = flakeStatus(report)
	report.Verdict = flakeVerdictLine(report)
	return report, nil
}

func (s FlakesService) signalsEvaluated() []string {
	signals := []string{"cross_run_flaps"}
	if s.Attempts != nil {
		signals = append([]string{"attempt_flips"}, signals...)
		if s.Logs != nil {
			signals = append(signals, "retry_masks")
		}
	}
	return signals
}

// jobFlip is one job that failed on an earlier attempt and succeeded
// later in the same run.
type jobFlip struct {
	job           string
	failedAttempt int
	finalJobID    int64
}

// attemptFlips reads job conclusions for every attempt of a rerun.
// Live-verified caveat: run_attempt > 1 does NOT imply an earlier
// attempt failed (reruns of green runs exist) — only an actual
// failed→succeeded conclusion pair counts.
func (s FlakesService) attemptFlips(ctx context.Context, repo string, run model.Run) ([]jobFlip, []string, error) {
	type jobTrack struct {
		firstFailed  int
		succeededAt  int
		successJobID int64
	}
	tracks := map[string]*jobTrack{}
	order := []string{}
	for attempt := 1; attempt <= run.RunAttempt; attempt++ {
		jobs, err := s.Attempts.ListJobsForAttempt(ctx, repo, run.ID, attempt)
		if err != nil {
			return nil, nil, err
		}
		for _, job := range jobs {
			name := strings.TrimSpace(job.Name)
			if name == "" {
				continue
			}
			track, ok := tracks[name]
			if !ok {
				track = &jobTrack{}
				tracks[name] = track
				order = append(order, name)
			}
			switch job.Conclusion {
			case model.ConclusionFailure, model.ConclusionTimedOut, model.ConclusionActionRequired:
				if track.firstFailed == 0 {
					track.firstFailed = attempt
				}
			case model.ConclusionSuccess:
				if track.firstFailed > 0 && attempt > track.firstFailed {
					track.succeededAt = attempt
					track.successJobID = job.ID
				}
			}
		}
	}
	flips := []jobFlip{}
	failed := []string{}
	for _, name := range order {
		track := tracks[name]
		if track.firstFailed > 0 {
			failed = append(failed, name)
		}
		if track.firstFailed > 0 && track.succeededAt > track.firstFailed {
			flips = append(flips, jobFlip{job: name, failedAttempt: track.firstFailed, finalJobID: track.successJobID})
		}
	}
	return flips, failed, nil
}

// filterSignalRuns keeps completed runs whose conclusion carries a
// green/red signal; cancelled, skipped, neutral, and stale runs say
// nothing about flakiness.
func filterSignalRuns(runs []model.Run) []model.Run {
	out := make([]model.Run, 0, len(runs))
	for _, run := range runs {
		if good, bad := regressionSignal(run); good || bad {
			out = append(out, run)
		}
	}
	return out
}

// scoreCrossRunFlaps finds same-sha fail/pass mixes first, then
// red→green→red alternations across adjacent commits. The commit
// range between alternating reds is SHOWN as evidence (a compare URL)
// — file relevance is never inferred.
func scoreCrossRunFlaps(scorer *flakeScorer, signalRuns []model.Run, failedJobsByRun map[int64][]string, repo string) {
	bySHA := map[string][]model.Run{}
	shaOrder := []string{}
	for _, run := range signalRuns {
		sha := strings.TrimSpace(run.HeadSHA)
		if sha == "" {
			continue
		}
		if _, ok := bySHA[sha]; !ok {
			shaOrder = append(shaOrder, sha)
		}
		bySHA[sha] = append(bySHA[sha], run)
	}
	flappedSHA := map[string]bool{}
	for _, sha := range shaOrder {
		group := bySHA[sha]
		var newestRed, newestGreen *model.Run
		for i := range group {
			good, bad := regressionSignal(group[i])
			if bad && newestRed == nil {
				newestRed = &group[i]
			}
			if good && newestGreen == nil {
				newestGreen = &group[i]
			}
		}
		if newestRed == nil || newestGreen == nil {
			continue
		}
		flappedSHA[sha] = true
		detail := fmt.Sprintf("#%d failed and #%d passed on %s", newestRed.RunNumber, newestGreen.RunNumber, shortFlakeSHA(sha))
		scorer.addFlap(*newestRed, failedJobsByRun[newestRed.ID], detail)
	}

	for i := 0; i+2 < len(signalRuns); i++ {
		newest, middle, oldest := signalRuns[i], signalRuns[i+1], signalRuns[i+2]
		if !redRun(newest) || !greenRunSignal(middle) || !redRun(oldest) {
			continue
		}
		if flappedSHA[newest.HeadSHA] || flappedSHA[middle.HeadSHA] || flappedSHA[oldest.HeadSHA] {
			continue
		}
		if newest.HeadSHA == middle.HeadSHA || middle.HeadSHA == oldest.HeadSHA || newest.HeadSHA == oldest.HeadSHA {
			continue
		}
		detail := fmt.Sprintf(
			"#%d failed → #%d passed → #%d failed; commits between: https://github.com/%s/compare/%s...%s",
			oldest.RunNumber, middle.RunNumber, newest.RunNumber, repo, oldest.HeadSHA, newest.HeadSHA,
		)
		scorer.addFlap(newest, failedJobsByRun[newest.ID], detail)
		i += 2
	}
}

func redRun(run model.Run) bool {
	_, bad := regressionSignal(run)
	return bad
}

func greenRunSignal(run model.Run) bool {
	good, _ := regressionSignal(run)
	return good
}

// flakeScorer aggregates evidence per job name. Flaps without job
// attribution (single-attempt runs carry no per-job conclusions under
// the budget) land on the workflow-level entry.
type flakeScorer struct {
	fallbackJob string
	jobs        map[string]*JobFlake
	order       []string
}

func newFlakeScorer(workflow string) *flakeScorer {
	return &flakeScorer{fallbackJob: workflow, jobs: map[string]*JobFlake{}}
}

func (s *flakeScorer) entry(job string) *JobFlake {
	job = strings.TrimSpace(job)
	if job == "" {
		job = s.fallbackJob
	}
	entry, ok := s.jobs[job]
	if !ok {
		entry = &JobFlake{Job: job}
		s.jobs[job] = entry
		s.order = append(s.order, job)
	}
	return entry
}

func (s *flakeScorer) addFlip(run model.Run, flip jobFlip) {
	entry := s.entry(flip.job)
	entry.Flips++
	entry.Evidence = append(entry.Evidence, FlakeEvidence{
		Run:     run,
		Attempt: flip.failedAttempt,
		Kind:    SignalAttemptFlip,
		Detail:  fmt.Sprintf("#%d: failed on attempt %d, passed on a later attempt", run.RunNumber, flip.failedAttempt),
	})
}

func (s *flakeScorer) addMask(run model.Run, job string, mask RetryMask) {
	entry := s.entry(job)
	entry.Masks++
	entry.Evidence = append(entry.Evidence, FlakeEvidence{
		Run:     run,
		Attempt: run.RunAttempt,
		Kind:    SignalRetryMask,
		Detail:  fmt.Sprintf("retry wrapper matched (%s): %s", mask.Pattern, mask.Excerpt),
	})
}

func (s *flakeScorer) addFlap(run model.Run, jobs []string, detail string) {
	if len(jobs) == 0 {
		jobs = []string{prettyRunName(run, s.fallbackJob)}
	}
	for _, job := range jobs {
		entry := s.entry(job)
		entry.Flaps++
		entry.Evidence = append(entry.Evidence, FlakeEvidence{
			Run:    run,
			Kind:   SignalCrossRunFlap,
			Detail: detail,
		})
	}
}

func prettyRunName(run model.Run, fallback string) string {
	if name := strings.TrimSpace(run.Name); name != "" {
		return name
	}
	return fallback
}

func (s *flakeScorer) finalize() []JobFlake {
	out := make([]JobFlake, 0, len(s.order))
	for _, name := range s.order {
		entry := s.jobs[name]
		entry.Score = flakeScore(entry.Flips, entry.Flaps, entry.Masks)
		entry.Verdict = jobVerdict(entry.Score)
		entry.FlakedRuns = distinctEvidenceRuns(entry.Evidence)
		out = append(out, *entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Job < out[j].Job
	})
	return out
}

func flakeScore(flips, flaps, masks int) float64 {
	score := float64(flips)*flipWeight + float64(flaps)*flapWeight + float64(masks)*maskWeight
	if score > 1 {
		score = 1
	}
	return math.Round(score*100) / 100
}

func jobVerdict(score float64) FlakeStatus {
	switch {
	case score >= FlakeFlakyThreshold:
		return FlakeStatusFlaky
	case score > 0:
		return FlakeStatusSuspect
	default:
		return FlakeStatusClean
	}
}

func distinctEvidenceRuns(evidence []FlakeEvidence) int {
	seen := map[int64]bool{}
	for _, item := range evidence {
		seen[item.Run.ID] = true
	}
	return len(seen)
}

// flakeStatus resolves the top-level verdict: the worst job verdict
// always wins; insufficient_data applies only when the window is both
// underfilled AND free of evidence.
func flakeStatus(report FlakeReport) FlakeStatus {
	worst := FlakeStatusClean
	for _, job := range report.Jobs {
		if job.Verdict == FlakeStatusFlaky {
			return FlakeStatusFlaky
		}
		if job.Verdict == FlakeStatusSuspect {
			worst = FlakeStatusSuspect
		}
	}
	if worst == FlakeStatusClean && report.SampleSize < FlakeMinSample {
		return FlakeStatusInsufficient
	}
	return worst
}

// flakeVerdictLine voices the verdict: a flake is a squirrel — chasing
// it is optional; a clean trail is a fresh scent worth chasing.
func flakeVerdictLine(report FlakeReport) string {
	switch report.Status {
	case FlakeStatusFlaky:
		job, _ := report.WorstJob()
		return fmt.Sprintf("seen this one before: it's a squirrel — %s flaked %d of the last %d runs.", job.Job, job.FlakedRuns, report.RunsScanned)
	case FlakeStatusSuspect:
		job, _ := report.WorstJob()
		return fmt.Sprintf("something rustled: %s wobbled %d of the last %d runs — watch it.", job.Job, job.FlakedRuns, report.RunsScanned)
	case FlakeStatusInsufficient:
		noun := "runs"
		if report.SampleSize == 1 {
			noun = "run"
		}
		return fmt.Sprintf("not enough trail to read: only %d completed %s on record.", report.SampleSize, noun)
	default:
		return fmt.Sprintf("fresh scent — worth chasing: no flake history in the last %d runs.", report.RunsScanned)
	}
}

func shortFlakeSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// RetryMask is one matched retry-wrapper pattern in a job log.
type RetryMask struct {
	Pattern string
	Line    int
	Excerpt string
}

// retryMaskPatterns is the data-driven matcher table: adding a known
// retry wrapper is a one-line change.
var retryMaskPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"nick-fields/retry", regexp.MustCompile(`(?i)nick-fields/retry`)},
	{"retrying in", regexp.MustCompile(`(?i)retrying in \d+`)},
	{"attempt counter", regexp.MustCompile(`(?i)attempt \d+\s*(?:of\s+|/)\s*\d+ failed`)},
}

// MatchRetryMask scans a log for known retry-wrapper traces. It is
// shared by the windowed scan (logs of flipped jobs only) and the
// failure screen (the log already in hand).
func MatchRetryMask(log string) (RetryMask, bool) {
	if strings.TrimSpace(log) == "" {
		return RetryMask{}, false
	}
	for i, line := range strings.Split(log, "\n") {
		for _, pattern := range retryMaskPatterns {
			if pattern.re.MatchString(line) {
				return RetryMask{Pattern: pattern.name, Line: i + 1, Excerpt: strings.TrimSpace(line)}, true
			}
		}
	}
	return RetryMask{}, false
}
