package log

import (
	"context"
	"maps"
	"sort"
	"sync/atomic"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs/fold"
)

const (
	// planBudget bounds folding a whole log off the input path.
	planBudget = 2 * time.Second
	// rangePlanBudget bounds planning a time range on the key path.
	rangePlanBudget = 50 * time.Millisecond
	minFoldHidden   = 5 // smaller repeat runs stay as lines
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
	rev     int64
	plan    *fold.Plan
	rows    []viewRow
	end     layoutKey
	row     int
	skip    int
	layout  layoutKey
	heights map[int]int
}

// layoutKey is everything screen heights and the last position depend on.
type layoutKey struct {
	rev           int64
	width, height int
	wrap          bool
	query         string
}

// revs is process-wide, so sibling copies never share a revision.
var revs atomic.Int64

func (m *Model) bump() { m.rev = revs.Add(1) }

// titleCache holds a plan's repeat titles; a summary scans its whole run,
// so each is built once.
type titleCache struct {
	plan   *fold.Plan
	titles map[int]string
}

func newTitles(p *fold.Plan) *titleCache {
	return &titleCache{plan: p, titles: map[int]string{}}
}

func (c *titleCache) title(seg int) string {
	if t, ok := c.titles[seg]; ok {
		return t
	}
	t := fold.Summarize(c.plan.Segments[seg], c.plan.Lines, fold.Shape{}).Title()
	c.titles[seg] = t
	return t
}

// SetPlan attaches a fold plan computed off the input path. compact picks
// the initial view.
func (m Model) SetPlan(plan fold.Plan, compact bool) Model {
	m.plan = &plan
	m.titles = newTitles(m.plan)
	for i, s := range plan.Segments {
		if shownFold(s) {
			m.titles.title(i)
		}
	}
	m.Compact = compact
	m.rangePlan = nil
	if m.RangeLabel != "" {
		m.planRange()
	}
	m.bump()
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
	m.bump()
	return m
}

func (m Model) HasPlan() bool { return m.plan != nil }

func shownFold(s fold.Segment) bool {
	return s.Kind == fold.Folded && s.End-s.Start >= minFoldHidden
}

// FoldCount is how many repeat folds the full-log plan shows.
func (m Model) FoldCount() int {
	n, _ := m.foldTotals()
	return n
}

// FoldedLines is how many lines the shown repeat folds hide.
func (m Model) FoldedLines() int {
	_, hidden := m.foldTotals()
	return hidden
}

func (m Model) foldTotals() (folds, hidden int) {
	if m.plan == nil {
		return 0, 0
	}
	for _, s := range m.plan.Segments {
		if shownFold(s) {
			folds++
			hidden += s.End - s.Start
		}
	}
	return folds, hidden
}

