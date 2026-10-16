package fold

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type panicky struct{}

func (panicky) Name() string            { return "panicky" }
func (panicky) Normalize(string) string { panic("boom") }

type liar struct{}

func (liar) Name() string { return "liar" }
func (liar) Detect(keys []int, _ Options) []Run {
	// claims everything is one repeating line, regardless of shape
	if len(keys) < 3 {
		return nil
	}
	return []Run{{Start: 0, Period: 1, Count: len(keys)}}
}

func TestSafePlanFallsBackOnPanic(t *testing.T) {
	e := Engine{Normalizer: panicky{}, Detector: Cycle{}, Options: DefaultOptions()}
	lines := []string{"a", "b"}
	p, err := e.SafePlan(lines, DefaultLimits())
	if !errors.Is(err, ErrFallback) || p.Rows() != 2 {
		t.Fatalf("want raw fallback, got rows=%d err=%v", p.Rows(), err)
	}
}

func TestSafePlanRejectsLyingDetector(t *testing.T) {
	e := Engine{Normalizer: Shape{}, Detector: liar{}, Options: DefaultOptions()}
	lines := []string{"x", "y", "z", "w"}
	p, err := e.SafePlan(lines, DefaultLimits())
	if !errors.Is(err, ErrFallback) || p.Rows() != 4 {
		t.Fatalf("want raw fallback, got rows=%d err=%v", p.Rows(), err)
	}
}

func TestSafePlanPinsHugeLines(t *testing.T) {
	big := strings.Repeat("x1 ", 10_000)
	lines := []string{big, big, big, big, big}
	e, _ := Build("shape", "cycle", "structural", DefaultOptions())
	p, err := e.SafePlan(lines, DefaultLimits())
	if err != nil || p.Rows() != 5 {
		t.Fatalf("huge lines must stay visible, rows=%d err=%v", p.Rows(), err)
	}
}

func TestSafePlanHostileInputs(t *testing.T) {
	inputs := [][]string{
		nil, {}, {""}, {"\x00\x00", "\x00\x00", "\x00\x00"},
		{"\xff\xfe bad utf8 1", "\xff\xfe bad utf8 2", "\xff\xfe bad utf8 3"},
		{"\x1b[", "\x1b[", "\x1b[", "\x1b["}, {"\x1b[999999999999m"},
		{"\r\r\r", "\r\r\r", "\r\r\r"}, {strings.Repeat("\x1b[31m", 5000)},
		{"0x", "0x", "0x"}, {"--", "__", "-_-"}, {"日本語 1", "日本語 2", "日本語 3"},
	}
	for _, n := range NormalizerNames() {
		for _, d := range DetectorNames() {
			e, _ := Build(n, d, "structural,outlier", DefaultOptions())
			for _, in := range inputs {
				if _, err := e.SafePlan(in, DefaultLimits()); err != nil {
					t.Errorf("%s/%s on %q: %v", n, d, in, err)
				}
			}
		}
	}
}

// A huge, pathological input under a tight deadline must come back fast,
// raw, and lossless — never hang the caller.
func TestSafePlanContextDeadline(t *testing.T) {
	lines := make([]string, 3_000_000)
	for i := range lines {
		lines[i] = fmt.Sprintf("worker-%d tick %d id=%08x", i%7, i, i*2654435761)
	}
	e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	p, err := e.SafePlanContext(ctx, lines, DefaultLimits())
	took := time.Since(start)
	if !errors.Is(err, ErrFallback) {
		t.Fatalf("want fallback on deadline, got %v", err)
	}
	if p.Rows() != len(lines) {
		t.Fatalf("fallback must be raw, rows=%d", p.Rows())
	}
	if took > 250*time.Millisecond {
		t.Fatalf("deadline 30ms but returned after %v", took)
	}
	t.Logf("3M lines, 30ms deadline: returned raw after %v", took)
}

func TestSafePlanContextCompletes(t *testing.T) {
	lines := []string{"a 1", "a 2", "a 3", "a 4", "b"}
	e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
	p, err := e.SafePlanContext(context.Background(), lines, DefaultLimits())
	if err != nil || p.Rows() != 4 {
		t.Fatalf("rows=%d err=%v", p.Rows(), err)
	}
}

func TestSafePlanContextGiantRun(t *testing.T) {
	lines := make([]string, 3_000_000)
	for i := range lines {
		lines[i] = "same"
	}
	e, _ := Build("shape", "cycle", "structural", DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = e.SafePlanContext(ctx, lines, DefaultLimits())
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Fatalf("giant run ignored deadline: %v", took)
	}
}
