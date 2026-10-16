// Package fold folds repeating log lines into a Plan: a projection over the
// lines that always expands back to the input. A Normalizer maps lines to
// shapes, Guards pin lines, a Detector finds repeating runs and Refiners pin
// lines whose values differ; an Engine wires them together.
package fold

// Normalizer maps a line to its shape; equal shapes are the same kind of line.
type Normalizer interface {
	Name() string
	Normalize(line string) string
}

// Slotter returns the values a Normalizer masked, in order.
type Slotter interface {
	Slots(line string) []Slot
}

// KeySpanner normalizes and returns masked values as offsets in one pass.
type KeySpanner interface {
	KeyAndSpans(dst []Span, line string) (string, []Span)
}

// Span is a masked value's [Start, End) byte range within its line.
type Span struct{ Start, End int32 }

type Slot struct {
	Class string
	Value string
}

// Guard pins lines that must never be folded.
type Guard interface {
	Name() string
	Protect(line string) bool
}

// Detector finds non-overlapping runs in interned shape keys. Pinned lines
// arrive as unique negative keys, so they can never be folded.
type Detector interface {
	Name() string
	Detect(keys []int, opts Options) []Run
}

// Run is a block of Period lines repeated Count times from Start (0-based).
// A Set run (Period 1, Count = length) mixes a few recurring shapes in any
// order; folding keeps the first line of each shape visible.
type Run struct {
	Start  int
	Period int
	Count  int
	Set    bool
}

func (r Run) End() int { return r.Start + r.Period*r.Count }

type Options struct {
	MinRepeats int
	MaxPeriod  int // longest block a detector considers
	KeepHead   int // cycles kept visible before a fold (at least 1)
	KeepTail   int // cycles kept visible after a fold
	MaxShapes  int // set detector: most recurring shapes per stretch
	// Stop is polled every few thousand steps; true abandons the work.
	Stop func() bool
}

type stopCheck struct {
	stop  func() bool
	ticks uint
	hit   bool
}

func (s *stopCheck) tick() bool {
	if s.hit {
		return true
	}
	s.ticks++
	if s.stop != nil && s.ticks&4095 == 0 && s.stop() {
		s.hit = true
	}
	return s.hit
}

func DefaultOptions() Options {
	return Options{MinRepeats: 3, MaxPeriod: 32, KeepHead: 1, KeepTail: 1, MaxShapes: 32}
}
