package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// countingGitHub counts the jobs and annotations calls a verdict costs,
// and can fail the annotations listing.
type countingGitHub struct {
	usecase.GitHub
	jobs, annotations int
	annotationsErr    error
}

func (c *countingGitHub) ListJobsForAttempt(ctx context.Context, repo string, runID int64, attempt int) ([]model.Job, error) {
	c.jobs++
	return c.GitHub.ListJobsForAttempt(ctx, repo, runID, attempt)
}

func (c *countingGitHub) ListAnnotations(ctx context.Context, repo string, job model.Job) ([]model.Annotation, error) {
	c.annotations++
	if c.annotationsErr != nil {
		return nil, c.annotationsErr
	}
	return c.GitHub.ListAnnotations(ctx, repo, job)
}

func TestVerdictServicePaysOnlyForRunsItCannotReadFromFields(t *testing.T) {
	ctx := context.Background()
	adapter := fake.New(fake.ScenarioNeverRan)
	runs, err := adapter.ListRuns(ctx, usecase.RunFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byNumber := map[int]model.Run{}
	for _, run := range runs {
		byNumber[run.RunNumber] = run
	}
	cases := []struct {
		number            int
		want              model.RunVerdict
		jobs, annotations int
	}{
		{9412, model.RunVerdict{Verdict: model.VerdictNeverRan, Reason: model.ReasonAwaitingApproval}, 0, 0},
		{9401, model.RunVerdict{Verdict: model.VerdictNeverRan, Reason: model.ReasonBlockedByPolicy}, 0, 0},
		{9396, model.RunVerdict{}, 0, 0},
		// The phantom check run has no runner: no annotations call.
		{9407, model.RunVerdict{Verdict: model.VerdictNeverRan, Reason: model.ReasonApprovalMissed}, 1, 0},
		// One failing job that ran, one green one: one annotations call.
		{9392, model.RunVerdict{Verdict: model.VerdictInfra, Reason: model.ReasonRunnerLost}, 1, 1},
		{9390, model.RunVerdict{Verdict: model.VerdictInfra, Reason: model.ReasonKilled}, 1, 1},
		{9405, model.RunVerdict{Verdict: model.VerdictCode}, 1, 1},
	}
	for _, tc := range cases {
		counting := &countingGitHub{GitHub: adapter}
		got, err := usecase.VerdictService{GitHub: counting}.Classify(ctx, "indrasvat/gh-hound", byNumber[tc.number])
		if err != nil {
			t.Fatalf("#%d: %v", tc.number, err)
		}
		if got != tc.want || counting.jobs != tc.jobs || counting.annotations != tc.annotations {
			t.Errorf("#%d = %+v (jobs %d, annotations %d), want %+v (jobs %d, annotations %d)",
				tc.number, got, counting.jobs, counting.annotations, tc.want, tc.jobs, tc.annotations)
		}
	}
}

// A failing annotations listing may hide a lost runner: the verdict is
// an error, never a guessed "code".
func TestVerdictServiceFailsLoudWhenAnnotationsAreUnreachable(t *testing.T) {
	ctx := context.Background()
	adapter := fake.New(fake.ScenarioNeverRan)
	runs, _ := adapter.ListRuns(ctx, usecase.RunFilter{})
	var integration model.Run
	for _, run := range runs {
		if run.RunNumber == 9392 {
			integration = run
		}
	}
	counting := &countingGitHub{GitHub: adapter, annotationsErr: errors.New("502")}
	if got, err := (usecase.VerdictService{GitHub: counting}).Classify(ctx, "indrasvat/gh-hound", integration); err == nil {
		t.Fatalf("verdict = %+v, want an error", got)
	}
}
