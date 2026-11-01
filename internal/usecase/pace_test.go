package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// paceFake serves one workflow's history and records every list call's
// filter, so the API budget and the server-side filters are pinned.
type paceFake struct {
	runs    []model.Run
	listErr error
	calls   []RunFilter
}

func (f *paceFake) ListWorkflowRuns(_ context.Context, _ string, _ string, filter RunFilter) ([]model.Run, error) {
	f.calls = append(f.calls, filter)
	if f.listErr != nil {
		return nil, f.listErr
	}
	matched := []model.Run{}
	for _, run := range f.runs {
		if filter.Branch != "" && run.HeadBranch != filter.Branch {
			continue
		}
		if filter.Event != "" && run.Event != filter.Event {
			continue
		}
		if filter.Status != "" && string(run.Conclusion) != filter.Status {
			continue
		}
		matched = append(matched, run)
	}
	start := (max(filter.Page, 1) - 1) * filter.PerPage
	if start >= len(matched) {
		return []model.Run{}, nil
	}
	return matched[start:min(start+filter.PerPage, len(matched))], nil
}

var paceEpoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// paceRun is a finished green first attempt that took d of wall time.
func paceRun(id int64, branch, event string, d time.Duration) model.Run {
	started := paceEpoch.Add(time.Duration(id) * time.Hour)
	return model.Run{
		ID:           id,
		RunNumber:    int(id),
		RunAttempt:   1,
		Status:       model.StatusCompleted,
		Conclusion:   model.ConclusionSuccess,
		HeadBranch:   branch,
		Event:        event,
		CreatedAt:    started.Add(-10 * time.Second),
		RunStartedAt: started,
		UpdatedAt:    started.Add(d),
	}
}

func seconds(values ...int) []time.Duration {
	out := make([]time.Duration, len(values))
	for i, v := range values {
		out[i] = time.Duration(v) * time.Second
	}
	return out
}

func baselineOf(durations ...time.Duration) PaceBaseline {
	b := PaceBaseline{Event: "push", BaselineBranch: "main"}
	for i, d := range durations {
		b.Samples = append(b.Samples, PaceSample{RunID: int64(9000 + i), Duration: d})
	}
	return b
}

