package fake

import (
	"strconv"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// ScenarioNeverRan is a fork-heavy repo's Monday morning: red runs for
// every reason a run never ran or failed on the platform, beside one
// code failure and one green run. Approving a held run starts it.
const ScenarioNeverRan Scenario = "never_ran"

const (
	neverRanHeldSHA = "a1f93c0"
	neverRanBase    = int64(30439400)
)

// neverRanRun is one run of the scenario, numbered from its run number.
func (a *Adapter) neverRanRun(number int, workflow, title, event string, conclusion model.Conclusion, age time.Duration, took time.Duration) model.Run {
	run := greenRun(number)
	run.ID = neverRanBase + int64(number)
	run.Name, run.DisplayTitle, run.Event, run.Conclusion = workflow, title, event, conclusion
	run.HeadBranch, run.HeadSHA, run.Actor, run.TriggeringActor = "main", "9e8d7c6", "maintainer", "maintainer"
	run.CreatedAt = a.started.Add(-age)
	run.RunStartedAt = run.CreatedAt
	run.UpdatedAt = run.CreatedAt.Add(took)
	run.HTMLURL = "https://github.com/indrasvat/gh-hound/actions/runs/" + strconv.FormatInt(run.ID, 10)
	run.PullRequests = nil
	return run
}

func (a *Adapter) neverRanRuns() []model.Run {
	fork := func(run model.Run, branch, sha, login string) model.Run {
		run.FromFork, run.HeadBranch, run.HeadSHA, run.Actor, run.TriggeringActor = true, branch, sha, login, login
		return run
	}
	runs := []model.Run{
		fork(a.neverRanRun(9412, "Unit tests", "fix typo in docs", "pull_request", model.ConclusionActionRequired, 2*time.Hour, 0), "patch-1", neverRanHeldSHA, "new-dev"),
		fork(a.neverRanRun(9411, "Lint", "fix typo in docs", "pull_request", model.ConclusionActionRequired, 2*time.Hour, 0), "patch-1", neverRanHeldSHA, "new-dev"),
		fork(a.neverRanRun(9408, "Unit tests", "add arm64 build", "pull_request", model.ConclusionFailure, 9*time.Hour, 4*time.Hour+14*time.Minute), "arm64", "7c2d4e1", "drive-by"),
		fork(a.neverRanRun(9407, "Code scanning", "add arm64 build", "pull_request", model.ConclusionFailure, 9*time.Hour, 4*time.Hour+14*time.Minute), "arm64", "7c2d4e1", "drive-by"),
		a.neverRanRun(9405, "Unit tests", "cache config parse", "push", model.ConclusionFailure, 11*time.Hour, 3*time.Minute+41*time.Second),
		fork(a.neverRanRun(9401, "Labeler", "triage new issues", "pull_request_target", model.ConclusionStartupFailure, 26*time.Hour, 0), "labels", "3b5a9f2", "drive-by"),
		a.neverRanRun(9399, "Docs", "readme badge", "push", model.ConclusionStartupFailure, 27*time.Hour, 0),
		a.neverRanRun(9396, "Release", "v3.2.0", "push", model.ConclusionSuccess, 30*time.Hour, 6*time.Minute+2*time.Second),
		a.neverRanRun(9392, "Integration", "docker 29 bump", "push", model.ConclusionFailure, 50*time.Hour, 8*time.Minute+55*time.Second),
		a.neverRanRun(9390, "Nightly build", "arm64 nightly", "schedule", model.ConclusionFailure, 56*time.Hour, 41*time.Minute),
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, run := range runs {
		if a.approved[run.ID] {
			runs[i].Conclusion, runs[i].Status = model.ConclusionNone, model.StatusQueued
		}
	}
	return runs
}

// neverRanJobs: held and blocked runs have no job; the arm64 PR's runs
// list a phantom check run (no steps, no runner) as live data does.
func neverRanJobs(runID int64) []model.Job {
	ran := func(id int64, name string, conclusion model.Conclusion) model.Job {
		j := job()
		j.ID, j.RunID, j.Name, j.Conclusion = id, runID, name, conclusion
		j.Steps[0].Conclusion = conclusion
		return j
	}
	switch runID - neverRanBase {
	case 9407:
		return []model.Job{{ID: neverRanBase*10 + 9407, RunID: runID, Name: "Docs Link Validation", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}}
	case 9405:
		return []model.Job{ran(neverRanBase*10+9405, "unit (linux)", model.ConclusionFailure)}
	case 9396:
		return []model.Job{ran(neverRanBase*10+9396, "release", model.ConclusionSuccess)}
	case 9392:
		return []model.Job{ran(neverRanBase*10+9392, "integration", model.ConclusionFailure), ran(neverRanBase*10+93920, "lint", model.ConclusionSuccess)}
	case 9390:
		return []model.Job{ran(neverRanBase*10+9390, "build (arm64)", model.ConclusionFailure)}
	}
	return []model.Job{}
}

func neverRanAnnotations(jobID int64) []model.Annotation {
	switch jobID {
	case neverRanBase*10 + 9392:
		return []model.Annotation{{Level: "failure", Message: "The self-hosted runner: ci-arm-07 lost communication with the server. Verify the machine is running and has a healthy network connection."}}
	case neverRanBase*10 + 9390:
		return []model.Annotation{{Level: "failure", Message: "Process completed with exit code 137."}}
	case neverRanBase*10 + 9405:
		return []model.Annotation{{Path: "internal/config/parse.go", StartLine: 88, EndLine: 88, Level: "failure", Message: "TestParseCache: cache key mismatch", Title: "go test"}}
	}
	return []model.Annotation{}
}
