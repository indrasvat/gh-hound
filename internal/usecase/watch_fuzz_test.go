package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// FuzzWatchSession drives a session through random ticks (job lists
// changing status, steps, order and membership; run switches; failing,
// expired (410) and not-yet (404) logs) and checks every tick against
// a model: a log is fetched only when the session does not hold it, a
// job with no steps is never fetched, lines and missing logs follow job
// order, an expired log is held and a not-yet one is not, and a failed
// tick keeps the logs it held and fetched.
func FuzzWatchSession(f *testing.F) {
	f.Add([]byte{0x31, 0x12, 0x33, 0x00, 0x31, 0x12, 0x33, 0x00, 0x31, 0x12, 0x33})
	f.Add([]byte{0x11, 0x22, 0x00, 0x91, 0x00, 0x51, 0x62, 0x00, 0x11, 0x22})
	f.Add([]byte{0x13, 0x25, 0x00, 0xff, 0x13, 0x25, 0x00, 0x80, 0x13, 0x25})
	f.Add([]byte{0x39, 0x3a, 0x7b, 0x5c, 0x00, 0x39, 0x1a, 0x1b, 0x1c, 0x00, 0x19, 0x1a, 0x1b, 0x1c})
	f.Fuzz(func(t *testing.T, data []byte) {
		gh := &watchGitHub{}
		answers := &scriptedLogs{watchGitHub: gh}
		session := &usecase.WatchSession{Service: usecase.WatchService{GitHub: answers}}
		runID := int64(570)
		var held map[int64]bool // the model of the session's cache: job → expired
		heldRun := int64(0)
		for _, tick := range splitTicks(data) {
			jobs, answer, switchRun := decodeTick(tick)
			if switchRun {
				runID ^= 1
			}
			gh.setJobs(jobs)
			answers.answer = answer
			before, beforePer := gh.fetchCounts()

			state, err := session.Tick(context.Background(), "o/r", model.Run{ID: runID})

			cached := held
			if heldRun != runID {
				cached = nil
			}
			next := map[int64]bool{}
			var wantLines, wantNoLog []string
			wantFetch := map[int64]int{}
			failed := false
			for _, job := range jobs {
				if job.Status != model.StatusCompleted || len(job.Steps) == 0 {
					continue
				}
				expired, ok := cached[job.ID]
				if !ok {
					wantFetch[job.ID]++
					switch answer[job.ID] {
					case answerFail:
						failed = true
					case answerNotYet:
						wantNoLog = append(wantNoLog, fmt.Sprintf("%d not yet", job.ID))
						continue
					case answerExpired:
						expired = true
					}
				}
				if failed {
					break
				}
				next[job.ID] = expired
				if expired {
					wantNoLog = append(wantNoLog, fmt.Sprintf("%d expired", job.ID))
					continue
				}
				wantLines = append(wantLines, fmt.Sprintf("job %d line 1", job.ID), fmt.Sprintf("job %d line 2", job.ID))
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
					t.Fatalf("tick %x: a failing fetch reported no error", tick)
				}
				// The session keeps what it held plus what this tick fetched.
				merged := maps.Clone(cached)
				if merged == nil {
					merged = map[int64]bool{}
				}
				maps.Copy(merged, next)
				held, heldRun = merged, runID
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
			var gotNoLog []string
			for _, missing := range state.NoLog {
				reason := "not yet"
				if missing.Expired {
					reason = "expired"
				}
				gotNoLog = append(gotNoLog, fmt.Sprintf("%d %s", missing.Job.ID, reason))
			}
			if !slices.Equal(gotNoLog, wantNoLog) {
				t.Fatalf("tick %x: no-log %q, want %q", tick, gotNoLog, wantNoLog)
			}
			held, heldRun = next, runID
			for id, entry := range state.JobLogs {
				if expired, ok := held[id]; !ok || entry.Expired != expired {
					t.Fatalf("tick %x: job logs %v, want %v", tick, state.JobLogs, held)
				}
			}
			if len(state.JobLogs) != len(held) {
				t.Fatalf("tick %x: %d job logs held, want %d", tick, len(state.JobLogs), len(held))
			}
		}
	})
}

type logAnswer int

const (
	answerLog logAnswer = iota
	answerFail
	answerExpired
	answerNotYet
)

// scriptedLogs answers each job's log fetch as the tick says; every
// answer counts as a request.
type scriptedLogs struct {
	*watchGitHub
	answer map[int64]logAnswer
}

func (s *scriptedLogs) FetchJobLog(ctx context.Context, repo string, id int64) (string, error) {
	raw, _ := s.watchGitHub.FetchJobLog(ctx, repo, id)
	switch s.answer[id] {
	case answerFail:
		return "", errors.New("log download failed")
	case answerExpired:
		return "", usecase.LogUnavailableError{JobID: id, Status: http.StatusGone}
	case answerNotYet:
		return "", usecase.LogUnavailableError{JobID: id, Status: http.StatusNotFound}
	}
	return raw, nil
}

// Each tick is a run of bytes up to a 0x00 separator. A job byte packs
// id (low 3 bits, 1–7), steps (bit 3), completed (bit 4) and how its
// log answers (bits 5–6: the log, 410, a failure, 404); bit 7 set on
// the first byte switches run.
func splitTicks(data []byte) [][]byte {
	var ticks [][]byte
	for tick := range bytes.SplitSeq(data, []byte{0}) {
		if len(tick) > 0 && len(ticks) < 32 {
			ticks = append(ticks, tick)
		}
	}
	return ticks
}

func decodeTick(tick []byte) (jobs []model.Job, answer map[int64]logAnswer, switchRun bool) {
	answer = map[int64]logAnswer{}
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
		answer[id] = [...]logAnswer{answerLog, answerExpired, answerFail, answerNotYet}[b>>5&0x03]
		jobs = append(jobs, watchJob(id, status, steps))
	}
	return jobs, answer, switchRun
}
