package fold

// Consecutive folds runs of one repeated shape.
type Consecutive struct{}

func (Consecutive) Name() string { return "consecutive" }

func (Consecutive) Detect(keys []int, opts Options) []Run {
	var runs []Run
	sc := stopCheck{stop: opts.Stop}
	for i := 0; i < len(keys); {
		if sc.tick() {
			return nil
		}
		j := i + 1
		for j < len(keys) && keys[j] == keys[i] && keys[i] >= 0 {
			if sc.tick() {
				return nil
			}
			j++
		}
		if j-i >= opts.MinRepeats {
			runs = append(runs, Run{Start: i, Period: 1, Count: j - i})
		}
		i = j
	}
	return runs
}

// Cycle folds blocks of 1..MaxPeriod lines repeating back to back, choosing
// at each position the period covering the most lines (smallest on ties).
type Cycle struct{}

func (Cycle) Name() string { return "cycle" }

func (Cycle) Detect(keys []int, opts Options) []Run {
	var runs []Run
	n := len(keys)
	sc := stopCheck{stop: opts.Stop}
	for i := 0; i < n; {
		if sc.tick() {
			return nil
		}
		best := Run{Start: i}
		for p := 1; p <= opts.MaxPeriod && i+2*p <= n; p++ {
			if !usableBlock(keys[i : i+p]) {
				break // a protected line inside the block poisons longer periods too
			}
			count := 1
			for next := i + count*p; next+p <= n && equalBlock(keys, i, next, p); next = i + count*p {
				if sc.tick() {
					return nil
				}
				count++
			}
			if count >= opts.MinRepeats && count*p > best.Count*best.Period {
				best = Run{Start: i, Period: p, Count: count}
			}
		}
		if best.Count > 0 {
			runs = append(runs, best)
			i = best.End()
			continue
		}
		i++
	}
	return runs
}

func usableBlock(block []int) bool {
	for _, k := range block {
		if k < 0 {
			return false
		}
	}
	return true
}

func equalBlock(keys []int, a, b, p int) bool {
	for k := range p {
		if keys[a+k] != keys[b+k] {
			return false
		}
	}
	return true
}
