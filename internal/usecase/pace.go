package usecase

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// PaceStatus is the verdict agents and the exit code branch on.
type PaceStatus string

const (
	PaceOnPace       PaceStatus = "on_pace"
	PaceRunning      PaceStatus = "running"
	PaceOverrun      PaceStatus = "overrun"
	PaceSlow         PaceStatus = "slow"
	PaceInsufficient PaceStatus = "insufficient_data"
	// PaceNotComparable covers runs a green-run baseline says nothing
	// about: queued or waiting, re-run attempts (only some jobs run
	// again), and finished runs that did not succeed.
	PaceNotComparable PaceStatus = "not_comparable"
)

const (
	DefaultPaceWindow = 20
	PaceMinSample     = 5

	paceSlowRatio = 1.5
	paceSlowFloor = 30 * time.Second
)

// PaceSample is one green first attempt's wall time.
type PaceSample struct {
	RunID    int64
	Duration time.Duration
}

// PaceBaseline is the sampled history for one workflow, branch and
// event. BaselineBranch is "" when the branch had too few samples and
// the baseline widened to the same event on any branch.
type PaceBaseline struct {
	Repo           string
	Workflow       string
	Branch         string
	BaselineBranch string
	Event          string
	Window         int
	RunsScanned    int
	Samples        []PaceSample
}

// PaceAssessment is one run judged against a baseline.
type PaceAssessment struct {
	Status     PaceStatus
	Elapsed    time.Duration
	Queued     time.Duration
	HasETA     bool
	ETA        time.Duration
	P50        time.Duration
	P90        time.Duration
	Ratio      float64
	Slow       bool
	SampleSize int
	// Reason says why a not_comparable run has no pace.
	Reason string
}

// Due is a running run past its median but not yet slow.
func (a PaceAssessment) Due() bool {
	return a.Status == PaceRunning && a.HasETA && a.ETA == 0
}

// PaceService samples a workflow's recent green runs.
type PaceService struct {
	History WorkflowRunHistory
	Window  int
	PerPage int
}

// Baseline samples the last Window green first attempts of the
// workflow on branch with event, widening to the same event on any
// branch when the branch has fewer than PaceMinSample. Matching the
// event keeps job sets comparable (push-only deploy legs); widening by
// branch rather than to the default branch is what works for PR runs,
// whose head branch is the PR's.
//
// It keeps one spare sample past the window: AssessPace leaves the
// judged run out of its own baseline, so whichever run in the sample is
// judged, a full window remains (one baseline serves every row of its
// key in the TUI). The branch widens below PaceMinSample plus that
// spare for the same reason.
//
// API budget, pinned by tests: ceil((Window+1)/PerPage) list calls per
// walk, at most two walks.
func (s PaceService) Baseline(ctx context.Context, repo, workflow, branch, event string) (PaceBaseline, error) {
	repo = strings.TrimSpace(repo)
	workflow = strings.TrimSpace(workflow)
	if repo == "" {
		return PaceBaseline{}, fmt.Errorf("repo is required")
	}
	if workflow == "" {
		return PaceBaseline{}, fmt.Errorf("workflow is required")
	}
	if s.History == nil {
		return PaceBaseline{}, fmt.Errorf("pace is unavailable for this adapter")
	}
	window := s.Window
	if window <= 0 {
		window = DefaultPaceWindow
	}
	perPage := s.PerPage
	if perPage <= 0 {
		perPage = DiffPerPage
	}
	b := PaceBaseline{Repo: repo, Workflow: workflow, Branch: branch, BaselineBranch: branch, Event: event, Window: window}
	samples, scanned, err := s.sample(ctx, repo, workflow, branch, event, window+1, perPage)
	if err != nil {
		return PaceBaseline{}, err
	}
	b.Samples, b.RunsScanned = samples, scanned
	if len(samples) > PaceMinSample || branch == "" {
		return b, nil
	}
	samples, scanned, err = s.sample(ctx, repo, workflow, "", event, window+1, perPage)
	if err != nil {
		return PaceBaseline{}, err
	}
	b.BaselineBranch, b.Samples, b.RunsScanned = "", samples, b.RunsScanned+scanned
	return b, nil
}