func TestPercentileIsNearestRank(t *testing.T) {
	sorted := seconds(10, 20, 30, 40, 50, 60, 70, 80, 90, 100)
	cases := []struct {
		p    int
		want time.Duration
	}{
		{50, 50 * time.Second},
		{90, 90 * time.Second},
		{100, 100 * time.Second},
		{1, 10 * time.Second},
	}
	for _, tc := range cases {
		if got := percentile(sorted, tc.p); got != tc.want {
			t.Errorf("p%d = %v, want %v", tc.p, got, tc.want)
		}
	}
	// Odd count: the median is the middle value, never an average.
	if got := percentile(seconds(1, 2, 300), 50); got != 2*time.Second {
		t.Errorf("p50 of 1,2,300 = %v, want 2s", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("p50 of nothing = %v, want 0", got)
	}
}

// A single 40-minute cache miss among twenty ~4m runs must move
// neither the median nor p90.
func TestBaselineIgnoresAnIsolatedOutlier(t *testing.T) {
	durations := seconds(230, 232, 234, 236, 238, 240, 240, 241, 242, 243, 244, 245, 246, 248, 250, 252, 254, 256, 258, 2400)
	assessed := AssessPace(paceRun(1, "main", "push", 245*time.Second), baselineOf(durations...), paceEpoch)
	if assessed.P50 != 243*time.Second || assessed.P90 != 256*time.Second {
		t.Fatalf("p50/p90 = %v/%v, want 243s/256s", assessed.P50, assessed.P90)
	}
	if assessed.Status != PaceOnPace || assessed.Slow {
		t.Fatalf("status = %s slow = %v, want on_pace", assessed.Status, assessed.Slow)
	}
}

func TestSlowNeedsP90RatioAndAbsoluteFloor(t *testing.T) {
	// p50 = 240s, p90 = 270s over 10 samples.
	base := baselineOf(seconds(200, 220, 230, 235, 240, 245, 250, 260, 270, 280)...)
	cases := []struct {
		name string
		d    int
		slow bool
	}{
		{"above p90 but under 1.5x", 300, false},
		{"exactly 1.5x", 360, true},
		{"well above", 571, true},
		{"at p90", 270, false},
	}
	for _, tc := range cases {
		got := AssessPace(paceRun(1, "main", "push", time.Duration(tc.d)*time.Second), base, paceEpoch)
		if got.Slow != tc.slow {
			t.Errorf("%s: slow = %v, want %v (p50 %v p90 %v)", tc.name, got.Slow, tc.slow, got.P50, got.P90)
		}
		wantStatus := PaceOnPace
		if tc.slow {
			wantStatus = PaceSlow
		}
		if got.Status != wantStatus {
			t.Errorf("%s: status = %s, want %s", tc.name, got.Status, wantStatus)
		}
	}
	// A 10s workflow taking 25s is 2.5x and past p90, but only 15s
	// over its median: under the 30s floor, so not flagged.
	quick := baselineOf(seconds(8, 9, 9, 10, 10, 10, 11, 11, 12, 12)...)
	if got := AssessPace(paceRun(1, "main", "push", 25*time.Second), quick, paceEpoch); got.Slow {
		t.Fatalf("25s against a 10s median flagged slow; the 30s floor must hold")
	}
}

func runningRun(elapsed time.Duration, now time.Time) model.Run {
	run := paceRun(1, "main", "push", 0)
	run.Status = model.StatusInProgress
	run.Conclusion = model.ConclusionNone
	run.RunStartedAt = now.Add(-elapsed)
	run.UpdatedAt = now.Add(-time.Second)
	run.CreatedAt = run.RunStartedAt.Add(-14 * time.Second)
	return run
}

func TestRunningRunStates(t *testing.T) {
	base := baselineOf(seconds(200, 220, 230, 235, 240, 245, 250, 260, 270, 280)...) // p50 240 p90 270
	now := paceEpoch.Add(1000 * time.Hour)

	early := AssessPace(runningRun(60*time.Second, now), base, now)
	if early.Status != PaceRunning || !early.HasETA || early.ETA != 180*time.Second {
		t.Fatalf("early: status %s hasETA %v eta %v, want running with 180s left", early.Status, early.HasETA, early.ETA)
	}
	// Elapsed is measured from run_started_at to now, never updated_at,
	// and queue time is reported apart.
	if early.Elapsed != 60*time.Second || early.Queued != 14*time.Second {
		t.Fatalf("early: elapsed %v queued %v, want 60s and 14s", early.Elapsed, early.Queued)
	}

	due := AssessPace(runningRun(250*time.Second, now), base, now)
	if due.Status != PaceRunning || !due.HasETA || due.ETA != 0 || !due.Due() {
		t.Fatalf("due: status %s eta %v due %v, want running, 0 left, due", due.Status, due.ETA, due.Due())
	}

	over := AssessPace(runningRun(400*time.Second, now), base, now)
	if over.Status != PaceOverrun || !over.Slow || over.HasETA {
		t.Fatalf("overrun: status %s slow %v hasETA %v, want overrun, slow, no ETA", over.Status, over.Slow, over.HasETA)
	}
}

func TestNotComparableRuns(t *testing.T) {
	base := baselineOf(seconds(200, 220, 230, 235, 240, 245, 250, 260, 270, 280)...)
	now := paceEpoch.Add(1000 * time.Hour)

	rerun := runningRun(60*time.Second, now)
	rerun.RunAttempt = 2
	queued := runningRun(0, now)
	queued.Status = model.StatusQueued
	queued.RunStartedAt = time.Time{}
	cancelled := paceRun(1, "main", "push", 900*time.Second)
	cancelled.Conclusion = model.ConclusionCancelled
	failed := paceRun(1, "main", "push", 900*time.Second)
	failed.Conclusion = model.ConclusionFailure

	for name, run := range map[string]model.Run{"rerun": rerun, "queued": queued, "cancelled": cancelled, "failed": failed} {
		got := AssessPace(run, base, now)
		if got.Status != PaceNotComparable || got.Slow || got.HasETA {
			t.Errorf("%s: status %s slow %v hasETA %v, want not_comparable, never flagged, no ETA", name, got.Status, got.Slow, got.HasETA)
		}
	}
}

func TestThinBaselineIsInsufficientAndNeverInventsAnEstimate(t *testing.T) {
	now := paceEpoch.Add(1000 * time.Hour)
	thin := baselineOf(seconds(240, 240, 240, 240)...)
	for _, run := range []model.Run{runningRun(60*time.Second, now), paceRun(1, "main", "push", 900*time.Second)} {
		got := AssessPace(run, thin, now)
		if got.Status != PaceInsufficient || got.Slow || got.HasETA || got.P50 != 0 {
			t.Fatalf("thin: status %s slow %v hasETA %v p50 %v, want insufficient_data with nothing estimated", got.Status, got.Slow, got.HasETA, got.P50)
		}
	}
}

// The run being judged is excluded from its own baseline: with five
// samples nearest-rank p90 is the max, so a slow run counted in its
// own sample could never exceed it.
func TestTargetIsExcludedFromItsOwnBaseline(t *testing.T) {
	target := paceRun(42, "main", "push", 600*time.Second)
	base := baselineOf(seconds(240, 241, 242, 243, 244)...)
	base.Samples = append(base.Samples, PaceSample{RunID: target.ID, Duration: 600 * time.Second})
	got := AssessPace(target, base, paceEpoch)
	if got.SampleSize != 5 || !got.Slow {
		t.Fatalf("sample %d slow %v, want 5 samples (target excluded) and slow", got.SampleSize, got.Slow)
	}
	// Excluding the target can drop a baseline under the minimum.
	edge := baselineOf(seconds(240, 241, 242, 243)...)
	edge.Samples = append(edge.Samples, PaceSample{RunID: target.ID, Duration: 600 * time.Second})
	if got := AssessPace(target, edge, paceEpoch); got.Status != PaceInsufficient {
		t.Fatalf("status %s, want insufficient_data once the target is excluded", got.Status)
	}
}

func TestBaselineFiltersSamplesAndPinsServerFilters(t *testing.T) {
	rerun := paceRun(3, "feat/x", "pull_request", 900*time.Second)
	rerun.RunAttempt = 2
	inverted := paceRun(4, "feat/x", "pull_request", 0)
	inverted.UpdatedAt = inverted.RunStartedAt.Add(-time.Minute)
	unstarted := paceRun(5, "feat/x", "pull_request", 240*time.Second)
	unstarted.RunStartedAt = time.Time{}
	runs := []model.Run{rerun, inverted, unstarted}
	for id := int64(10); id < 16; id++ {
		runs = append(runs, paceRun(id, "feat/x", "pull_request", 240*time.Second))
	}
	runs = append(runs, paceRun(30, "feat/x", "push", 2000*time.Second)) // other event
	fake := &paceFake{runs: runs}

	b, err := PaceService{History: fake, Window: 20, PerPage: 100}.Baseline(context.Background(), "o/r", "ci.yml", "feat/x", "pull_request")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Samples) != 6 || b.BaselineBranch != "feat/x" || b.Event != "pull_request" {
		t.Fatalf("samples %d branch %q event %q, want 6 valid first-attempt pull_request samples on feat/x", len(b.Samples), b.BaselineBranch, b.Event)
	}
	if b.RunsScanned != 9 {
		t.Fatalf("runs scanned = %d, want 9", b.RunsScanned)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("list calls = %d, want exactly 1 (ceil(20/100))", len(fake.calls))
	}
	call := fake.calls[0]
	if call.Status != "success" || call.Event != "pull_request" || call.Branch != "feat/x" || call.PerPage != 100 {
		t.Fatalf("filter = %+v, want status=success event=pull_request branch=feat/x per_page=100", call)
	}
}

