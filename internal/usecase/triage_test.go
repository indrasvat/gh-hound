package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestTriageLoadsFailurePayloadForFailedRun(t *testing.T) {
	service := usecase.TriageService{GitHub: fake.New(fake.ScenarioFailing)}
	run := model.Run{ID: 30433642, Conclusion: model.ConclusionFailure}

	failures, err := service.LoadRunFailures(context.Background(), "indrasvat/gh-hound", run)
	if err != nil {
		t.Fatalf("LoadRunFailures returned error: %v", err)
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

	failures, err := service.LoadRunFailures(context.Background(), "indrasvat/gh-hound", run)
	if err != nil {
		t.Fatalf("LoadRunFailures returned error: %v", err)
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

	failures, err := service.LoadRunFailures(context.Background(), "indrasvat/gh-hound", run)
	if err != nil {
		t.Fatalf("LoadRunFailures returned error: %v", err)
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

	failures, err := service.LoadRunFailures(context.Background(), "indrasvat/gh-hound", run)
	if err != nil {
		t.Fatalf("LoadRunFailures returned error: %v", err)
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

	failures, err := service.LoadRunFailures(context.Background(), "indrasvat/gh-hound", run)
	if err != nil {
		t.Fatalf("LoadRunFailures returned error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	excerpt := failures[0].LogExcerpt
	if !strings.Contains(excerpt, "line three") {
		t.Fatalf("excerpt should keep the log tail when no anchor matches:\n%s", excerpt)
	}
}
