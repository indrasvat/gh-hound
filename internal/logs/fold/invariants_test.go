package fold

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"testing"
)

// checkInvariants asserts the safety contract every engine must satisfy on
// every input:
//
//  1. lossless: Expand() == input, byte for byte
//  2. segments tile [0,n) in order with no gaps or overlaps
//  3. a protected line is never hidden
//  4. every hidden line has exactly the shape of the corresponding line in
//     the run's first cycle (folds never merge different shapes)
//  5. each fold keeps >= KeepHead cycles before and >= KeepTail after it
//  6. compaction never increases row count
func checkInvariants(t *testing.T, e Engine, lines []string) Plan {
	t.Helper()
	p := e.Plan(lines)
	if !slices.Equal(p.Expand(), lines) {
		t.Fatalf("[%s/%s] lossless roundtrip failed", e.Normalizer.Name(), e.Detector.Name())
	}
	cursor := 0
	for _, s := range p.Segments {
		if s.Start != cursor || s.End <= s.Start {
			t.Fatalf("segments do not tile: %+v at cursor %d", s, cursor)
		}
		cursor = s.End
		if s.Kind != Folded {
			continue
		}
		r := s.Run
		if !r.Set && (s.Start < r.Start+max(1, e.Options.KeepHead)*r.Period || s.End > r.End()-e.Options.KeepTail*r.Period) {
			t.Fatalf("fold %+v eats kept head/tail of run %+v", s, r)
		}
		for i := s.Start; i < s.End; i++ {
			if e.protected(lines[i]) || p.Pinned[i] {
				t.Fatalf("protected line %d hidden: %q", i+1, lines[i])
			}
		}
	}
	if err := witnessed(p, p.HiddenMask()); err != nil {
		t.Fatal(err)
	}
	if err := naiveWitnessed(p); err != nil {
		t.Fatal("naive checker:", err)
	}
	if cursor != len(lines) {
		t.Fatalf("segments end at %d, want %d", cursor, len(lines))
	}
	if p.Rows() > len(lines) {
		t.Fatalf("rows %d > lines %d", p.Rows(), len(lines))
	}
	return p
}

func allEngines(t testing.TB) []Engine {
	t.Helper()
	var out []Engine
	for _, n := range NormalizerNames() {
		for _, d := range DetectorNames() {
			for _, g := range []string{"none", "structural", "structural,outlier"} {
				for _, o := range []Options{DefaultOptions(), {MinRepeats: 2, MaxPeriod: 16}, {MinRepeats: 5, MaxPeriod: 1, KeepHead: 2, KeepTail: 3}, {MinRepeats: 3, MaxPeriod: 4, MaxShapes: 2}} {
					e, err := Build(n, d, g, o)
					if err != nil {
						t.Fatal(err)
					}
					out = append(out, e)
				}
			}
		}
	}
	return out
}

func TestInvariantsRandom(t *testing.T) {
	alphabet := []string{
		"INFO checking status 1", "INFO checking status 2", "ERROR boom 1",
		"##[group]x", "a", "a ", "b 0x1f", "b 0xff", "", "\x1b[31mred\x1b[0m 3",
		"2026-10-16T10:00:00.1Z poll", "2026-10-16T10:00:01.9Z poll", "::error::nope",
	}
	rng := rand.New(rand.NewPCG(1, 2))
	engines := allEngines(t)
	for range 400 {
		var lines []string
		for len(lines) < rng.IntN(300) {
			// mix random lines with injected repeating blocks
			if rng.IntN(3) == 0 {
				block := make([]string, 1+rng.IntN(5))
				for i := range block {
					block[i] = alphabet[rng.IntN(len(alphabet))]
				}
				for range 1 + rng.IntN(20) {
					lines = append(lines, block...)
				}
			} else {
				lines = append(lines, alphabet[rng.IntN(len(alphabet))])
			}
		}
		for _, e := range engines {
			checkInvariants(t, e, lines)
		}
	}
}

func TestCycleFindsPeriodTwoWithVolatileValues(t *testing.T) {
	var lines []string
	for i := range 50 {
		lines = append(lines,
			"INFO[2026-10-15 18:16:5"+string(rune('0'+i%10))+"] polling: is it running   name=svc",
			"INFO[2026-10-15 18:16:5"+string(rune('0'+i%10))+"] state :: &{ 0x38fa2888"+string(rune('a'+i%6))+"80 0x38fa}   name=svc",
		)
	}
	lines = append(lines, "ERROR timed out")
	e, _ := Build("shape", "cycle", "structural", DefaultOptions())
	p := checkInvariants(t, e, lines)
	if p.Rows() != 2+1+2+1 {
		t.Errorf("rows=%d want 6 (head cycle, marker, tail cycle, error)", p.Rows())
	}
}

