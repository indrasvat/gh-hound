package usecase

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/indrasvat/gh-hound/internal/model"
)

// fuzzDurations turns bytes into up to 64 sample durations of 0..~18h.
func fuzzDurations(raw []byte) []time.Duration {
	out := []time.Duration{}
	for len(raw) >= 2 && len(out) < 64 {
		out = append(out, time.Duration(binary.LittleEndian.Uint16(raw))*time.Second)
		raw = raw[2:]
	}
	return out
}

// FuzzAssessPace: whatever the history and the run, the assessment
// keeps its contract. A flag always meets the documented rule, an ETA
// only exists for a comparable running run and never exceeds the
// median, a thin baseline estimates nothing, and slowness is monotone
// in duration (a longer run is never less slow).
func FuzzAssessPace(f *testing.F) {
	f.Add([]byte{240, 0, 241, 0, 242, 0, 243, 0, 244, 0, 245, 0}, uint32(571), uint8(0), uint8(1), uint8(0))
	f.Add([]byte{10, 0, 10, 0, 10, 0, 10, 0, 10, 0}, uint32(25), uint8(1), uint8(1), uint8(0))
	f.Add([]byte{}, uint32(0), uint8(2), uint8(0), uint8(3))
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, uint32(1), uint8(1), uint8(2), uint8(1))
	f.Fuzz(func(t *testing.T, raw []byte, elapsedSeconds uint32, state, attempt, conclusion uint8) {
		durations := fuzzDurations(raw)
		b := PaceBaseline{}
		for i, d := range durations {
			b.Samples = append(b.Samples, PaceSample{RunID: int64(i + 1), Duration: d})
		}
		now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		elapsed := time.Duration(elapsedSeconds%(200*3600)) * time.Second
		run := model.Run{ID: 1_000_000, RunAttempt: int(attempt % 4), CreatedAt: now.Add(-elapsed - time.Minute)}
		switch state % 3 {
		case 0:
			run.Status = model.StatusCompleted
			run.RunStartedAt = now.Add(-elapsed)
			run.UpdatedAt = now
		case 1:
			run.Status = model.StatusInProgress
			run.RunStartedAt = now.Add(-elapsed)
		case 2:
			run.Status = model.StatusQueued
		}
		run.Conclusion = []model.Conclusion{model.ConclusionSuccess, model.ConclusionFailure, model.ConclusionCancelled, model.ConclusionNone}[conclusion%4]

		a := AssessPace(run, b, now)
		if a.Elapsed < 0 || a.Queued < 0 || a.ETA < 0 {
			t.Fatalf("negative durations: %+v", a)
		}
		if a.HasETA != (a.Status == PaceRunning) {
			t.Fatalf("ETA iff running: %+v", a)
		}
		if a.Slow != (a.Status == PaceSlow || a.Status == PaceOverrun) {
			t.Fatalf("slow iff slow/overrun: %+v", a)
		}
		switch a.Status {
		case PaceNotComparable:
			if a.Reason == "" || a.P50 != 0 {
				t.Fatalf("not_comparable needs a reason and no stats: %+v", a)
			}
			return
		case PaceInsufficient:
			if a.SampleSize >= PaceMinSample || a.P50 != 0 || a.P90 != 0 {
				t.Fatalf("insufficient with an estimate: %+v", a)
			}
			return
		}
		if a.SampleSize < PaceMinSample || a.P50 > a.P90 {
			t.Fatalf("bad stats: %+v", a)
		}
		if !slices.Contains(durations, a.P50) || !slices.Contains(durations, a.P90) {
			t.Fatalf("percentiles must be observed samples: %+v", a)
		}
		if a.Slow && !(a.Elapsed > a.P90 && float64(a.Elapsed) >= 1.5*float64(a.P50) && a.Elapsed-a.P50 >= 30*time.Second) {
			t.Fatalf("flag breaks the documented rule: %+v", a)
		}
		if a.HasETA && (a.ETA > a.P50 || a.ETA != max(a.P50-a.Elapsed, 0)) {
			t.Fatalf("ETA is not p50 - elapsed: %+v", a)
		}
		if a.Slow {
			longer := run
			longer.RunStartedAt = run.RunStartedAt.Add(-time.Hour)
			if !AssessPace(longer, b, now).Slow {
				t.Fatalf("a longer run is less slow: %+v", a)
			}
		}
		if mark := PaceMarkFor(run, b, now); utf8.RuneCountInString(mark.Text) > 8 {
			t.Fatalf("mark %q overflows the 8-cell Duration column", mark.Text)
		}
		if verdict := PaceVerdictLine(a); verdict == "" || !utf8.ValidString(verdict) {
			t.Fatalf("empty or invalid verdict for %+v", a)
		}
	})
}

