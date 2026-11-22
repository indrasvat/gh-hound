package model

import (
	"regexp"
	"strings"
)

// Verdict says why a red run is red. GitHub's conclusion stays as it
// reports it; the verdict sits beside it. Empty means the run is not red.
type Verdict string

const (
	// VerdictCode is a job that ran and a step that failed.
	VerdictCode Verdict = "code"
	// VerdictNeverRan is a red run in which no job ran.
	VerdictNeverRan Verdict = "never_ran"
	// VerdictInfra is a job that ran but the runner or platform failed.
	VerdictInfra Verdict = "infra"
)

// VerdictReason names the cause behind a never_ran or infra verdict.
type VerdictReason string

const (
	ReasonAwaitingApproval VerdictReason = "awaiting_approval"
	ReasonApprovalMissed   VerdictReason = "approval_missed"
	ReasonBlockedByPolicy  VerdictReason = "blocked_by_policy"
	ReasonCouldNotStart    VerdictReason = "could_not_start"
	ReasonNoJobRan         VerdictReason = "no_job_ran"
	ReasonRunnerLost       VerdictReason = "runner_lost"
	ReasonKilled           VerdictReason = "killed"
	ReasonDiskFull         VerdictReason = "disk_full"
)

type RunVerdict struct {
	Verdict Verdict
	Reason  VerdictReason
}

// WaitingOnPerson reports a run held until someone approves it: the
// one red verdict that is pending rather than finished.
func (v RunVerdict) WaitingOnPerson() bool {
	return v.Reason == ReasonAwaitingApproval
}

// RunEvidence is what a run's jobs say about it: the jobs of its
// attempt, and for failing jobs their annotations and log exit codes.
type RunEvidence struct {
	Jobs        []Job
	Annotations map[int64][]Annotation
	ExitCodes   map[int64]int
}

// Ran reports a job that a runner picked up.
func (j Job) Ran() bool {
	return len(j.Steps) > 0 || j.RunnerName != ""
}

// NoJobRan reports a listing in which no job ran or failed. GitHub can
// list a check run created through the API among a run's jobs: it has
// neither steps nor a runner, and did not fail. A job that failed with
// no runner (no matching label) is real and its annotations say why.
func NoJobRan(jobs []Job) bool {
	for _, job := range jobs {
		if job.Ran() || job.Conclusion.Failing() {
			return false
		}
	}
	return true
}

// ClassifyRun gives a red run its verdict. evidence is nil when the
// run's jobs are not loaded; known is false when the verdict needs them.
func ClassifyRun(run Run, evidence *RunEvidence) (RunVerdict, bool) {
	if run.Status != StatusCompleted || !run.Conclusion.Failing() {
		return RunVerdict{}, true
	}
	switch run.Conclusion {
	case ConclusionActionRequired:
		return RunVerdict{VerdictNeverRan, ReasonAwaitingApproval}, true
	case ConclusionStartupFailure:
		if run.Event == "pull_request_target" {
			return RunVerdict{VerdictNeverRan, ReasonBlockedByPolicy}, true
		}
		return RunVerdict{VerdictNeverRan, ReasonCouldNotStart}, true
	}
	if evidence == nil {
		return RunVerdict{}, false
	}
	for _, job := range evidence.Jobs {
		if job.RunAttempt != 0 && run.RunAttempt != 0 && job.RunAttempt != run.RunAttempt {
			// The listing moved on to another attempt (a re-run began):
			// it says nothing about this one.
			return RunVerdict{}, false
		}
	}
	if !NoJobRan(evidence.Jobs) {
		infra := VerdictReason("")
		for _, job := range evidence.Jobs {
			if !job.Conclusion.Failing() {
				continue
			}
			reason := infraReason(evidence.Annotations[job.ID], evidence.ExitCodes[job.ID])
			if reason == "" {
				return RunVerdict{Verdict: VerdictCode}, true
			}
			if infra == "" {
				infra = reason
			}
		}
		if infra != "" {
			return RunVerdict{VerdictInfra, infra}, true
		}
		// Red with no failing job: nothing points away from the code.
		return RunVerdict{Verdict: VerdictCode}, true
	}
	if run.FromFork && (run.Event == "pull_request" || run.Event == "pull_request_target") {
		// GitHub ends a held fork run red when its pull request closes.
		return RunVerdict{VerdictNeverRan, ReasonApprovalMissed}, true
	}
	return RunVerdict{VerdictNeverRan, ReasonNoJobRan}, true
}

var exitCode137 = regexp.MustCompile(`exit code 137\b`)

// infraReason reads a failing job's platform signal, if it has one.
func infraReason(annotations []Annotation, exitCode int) VerdictReason {
	for _, annotation := range annotations {
		message := strings.ToLower(annotation.Message)
		switch {
		case strings.Contains(message, "lost communication with the server"),
			strings.Contains(message, "runner has received a shutdown signal"):
			return ReasonRunnerLost
		case annotation.Level == "failure" && strings.Contains(message, "no space left on device"):
			return ReasonDiskFull
		case exitCode137.MatchString(message):
			return ReasonKilled
		}
	}
	if exitCode == 137 {
		return ReasonKilled
	}
	return ""
}