func TestShapeKeepsIdentifiersLiteral(t *testing.T) {
	cases := map[string]string{
		"=== RUN   TestFoo2":         "=== RUN   TestFoo2",
		"apiVersion: v1beta1":        "apiVersion: v1beta1",
		"took 12ms":                  "took #",
		"at 0x38fa28888080":          "at #",
		"commit 9f2c3e1a":            "commit #",
		"2026-10-16T10:00:00.12Z ok": "# ok",
		"bad1 face2 cafe":            "bad1 face2 cafe",
	}
	for in, want := range cases {
		if got := (Shape{}).Normalize(in); got != want {
			t.Errorf("Shape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFastSeverityMatchesRegex(t *testing.T) {
	cases := []string{"ERROR boom", "no errors here", "Failed:", "xfail", "panic(", "terror", "Traceback (most", "fatal_x", "ok", "_error", "error_"}
	for _, c := range cases {
		if got, want := containsSeverity(c), severityRE.MatchString(c); got != want {
			t.Errorf("%q: fast=%v regex=%v", c, got, want)
		}
	}
}

func TestOutlierKeepsRareValueVisible(t *testing.T) {
	var lines []string
	for range 200 {
		lines = append(lines, "probe finished with exit code 0")
	}
	lines = append(lines, "probe finished with exit code 137")
	for range 200 {
		lines = append(lines, "probe finished with exit code 0")
	}
	e, _ := Build("shape", "cycle", "structural,outlier", DefaultOptions())
	p := checkInvariants(t, e, lines)
	if p.HiddenMask()[200] {
		t.Fatal("exit code 137 hidden")
	}
}

func TestOutlierSmallRuns(t *testing.T) {
	for _, n := range []int{3, 4, 5, 9, 10, 11, 50} {
		for at := range n {
			lines := make([]string, n)
			for i := range lines {
				lines[i] = "probe exit code 0"
			}
			lines[at] = "probe exit code 137"
			e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
			p, err := e.SafePlan(lines, DefaultLimits())
			if err != nil || p.HiddenMask()[at] {
				t.Fatalf("n=%d at=%d: rare value hidden (err=%v)", n, at, err)
			}
		}
	}
}

// naiveWitnessed is a deliberately simple, quadratic restatement of the
// witness rule, independent of the O(n) production verifier.
func naiveWitnessed(p Plan) error {
	hidden := p.HiddenMask()
	for _, s := range p.Segments {
		if s.Kind != Folded {
			continue
		}
		r := s.Run
		for i := s.Start; i < s.End; i++ {
			found := false
			if r.Set {
				for j := r.Start; j < i && !found; j++ {
					found = !hidden[j] && p.Keys[j] == p.Keys[i]
				}
			} else {
				j := r.Start + (i-r.Start)%r.Period
				found = !hidden[j] && p.Keys[j] == p.Keys[i]
			}
			if !found {
				return fmt.Errorf("line %d: no visible witness", i+1)
			}
		}
	}
	return nil
}

func TestShapeSemanticValues(t *testing.T) {
	cases := map[string]string{
		"exit code 137":                 "exit code 137",
		"status=503 took 12ms":          "status=503 took #",
		"HTTP/1.1 404 Not Found":        "HTTP/1.1 404 Not Found",
		"state :: &{0x38fa2888 0xab12}": "state :: &{# #}",
		"exit_code=2 id=9f2c3e1a77":     "exit_code=2 id=#",
		"Connecting to 2001:db8::1":     "Connecting to #",
		"PASS abc100":                   "PASS abc100",
		"ciflow/b200/200095":            "ciflow/b200/#",
	}
	for in, want := range cases {
		if got := (Shape{}).Normalize(in); got != want {
			t.Errorf("Shape(%q) = %q, want %q", in, got, want)
		}
	}
	for v, want := range map[string]bool{"18:16:52": true, "2026-10-16T15:51:45.39Z": true, "2001:db8::1": false, "1:2:3": false, "9:05:07": true} {
		if isClock(v) != want {
			t.Errorf("isClock(%q) != %v", v, want)
		}
	}
}

// severityRE is the reference the regex-free containsSeverity must match.
var severityRE = regexp.MustCompile(`(?i)\b(error|errors|fail|failed|failure|fatal|panic|exception|traceback)\b`)
