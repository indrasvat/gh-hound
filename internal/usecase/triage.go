package usecase

import (
	"context"
	"regexp"
	"strconv"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

// TriageService assembles the failed[] payload of the structured pipe
// surface. It is the batch sibling of FailureService, which powers the
// interactive failure screen.
type TriageService struct {
	GitHub GitHub
}

// RunFailure carries the triage data for one failed job of a run.
type RunFailure struct {
	Job         model.Job
	Step        model.Step
	ExitCode    int
	Annotations []model.Annotation
	LogExcerpt  string
	Repeats     []Repeat
}

const (
	// fallbackExitCode is reported when the job failed but the log is
	// unavailable or carries no "exit code N" marker.
	fallbackExitCode = 1
)

var exitCodeRE = regexp.MustCompile(`exit code ([0-9]+)`)

// RunTriage is a red run's failed[] payload and its verdict, both built
// from one jobs listing.
type RunTriage struct {
	Failures []RunFailure
	Verdict  model.RunVerdict
}

// LoadRunTriage returns one RunFailure per failed job of an actionable
// run, and the run's verdict. Log and annotation lookups degrade per
// job: an expired log or a failed annotation listing never drops the
// job from the payload. attempt 0 means the latest.
func (s TriageService) LoadRunTriage(ctx context.Context, repo string, run model.Run, attempt int) (RunTriage, error) {
	if !run.Conclusion.Failing() {
		return RunTriage{}, nil
	}
	var jobs []model.Job
	var err error
	if attempt > 0 {
		jobs, err = s.GitHub.ListJobsForAttempt(ctx, repo, run.ID, attempt)
	} else {
		jobs, err = s.GitHub.ListJobs(ctx, repo, run.ID)
	}
	if err != nil {
		return RunTriage{}, err
	}
	evidence := &model.RunEvidence{Jobs: jobs, Annotations: map[int64][]model.Annotation{}, ExitCodes: map[int64]int{}}
	signalsComplete := true
	var triage RunTriage
	for _, job := range jobs {
		if !job.Conclusion.Failing() {
			continue
		}
		failure := RunFailure{Job: job, Step: firstFailedStep(job), ExitCode: fallbackExitCode}
		if raw, logErr := s.GitHub.FetchJobLog(ctx, repo, job.ID); logErr == nil {
			document := logs.Parse(raw)
			failure.LogExcerpt, failure.Repeats = excerpt(ctx, document)
			if code, ok := exitCodeFrom(document); ok {
				failure.ExitCode = code
				evidence.ExitCodes[job.ID] = code
			}
		}
		if annotations, annErr := s.GitHub.ListAnnotations(ctx, repo, job); annErr == nil {
			failure.Annotations = annotations
			evidence.Annotations[job.ID] = annotations
		} else {
			signalsComplete = false
		}
		triage.Failures = append(triage.Failures, failure)
	}
	triage.Verdict, _ = model.ClassifyRun(run, evidence)
	if triage.Verdict.Verdict == model.VerdictCode && !signalsComplete {
		// A missing annotation listing may hide a platform signal: no
		// verdict beats a wrong one.
		triage.Verdict = model.RunVerdict{}
	}
	return triage, nil
}

func firstFailedStep(job model.Job) model.Step {
	for _, step := range job.Steps {
		if step.Conclusion.Failing() {
			return step
		}
	}
	return model.Step{}
}

func exitCodeFrom(document logs.Document) (int, bool) {
	for i := len(document.Lines) - 1; i >= 0; i-- {
		match := exitCodeRE.FindStringSubmatch(document.Lines[i].Text)
		if len(match) != 2 {
			continue
		}
		if code, err := strconv.Atoi(match[1]); err == nil {
			return code, true
		}
	}
	return 0, false
}
