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
	lines := make([]string, len(doc.Lines))
	for i, line := range doc.Lines {
		lines[i] = line.Text
	}
	pins := make(map[int]bool, 2*len(doc.Folds))
	for i := range e.Pins {
		pins[i] = true
	}
	for _, f := range doc.Folds {
		pins[f.StartLine-1] = true
		pins[f.EndLine] = true
	}
	e.Pins = pins
	return e.SafePlanContext(ctx, lines, DefaultLimits())
}
