package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestActionServiceSpacesMutationsWithoutRealSleep(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 6, 7, 18, 0, 0, 0, time.UTC)}
	service := usecase.ActionService{
		GitHub: fake.New(fake.ScenarioGreen),
		Limiter: &usecase.MutationLimiter{
			MinSpacing: time.Second,
			Clock:      clock,
		},
	}

	if _, err := service.RerunRun(context.Background(), "indrasvat/gh-hound", 571, false); err != nil {
		t.Fatalf("first rerun returned error: %v", err)
	}
	if _, err := service.RerunFailedJobs(context.Background(), "indrasvat/gh-hound", 571, true); err != nil {
		t.Fatalf("second rerun returned error: %v", err)
	}
	if clock.slept != time.Second {
		t.Fatalf("slept = %s, want 1s", clock.slept)
	}
}

func TestDispatchValidatesRequiredInputs(t *testing.T) {
	service := usecase.ActionService{GitHub: fake.New(fake.ScenarioGreen)}
	_, err := service.DispatchWorkflow(context.Background(), "indrasvat/gh-hound", "release.yml", usecase.DispatchRequest{
		Ref:            "main",
		Inputs:         map[string]string{"channel": "beta"},
		RequiredInputs: []string{"version"},
	})
	if actionErr, ok := usecase.AsActionError(err); !ok || actionErr.Kind != usecase.ActionErrorValidation {
		t.Fatalf("dispatch validation err = %#v", err)
	}
}

type fakeClock struct {
	now   time.Time
	slept time.Duration
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Sleep(d time.Duration) {
	f.slept += d
	f.now = f.now.Add(d)
}

// Approvals run in the background while the screen can fire a rerun:
// two Waits at once must still be spaced, and must not race.
func TestMutationLimiterSpacesConcurrentCallers(t *testing.T) {
	limiter := &usecase.MutationLimiter{MinSpacing: 40 * time.Millisecond}
	start := time.Now()
	done := make(chan time.Time, 3)
	for range 3 {
		go func() {
			_ = limiter.Wait(context.Background())
			done <- time.Now()
		}()
	}
	var last time.Time
	for range 3 {
		if at := <-done; at.After(last) {
			last = at
		}
	}
	if elapsed := last.Sub(start); elapsed < 80*time.Millisecond {
		t.Fatalf("3 concurrent mutations went out within %s, want them 40ms apart", elapsed)
	}
}
