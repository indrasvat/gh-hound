package fold

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Regression tests for adversarial-review findings.

func TestIPv6IsNotAClock(t *testing.T) {
	for _, v := range []string{"2001:db8:12:34:56::1", "fe80::12:34:56", "12:34:56:78"} {
		if isClock(v) {
			t.Errorf("isClock(%q) = true", v)
		}
	}
	lines := []string{}
	for _, s := range []string{"1", "1", "2", "3", "1", "1", "1", "1", "1", "1"} {
		lines = append(lines, "Connecting to 2001:db8:12:34:56::"+s)
	}
	e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
	p, _ := e.SafePlan(lines, DefaultLimits())
	for _, seg := range p.Segments {
		if seg.Kind == Folded {
			if m := Summarize(seg, lines, e.Normalizer).Marker(); !strings.Contains(m, "::2") && !strings.Contains(m, "::3") {
				t.Errorf("changed endpoints missing from marker %q", m)
			}
		}
	}
}

func TestSummariesStayLinearOnFragmentedFolds(t *testing.T) {
	build := func(n int) []string {
		var lines []string
		for i := 0; len(lines) < n; i++ {
			lines = append(lines, "poll", "poll", fmt.Sprintf("unique-%c%c%d", 'a'+i%26, 'a'+i/26%26, i))
		}
		return lines
	}
	e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
	timeIt := func(n int) time.Duration {
		lines := build(n)
		p, _ := e.SafePlan(lines, DefaultLimits())
		start := time.Now()
		for _, seg := range p.Segments {
			if seg.Kind == Folded {
				_ = Summarize(seg, lines, e.Normalizer).Marker()
			}
		}
		return time.Since(start)
	}
	small, big := timeIt(6000), timeIt(48000)
	if big > 20*small+50*time.Millisecond {
		t.Fatalf("summaries not linear: 6k=%v 48k=%v", small, big)
	}
}

func TestSetDetectorHonoursStop(t *testing.T) {
	keys := make([]int, 4_000_000)
	for i := range keys {
		keys[i] = i % 3
	}
	stopped := false
	o := DefaultOptions()
	o.MaxPeriod = 1
	o.Stop = func() bool { return stopped }
	stopped = true
	start := time.Now()
	NewSet().Detect(keys, o)
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Fatalf("set detector ignored stop for %v", took)
	}
}

func TestPlateauOnRepeatedCyclePosition(t *testing.T) {
	var lines []string
	for c := range 10 {
		v := "0"
		if c >= 5 {
			v = "1"
		}
		lines = append(lines, "progress "+v, "tick", "progress 100", "tock")
	}
	e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
	p, _ := e.SafePlan(lines, DefaultLimits())
	if p.HiddenMask()[20] {
		t.Fatal("plateau transition progress 0 -> 1 hidden")
	}
}

func TestSemanticValueWindow(t *testing.T) {
	cases := map[string]string{
		"Sending status update 7": "Sending status update #",
		"HTTP/1.1 503 busy":       "HTTP/1.1 503 busy",
		"exit code 137":           "exit code 137",
		"status: 404 in 12ms":     "status: 404 in #",
	}
	for in, want := range cases {
		if got := (Shape{}).Normalize(in); got != want {
			t.Errorf("Shape(%q) = %q, want %q", in, got, want)
		}
	}
}
