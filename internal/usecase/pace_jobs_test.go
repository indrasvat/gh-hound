package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// jobsFake serves one job list per run and counts listings.
type jobsFake struct {
	jobs  map[int64][]model.Job
	err   error
	calls []int64
}

func (f *jobsFake) ListJobs(_ context.Context, _ string, runID int64) ([]model.Job, error) {
	f.calls = append(f.calls, runID)
	if f.err != nil {
		return nil, f.err
	}
	return f.jobs[runID], nil
}

var jobEpoch = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func paceJob(name string, took time.Duration, steps ...model.Step) model.Job {
	return model.Job{
		Name: name, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess,
		StartedAt: jobEpoch, CompletedAt: jobEpoch.Add(took), Steps: steps,
	}
}

func paceStep(number int, name string, took time.Duration) model.Step {
	return model.Step{
		Number: number, Name: name, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess,
		StartedAt: jobEpoch, CompletedAt: jobEpoch.Add(took),
	}
}

// sampledBaseline is a run baseline over runs 1..n (n = window + spare).
func sampledBaseline(window, n int) PaceBaseline {
	b := PaceBaseline{BaselineBranch: "main", Window: window}
	for i := 1; i <= n; i++ {
		b.Samples = append(b.Samples, PaceSample{RunID: int64(i), Duration: 4 * time.Minute})
	}
	return b
}

