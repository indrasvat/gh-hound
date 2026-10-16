package fold

import (
	"context"

	"github.com/indrasvat/gh-hound/internal/logs"
)

// Default is the engine the product uses.
func Default() Engine {
	e, err := Build("shape", "set", "structural,outlier", DefaultOptions())
	if err != nil {
		panic(err)
	}
	return e
}

// ForDocument plans a parsed log. Step-group lines and the first line after
// each group are pinned, so no fold crosses a group boundary.
func ForDocument(ctx context.Context, doc logs.Document, e Engine) (Plan, error) {
	return ForRange(ctx, doc, 1, len(doc.Lines), e)
}

// ForRange plans lines first..last (1-based, inclusive); plan indices are
// relative to first.
func ForRange(ctx context.Context, doc logs.Document, first, last int, e Engine) (Plan, error) {
	first, last = max(1, first), min(len(doc.Lines), last)
	if last < first {
		return Identity(nil), nil
	}
	lines := make([]string, 0, last-first+1)
	for _, line := range doc.Lines[first-1 : last] {
		lines = append(lines, line.Text)
	}
	pins := make(map[int]bool, len(e.Pins)+2*len(doc.Folds))
	for i := range e.Pins {
		pins[i] = true
	}
	for _, f := range doc.Folds {
		pins[f.StartLine-first] = true
		pins[f.EndLine+1-first] = true
	}
	e.Pins = pins
	return e.SafePlanContext(ctx, lines, DefaultLimits())
}