// A PR branch rarely has five green runs of its own. The baseline then
// widens to the same event on any branch: pull_request runs carry the
// PR's head branch, so the default branch has none to offer.
func TestBaselineWidensToTheSameEventOnAnyBranch(t *testing.T) {
	runs := []model.Run{paceRun(1, "feat/x", "pull_request", 240*time.Second), paceRun(2, "feat/x", "pull_request", 240*time.Second)}
	for id := int64(10); id < 18; id++ {
		runs = append(runs, paceRun(id, "feat/other", "pull_request", 250*time.Second))
	}
	for id := int64(30); id < 40; id++ {
		runs = append(runs, paceRun(id, "main", "push", 900*time.Second)) // pushes must not leak in
	}
	fake := &paceFake{runs: runs}
	b, err := PaceService{History: fake, Window: 20, PerPage: 100}.Baseline(context.Background(), "o/r", "ci.yml", "feat/x", "pull_request")
	if err != nil {
		t.Fatal(err)
	}
	if b.BaselineBranch != "" || len(b.Samples) != 10 {
		t.Fatalf("branch %q samples %d, want widened (\"\") with 10 pull_request samples", b.BaselineBranch, len(b.Samples))
	}
	for _, s := range b.Samples {
		if s.Duration != 240*time.Second && s.Duration != 250*time.Second {
			t.Fatalf("a push run leaked into a pull_request baseline: %v", s.Duration)
		}
	}
	if len(fake.calls) != 2 || fake.calls[1].Branch != "" || fake.calls[1].Event != "pull_request" {
		t.Fatalf("calls = %+v, want the branch walk then one any-branch walk with the same event", fake.calls)
	}
}

