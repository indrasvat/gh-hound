package fold

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// Limits bound SafePlan's work; inputs beyond them get the raw view.
type Limits struct {
	MaxLineBytes int // longer lines are pinned visible, never folded
	MaxLines     int // larger inputs are returned raw
}

func DefaultLimits() Limits { return Limits{MaxLineBytes: 16 << 10, MaxLines: 5_000_000} }

var ErrFallback = errors.New("fold: fell back to raw view")

// Identity is the raw view: nothing hidden. Keys share lines (read-only).
func Identity(lines []string) Plan {
	p := Plan{Lines: lines, Keys: lines, Pinned: make([]bool, len(lines))}
	if len(lines) > 0 {
		p.Segments = []Segment{{Kind: Literal, Start: 0, End: len(lines)}}
	}
	return p
}

// SafePlanContext is SafePlan that returns the raw view once ctx is done.
func (e Engine) SafePlanContext(ctx context.Context, lines []string, lim Limits) (Plan, error) {
	e.Options.Stop = func() bool { return ctx.Err() != nil }
	p, err := e.SafePlan(lines, lim)
	if ctx.Err() != nil {
		return Identity(lines), fmt.Errorf("%w: %v", ErrFallback, ctx.Err())
	}
	return p, err
}

// SafePlan never panics and never returns a plan that loses data: on any
// failure it returns the raw view and an error wrapping ErrFallback.
func (e Engine) SafePlan(lines []string, lim Limits) (plan Plan, err error) {
	if lim.MaxLines > 0 && len(lines) > lim.MaxLines {
		return Identity(lines), fmt.Errorf("%w: %d lines exceeds limit %d", ErrFallback, len(lines), lim.MaxLines)
	}
	defer func() {
		if r := recover(); r != nil {
			plan, err = Identity(lines), fmt.Errorf("%w: panic: %v", ErrFallback, r)
		}
	}()
	guarded := e
	if lim.MaxLineBytes > 0 {
		guarded.Guards = append(slices.Clone(e.Guards), longLine(lim.MaxLineBytes))
	}
	plan = guarded.Plan(lines)
	if stop := e.Options.Stop; stop != nil && stop() {
		// stopped: the raw view needs no verification
		return Identity(lines), fmt.Errorf("%w: stopped", ErrFallback)
	}
	if verr := verify(plan, lines); verr != nil {
		return Identity(lines), fmt.Errorf("%w: %v", ErrFallback, verr)
	}
	return plan, nil
}

type longLine int

func (longLine) Name() string               { return "long-line" }
func (l longLine) Protect(line string) bool { return len(line) > int(l) }

// verify re-checks the plan in O(n): segments tile the input, hidden lines
// are unpinned and witnessed, and expansion reproduces the input.
func verify(p Plan, lines []string) error {
	hidden := p.HiddenMask()
	cursor := 0
	for _, s := range p.Segments {
		if s.Start != cursor || s.End <= s.Start || s.End > len(lines) {
			return fmt.Errorf("segment %+v does not tile at %d", s, cursor)
		}
		cursor = s.End
		if s.Kind != Folded {
			continue
		}
		r := s.Run
		if r.Period < 1 || s.Start < r.Start || s.End > r.End() {
			return fmt.Errorf("fold %+v outside run %+v", s, r)
		}
	}
	if cursor != len(lines) {
		return fmt.Errorf("segments cover %d of %d lines", cursor, len(lines))
	}
	if err := witnessed(p, hidden); err != nil {
		return err
	}
	if !slices.Equal(p.Expand(), lines) {
		return errors.New("expand != input")
	}
	return nil
}

// witnessed checks that every hidden line has a visible line of the same
// shape earlier in its run: the matching line of a cycle's kept first cycle,
// or any visible line since a set run began.
func witnessed(p Plan, hidden []bool) error {
	runOf := make(map[int]Run) // fold start -> run, to find each hidden line's run
	for _, s := range p.Segments {
		if s.Kind == Folded {
			runOf[s.Start] = s.Run
		}
	}
	var cur Run
	var visible map[string]bool
	for i := range p.Lines {
		if r, ok := runOf[i]; ok && r != cur {
			cur = r
			visible = nil
			if r.Set {
				visible = map[string]bool{}
				for j := r.Start; j < i; j++ {
					if !hidden[j] {
						visible[p.Keys[j]] = true
					}
				}
			}
		}
		if cur.Set && i >= cur.Start && i < cur.End() && !hidden[i] && visible != nil {
			visible[p.Keys[i]] = true
		}
		if !hidden[i] {
			continue
		}
		if p.Pinned[i] {
			return fmt.Errorf("pinned line %d hidden", i+1)
		}
		if i < cur.Start || i >= cur.End() {
			return fmt.Errorf("hidden line %d outside its run", i+1)
		}
		if cur.Set {
			if !visible[p.Keys[i]] {
				return fmt.Errorf("line %d hidden with no visible same-shape line above it", i+1)
			}
			continue
		}
		ref := cur.Start + (i-cur.Start)%cur.Period
		if hidden[ref] || p.Keys[i] != p.Keys[ref] {
			return fmt.Errorf("line %d has no visible same-shape twin in its cycle", i+1)
		}
	}
	return nil
}
