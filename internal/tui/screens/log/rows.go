package log

import (
	"context"
	"sort"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs/fold"
)

const (
	// planBudget bounds folding a whole log off the input path.
	planBudget = 2 * time.Second
	// rangePlanBudget bounds planning a time range on the key path.
	rangePlanBudget = 50 * time.Millisecond
)

type rowKind uint8

const (
	rowLine rowKind = iota
	rowGroup
	rowRepeat
)

// viewRow is one screen row. line is its anchor: the line number for line
// and group rows, the first hidden line for repeat rows.
type viewRow struct {
	kind rowKind
	line int
	end  int
	seg  int
	open bool
}

// rowCache is shared by copies of a Model; rev identifies the state the
// rows were built for.
type rowCache struct {
	rev  int
	plan *fold.Plan
	rows []viewRow
}

// SetPlan attaches a fold plan computed off the input path. compact picks
// the initial view.
func (m Model) SetPlan(plan fold.Plan, compact bool) Model {
	m.plan = &plan
	m.Compact = compact
	m.rangePlan = nil
	if m.RangeLabel != "" {
		m.planRange()
	}
	m.rev++
	return m
}

// Fold plans the document within ctx and planBudget; on any fallback the
// model keeps the full log. Call it off the input path.
func Fold(ctx context.Context, m Model) Model {
	ctx, cancel := context.WithTimeout(ctx, planBudget)
	defer cancel()
	plan, err := fold.ForDocument(ctx, m.Document, fold.Default())
	if err != nil {
		return m
	}
	return m.SetPlan(plan, true)
}

// WithCompact sets the initial view; compact needs a plan.
func (m Model) WithCompact(on bool) Model {
	m.Compact = on && m.plan != nil
	m.topRow = -1
	m.rev++
	return m
}

func (m Model) HasPlan() bool { return m.plan != nil }

// FoldCount is how many repeat folds the full-log plan has.
func (m Model) FoldCount() int {
	if m.plan == nil {
		return 0
	}
	n := 0
	for _, s := range m.plan.Segments {
		if s.Kind == fold.Folded {
			n++
		}
	}
	return n
}

// CompactRows is the row count of the compact view with every fold closed.
func (m Model) CompactRows() int {
	if m.plan == nil {
		return len(m.Document.Lines)
	}
	return m.plan.Rows()
}

// Folded reports whether the compact view is showing and has folds.
func (m Model) Folded() bool { return m.Compact && m.activePlan() != nil && m.FoldCount() > 0 }

func (m Model) activePlan() *fold.Plan {
	if !m.Compact {
		return nil
	}
	if m.RangeLabel != "" {
		return m.rangePlan
	}
	return m.plan
}

func (m *Model) planRange() {
	m.rangePlan = nil
	if m.plan == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rangePlanBudget)
	defer cancel()
	p, err := fold.ForRange(ctx, m.Document, m.RangeStart, m.RangeEnd, fold.Default())
	if err == nil {
		m.rangePlan = &p
	}
}

func (m Model) rows() []viewRow {
	if m.cache == nil {
		return m.buildRows()
	}
	if m.cache.rows == nil || m.cache.rev != m.rev || m.cache.plan != m.activePlan() {
		m.cache.rows, m.cache.rev, m.cache.plan = m.buildRows(), m.rev, m.activePlan()
	}
	return m.cache.rows
}

func (m Model) buildRows() []viewRow {
	first, last := 1, len(m.Document.Lines)
	if m.RangeLabel != "" {
		first, last = max(1, m.RangeStart), min(last, m.RangeEnd)
	}
	rows := make([]viewRow, 0, max(0, last-first+1))
	skip := 0
	emit := func(n int) {
		if n <= skip {
			return
		}
		if g, ok := m.groups[n]; ok {
			collapsed := m.collapsed[n]
			rows = append(rows, viewRow{kind: rowGroup, line: n, end: g.EndLine, open: !collapsed})
			if collapsed {
				skip = g.EndLine
			}
			return
		}
		rows = append(rows, viewRow{kind: rowLine, line: n})
	}
	plan := m.activePlan()
	if plan == nil {
		for n := first; n <= last; n++ {
			emit(n)
		}
		return rows
	}
	base := first
	if m.RangeLabel == "" {
		base = 1
	}
	for i, seg := range plan.Segments {
		a, b := seg.Start+base, seg.End+base-1
		if seg.Kind == fold.Literal {
			for n := a; n <= b; n++ {
				emit(n)
			}
			continue
		}
		if b <= skip {
			continue
		}
		open := m.open[a]
		rows = append(rows, viewRow{kind: rowRepeat, line: a, end: b, seg: i, open: open})
		if open {
			for n := a; n <= b; n++ {
				emit(n)
			}
		}
	}
	return rows
}

// rowIndexFor is the row showing line, or the closed fold that hides it.
func rowIndexFor(rows []viewRow, line int) int {
	if len(rows) == 0 {
		return 0
	}
	i := sort.Search(len(rows), func(i int) bool { return rows[i].line >= line })
	if i > 0 {
		prev := rows[i-1]
		hides := (prev.kind == rowRepeat || prev.kind == rowGroup) && !prev.open && line <= prev.end
		if hides && prev.line < line {
			return i - 1
		}
	}
	return min(i, len(rows)-1)
}

func (m Model) top() int {
	rows := m.rows()
	if m.topRow >= 0 && m.topRow < len(rows) && rows[m.topRow].line == m.Offset {
		return m.topRow
	}
	return rowIndexFor(rows, m.Offset)
}

func (m Model) maxTopRow() int {
	return max(0, len(m.rows())-max(m.Height, 1))
}

func (m *Model) scrollTo(row int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	row = min(max(0, row), m.maxTopRow())
	m.topRow = row
	m.Offset = rows[row].line
}

// reveal opens the repeat fold or step group hiding line.
func (m *Model) reveal(line int) {
	changed := false
	if plan := m.activePlan(); plan != nil {
		for _, r := range m.rows() {
			if r.kind == rowRepeat && !r.open && r.line <= line && line <= r.end {
				m.open[r.line] = true
				changed = true
			}
		}
	}
	for _, f := range m.Document.Folds {
		if m.collapsed[f.StartLine] && f.StartLine < line && line <= f.EndLine {
			m.collapsed[f.StartLine] = false
			changed = true
		}
	}
	if changed {
		m.rev++
	}
}

func (m *Model) setRepeatsOpen(open bool) {
	clear(m.open)
	if open {
		for _, r := range m.rows() {
			if r.kind == rowRepeat {
				m.open[r.line] = true
			}
		}
	}
	m.rev++
}

func (m *Model) toggleCompact() {
	if m.plan == nil || m.FoldCount() == 0 {
		return
	}
	m.Compact = !m.Compact
	m.rev++
	m.topRow = -1
}

// repeatTitle summarizes the fold behind a repeat row.
func (m Model) repeatTitle(r viewRow) string {
	plan := m.activePlan()
	if plan == nil || r.seg >= len(plan.Segments) {
		return ""
	}
	return fold.Summarize(plan.Segments[r.seg], plan.Lines, fold.Shape{}).Title()
}
