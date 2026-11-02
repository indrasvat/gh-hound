package usecase

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// paceStepFloor is the slow rule's floor for steps: GitHub stamps steps
// to the second, so a few seconds over the median is noise.
const paceStepFloor = 5 * time.Second

// PaceJobLister lists a run's jobs (one call per 100 jobs).
type PaceJobLister interface {
	ListJobs(ctx context.Context, repo string, runID int64) ([]model.Job, error)
}

// PaceJobBaseline is the green jobs and steps of a run baseline's runs,
// keyed by job name and by job name plus normalised step name.
type PaceJobBaseline struct {
	BaselineBranch string
	// Runs is how many sample runs' jobs were read.
	Runs  int
	Jobs  map[string][]time.Duration
	Steps map[string][]time.Duration
}

// JobBaseline reads the jobs of b's runs, leaving out exclude (the
// judged run): exactly min(Window, samples) listings, the same history
// the run itself is judged against, so no second walk and no separate
// fallback.
func (s PaceService) JobBaseline(ctx context.Context, repo string, b PaceBaseline, exclude int64) (PaceJobBaseline, error) {
	if s.Jobs == nil {
		return PaceJobBaseline{}, fmt.Errorf("job pace is unavailable for this adapter")
	}
	window := b.Window
	if window <= 0 {
		window = DefaultPaceWindow
	}
	jb := PaceJobBaseline{BaselineBranch: b.BaselineBranch, Jobs: map[string][]time.Duration{}, Steps: map[string][]time.Duration{}}
	for _, sample := range b.Samples {
		if jb.Runs == window {
			break
		}
		if sample.RunID == exclude {
			continue
		}
		jobs, err := s.Jobs.ListJobs(ctx, repo, sample.RunID)
		if err != nil {
			return PaceJobBaseline{}, err
		}
		jb.Runs++
		for _, job := range jobs {
			if d, ok := greenSpan(job.Status, job.Conclusion, job.StartedAt, job.CompletedAt); ok {
				jb.Jobs[job.Name] = append(jb.Jobs[job.Name], d)
			}
			for _, step := range job.Steps {
				if d, ok := greenSpan(step.Status, step.Conclusion, step.StartedAt, step.CompletedAt); ok {
					key := stepBaselineKey(job.Name, step.Name)
					jb.Steps[key] = append(jb.Steps[key], d)
				}
			}
		}
	}
	return jb, nil
}

func greenSpan(status model.Status, conclusion model.Conclusion, start, end time.Time) (time.Duration, bool) {
	if status != model.StatusCompleted || conclusion != model.ConclusionSuccess || start.IsZero() || end.Before(start) {
		return 0, false
	}
	return end.Sub(start).Truncate(time.Second), true
}

// pinnedAction is a step GitHub labelled from its uses: line, ref and
// all; Dependabot bumps of a pinned ref would rename it every time.
var pinnedAction = regexp.MustCompile(`^((?:Post )?Run [\w.-]+/[\w./-]+)@\S+$`)

func stepKey(name string) string {
	if m := pinnedAction.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return name
}

func stepBaselineKey(job, step string) string { return job + "\x00" + stepKey(step) }

// PaceItem is one job or step judged against its samples.
type PaceItem struct {
	Elapsed    time.Duration
	P50        time.Duration
	P90        time.Duration
	Ratio      float64
	SampleSize int
	Running    bool
	HasETA     bool
	ETA        time.Duration
	// Slow: finished slow, or still running and already past the line.
	Slow bool
}

// Thin is comparable work with too few samples to judge.
func (a PaceItem) Thin() bool { return a.SampleSize > 0 && a.SampleSize < PaceMinSample }

// Judged says the item has an estimate (p50 and p90).
func (a PaceItem) Judged() bool { return a.SampleSize >= PaceMinSample }

// AssessJob judges a job: green or running work only.
func AssessJob(job model.Job, b PaceJobBaseline, now time.Time) PaceItem {
	return assessSpan(job.Status, job.Conclusion, job.StartedAt, job.CompletedAt, b.Jobs[job.Name], paceSlowFloor, now)
}

// AssessStep judges one of job's steps by its normalised name.
func AssessStep(job model.Job, step model.Step, b PaceJobBaseline, now time.Time) PaceItem {
	return assessSpan(step.Status, step.Conclusion, step.StartedAt, step.CompletedAt, b.Steps[stepBaselineKey(job.Name, step.Name)], paceStepFloor, now)
}

func assessSpan(status model.Status, conclusion model.Conclusion, start, end time.Time, samples []time.Duration, floor time.Duration, now time.Time) PaceItem {
	var a PaceItem
	switch {
	case start.IsZero():
		return a
	case status == model.StatusInProgress:
		a.Running = true
		a.Elapsed = max(now.Sub(start), 0).Truncate(time.Second)
	case status == model.StatusCompleted && conclusion == model.ConclusionSuccess && !end.Before(start):
		a.Elapsed = end.Sub(start).Truncate(time.Second)
	default:
		return a
	}
	a.SampleSize = len(samples)
	if !a.Judged() {
		return a
	}
	sorted := slices.Sorted(slices.Values(samples))
	a.P50, a.P90 = percentile(sorted, 50), percentile(sorted, 90)
	if a.P50 > 0 {
		a.Ratio = math.Round(float64(a.Elapsed)/float64(a.P50)*100) / 100
	}
	a.Slow = a.Elapsed > a.P90 && float64(a.Elapsed) >= paceSlowRatio*float64(a.P50) && a.Elapsed-a.P50 >= floor
	if a.Running && !a.Slow {
		a.HasETA, a.ETA = true, max(a.P50-a.Elapsed, 0)
	}
	return a
}
