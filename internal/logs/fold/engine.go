package fold

// Engine composes one Normalizer, one Detector, and any Guards and Refiners.
type Engine struct {
	Normalizer Normalizer
	Detector   Detector
	Guards     []Guard
	Refiners   []Refiner
	Options    Options
	Pins       map[int]bool // line indices kept visible regardless of content
}

type SegmentKind int

const (
	Literal SegmentKind = iota // source lines shown as-is
	Folded                     // source lines hidden behind one marker row
)

// Segment is the span [Start, End) of source lines.
type Segment struct {
	Kind  SegmentKind
	Start int
	End   int
	Run   Run // the run that produced this fold (Folded only)
}

type Plan struct {
	Lines    []string
	Keys     []string
	Pinned   []bool // protected by a guard or refiner: never hidden
	Segments []Segment
	Runs     []Run
}

func (e Engine) Plan(lines []string) Plan {
	opts := e.Options
	if opts.MinRepeats < 2 {
		opts.MinRepeats = 2
	}
	if opts.MaxPeriod < 1 {
		opts.MaxPeriod = 1
	}
	if opts.KeepHead < 1 {
		opts.KeepHead = 1 // safety rule: every fold needs a visible example
	}
	if opts.KeepTail < 0 {
		opts.KeepTail = 0
	}

	keys := make([]string, len(lines))
	ids := make([]int, len(lines))
	in := &RefineInput{Lines: lines, Keys: keys, Normalizer: e.Normalizer, Stop: opts.Stop}
	fused, canFuse := e.Normalizer.(KeySpanner)
	if canFuse && len(e.Refiners) > 0 {
		in.spans = make([][]Span, len(lines))
		in.arena = make([]Span, 0, 4*len(lines))
	}
	intern := make(map[string]int, len(lines)/4+1)
	sc := stopCheck{stop: opts.Stop}
	for i, line := range lines {
		if sc.tick() {
			return Identity(lines)
		}
		if e.Pins[i] || e.protected(line) {
			ids[i] = -1 - i // unique, can never match another line
			keys[i] = line
			continue
		}
		var key string
		if in.spans != nil {
			start := len(in.arena)
			key, in.arena = fused.KeyAndSpans(in.arena, line)
			in.spans[i] = in.arena[start:len(in.arena):len(in.arena)]
		} else {
			key = e.Normalizer.Normalize(line)
		}
		keys[i] = key
		id, ok := intern[key]
		if !ok {
			id = len(intern)
			intern[key] = id
		}
		ids[i] = id
	}

	runs := e.refineToFixpoint(in, ids, opts)
	if opts.Stop != nil && opts.Stop() {
		return Identity(lines)
	}
	pinned := make([]bool, len(ids))
	for i, id := range ids {
		pinned[i] = id < 0
	}
	return Plan{Lines: lines, Keys: keys, Pinned: pinned, Runs: runs, Segments: fold(ids, runs, opts)}
}

const maxRefineRounds = 8

// refineToFixpoint re-detects until no refiner objects to any run; runs
// still objected to after maxRefineRounds are left unfolded.
func (e Engine) refineToFixpoint(in *RefineInput, ids []int, opts Options) []Run {
	runs := e.Detector.Detect(ids, opts)
	for round := 0; round < maxRefineRounds && len(e.Refiners) > 0; round++ {
		flagged := e.refine(in, runs)
		pinnedAny := false
		for _, i := range flagged {
			if ids[i] >= 0 {
				ids[i] = -1 - i
				pinnedAny = true
			}
		}
		if !pinnedAny {
			return runs
		}
		runs = e.Detector.Detect(ids, opts)
	}
	if len(e.Refiners) == 0 {
		return runs
	}
	objected := map[int]bool{}
	for _, i := range e.refine(in, runs) {
		objected[i] = true
	}
	kept := runs[:0]
	for _, r := range runs {
		ok := true
		for i := r.Start; i < r.End() && ok; i++ {
			ok = !objected[i]
		}
		if ok {
			kept = append(kept, r)
		}
	}
	return kept
}

func (e Engine) refine(in *RefineInput, runs []Run) []int {
	in.Runs = runs
	var out []int
	for _, r := range e.Refiners {
		out = append(out, r.Refine(in)...)
	}
	return out
}

func (e Engine) protected(line string) bool {
	for _, g := range e.Guards {
		if g.Protect(line) {
			return true
		}
	}
	return false
}

// fold turns runs into segments. Cycle runs keep head and tail cycles; set
// runs keep the first line of each shape and the last line. A fold hides at
// least two lines.
func fold(ids []int, runs []Run, opts Options) []Segment {
	var segs []Segment
	cursor := 0
	emitLiteral := func(end int) {
		if end > cursor {
			segs = append(segs, Segment{Kind: Literal, Start: cursor, End: end})
			cursor = end
		}
	}
	emitFold := func(start, end int, r Run) {
		emitLiteral(start)
		segs = append(segs, Segment{Kind: Folded, Start: start, End: end, Run: r})
		cursor = end
	}
	for _, r := range runs {
		if r.Set {
			seen := map[int]bool{}
			hideFrom := -1
			flush := func(i int) {
				if hideFrom >= 0 && i-hideFrom >= 2 {
					emitFold(hideFrom, i, r)
				}
				hideFrom = -1
			}
			last := r.End() - 1
			for i := r.Start; i <= last; i++ {
				k := ids[i]
				hide := k >= 0 && seen[k] && i != last
				seen[k] = true
				if hide {
					if hideFrom < 0 {
						hideFrom = i
					}
					continue
				}
				flush(i)
			}
			flush(last)
			continue
		}
		hiddenCycles := r.Count - opts.KeepHead - opts.KeepTail
		if hiddenCycles < 1 || hiddenCycles*r.Period < 2 {
			continue
		}
		foldStart := r.Start + opts.KeepHead*r.Period
		emitFold(foldStart, foldStart+hiddenCycles*r.Period, r)
	}
	emitLiteral(len(ids))
	return segs
}

// Rows counts visible rows: literal lines plus one per fold.
func (p Plan) Rows() int {
	rows := 0
	for _, s := range p.Segments {
		if s.Kind == Folded {
			rows++
		} else {
			rows += s.End - s.Start
		}
	}
	return rows
}

func (p Plan) HiddenMask() []bool {
	mask := make([]bool, len(p.Lines))
	for _, s := range p.Segments {
		if s.Kind == Folded {
			for i := s.Start; i < s.End; i++ {
				mask[i] = true
			}
		}
	}
	return mask
}

// Expand reconstructs the source from the segments.
func (p Plan) Expand() []string {
	out := make([]string, 0, len(p.Lines))
	for _, s := range p.Segments {
		out = append(out, p.Lines[s.Start:s.End]...)
	}
	return out
}
