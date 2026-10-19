package usecase_test

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// FuzzWatchSession drives a session through random ticks (job lists
// changing status, steps, order and membership; run switches; failing
// fetches) and checks every tick against a model: a log is fetched
// only when the session does not hold it, a job with no steps is never
// fetched, lines follow job order, and a failed tick keeps the logs it
// held and fetched.
func FuzzWatchSession(f *testing.F) {
	f.Add([]byte{0x31, 0x12, 0x33, 0x00, 0x31, 0x12, 0x33, 0x00, 0x31, 0x12, 0x33})
	f.Add([]byte{0x11, 0x22, 0x00, 0x91, 0x00, 0x51, 0x62, 0x00, 0x11, 0x22})
	f.Add([]byte{0x13, 0x25, 0x00, 0xff, 0x13, 0x25, 0x00, 0x80, 0x13, 0x25})
	f.Fuzz(func(t *testing.T, data []byte) {
		gh := &watchGitHub{}
		flaky := &failingLogs{watchGitHub: gh}
		session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: flaky}}
		runID := int64(570)
		var held map[int64]bool // the model of the session's cache
		heldRun := int64(0)
		for _, tick := range splitTicks(data) {
			jobs, failFor, switchRun := decodeTick(tick)
			if switchRun {
				runID ^= 1
			}
			gh.setJobs(jobs)
			flaky.failFor = failFor
			before, beforePer := gh.fetchCounts()

			state, err := session.Tick(context.Background(), "o/r", model.Run{ID: runID})

			cached := held
			if heldRun != runID {
				cached = nil
			}
			var wantLines []string
			wantFetch := map[int64]int{}
			failed := false
			for _, job := range jobs {
				if job.Status != model.StatusCompleted {
					continue
				}
				if len(job.Steps) > 0 && !cached[job.ID] {
					if job.ID == failFor {
						failed = true
						break
					}
					wantFetch[job.ID]++
				}
				if len(job.Steps) > 0 {
					wantLines = append(wantLines, fmt.Sprintf("job %d line 1", job.ID), fmt.Sprintf("job %d line 2", job.ID))
				}
			}
			after, afterPer := gh.fetchCounts()
			gotFetch := map[int64]int{}
			for id, n := range afterPer {
				if n != beforePer[id] {
					gotFetch[id] = n - beforePer[id]
				}
			}
			if !maps.Equal(gotFetch, wantFetch) {
				t.Fatalf("tick %x: fetched %v, want %v (cached %v)", tick, gotFetch, wantFetch, cached)
			}
			if failed {
				if err == nil {
					t.Fatalf("tick %x: failing fetch of job %d reported no error", tick, failFor)
				}
				// The session keeps what it held plus what this tick fetched.
				held, heldRun = maps.Clone(cached), runID
				if held == nil {
					held = map[int64]bool{}
				}
				for id := range wantFetch {
					held[id] = true
				}
				continue
			}
			if err != nil {
				t.Fatalf("tick %x: %v", tick, err)
			}
			if after-before != len(wantFetch) {
				t.Fatalf("tick %x: %d fetches, want %d", tick, after-before, len(wantFetch))
			}
			if got := lineTexts(state.Lines); !slices.Equal(got, wantLines) {
				t.Fatalf("tick %x: lines %q, want %q", tick, got, wantLines)
			}
			held, heldRun = map[int64]bool{}, runID
			for _, job := range jobs {
				if job.Status == model.StatusCompleted && len(job.Steps) > 0 {
					held[job.ID] = true
				}
			}
			if got, want := slices.Sorted(maps.Keys(state.JobLogs)), slices.Sorted(maps.Keys(held)); !slices.Equal(got, want) {
				t.Fatalf("tick %x: job logs for %v, want the completed jobs with steps %v", tick, got, want)
			}
		}
	})
}

// Each tick is a run of bytes up to a 0x00 separator. A job byte packs
// id (low 3 bits, 1–7), steps (bit 3), completed (bit 4); bit 7 set on
// the first byte switches run, and bit 6 marks that job's fetch to fail.
func splitTicks(data []byte) [][]byte {
	var ticks [][]byte
	for tick := range bytes.SplitSeq(data, []byte{0}) {
		if len(tick) > 0 && len(ticks) < 32 {
			ticks = append(ticks, tick)
		}
	}
	return ticks
}

func decodeTick(tick []byte) (jobs []model.Job, failFor int64, switchRun bool) {
	switchRun = tick[0]&0x80 != 0
	seen := map[int64]bool{}
	for _, b := range tick {
		id := int64(b&0x07) + 1
		if seen[id] {
			continue
		}
		seen[id] = true
		status := model.StatusInProgress
		if b&0x10 != 0 {
			status = model.StatusCompleted
		}
		steps := 0
		if b&0x08 != 0 {
			steps = 1
		}
		if b&0x40 != 0 {
			failFor = id
		}
		jobs = append(jobs, watchJob(id, status, steps))
	}
	return jobs, failFor, switchRun
}