func (s PaceService) sample(ctx context.Context, repo, workflow, branch, event string, window, perPage int) ([]PaceSample, int, error) {
	walker := newRunHistoryWalker(s.History, repo, workflow, branch, (window+perPage-1)/perPage, perPage)
	walker.filter.Status = string(model.ConclusionSuccess)
	walker.filter.Event = event
	samples := make([]PaceSample, 0, window)
	scanned := 0
	for len(samples) < window {
		run, ok, err := walker.Next(ctx)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			break
		}
		scanned++
		if d, ok := sampleDuration(run); ok {
			samples = append(samples, PaceSample{RunID: run.ID, Duration: d})
		}
	}
	return samples, scanned, nil
}

// sampleDuration is a green first attempt's run_started_at → updated_at.
// A re-run attempt restarts run_started_at but re-executes only some
// jobs, so its wall time is not comparable.
func sampleDuration(run model.Run) (time.Duration, bool) {
	if run.Status != model.StatusCompleted || run.Conclusion != model.ConclusionSuccess || run.RunAttempt > 1 {
		return 0, false
	}
	if run.RunStartedAt.IsZero() || run.UpdatedAt.IsZero() || !run.UpdatedAt.After(run.RunStartedAt) {
		return 0, false
	}
	return run.UpdatedAt.Sub(run.RunStartedAt), true
}

// AssessPace judges run against b at now. The run itself is excluded
// from its own baseline. Pure: no I/O.
func AssessPace(run model.Run, b PaceBaseline, now time.Time) PaceAssessment {
	var a PaceAssessment
	if !run.RunStartedAt.IsZero() && !run.CreatedAt.IsZero() && run.RunStartedAt.After(run.CreatedAt) {
		a.Queued = run.RunStartedAt.Sub(run.CreatedAt)
	}
	running := run.Status == model.StatusInProgress
	switch {
	case run.RunStartedAt.IsZero():
	case running:
		a.Elapsed = max(now.Sub(run.RunStartedAt), 0)
	case run.Status == model.StatusCompleted && run.UpdatedAt.After(run.RunStartedAt):
		a.Elapsed = run.UpdatedAt.Sub(run.RunStartedAt)
	}
	if reason := notComparable(run); reason != "" {
		a.Status, a.Reason = PaceNotComparable, reason
		return a
	}

	durations := make([]time.Duration, 0, len(b.Samples))
	for _, sample := range b.Samples {
		if sample.RunID != run.ID && (b.Window <= 0 || len(durations) < b.Window) {
			durations = append(durations, sample.Duration)
		}
	}
	a.SampleSize = len(durations)
	if a.SampleSize < PaceMinSample {
		a.Status = PaceInsufficient
		return a
	}
	slices.Sort(durations)
	a.P50, a.P90 = percentile(durations, 50), percentile(durations, 90)
	if a.P50 > 0 {
		a.Ratio = math.Round(float64(a.Elapsed)/float64(a.P50)*100) / 100
	}
	a.Slow = isSlow(a.Elapsed, a.P50, a.P90)
	switch {
	case running && a.Slow:
		a.Status = PaceOverrun
	case running:
		a.Status = PaceRunning
		a.HasETA = true
		a.ETA = max(a.P50-a.Elapsed, 0)
	case a.Slow:
		a.Status = PaceSlow
	default:
		a.Status = PaceOnPace
	}
	return a
}

func notComparable(run model.Run) string {
	switch {
	case run.RunAttempt > 1:
		return fmt.Sprintf("attempt %d is a re-run", run.RunAttempt)
	case run.RunStartedAt.IsZero() || run.Status != model.StatusInProgress && run.Status != model.StatusCompleted:
		return "the run has not started"
	case run.Status == model.StatusCompleted && run.Conclusion != model.ConclusionSuccess:
		return fmt.Sprintf("the run finished %s", run.Conclusion)
	}
	return ""
}

// isSlow: past p90, at least 1.5x the median, and at least 30s over
// it, so a quick workflow's jitter never flags.
func isSlow(d, p50, p90 time.Duration) bool {
	return d > p90 && float64(d) >= paceSlowRatio*float64(p50) && d-p50 >= paceSlowFloor
}

// percentile is nearest-rank over an ascending slice.
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(float64(p) / 100 * float64(len(sorted))))
	return sorted[min(max(rank, 1), len(sorted))-1]
}

