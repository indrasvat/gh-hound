package fold

// Refiner names lines inside detected runs that must stay visible; the
// engine pins them and detects again.
type Refiner interface {
	Name() string
	Refine(in *RefineInput) []int
}

type RefineInput struct {
	Lines      []string
	Keys       []string
	Runs       []Run
	Normalizer Normalizer
	Stop       func() bool
	spans      [][]Span // captured during normalization by a KeySpanner
	arena      []Span
	slots      [][]Slot // Slotter fallback, computed on demand
}

// ValuesAt returns line i's masked values.
func (in *RefineInput) ValuesAt(i int, dst []string) []string {
	dst = dst[:0]
	if in.spans != nil {
		for _, sp := range in.spans[i] {
			dst = append(dst, in.Lines[i][sp.Start:sp.End])
		}
		return dst
	}
	if in.slots == nil {
		in.slots = make([][]Slot, len(in.Lines))
	}
	if in.slots[i] == nil {
		if s, ok := in.Normalizer.(Slotter); ok {
			in.slots[i] = append(s.Slots(in.Lines[i]), Slot{}) // sentinel marks "computed"
		} else {
			in.slots[i] = []Slot{{}}
		}
	}
	for _, sl := range in.slots[i][:len(in.slots[i])-1] {
		dst = append(dst, sl.Value)
	}
	return dst
}

// Outlier keeps changes visible in low-cardinality slots (at most MaxValues
// values among comparable lines): a rare value ("exit code 137" among "exit
// code 0") and the first line of each plateau when a slot changes only a few
// times ("1 of 3" -> "2 of 3"). Slots that flip constantly are left to the
// fold summary.
type Outlier struct {
	MinCycles int
	Minority  float64 // share of lines a rare value may cover (at least one)
	MaxValues int
}

func NewOutlier() Outlier { return Outlier{MinCycles: 3, Minority: 0.1, MaxValues: 4} }

func (Outlier) Name() string { return "outlier" }

func (o Outlier) Refine(in *RefineInput) []int {
	_, spanner := in.Normalizer.(KeySpanner)
	if _, slotter := in.Normalizer.(Slotter); !slotter && !spanner {
		return nil
	}
	var protect []int
	for _, r := range in.Runs {
		if in.Stop != nil && in.Stop() {
			return nil
		}
		if !r.Set {
			// compare by cycle position, even when positions share a shape
			for pos := range r.Period {
				members := make([]int, 0, r.Count)
				for c := range r.Count {
					members = append(members, r.Start+c*r.Period+pos)
				}
				protect = append(protect, o.outliers(members, in)...)
			}
			continue
		}
		groups := map[string][]int{}
		var order []string
		for i := r.Start; i < r.End(); i++ {
			g := in.Keys[i]
			if groups[g] == nil {
				order = append(order, g)
			}
			groups[g] = append(groups[g], i)
		}
		for _, g := range order {
			protect = append(protect, o.outliers(groups[g], in)...)
		}
	}
	return protect
}

func (o Outlier) outliers(members []int, in *RefineInput) []int {
	if len(members) < o.MinCycles {
		return nil
	}
	slots := make([][]string, len(members))
	for c, i := range members {
		slots[c] = in.ValuesAt(i, nil)
	}
	var protect []int
	for idx := range slots[0] {
		counts := map[string]int{}
		for c := range members {
			if idx < len(slots[c]) {
				counts[slots[c][idx]]++
			}
			if len(counts) > o.MaxValues {
				break
			}
		}
		if len(counts) < 2 || len(counts) > o.MaxValues {
			continue
		}
		allowed := max(1, int(o.Minority*float64(len(members))))
		var changes []int
		for c := 1; c < len(members); c++ {
			if idx < len(slots[c]) && idx < len(slots[c-1]) && slots[c][idx] != slots[c-1][idx] {
				changes = append(changes, members[c])
			}
		}
		if len(changes) <= max(2, allowed) {
			protect = append(protect, changes...)
		}
		// a tie means no value is rare
		ref, top, tied := "", 0, false
		for v, n := range counts {
			switch {
			case n > top:
				ref, top, tied = v, n, false
			case n == top:
				tied = true
			}
		}
		if tied {
			continue
		}
		if top < len(members)-allowed {
			continue
		}
		for c, i := range members {
			if idx < len(slots[c]) && slots[c][idx] != ref {
				protect = append(protect, i)
			}
		}
	}
	return protect
}
