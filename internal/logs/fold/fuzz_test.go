package fold

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// 1. Every engine, arbitrary bytes: full invariant contract, and the
// SafePlan safety net must never have to fire for built-in strategies.
func FuzzInvariants(f *testing.F) {
	for _, seed := range []string{
		"a\na\na\na\nb\n",
		"x 1\ny 2\nx 3\ny 4\nx 5\ny 6\nx 7\ny 8\n",
		"##[group]a\n##[group]a\n##[group]a\n",
		"exit code 0\nexit code 0\nexit code 0\nexit code 9\nexit code 0\nexit code 0\nexit code 0\n",
		"\x1b[31mINFO\x1b[0m 0xab12\n\x1b[31mINFO\x1b[0m 0xcd34\n\x1b[31mINFO\x1b[0m 0xef56\n",
		"\xff\xfe\x00\r\n\xff\xfe\x00\r\n\xff\xfe\x00\r\n",
	} {
		f.Add(seed)
	}
	engines := allEngines(f)
	f.Fuzz(func(t *testing.T, raw string) {
		lines := strings.Split(raw, "\n")
		for _, e := range engines {
			checkInvariants(t, e, lines)
			if _, err := e.SafePlan(lines, DefaultLimits()); err != nil {
				t.Fatalf("[%s/%s] safety net fired: %v", e.Normalizer.Name(), e.Detector.Name(), err)
			}
		}
	})
}

// 2. Shape scanner: literal pieces + volatile values reassemble the
// ANSI-stripped line, and each volatile value becomes exactly one '#'.
func FuzzShapeScanner(f *testing.F) {
	for _, s := range []string{"INFO[2026-10-15 18:16:52] x 0x38fa", "w-a3f9c1 shard-7", "\x1b[1;31m#1\x1b[0m", "\x1b[", "日本 12ms",
		"\x1b]8;;https://x.io/a1b2c3\x07link 12\x1b]8;;\x07", "\x1b]0;title 9\x1b\\ ok", "\x1b(B\x1b[38;5;196mred 3", "\x1b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		var rebuilt strings.Builder
		literalHashes, volatiles := 0, 0
		scanShape(line, func(a, z int, volatile bool) {
			text := line[a:z]
			rebuilt.WriteString(text)
			if volatile {
				volatiles++
			} else {
				literalHashes += strings.Count(text, "#")
			}
		})
		if got, want := rebuilt.String(), stripEscapes(line); got != want {
			t.Fatalf("rebuild mismatch:\n got %q\nwant %q", got, want)
		}
		norm := Shape{}.Normalize(line)
		if strings.Contains(norm, "\x1b") {
			t.Fatalf("escape survived normalize: %q", norm)
		}
		if got := strings.Count(norm, "#"); got != literalHashes+volatiles {
			t.Fatalf("hash count %d != literal %d + volatile %d", got, literalHashes, volatiles)
		}
		if len(Shape{}.Slots(line)) != volatiles {
			t.Fatal("Slots disagrees with scanner")
		}
	})
}

// stripEscapes removes terminal escapes the way the scanner defines them.
func stripEscapes(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			i += escapeLen(line, i)
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

// 3. Differential: hand-rolled severity matcher == the regex it replaced.
func FuzzSeverityDifferential(f *testing.F) {
	for _, s := range []string{"ERROR x", "xerror", "error_", "Fatal:", "PANIC!", "tracebacks", "fail-fast"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		if got, want := containsSeverity(line), severityRE.MatchString(line); got != want {
			t.Fatalf("%q: fast=%v regex=%v", line, got, want)
		}
	})
}

// 4. Differential: Cycle restricted to period 1 == Consecutive.
func FuzzDetectorDifferential(f *testing.F) {
	f.Add([]byte{1, 1, 1, 2, 2, 2, 2, 3}, uint8(3))
	f.Fuzz(func(t *testing.T, data []byte, minRep uint8) {
		keys := make([]int, len(data))
		for i, b := range data {
			keys[i] = int(b%5) - 1 // includes -1 "protected"
			if keys[i] < 0 {
				keys[i] = -1 - i
			}
		}
		o := Options{MinRepeats: 2 + int(minRep%6), MaxPeriod: 1}
		a, b := Consecutive{}.Detect(keys, o), Cycle{}.Detect(keys, o)
		if !slices.Equal(a, b) {
			t.Fatalf("consecutive %v != cycle(p=1) %v", a, b)
		}
	})
}

// 5 + 6. Planted signal: random repeating blocks of volatile lines with a
// unique line (new word) and a rare-value line (constant slot changes once)
// planted inside. Neither may ever be hidden by the default engine.
func FuzzPlantedSignal(f *testing.F) {
	f.Add(uint64(1), uint8(2), uint16(300), uint16(150), uint16(77))
	f.Fuzz(func(t *testing.T, seed uint64, period uint8, count uint16, uniqueAt, rareAt uint16) {
		rng := rand.New(rand.NewPCG(seed, 9))
		p := 1 + int(period%12)
		c := 10 + int(count%500)
		templates := make([]string, p)
		for i := range templates {
			templates[i] = fmt.Sprintf("%s %s status=%%d id=%%s took %%dms pool 4 code 0", words[rng.IntN(len(words))], words[rng.IntN(len(words))])
		}
		var lines []string
		for cy := range c {
			for _, tpl := range templates {
				lines = append(lines, fmt.Sprintf(tpl, 200, fmt.Sprintf("%08x", rng.Uint32()), rng.IntN(999)))
			}
			_ = cy
		}
		u := int(uniqueAt) % len(lines)
		r := int(rareAt) % len(lines)
		if r == u {
			r = (r + 1) % len(lines)
		}
		lines[u] = "plantedsignalword " + lines[u]
		lines[r] = strings.Replace(lines[r], "code 0", "code 137", 1) // semantic slot
		q := (r + 7) % len(lines)
		if q == u || q == r {
			q = (q + 1) % len(lines)
		}
		lines[q] = strings.Replace(lines[q], "pool 4", "pool 9", 1) // plain slot
		e, _ := Build("shape", "set", "structural,outlier", DefaultOptions())
		plan, err := e.SafePlan(lines, DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		hidden := plan.HiddenMask()
		if hidden[u] {
			t.Fatalf("planted unique line %d hidden (p=%d c=%d)", u, p, c)
		}
		if hidden[r] {
			t.Fatalf("planted rare semantic value line %d hidden (p=%d c=%d)", r, p, c)
		}
		// A rare plain value may be hidden only inside a fold whose marker
		// names it (tiny groups, where "rare" is statistically undefined).
		if hidden[q] {
			for _, seg := range plan.Segments {
				if seg.Kind == Folded && seg.Start <= q && q < seg.End {
					if m := Summarize(seg, lines, e.Normalizer).Marker(); !strings.Contains(m, "4|9") {
						t.Fatalf("rare plain value line %d hidden and not in marker %q (p=%d c=%d)", q, m, p, c)
					}
				}
			}
		}
	})
}

var words = []string{"alpha", "bravo", "cache", "deploy", "engine", "fetch", "graph", "index", "queue", "router"}
