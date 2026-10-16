package usecase

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/logs/fold"
)

const (
	excerptTailLines  = 8
	maxRepeats        = 5
	excerptFoldBudget = time.Second
)

// Repeat is one folded run of repeating lines in a job log.
type Repeat struct {
	StartLine int
	EndLine   int
	Cycles    int
	Period    int
	Hidden    int
	From      string
	To        string
	Summary   string
}

// excerpt is the failure window (or the log tail) with folded repeats shown
// as one "⋯ summary" row each; the window then reaches back so it keeps as
// many visible rows as it had lines. repeats are the largest folds.
func excerpt(ctx context.Context, document logs.Document) (string, []Repeat) {
	first, last := tailWindow(document)
	if document.Failure.Found {
		first, last = document.Failure.StartLine, document.Failure.EndLine
	}
	ctx, cancel := context.WithTimeout(ctx, excerptFoldBudget)
	defer cancel()
	plan, err := fold.ForDocument(ctx, document, fold.Default())
	if err != nil || plan.Rows() == len(plan.Lines) {
		return joinExcerpt(document, first, last, nil), nil
	}
	return joinExcerpt(document, first, last, &plan), repeatsOf(plan)
}

func tailWindow(document logs.Document) (int, int) {
	n := len(document.Lines)
	return max(1, n-excerptTailLines+1), n
}

func joinExcerpt(document logs.Document, first, last int, plan *fold.Plan) string {
	if plan == nil {
		parts := make([]string, 0, last-first+1)
		for _, line := range document.Lines[max(0, first-1):last] {
			parts = append(parts, logs.StripTimestamp(line.Text))
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	type row struct{ text string }
	var rows []row
	end := -1
	for _, seg := range plan.Segments {
		if seg.Start >= last {
			break
		}
		if seg.Kind == fold.Folded {
			rows = append(rows, row{fold.Summarize(seg, plan.Lines, fold.Shape{}).Marker()})
		} else {
			for i := seg.Start; i < min(seg.End, last); i++ {
				rows = append(rows, row{logs.StripTimestamp(plan.Lines[i])})
			}
		}
		end = len(rows)
	}
	want := last - first + 1
	start := max(0, end-want)
	parts := make([]string, 0, end-start)
	for _, r := range rows[start:end] {
		parts = append(parts, r.text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func repeatsOf(plan fold.Plan) []Repeat {
	var out []Repeat
	for _, seg := range plan.Segments {
		if seg.Kind != fold.Folded {
			continue
		}
		s := fold.Summarize(seg, plan.Lines, fold.Shape{})
		from, to := s.Span()
		cycles, period := seg.Run.Count, seg.Run.Period
		if seg.Run.Set {
			cycles, period = 0, s.Shapes
		}
		out = append(out, Repeat{
			StartLine: seg.Start + 1, EndLine: seg.End, Cycles: cycles, Period: period,
			Hidden: seg.End - seg.Start, From: from, To: to, Summary: s.Title(),
		})
	}
	slices.SortStableFunc(out, func(a, b Repeat) int { return cmp.Compare(b.Hidden, a.Hidden) })
	if len(out) > maxRepeats {
		out = out[:maxRepeats]
	}
	return out
}