func TestStepKeyDropsPinnedRefsOnly(t *testing.T) {
	cases := []struct{ name, want string }{
		{"Run actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5", "Run actions/checkout"},
		{"Run actions/checkout@v4", "Run actions/checkout"},
		{"Post Run actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5", "Post Run actions/setup-go"},
		{"Run docker/build-push-action/sub@v5", "Run docker/build-push-action/sub"},
		{"go build ./...", "go build ./..."},
		{"Run go test ./... -run 'X@Y'", "Run go test ./... -run 'X@Y'"},
		{"Notify team@example.com", "Notify team@example.com"},
	}
	for _, c := range cases {
		if got := stepKey(c.name); got != c.want {
			t.Errorf("stepKey(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// A Dependabot bump renames the step; its baseline must survive it.
func TestStepBaselineSurvivesARefBump(t *testing.T) {
	b := sampledBaseline(5, 6)
	fake := &jobsFake{jobs: map[int64][]model.Job{}}
	for i := int64(1); i <= 6; i++ {
		ref := "aaaa"
		if i%2 == 0 {
			ref = "bbbb"
		}
		fake.jobs[i] = []model.Job{paceJob("build", time.Minute, paceStep(1, "Run actions/checkout@"+ref, 3*time.Second))}
	}
	jb, err := PaceService{Jobs: fake}.JobBaseline(context.Background(), "o/r", b, 0)
	if err != nil {
		t.Fatal(err)
	}
	target := paceJob("build", time.Minute, paceStep(1, "Run actions/checkout@cccc", 3*time.Second))
	if got := AssessStep(target, target.Steps[0], jb, jobEpoch); got.SampleSize != 5 {
		t.Fatalf("sample size = %d across ref bumps, want 5", got.SampleSize)
	}
}

// Exactly window listings, the judged run left out, and only the
// baseline's own runs.
func TestJobBaselineListsExactlyWindowRuns(t *testing.T) {
	for _, exclude := range []int64{0, 3, 99} {
		b := sampledBaseline(20, 21)
		fake := &jobsFake{}
		jb, err := PaceService{Jobs: fake}.JobBaseline(context.Background(), "o/r", b, exclude)
		if err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 20 {
			t.Fatalf("exclude %d: %d listings, want exactly the window (20)", exclude, len(fake.calls))
		}
		for _, id := range fake.calls {
			if id == exclude {
				t.Fatalf("listed the judged run %d", exclude)
			}
		}
		if jb.Runs != 20 || jb.BaselineBranch != "main" {
			t.Fatalf("baseline = %+v", jb)
		}
	}
	// A thin run baseline lists what it has, never more.
	fake := &jobsFake{}
	if _, err := (PaceService{Jobs: fake}).JobBaseline(context.Background(), "o/r", sampledBaseline(20, 3), 0); err != nil || len(fake.calls) != 3 {
		t.Fatalf("thin: %d listings, err %v", len(fake.calls), err)
	}
}

func TestJobBaselineErrors(t *testing.T) {
	if _, err := (PaceService{}).JobBaseline(context.Background(), "o/r", sampledBaseline(5, 6), 0); err == nil {
		t.Fatal("no job lister must be an error")
	}
	boom := errors.New("boom")
	if _, err := (PaceService{Jobs: &jobsFake{err: boom}}).JobBaseline(context.Background(), "o/r", sampledBaseline(5, 6), 0); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the listing error", err)
	}
}

// Only green work is a sample: failed or skipped jobs, skipped steps
// and unfinished steps say nothing about a green run's pace.
func TestJobBaselineSamplesGreenWorkOnly(t *testing.T) {
	b := sampledBaseline(5, 5)
	fake := &jobsFake{jobs: map[int64][]model.Job{}}
	for i := int64(1); i <= 5; i++ {
		skippedStep := paceStep(2, "deploy", 0)
		skippedStep.Conclusion = model.ConclusionSkipped
		failed := paceJob("lint", time.Hour)
		failed.Conclusion = model.ConclusionFailure
		fake.jobs[i] = []model.Job{paceJob("build", 90*time.Second, paceStep(1, "go build", 6*time.Second), skippedStep), failed}
	}
	jb, err := PaceService{Jobs: fake}.JobBaseline(context.Background(), "o/r", b, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(jb.Jobs["lint"]); n != 0 {
		t.Fatalf("failed jobs sampled: %d", n)
	}
	if n := len(jb.Steps[stepBaselineKey("build", "deploy")]); n != 0 {
		t.Fatalf("skipped steps sampled: %d", n)
	}
	if got := jb.Jobs["build"]; len(got) != 5 || got[0] != 90*time.Second {
		t.Fatalf("build samples = %v", got)
	}
}

func TestAssessJobsAndSteps(t *testing.T) {
	jb := PaceJobBaseline{Runs: 20, Jobs: map[string][]time.Duration{}, Steps: map[string][]time.Duration{}}
	for i := range 20 {
		jb.Jobs["build"] = append(jb.Jobs["build"], time.Duration(85+i)*time.Second)                                                              // p50 94s, p90 102s
		jb.Steps[stepBaselineKey("build", "go build")] = append(jb.Steps[stepBaselineKey("build", "go build")], time.Duration(5+i%3)*time.Second) // p50 6s, p90 7s
	}
	cases := []struct {
		name              string
		took              time.Duration
		stepTook          time.Duration
		jobSlow, stepSlow bool
	}{
		{"on pace", 95 * time.Second, 6 * time.Second, false, false},
		{"past p90 but not 1.5x", 130 * time.Second, 8 * time.Second, false, false},
		{"slow", 150 * time.Second, 19 * time.Second, true, true},
		// 1.5x and past p90 but under the floors (30s job, 5s step).
		{"under the floors", 141 * time.Second, 10 * time.Second, true, false},
	}
	for _, c := range cases {
		job := paceJob("build", c.took, paceStep(3, "go build", c.stepTook))
		ja := AssessJob(job, jb, jobEpoch)
		sa := AssessStep(job, job.Steps[0], jb, jobEpoch)
		if ja.Slow != c.jobSlow || sa.Slow != c.stepSlow {
			t.Errorf("%s: job slow %v (want %v), step slow %v (want %v): %+v %+v", c.name, ja.Slow, c.jobSlow, sa.Slow, c.stepSlow, ja, sa)
		}
		if sa.Elapsed%time.Second != 0 || ja.P50 != 94*time.Second || sa.P50 != 6*time.Second {
			t.Errorf("%s: stats %+v %+v", c.name, ja, sa)
		}
	}
}

// Running work judges against now; queued or unsampled work has no
// estimate; fewer than 5 samples is thin.
func TestAssessJobStates(t *testing.T) {
	jb := PaceJobBaseline{Runs: 20, Jobs: map[string][]time.Duration{}, Steps: map[string][]time.Duration{}}
	for range 20 {
		jb.Jobs["build"] = append(jb.Jobs["build"], 90*time.Second)
	}
	jb.Jobs["new"] = []time.Duration{time.Second, time.Second}
	running := model.Job{Name: "build", Status: model.StatusInProgress, StartedAt: jobEpoch}
	if a := AssessJob(running, jb, jobEpoch.Add(30*time.Second)); !a.Running || !a.HasETA || a.ETA != time.Minute || a.Slow {
		t.Fatalf("running: %+v", a)
	}
	if a := AssessJob(running, jb, jobEpoch.Add(5*time.Minute)); !a.Slow || a.HasETA {
		t.Fatalf("overrun: %+v", a)
	}
	if a := AssessJob(model.Job{Name: "build", Status: model.StatusQueued}, jb, jobEpoch); a.SampleSize != 0 || a.Slow {
		t.Fatalf("queued: %+v", a)
	}
	failed := paceJob("build", 10*time.Minute)
	failed.Conclusion = model.ConclusionFailure
	if a := AssessJob(failed, jb, jobEpoch); a.Slow || a.SampleSize != 0 {
		t.Fatalf("a failed job is not comparable: %+v", a)
	}
	if a := AssessJob(paceJob("new", time.Hour), jb, jobEpoch); a.SampleSize != 2 || !a.Thin() || a.Slow {
		t.Fatalf("thin: %+v", a)
	}
}

func ExampleFormatPaceDuration_steps() {
	for _, d := range []time.Duration{0, 400 * time.Millisecond, 19 * time.Second, 91 * time.Second} {
		fmt.Println(FormatPaceDuration(d))
	}
	// Output:
	// 0s
	// 0s
	// 19s
	// 1m31s
}
