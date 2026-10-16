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
	if took := time.Since(start); took > raceSlowdown*50*time.Millisecond {
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

func TestTitleAndMarker(t *testing.T) {
	var lines []string
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("10:00:%02d poll: not ready", i), fmt.Sprintf("10:00:%02d retry error 0x%04x", i, i*7919))
	}
	p, _ := Default().SafePlan(lines, DefaultLimits())
	for _, seg := range p.Segments {
		if seg.Kind != Folded {
			continue
		}
		s := Summarize(seg, lines, Default().Normalizer)
		want := "×40 2-line block · 10:00:00 → 10:00:39 · 1 changing id · ⚠ 38 error-like inside"
		if got := s.Title(); got != want {
			t.Errorf("Title = %q\nwant  %q", got, want)
		}
		if got, want := s.Marker(), "⋯ "+want+" · 76 lines hidden (L3–L78)"; got != want {
			t.Errorf("Marker = %q\nwant   %q", got, want)
		}
		return
	}
	t.Fatal("no fold")
}

func TestSummaryListsOnlyRecurringValues(t *testing.T) {
	var lines []string
	for i, ms := range []int{110, 299, 336, 73} {
		lines = append(lines, fmt.Sprintf("probe took %dms request-id: %08x-%04x", ms, 0x7094ba37+i*7919, 0x018f+i))
	}
	seg := Segment{Kind: Folded, Start: 1, End: 3, Run: Run{Start: 0, Period: 1, Count: 4}}
	got := Summarize(seg, lines, Shape{}).Title()
	if strings.Contains(got, "|") {
		t.Fatalf("all-distinct values listed: %q", got)
	}
	if !strings.Contains(got, "110ms → 73ms (4 values)") || !strings.Contains(got, "changing id") {
		t.Fatalf("want range and id count: %q", got)
	}
	codes := []string{"exit code 0", "exit code 0", "exit code 2", "exit code 0"}
	if got := Summarize(Segment{Kind: Folded, Start: 1, End: 3, Run: Run{Period: 1, Count: 4}}, codes, Shape{}).Title(); strings.Contains(got, "|") {
		t.Fatalf("semantic codes are part of the shape, not a slot: %q", got)
	}
}

func TestOutlierHonoursStop(t *testing.T) {
	lines := make([]string, 20_000)
	for i := range lines {
		lines[i] = strings.Repeat("123 ", 750)
	}
	keys := make([]string, len(lines))
	for i := range keys {
		keys[i] = Shape{}.Normalize(lines[i])
	}
	in := &RefineInput{Lines: lines, Keys: keys, Normalizer: Shape{}, Runs: []Run{{Start: 0, Period: 1, Count: len(lines)}}}
	start := time.Now()
	in.Stop = func() bool { return time.Since(start) > 10*time.Millisecond }
	NewOutlier().Refine(in)
	if took := time.Since(start); took > raceSlowdown*60*time.Millisecond {
		t.Fatalf("outlier refiner ignored stop for %v", took)
	}
}

func TestSummarySpanFollowsOneClock(t *testing.T) {
	var lines []string
	for i := range 8 {
		lines = append(lines, fmt.Sprintf("2026-10-16T16:00:%02d.0000000Z time=\"2026-10-16T15:53:%02dZ\" level=info msg=\"waiting\"", 45+i, 23+i))
	}
	seg := Segment{Kind: Folded, Start: 1, End: 7, Run: Run{Start: 0, Period: 1, Count: 8}}
	s := Summarize(seg, lines, Shape{})
	if from, to := s.Span(); from != "16:00:45" || to != "16:00:52" {
		t.Fatalf("span %s → %s mixes clocks", from, to)
	}
}
