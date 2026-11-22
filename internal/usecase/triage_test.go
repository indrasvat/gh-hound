package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// jobsByAttempt answers each attempt's jobs and counts the listings.
type jobsByAttempt struct {
	usecase.GitHub
	attempts map[int][]model.Job
	calls    int
}

func (j *jobsByAttempt) ListJobsForAttempt(_ context.Context, _ string, _ int64, attempt int) ([]model.Job, error) {
	j.calls++
	return j.attempts[attempt], nil
}

func (j *jobsByAttempt) ListJobs(context.Context, string, int64) ([]model.Job, error) {
	j.calls++
	return j.attempts[0], nil
}

func (j *jobsByAttempt) ListAnnotations(context.Context, string, model.Job) ([]model.Annotation, error) {
	return nil, nil
}

func (j *jobsByAttempt) FetchJobLog(context.Context, string, int64) (string, error) {
	return "", errors.New("no log")
}

// A run the fields settle as never ran costs no jobs call, and its
// failed[] stays empty even if GitHub lists a failing check run.
func TestTriageSkipsJobsOfARunThatNeverRan(t *testing.T) {
	phantom := model.Job{ID: 9, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	gh := &jobsByAttempt{attempts: map[int][]model.Job{0: {phantom}, 1: {phantom}}}
	run := model.Run{ID: 1, Status: model.StatusCompleted, Conclusion: model.ConclusionActionRequired, RunAttempt: 1}
	got, err := usecase.TriageService{GitHub: gh}.LoadRunTriage(context.Background(), "acme/forge", run, 0)
	if err != nil || gh.calls != 0 || len(got.Failures) != 0 || got.Verdict.Reason != model.ReasonAwaitingApproval {
		t.Fatalf("triage = %+v err=%v calls=%d, want awaiting_approval with no call", got, err, gh.calls)
	}
}

// The latest listing may already be a re-run's queued jobs: triage reads
// the attempt the run says it shows.
func TestTriageReadsTheRunsOwnAttempt(t *testing.T) {
	failed := model.Job{ID: 1, RunAttempt: 1, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunnerName: "r1",
		Steps: []model.Step{{Name: "test", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}}}
	queued := model.Job{ID: 2, RunAttempt: 2, Status: model.StatusQueued}
	gh := &jobsByAttempt{attempts: map[int][]model.Job{0: {queued}, 1: {failed}, 2: {queued}}}
	run := model.Run{ID: 1, Event: "pull_request", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunAttempt: 1}
	got, err := usecase.TriageService{GitHub: gh}.LoadRunTriage(context.Background(), "acme/forge", run, 0)
	if err != nil || got.Verdict.Verdict != model.VerdictCode || len(got.Failures) != 1 {
		t.Fatalf("triage = %+v err=%v, want a code failure from attempt 1", got, err)
	}
}

func TestTriageLoadsFailurePayloadForFailedRun(t *testing.T) {
	service := usecase.TriageService{GitHub: fake.New(fake.ScenarioFailing)}
	run := model.Run{ID: 30433642, Conclusion: model.ConclusionFailure}

	triage, err := service.LoadRunTriage(context.Background(), "indrasvat/gh-hound", run, 0)
	failures := triage.Failures
	if err != nil {
		t.Fatalf("LoadRunTriage returned error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	failure := failures[0]
	if failure.Job.Name != "build" {
		t.Fatalf("job = %q, want build", failure.Job.Name)
	}
	if failure.Step.Name != "go test ./..." {
		t.Fatalf("step = %q, want go test ./...", failure.Step.Name)
	}
	if failure.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", failure.ExitCode)
	}
	if len(failure.Annotations) != 1 || failure.Annotations[0].Path != "internal/parser/lexer.go" {
		t.Fatalf("annotations = %#v", failure.Annotations)
	}
	if !strings.Contains(failure.LogExcerpt, "--- FAIL: TestLexIdent/trailing_underscore") {
		t.Fatalf("log excerpt missing failure anchor:\n%s", failure.LogExcerpt)
	}
}

func TestTriageSkipsRunsWithoutActionableConclusion(t *testing.T) {
	// The fake always returns a failing job, so a non-empty result here
	// would mean the service inspected jobs for a green run.
	service := usecase.TriageService{GitHub: fake.New(fake.ScenarioFailing)}
	run := model.Run{ID: 30433642, Conclusion: model.ConclusionSuccess}

	triage, err := service.LoadRunTriage(context.Background(), "indrasvat/gh-hound", run, 0)
	failures := triage.Failures
	if err != nil {
		t.Fatalf("LoadRunTriage returned error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("failures = %#v, want none for a green run", failures)
	}
}

func TestTriageDegradesWhenLogUnavailable(t *testing.T) {
	// ScenarioLogRender fails FetchJobLog; the failure entry must survive
	// with job, step, and annotations intact instead of erroring out.
	service := usecase.TriageService{GitHub: fake.New(fake.ScenarioLogRender)}
	run := model.Run{ID: 30433642, Conclusion: model.ConclusionFailure}

	triage, err := service.LoadRunTriage(context.Background(), "indrasvat/gh-hound", run, 0)
	failures := triage.Failures
	if err != nil {
		t.Fatalf("LoadRunTriage returned error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	failure := failures[0]
	if failure.Job.Name != "build" || failure.Step.Name != "go test ./..." {
		t.Fatalf("degraded entry lost job context: %#v", failure)
	}
	if failure.LogExcerpt != "" {
		t.Fatalf("log excerpt = %q, want empty when the log is unavailable", failure.LogExcerpt)
	}
	if failure.ExitCode != 1 {
		t.Fatalf("exit code = %d, want fallback 1", failure.ExitCode)
	}
	if len(failure.Annotations) != 1 {
		t.Fatalf("annotations = %#v, want the API annotations even without a log", failure.Annotations)
	}
}

type triageGitHub struct {
	*fake.Adapter
	log string
}

func (g triageGitHub) FetchJobLog(context.Context, string, int64) (string, error) {
	return g.log, nil
}

func TestTriageParsesExitCodeFromLog(t *testing.T) {
	github := triageGitHub{
		Adapter: fake.New(fake.ScenarioFailing),
		log:     "make: *** [ci] Error 2\n##[error]Process completed with exit code 2",
	}
	service := usecase.TriageService{GitHub: github}
	run := model.Run{ID: 30433642, Conclusion: model.ConclusionFailure}

	triage, err := service.LoadRunTriage(context.Background(), "indrasvat/gh-hound", run, 0)
	failures := triage.Failures
	if err != nil {
		t.Fatalf("LoadRunTriage returned error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 parsed from the log", failures[0].ExitCode)
	}
}

func TestTriageExcerptFallsBackToLogTail(t *testing.T) {
	github := triageGitHub{
		Adapter: fake.New(fake.ScenarioFailing),
		log:     "line one\nline two\nline three",
	}
	service := usecase.TriageService{GitHub: github}
	run := model.Run{ID: 30433642, Conclusion: model.ConclusionFailure}

	triage, err := service.LoadRunTriage(context.Background(), "indrasvat/gh-hound", run, 0)
	failures := triage.Failures
	if err != nil {
		t.Fatalf("LoadRunTriage returned error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	excerpt := failures[0].LogExcerpt
	if !strings.Contains(excerpt, "line three") {
		t.Fatalf("excerpt should keep the log tail when no anchor matches:\n%s", excerpt)
	}
}

// A failed job of another attempt never reaches failed[]: the agent
// would fix what this attempt did not do.
func TestTriageKeepsOtherAttemptsOutOfFailed(t *testing.T) {
	other := model.Job{ID: 3, RunAttempt: 2, Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunnerName: "r2"}
	gh := &jobsByAttempt{attempts: map[int][]model.Job{1: {other}}}
	run := model.Run{ID: 1, Event: "push", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure, RunAttempt: 1}
	got, err := usecase.TriageService{GitHub: gh}.LoadRunTriage(context.Background(), "acme/forge", run, 0)
	if err != nil || len(got.Failures) != 0 || got.Verdict != (model.RunVerdict{}) {
		t.Fatalf("triage = %+v err=%v, want no failures and no verdict", got, err)
	}
}
