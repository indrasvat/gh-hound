package fold

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

func benchLines(n int) []string {
	rng := rand.New(rand.NewPCG(3, 4))
	lines := make([]string, 0, n)
	for len(lines) < n {
		if rng.IntN(4) == 0 {
			for range 50 + rng.IntN(200) {
				lines = append(lines,
					fmt.Sprintf("2026-10-16T10:%02d:%02d.%07dZ INFO[2026-10-15 18:16:52] polling status=%d", rng.IntN(60), rng.IntN(60), rng.IntN(1e7), 200),
					fmt.Sprintf("2026-10-16T10:%02d:%02d.%07dZ state :: &{ 0x%012x 0x%012x }", rng.IntN(60), rng.IntN(60), rng.IntN(1e7), rng.Uint64()>>16, rng.Uint64()>>16))
			}
		} else {
			lines = append(lines, fmt.Sprintf("2026-10-16T10:00:00.0000000Z compiling pkg/%d/file_%d.go with flags -O%d", rng.IntN(1e6), rng.IntN(1e6), rng.IntN(3)))
		}
	}
	return lines[:n]
}

func BenchmarkSafePlan(b *testing.B) {
	lines := benchLines(200_000)
	size := 0
	for _, l := range lines {
		size += len(l) + 1
	}
	for _, cfg := range []struct{ n, d, g string }{
		{"shape", "set", "structural,outlier"}, // the default
		{"shape", "cycle", "structural"},
		{"shape", "cycle", "structural,outlier"},
	} {
		b.Run(cfg.n+"/"+cfg.d+"/"+cfg.g, func(b *testing.B) {
			o := DefaultOptions()
			e, _ := Build(cfg.n, cfg.d, cfg.g, o)
			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := e.SafePlan(lines, DefaultLimits()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Worst case for the cycle detector: no repeats at all, so every position
// tries every period.
func BenchmarkCycleWorstCase(b *testing.B) {
	keys := make([]int, 1_000_000)
	for i := range keys {
		keys[i] = i
	}
	o := Options{MinRepeats: 3, MaxPeriod: 32}
	for b.Loop() {
		Cycle{}.Detect(keys, o)
	}
}