func TestBaselineBudgetIsCeilWindowOverPerPage(t *testing.T) {
	runs := []model.Run{}
	for id := int64(1); id <= 60; id++ {
		runs = append(runs, paceRun(id, "main", "push", 240*time.Second))
	}
	fake := &paceFake{runs: runs}
	b, err := PaceService{History: fake, Window: 25, PerPage: 10}.Baseline(context.Background(), "o/r", "ci.yml", "main", "push")
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 || len(b.Samples) != 25 {
		t.Fatalf("calls %d samples %d, want 3 calls (ceil(25/10)) and 25 samples", len(fake.calls), len(b.Samples))
	}
}

func TestBaselineWithoutBranchWalksOnce(t *testing.T) {
	fake := &paceFake{runs: []model.Run{paceRun(1, "main", "push", time.Minute)}}
	if _, err := (PaceService{History: fake, Window: 20, PerPage: 100}).Baseline(context.Background(), "o/r", "ci.yml", "", "push"); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %d, want 1: with no branch the first walk is already any-branch", len(fake.calls))
	}
}

func TestBaselineErrors(t *testing.T) {
	boom := errors.New("network down")
	if _, err := (PaceService{History: &paceFake{listErr: boom}}).Baseline(context.Background(), "o/r", "ci.yml", "main", "push"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the list error", err)
	}
	if _, err := (PaceService{}).Baseline(context.Background(), "o/r", "ci.yml", "main", "push"); err == nil {
		t.Fatal("no history adapter: want an error")
	}
	if _, err := (PaceService{History: &paceFake{}}).Baseline(context.Background(), "o/r", " ", "main", "push"); err == nil {
		t.Fatal("blank workflow: want an error")
	}
	if _, err := (PaceService{History: &paceFake{}}).Baseline(context.Background(), "", "ci.yml", "main", "push"); err == nil {
		t.Fatal("blank repo: want an error")
	}
}

func TestPaceVerdictLines(t *testing.T) {
	base := baselineOf(seconds(200, 220, 230, 235, 240, 245, 250, 260, 270, 280)...)
	now := paceEpoch.Add(1000 * time.Hour)
	cases := map[string]PaceAssessment{
		"the hound ran slow: 9m31s against its usual 4m00s":         AssessPace(paceRun(1, "main", "push", 571*time.Second), base, now),
		"on pace: 4m05s against its usual 4m00s":                    AssessPace(paceRun(1, "main", "push", 245*time.Second), base, now),
		"on the trail: about 3m left of its usual 4m00s":            AssessPace(runningRun(60*time.Second, now), base, now),
		"due any moment: 4m10s in, usually done by 4m00s":           AssessPace(runningRun(250*time.Second, now), base, now),
		"overrun: 6m40s in, past its usual 4m00s":                   AssessPace(runningRun(400*time.Second, now), base, now),
		"not enough trail to read: 4 green runs on record, needs 5": AssessPace(paceRun(1, "main", "push", time.Minute), baselineOf(seconds(1, 2, 3, 4)...), now),
	}
	for want, assessed := range cases {
		if got := PaceVerdictLine(assessed); got != want {
			t.Errorf("verdict = %q, want %q", got, want)
		}
	}
}

func TestFormatPaceETA(t *testing.T) {
	cases := map[time.Duration]string{
		0:                            "<1m left",
		59 * time.Second:             "<1m left",
		60 * time.Second:             "1m left",
		61 * time.Second:             "2m left",
		179 * time.Second:            "3m left",
		59 * time.Minute:             "59m left",
		59*time.Minute + time.Second: "~1h left",
		60 * time.Minute:             "~1h left",
		89 * time.Minute:             "~1h left",
		90 * time.Minute:             "~2h left",
		26 * time.Hour:               "~26h left",
	}
	for d, want := range cases {
		if got := FormatPaceETA(d); got != want {
			t.Errorf("FormatPaceETA(%v) = %q, want %q", d, got, want)
		}
	}
}
