package fold

// Set folds stretches made of a few shapes recurring in any order (pollers
// cycling through resources, interleaved workers) and, region by region,
// prefers Cycle's runs when they explain the region. A shape recurs if it
// appears again within Window lines; a stretch grows while it needs at most
// MaxShapes shapes and sees at most MaxGap non-recurring lines in a row, and
// is kept when it is at least MinLen lines and half of it would fold. A
// rejected stretch advances the scan by half its length, keeping it O(n).
type Set struct {
	MaxShapes int
	Window    int
	MaxGap    int
	MinLen    int
}

func NewSet() Set { return Set{MaxShapes: 8, Window: 64, MaxGap: 3, MinLen: 20} }

func (Set) Name() string { return "set" }

func (s Set) Detect(keys []int, opts Options) []Run {
	cycles := Cycle{}.Detect(keys, opts)
	if opts.MaxShapes > 0 {
		s.MaxShapes = opts.MaxShapes
	}
	sets := s.detectSets(keys, opts.Stop)
	// A set mostly (>= 80%) covered by cycles yields to them; otherwise it
	// replaces the cycles it overlaps.
	var keptSets []Run
	c := 0
	for _, set := range sets {
		for c < len(cycles) && cycles[c].End() <= set.Start {
			c++
		}
		covered := 0
		for k := c; k < len(cycles) && cycles[k].Start < set.End(); k++ {
			covered += min(cycles[k].End(), set.End()) - max(cycles[k].Start, set.Start)
		}
		if covered*5 < set.Count*4 {
			keptSets = append(keptSets, set)
		}
	}
	var keptCycles []Run
	k := 0
	for _, cy := range cycles {
		for k < len(keptSets) && keptSets[k].End() <= cy.Start {
			k++
		}
		if k < len(keptSets) && keptSets[k].Start < cy.End() {
			continue // overlaps a kept set
		}
		keptCycles = append(keptCycles, cy)
	}
	return mergeRuns(keptCycles, keptSets)
}

func (s Set) detectSets(keys []int, stop func() bool) []Run {
	sc := stopCheck{stop: stop}
	n := len(keys)
	next := make([]int, n)
	last := map[int]int{}
	for i := n - 1; i >= 0; i-- {
		if sc.tick() {
			return nil
		}
		next[i] = n + s.Window + 1
		if k := keys[i]; k >= 0 {
			if j, ok := last[k]; ok {
				next[i] = j
			}
			last[k] = i
		}
	}
	recurs := func(i int) bool { return keys[i] >= 0 && next[i]-i <= s.Window }

	var runs []Run
	for i := 0; i < n; {
		if sc.tick() {
			return nil
		}
		if !recurs(i) {
			i++
			continue
		}
		counts := map[int]int{}
		gap, end := 0, i
		for j := i; j < n; j++ {
			if sc.tick() {
				return nil
			}
			k := keys[j]
			if k < 0 {
				break // pinned: a stretch never spans a protected line
			}
			if counts[k] == 0 && !recurs(j) {
				gap++
				if gap > s.MaxGap {
					break
				}
				continue
			}
			if counts[k] == 0 && len(counts) == s.MaxShapes {
				break
			}
			counts[k]++
			gap = 0
			end = j + 1
		}
		length := end - i
		hidden := 0
		for _, c := range counts {
			hidden += c - 1
		}
		if length >= s.MinLen && 2*hidden >= length {
			runs = append(runs, Run{Start: i, Period: 1, Count: length, Set: true})
			i = end
			continue
		}
		i += max(1, length/2)
	}
	return runs
}

func mergeRuns(a, b []Run) []Run {
	out := make([]Run, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if j == len(b) || i < len(a) && a[i].Start < b[j].Start {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}
	return out
}