// Rows is how many rows the view has now, folds open or closed.
func (m Model) Rows() int { return len(m.rows()) }

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
		m.rangeTitles = newTitles(m.rangePlan)
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
		if !shownFold(seg) {
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

// bottom is the last scroll position: a row and how many of its wrapped
// screen lines are scrolled past.
func (m Model) bottom() (row, skip int) {
	if m.cache == nil {
		return m.findBottom()
	}
	key := m.layoutKey()
	if m.cache.end != key {
		m.cache.row, m.cache.skip = m.findBottom()
		m.cache.end = key
	}
	return m.cache.row, m.cache.skip
}

func (m Model) findBottom() (row, skip int) {
	rows := m.rows()
	if !m.wrapping() {
		return max(0, len(rows)-max(m.Height, 1)), 0
	}
	used := 0
	for i := len(rows) - 1; i >= 0; i-- {
		lines := m.screenLines(rows[i])
		used += lines
		if over := used - max(m.Height, 1); over >= lines {
			return i + 1, 0
		} else if over > 0 {
			return i, over
		}
	}
	return 0, 0
}

func (m Model) wrapping() bool { return m.Wrap && m.Width > 0 }

// position is where drawing starts: the top row and its skipped screen
// lines, clamped so the viewport stays full.
func (m Model) position() (row, skip int) {
	if m.atEnd {
		return m.bottom()
	}
	row = m.top()
	if rows := m.rows(); m.wrapping() && m.skipRow == row && row < len(rows) {
		skip = min(m.skip, m.screenLines(rows[row])-1)
	}
	if br, bs := m.bottom(); row > br || row == br && skip > bs {
		return br, bs
	}
	return row, skip
}

func (m *Model) setPosition(row, skip int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	row = min(max(0, row), len(rows)-1)
	br, bs := m.bottom()
	if row > br || row == br && skip > bs {
		row, skip = br, bs
	}
	m.topRow, m.Offset = row, rows[row].line
	m.skip, m.skipRow = skip, row
	// at the end of a log taller than the screen: stay there across resizes
	m.atEnd = row == br && skip == bs && (br > 0 || bs > 0)
}

// down scrolls one screen line through a wrapped row taller than the
// viewport, else one row.
func (m *Model) down() {
	row, skip := m.position()
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	if m.wrapping() && m.screenLines(rows[row])-skip > max(m.Height, 1) {
		m.setPosition(row, skip+1)
		return
	}
	m.setPosition(row+1, 0)
}

func (m *Model) up() {
	if row, skip := m.position(); skip > 0 {
		m.setPosition(row, skip-1)
	} else {
		m.setPosition(row-1, 0)
	}
}

// screenLines is how many screen lines a row takes.
func (m Model) screenLines(r viewRow) int {
	if r.kind != rowLine || !m.wrapping() {
		return 1
	}
	if m.cache == nil {
		return len(m.renderRow(r))
	}
	if key := m.layoutKey(); m.cache.layout != key {
		m.cache.layout, m.cache.heights = key, map[int]int{}
	}
	h, ok := m.cache.heights[r.line]
	if !ok {
		h = len(m.renderRow(r))
		m.cache.heights[r.line] = h
	}
	return h
}

func (m Model) layoutKey() layoutKey {
	m.rows() // brings cache.rev up to this model
	return layoutKey{m.cache.rev, m.Width, m.Height, m.wrapping(), m.Search.Query}
}

func (m *Model) scrollTo(row int) { m.setPosition(row, 0) }

// reveal opens the repeat fold or step group hiding line.
func (m *Model) reveal(line int) {
	m.skip, m.atEnd = 0, false
	// open groups first: a closed group hides the repeat rows inside it
	m.openGroupsAround(line)
	if m.activePlan() == nil {
		return
	}
	for _, r := range m.rows() {
		if r.kind == rowRepeat && !r.open && r.line <= line && line <= r.end {
			m.open = maps.Clone(m.open)
			m.open[r.line] = true
			m.bump()
		}
	}
}

func (m *Model) openGroupsAround(line int) {
	for _, f := range m.Document.Folds {
		if m.collapsed[f.StartLine] && f.StartLine < line && line <= f.EndLine {
			m.collapsed = maps.Clone(m.collapsed)
			m.collapsed[f.StartLine] = false
			m.bump()
		}
	}
}

// repeatStarts is the first hidden line of each shown repeat fold, in order.
func (m Model) repeatStarts() []int {
	plan := m.activePlan()
	if plan == nil {
		return nil
	}
	base := 1
	if m.RangeLabel != "" {
		base = max(1, m.RangeStart)
	}
	var starts []int
	for _, seg := range plan.Segments {
		if shownFold(seg) {
			starts = append(starts, seg.Start+base)
		}
	}
	return starts
}

func (m *Model) setRepeatsOpen(open bool) {
	m.open = map[int]bool{}
	if open {
		for _, r := range m.rows() {
			if r.kind == rowRepeat {
				m.open[r.line] = true
			}
		}
	}
	m.bump()
}

// jumpRepeat scrolls to the next (dir 1) or previous (dir -1) repeat fold.
// It walks the plan, not the rows, so repeats inside closed groups count;
// their groups open and the repeat stays closed.
func (m *Model) jumpRepeat(dir int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	top, _ := m.position()
	cur, target := rows[top].line, 0
	starts := m.repeatStarts()
	for i := range starts {
		a := starts[i]
		if dir < 0 {
			a = starts[len(starts)-1-i]
		}
		if a > cur && dir > 0 || a < cur && dir < 0 {
			target = a
			break
		}
	}
	if target == 0 {
		return
	}
	m.openGroupsAround(target)
	rows = m.rows()
	for i, r := range rows {
		if r.kind == rowRepeat && r.line == target {
			m.skip, m.atEnd = 0, false
			m.topRow, m.Offset = i, r.line
			return
		}
	}
}

func (m *Model) toggleCompact() {
	if m.plan == nil || m.FoldCount() == 0 {
		return
	}
	m.Compact = !m.Compact
	m.bump()
	m.topRow, m.atEnd = -1, false
}

// repeatTitle summarizes the fold behind a repeat row.
func (m Model) repeatTitle(r viewRow) string {
	plan, titles := m.activePlan(), m.titles
	if m.RangeLabel != "" {
		titles = m.rangeTitles
	}
	if plan == nil || titles == nil || titles.plan != plan || r.seg >= len(plan.Segments) {
		return ""
	}
	return titles.title(r.seg)
}
