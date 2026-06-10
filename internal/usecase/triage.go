package usecase

import (
	"context"
	"regexp"
	"strconv"
	"strings"

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
}

const (
	// fallbackExitCode is reported when the job failed but the log is
	// unavailable or carries no "exit code N" marker.
	fallbackExitCode = 1
	excerptTailLines = 8
)

var exitCodeRE = regexp.MustCompile(`exit code ([0-9]+)`)

// LoadRunFailures returns one RunFailure per failed job of an actionable
// run. Log and annotation lookups degrade per job: an expired log or a
// failed annotation listing never drops the job from the payload.
func (s TriageService) LoadRunFailures(ctx context.Context, repo string, run model.Run) ([]RunFailure, error) {
	return s.LoadRunFailuresAttempt(ctx, repo, run, 0)
}

// LoadRunFailuresAttempt targets a specific run attempt; attempt 0
// means the latest.
func (s TriageService) LoadRunFailuresAttempt(ctx context.Context, repo string, run model.Run, attempt int) ([]RunFailure, error) {
	if !actionableConclusion(run.Conclusion) {
		return nil, nil
	}
	var jobs []model.Job
	var err error
	if attempt > 0 {
		jobs, err = s.GitHub.ListJobsForAttempt(ctx, repo, run.ID, attempt)
	} else {
		jobs, err = s.GitHub.ListJobs(ctx, repo, run.ID)
	}
	if err != nil {
		return nil, err
	}
	var failures []RunFailure
	for _, job := range jobs {
		if !actionableConclusion(job.Conclusion) {
			continue
		}
		failure := RunFailure{Job: job, Step: firstFailedStep(job), ExitCode: fallbackExitCode}
		if raw, logErr := s.GitHub.FetchJobLog(ctx, repo, job.ID); logErr == nil {
			document := logs.Parse(raw)
			failure.LogExcerpt = excerptFor(document)
			if code, ok := exitCodeFrom(document); ok {
				failure.ExitCode = code
			}
		}
		if annotations, annErr := s.GitHub.ListAnnotations(ctx, repo, job); annErr == nil {
			failure.Annotations = annotations
		}
		failures = append(failures, failure)
	}
	return failures, nil
}

func actionableConclusion(conclusion model.Conclusion) bool {
	switch conclusion {
	case model.ConclusionFailure, model.ConclusionTimedOut, model.ConclusionActionRequired:
		return true
	default:
		return false
	}
}

func firstFailedStep(job model.Job) model.Step {
	for _, step := range job.Steps {
		if actionableConclusion(step.Conclusion) {
			return step
		}
	}
	return model.Step{}
}

// excerptFor prefers the parser's failure window and falls back to the
// log tail. Both come from the parsed document, so masking and
// timestamp stripping already applied.
func excerptFor(document logs.Document) string {
	lines := document.Failure.Lines
	if !document.Failure.Found {
		all := document.Lines
		start := max(0, len(all)-excerptTailLines)
		lines = all[start:]
	}
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, logs.StripTimestamp(line.Text))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
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