// FuzzPaceBaseline: over any history the walk stays inside its budget
// and keeps only valid green first attempts of the requested event.
func FuzzPaceBaseline(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, uint8(20), uint8(100), true)
	f.Add([]byte{0xff, 0x00, 0x10, 0x20}, uint8(3), uint8(1), false)
	f.Fuzz(func(t *testing.T, raw []byte, window, perPage uint8, withBranch bool) {
		runs := []model.Run{}
		base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		for i, c := range raw {
			if i >= 300 {
				break
			}
			start := base.Add(time.Duration(i) * time.Hour)
			run := model.Run{
				ID:           int64(i + 1),
				RunAttempt:   int(c%3) + 1 - int(c>>7), // 0..3
				Status:       []model.Status{model.StatusCompleted, model.StatusInProgress}[c>>6&1],
				Conclusion:   []model.Conclusion{model.ConclusionSuccess, model.ConclusionFailure}[c>>5&1],
				HeadBranch:   []string{"main", "feat/x"}[c>>4&1],
				Event:        []string{"push", "pull_request"}[c>>3&1],
				RunStartedAt: start,
				UpdatedAt:    start.Add(time.Duration(int(c)-64) * time.Second),
			}
			runs = append(runs, run)
		}
		fake := &paceFake{runs: runs}
		branch := ""
		if withBranch {
			branch = "feat/x"
		}
		svc := PaceService{History: fake, Window: int(window%120) + 1, PerPage: int(perPage%100) + 1}
		b, err := svc.Baseline(context.Background(), "o/r", "ci.yml", branch, "pull_request")
		if err != nil {
			t.Fatal(err)
		}
		budget := (svc.Window + 1 + svc.PerPage - 1) / svc.PerPage
		if len(fake.calls) > 2*budget {
			t.Fatalf("calls %d over budget %d", len(fake.calls), 2*budget)
		}
		if len(b.Samples) > svc.Window+1 {
			t.Fatalf("samples %d over window %d", len(b.Samples), svc.Window)
		}
		byID := map[int64]model.Run{}
		for _, r := range runs {
			byID[r.ID] = r
		}
		for _, s := range b.Samples {
			r := byID[s.RunID]
			if s.Duration <= 0 || r.RunAttempt > 1 || r.Conclusion != model.ConclusionSuccess || r.Status != model.StatusCompleted || r.Event != "pull_request" {
				t.Fatalf("invalid sample %+v from run %+v", s, r)
			}
			if b.BaselineBranch != "" && r.HeadBranch != b.BaselineBranch {
				t.Fatalf("sample from %q in a %q baseline", r.HeadBranch, b.BaselineBranch)
			}
		}
	})
}

// FuzzAssessStep: a step flag always meets the step rule (5s floor),
// elapsed is whole seconds, an ETA only for running work and never past
// the median, and thin samples never judge.
func FuzzAssessStep(f *testing.F) {
	f.Add([]byte{6, 0, 6, 0, 7, 0, 5, 0, 6, 0, 6, 0}, uint32(19), uint8(0))
	f.Add([]byte{1, 0, 1, 0}, uint32(400), uint8(1))
	f.Fuzz(func(t *testing.T, raw []byte, elapsedSeconds uint32, state uint8) {
		durations := fuzzDurations(raw)
		jb := PaceJobBaseline{Steps: map[string][]time.Duration{stepBaselineKey("build", "go test"): durations}}
		now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		elapsed := time.Duration(elapsedSeconds%(48*3600))*time.Second + 400*time.Millisecond
		step := model.Step{Name: "go test", StartedAt: now.Add(-elapsed)}
		switch state % 3 {
		case 0:
			step.Status, step.Conclusion, step.CompletedAt = model.StatusCompleted, model.ConclusionSuccess, now
		case 1:
			step.Status = model.StatusInProgress
		case 2:
			step.Status, step.Conclusion, step.CompletedAt = model.StatusCompleted, model.ConclusionSkipped, now
		}
		a := AssessStep(model.Job{Name: "build"}, step, jb, now)
		if a.Elapsed%time.Second != 0 || a.Elapsed < 0 {
			t.Fatalf("elapsed not whole seconds: %+v", a)
		}
		if !a.Judged() && (a.Slow || a.HasETA || a.P50 != 0) {
			t.Fatalf("judged without enough samples: %+v", a)
		}
		if a.Slow && !(a.Elapsed > a.P90 && float64(a.Elapsed) >= 1.5*float64(a.P50) && a.Elapsed-a.P50 >= 5*time.Second) {
			t.Fatalf("flag breaks the step rule: %+v", a)
		}
		if a.HasETA && (!a.Running || a.Slow || a.ETA > a.P50) {
			t.Fatalf("bad ETA: %+v", a)
		}
		if step.Conclusion == model.ConclusionSkipped && a.SampleSize != 0 {
			t.Fatalf("a skipped step was judged: %+v", a)
		}
	})
}
