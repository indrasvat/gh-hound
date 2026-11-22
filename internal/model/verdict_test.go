package model

import "testing"

func ranJob(id int64, conclusion Conclusion) Job {
	return Job{ID: id, Name: "test", Status: StatusCompleted, Conclusion: conclusion, RunnerName: "GitHub Actions 7",
		Steps: []Step{{Name: "Run tests", Number: 2, Status: StatusCompleted, Conclusion: conclusion}}}
}

// Every reason a red run gets, decided from the shapes the 22 Nov spike
// found on live runs (docs/tasks/470-never-ran-verdicts.md).
func TestClassifyRunNamesWhyARedRunIsRed(t *testing.T) {
	pr := Run{Status: StatusCompleted, Event: "pull_request", FromFork: true}
	with := func(run Run, conclusion Conclusion, event string) Run {
		run.Conclusion = conclusion
		if event != "" {
			run.Event = event
		}
		return run
	}
	// The next.js blocked run listed a "job" with no steps and no runner.
	phantom := Job{ID: 9, Name: "Docs Link Validation", Status: StatusCompleted, Conclusion: ConclusionSuccess}
	annotated := func(message string) *RunEvidence {
		return &RunEvidence{Jobs: []Job{ranJob(1, ConclusionFailure)}, Annotations: map[int64][]Annotation{1: {{Level: "failure", Message: message}}}}
	}
	cases := []struct {
		name     string
		run      Run
		evidence *RunEvidence
		want     RunVerdict
		known    bool
	}{
		{"green run has no verdict", with(pr, ConclusionSuccess, ""), nil, RunVerdict{}, true},
		{"running run has no verdict", Run{Status: StatusInProgress}, nil, RunVerdict{}, true},
		{"held fork run waits for approval", with(pr, ConclusionActionRequired, ""), nil, RunVerdict{VerdictNeverRan, ReasonAwaitingApproval}, true},
		{"blocked pull_request_target", with(pr, ConclusionStartupFailure, "pull_request_target"), nil, RunVerdict{VerdictNeverRan, ReasonBlockedByPolicy}, true},
		{"other startup failure", with(pr, ConclusionStartupFailure, "push"), nil, RunVerdict{VerdictNeverRan, ReasonCouldNotStart}, true},
		{"failure needs its jobs", with(pr, ConclusionFailure, ""), nil, RunVerdict{}, false},
		{"PR run that ended before approval", with(pr, ConclusionFailure, ""), &RunEvidence{}, RunVerdict{VerdictNeverRan, ReasonApprovalMissed}, true},
		{"a phantom check run is no job", with(pr, ConclusionFailure, "pull_request_target"), &RunEvidence{Jobs: []Job{phantom}}, RunVerdict{VerdictNeverRan, ReasonApprovalMissed}, true},
		{"push run with no job", with(pr, ConclusionFailure, "push"), &RunEvidence{}, RunVerdict{VerdictNeverRan, ReasonNoJobRan}, true},
		// A job no runner took (no matching label) still failed: its
		// annotations say why, so the run is not "never ran".
		{"a failed job no runner took", with(pr, ConclusionFailure, "push"), &RunEvidence{Jobs: []Job{{ID: 3, Conclusion: ConclusionFailure}}}, RunVerdict{VerdictCode, ""}, true},
		{"a failed step is code", with(pr, ConclusionFailure, "push"), &RunEvidence{Jobs: []Job{ranJob(1, ConclusionFailure)}}, RunVerdict{VerdictCode, ""}, true},
		{"timed out stays code", with(pr, ConclusionTimedOut, "push"), &RunEvidence{Jobs: []Job{ranJob(1, ConclusionTimedOut)}}, RunVerdict{VerdictCode, ""}, true},
		{"lost runner", with(pr, ConclusionFailure, "push"), annotated("The self-hosted runner: build-7 lost communication with the server."), RunVerdict{VerdictInfra, ReasonRunnerLost}, true},
		{"runner shut down", with(pr, ConclusionFailure, "push"), annotated("The runner has received a shutdown signal. This can happen when the runner service is stopped"), RunVerdict{VerdictInfra, ReasonRunnerLost}, true},
		{"killed by exit 137", with(pr, ConclusionFailure, "push"), annotated("Process completed with exit code 137."), RunVerdict{VerdictInfra, ReasonKilled}, true},
		{"disk full", with(pr, ConclusionFailure, "push"), annotated("System.IO.IOException: No space left on device"), RunVerdict{VerdictInfra, ReasonDiskFull}, true},
		{"exit 137 from the log", with(pr, ConclusionFailure, "push"), &RunEvidence{Jobs: []Job{ranJob(1, ConclusionFailure)}, ExitCodes: map[int64]int{1: 137}}, RunVerdict{VerdictInfra, ReasonKilled}, true},
		{"exit code 1370 is no kill", with(pr, ConclusionFailure, "push"), annotated("Process completed with exit code 1370."), RunVerdict{VerdictCode, ""}, true},
		{"one code failure beside a lost runner is code", with(pr, ConclusionFailure, "push"), &RunEvidence{
			Jobs:        []Job{ranJob(1, ConclusionFailure), ranJob(2, ConclusionFailure)},
			Annotations: map[int64][]Annotation{1: {{Message: "The runner has received a shutdown signal."}}},
		}, RunVerdict{VerdictCode, ""}, true},
		{"a green job beside a lost runner stays infra", with(pr, ConclusionFailure, "push"), &RunEvidence{
			Jobs:        []Job{ranJob(1, ConclusionFailure), ranJob(2, ConclusionSuccess)},
			Annotations: map[int64][]Annotation{1: {{Message: "lost communication with the server"}}},
		}, RunVerdict{VerdictInfra, ReasonRunnerLost}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, known := ClassifyRun(tc.run, tc.evidence)
			if got != tc.want || known != tc.known {
				t.Fatalf("ClassifyRun = %+v known=%v, want %+v known=%v", got, known, tc.want, tc.known)
			}
		})
	}
}

// A held run waits on a person: it is the only verdict that is pending.
func TestOnlyAwaitingApprovalWaitsOnAPerson(t *testing.T) {
	for _, reason := range []VerdictReason{ReasonApprovalMissed, ReasonBlockedByPolicy, ReasonCouldNotStart, ReasonNoJobRan, ReasonRunnerLost, ReasonKilled, ReasonDiskFull} {
		if (RunVerdict{Verdict: VerdictNeverRan, Reason: reason}).WaitingOnPerson() {
			t.Fatalf("%s waits on a person", reason)
		}
	}
	if !(RunVerdict{Verdict: VerdictNeverRan, Reason: ReasonAwaitingApproval}).WaitingOnPerson() {
		t.Fatal("awaiting_approval does not wait on a person")
	}
}

func TestJobRanNeedsStepsOrARunner(t *testing.T) {
	if (Job{Conclusion: ConclusionFailure}).Ran() {
		t.Fatal("a job with no steps and no runner ran")
	}
	if !(Job{RunnerName: "r"}).Ran() || !(Job{Steps: []Step{{Name: "Set up job"}}}).Ran() {
		t.Fatal("a job with a runner or steps did not run")
	}
}