// FormatPaceETA renders time left in whole minutes, rounded up, since
// an estimate from a median should not claim seconds: "<1m left",
// "3m left", "~1h left", and ">9h left" from ten hours on. Every form
// keeps "left" and fits the 8-cell Duration column.
func FormatPaceETA(d time.Duration) string {
	if d < time.Minute {
		return "<1m left"
	}
	if minutes := int(math.Ceil(d.Minutes())); minutes < 60 {
		return fmt.Sprintf("%dm left", minutes)
	}
	hours := max(int(math.Round(d.Hours())), 1)
	if hours > 9 {
		return ">9h left"
	}
	return fmt.Sprintf("~%dh left", hours)
}

// FormatPaceDuration is a measured duration: "38s", "4m05s", "1h05m",
// and whole hours from 100h ("140h"). It fits 6 cells below 100,000
// hours, so "↑ " plus it fits the 8-cell Duration column.
func FormatPaceDuration(d time.Duration) string {
	d = max(d, 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 100*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// PaceMark is what a duration cell says about a run: time left, a slow
// flag, or the plain measured duration.
type PaceMark struct {
	Text string
	// ETA: running within its baseline ("3m left", "due").
	ETA bool
	// Slow: finished slow or still running and already slow.
	Slow bool
	// Waiting: queued or waiting, no duration yet.
	Waiting bool
	// Running: in progress (elapsed, ETA or overrun).
	Running bool
}

// Note says whether the mark carries Pace information worth a cell of
// its own where the Duration column is dropped.
func (m PaceMark) Note() bool { return m.ETA || m.Slow }

// PaceMarkFor renders run against b (has is false without a baseline,
// or with Pace off) at now. Without a baseline it is the measured
// duration: elapsed while running, wall time once finished.
func PaceMarkFor(run model.Run, b PaceBaseline, has bool, now time.Time) PaceMark {
	switch {
	case run.Status == model.StatusWaiting:
		return PaceMark{Text: "waiting", Waiting: true}
	case run.RunStartedAt.IsZero() || run.Status != model.StatusInProgress && run.Status != model.StatusCompleted:
		return PaceMark{Text: "queued", Waiting: true}
	}
	running := run.Status == model.StatusInProgress
	elapsed := run.UpdatedAt.Sub(run.RunStartedAt)
	if running {
		elapsed = now.Sub(run.RunStartedAt)
	}
	mark := PaceMark{Text: FormatPaceDuration(elapsed), Running: running}
	if !has {
		return mark
	}
	a := AssessPace(run, b, now)
	switch {
	case a.Slow:
		mark.Text, mark.Slow = "↑ "+FormatPaceDuration(a.Elapsed), true
	case a.Due():
		mark.Text, mark.ETA = "due", true
	case a.HasETA:
		mark.Text, mark.ETA = FormatPaceETA(a.ETA), true
	}
	return mark
}

// PaceVerdictLine is the only place Pace speaks in the hound's voice.
func PaceVerdictLine(a PaceAssessment) string {
	usual := FormatPaceDuration(a.P50)
	switch a.Status {
	case PaceSlow:
		return fmt.Sprintf("the hound ran slow: %s against its usual %s", FormatPaceDuration(a.Elapsed), usual)
	case PaceOnPace:
		return fmt.Sprintf("on pace: %s against its usual %s", FormatPaceDuration(a.Elapsed), usual)
	case PaceOverrun:
		return fmt.Sprintf("overrun: %s in, past its usual %s", FormatPaceDuration(a.Elapsed), usual)
	case PaceRunning:
		if a.Due() {
			return fmt.Sprintf("due any moment: %s in, usually done by %s", FormatPaceDuration(a.Elapsed), usual)
		}
		return fmt.Sprintf("on the trail: about %s of its usual %s", FormatPaceETA(a.ETA), usual)
	case PaceInsufficient:
		noun := "runs"
		if a.SampleSize == 1 {
			noun = "run"
		}
		return fmt.Sprintf("not enough trail to read: %d green %s on record, needs %d", a.SampleSize, noun, PaceMinSample)
	default:
		return fmt.Sprintf("no pace to read: %s, and only first attempts that succeed compare with green runs", a.Reason)
	}
}
