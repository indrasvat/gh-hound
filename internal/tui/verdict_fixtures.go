package tui

import (
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// sampleNeverRanRuns is a fork-heavy repo's Monday morning (the fake
// never_ran scenario): every reason a red run is red, one code failure,
// one green run.
func sampleNeverRanRuns() []model.Run {
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(number int, name, title, event, actor string, conclusion model.Conclusion, age, took time.Duration) model.Run {
		created := now.Add(-age)
		return model.Run{ID: int64(30439400 + number), RunNumber: number, Name: name, DisplayTitle: title, Event: event,
			Status: model.StatusCompleted, Conclusion: conclusion, Actor: actor, TriggeringActor: actor, RunAttempt: 1,
			HeadBranch: "main", HeadSHA: "9e8d7c6", CreatedAt: created, RunStartedAt: created, UpdatedAt: created.Add(took)}
	}
	fork := func(run model.Run, branch, sha string) model.Run {
		run.FromFork, run.HeadRepository, run.HeadBranch, run.HeadSHA = true, run.Actor+"/forge", branch, sha
		return run
	}
	return []model.Run{
		fork(mk(9412, "Unit tests", "fix typo in docs", "pull_request", "new-dev", model.ConclusionActionRequired, 2*time.Hour, 0), "patch-1", "a1f93c0"),
		fork(mk(9411, "Lint", "fix typo in docs", "pull_request", "new-dev", model.ConclusionActionRequired, 2*time.Hour, 0), "patch-1", "a1f93c0"),
		fork(mk(9408, "Unit tests", "add arm64 build", "pull_request", "drive-by", model.ConclusionFailure, 9*time.Hour, 4*time.Hour+14*time.Minute), "arm64", "7c2d4e1"),
		fork(mk(9407, "Code scanning", "add arm64 build", "pull_request", "drive-by", model.ConclusionFailure, 9*time.Hour, 4*time.Hour+14*time.Minute), "arm64", "7c2d4e1"),
		mk(9405, "Unit tests", "cache config parse", "push", "maintainer", model.ConclusionFailure, 11*time.Hour, 3*time.Minute+41*time.Second),
		fork(mk(9401, "Labeler", "triage new issues", "pull_request_target", "drive-by", model.ConclusionStartupFailure, 26*time.Hour, 0), "labels", "3b5a9f2"),
		mk(9399, "Docs", "readme badge", "push", "maintainer", model.ConclusionStartupFailure, 27*time.Hour, 0),
		mk(9396, "Release", "v3.2.0", "push", "maintainer", model.ConclusionSuccess, 30*time.Hour, 6*time.Minute+2*time.Second),
		mk(9392, "Integration", "docker 29 bump", "push", "maintainer", model.ConclusionFailure, 50*time.Hour, 8*time.Minute+55*time.Second),
	}
}

// sampleNeverRanApp is the runs list of sampleNeverRanRuns with the
// verdicts its jobs gave, as the background read leaves them.
func sampleNeverRanApp(app App) App {
	rows := sampleNeverRanRuns()
	app.runs = runs.NewModel(usecase.LaunchContext{Repo: "acme/forge", Actor: "indrasvat", Scope: usecase.LaunchScopeRepo, State: usecase.LaunchStateRuns, Runs: rows, RepoRuns: rows})
	app.runsConfirmed = confirmedAt(nil, rows, time.Now())
	app.routes = []Route{RouteRuns}
	app.verdicts = map[verdictKey]model.RunVerdict{}
	for _, run := range rows {
		switch run.RunNumber {
		case 9408, 9407:
			app.verdicts[verdictKey{run.ID, 1}] = model.RunVerdict{Verdict: model.VerdictNeverRan, Reason: model.ReasonApprovalMissed}
		case 9405:
			app.verdicts[verdictKey{run.ID, 1}] = model.RunVerdict{Verdict: model.VerdictCode}
		case 9392:
			app.verdicts[verdictKey{run.ID, 1}] = model.RunVerdict{Verdict: model.VerdictInfra, Reason: model.ReasonRunnerLost}
		}
	}
	return app
}
