package log

import (
	"strings"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/logs/fold"
	"github.com/indrasvat/gh-hound/internal/theme"
)

type KeyMsg struct {
	Key string
}

type SearchState struct {
	Query   string
	Matches []int
	Current int
	Total   int
}

type Model struct {
	Document   logs.Document
	Offset     int
	Height     int
	Width      int // set by the caller; lets wrapped scrolling reach the end
	Wrap       bool
	Theme      theme.Theme
	InputMode  bool
	LastJump   string
	jumpLine   int // LastJump's line; the echo clears once it is off screen
	RangeStart int
	RangeEnd   int
	RangeLabel string
	input      string
	collapsed  map[int]bool
	Search     SearchState
	// Compact shows the fold plan's view; it needs a plan (SetPlan).
	Compact     bool
	plan        *fold.Plan
	rangePlan   *fold.Plan
	open        map[int]bool
	hits        map[int]bool
	groups      map[int]logs.Fold
	rev         int64
	cache       *rowCache
	titles      *titleCache
	rangeTitles *titleCache
	topRow      int
	skip        int // screen lines of the wrapped top row scrolled past
	skipRow     int
	atEnd       bool // pinned to the last line until the view moves away
}

func NewModel(doc logs.Document, offset, height int) Model {
	if offset < 1 {
		offset = 1
	}
	if height <= 0 {
		height = 10
	}
	groups := make(map[int]logs.Fold, len(doc.Folds))
	for _, f := range doc.Folds {
		groups[f.StartLine] = f
	}
	return Model{
		groups:    groups,
		Document:  doc,
		Offset:    offset,
		Height:    height,
		collapsed: map[int]bool{},
		open:      map[int]bool{},
		cache:     &rowCache{},
		topRow:    -1,
	}
}

func (m Model) Update(msg KeyMsg) Model {
	m = m.update(msg)
	if m.LastJump != "" && !m.onScreen(m.jumpLine) {
		m.LastJump = ""
	}
	return m
}

func (m Model) update(msg KeyMsg) Model {
	if m.InputMode {
		return m.updateInput(msg)
	}
	switch msg.Key {
	case "esc":
		if m.RangeLabel != "" {
			m = m.ClearRange()
		} else {
			m.Search, m.hits = SearchState{}, nil
		}
	case "j", "down":
		m.down()
	case "k", "up":
		m.up()
	case "g":
		m.scrollTo(0)
	case "G":
		m.setPosition(m.bottom())
		m.atEnd = true
	case "ctrl+d":
		for range max(1, m.Height/2) { // like j, so wrapped lines are walked too
			m.down()
		}
	case "ctrl+u":
		for range max(1, m.Height/2) {
			m.up()
		}
	case "/":
		m.InputMode = true
		m.input = ""
	case "n":
		m.moveMatch(1)
	case "N":
		m.moveMatch(-1)
	case "z":
		m.setCollapsed(true)
		m.setRepeatsOpen(false)
	case "Z":
		m.setCollapsed(false)
		m.setRepeatsOpen(true)
	case "c":
		m.toggleCompact()
	case "]":
		m.jumpRepeat(1)
	case "[":
		m.jumpRepeat(-1)
	case "w":
		m.Wrap = !m.Wrap
	}
	return m
}

func (m Model) updateInput(msg KeyMsg) Model {
	switch msg.Key {
	case "esc":
		m.InputMode = false
	case "enter":
		m.InputMode = false
		m.runSearch(m.input)
		m.gotoCurrentMatch()
	case "backspace":
		if runes := []rune(m.input); len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case "space":
		m.input += " "
	default:
		if len([]rune(msg.Key)) == 1 {
			m.input += msg.Key
		}
	}
	return m
}

// jumpToTime moves the viewport to the first line whose runner clock
// is at or after the query ("15:53:14" or a prefix like "15:53").
// Clocks can wrap past midnight: lines are bucketed into days by
// rollover detection and days are searched in order, so a 00:01 query
// against a log starting at 23:59 lands after the wrap.
// JumpTo is the modal's commit: it returns the model scrolled to the
// queried wall-clock moment.
func (m Model) JumpTo(query string) Model {
	m.jumpToTime(query)
	m.reveal(m.Offset)
	m.jumpLine = m.Offset
	return m
}

// JumpToLine scrolls to an exact line (picker entries).
func (m Model) JumpToLine(line int) Model {
	if line >= 1 && line <= len(m.Document.Lines) {
		m.Offset, m.atEnd = line, false
		m.reveal(line)
		m.jumpLine = line
	}
	return m
}

// JumpRelative moves by a signed number of seconds from the line at
// the top of the viewport, clamping to the log's stamped span.
func (m Model) JumpRelative(deltaSeconds float64) Model {
	m = m.jumpRelative(deltaSeconds)
	m.reveal(m.Offset)
	m.jumpLine = m.Offset
	return m
}

func (m Model) jumpRelative(deltaSeconds float64) Model {
	timeline := logs.Timeline(m.Document)
	if len(timeline) == 0 {
		return m
	}
	current := timeline[0]
	for _, stamp := range timeline {
		if stamp.LineNumber <= m.Offset {
			current = stamp
		}
	}
	target := current.Seconds + deltaSeconds
	if target <= timeline[0].Seconds {
		m.Offset = timeline[0].LineNumber
		return m
	}
	for _, stamp := range timeline {
		if stamp.Seconds >= target {
			m.Offset = stamp.LineNumber
			return m
		}
	}
	m.Offset = timeline[len(timeline)-1].LineNumber
	return m
}

// SetRange restricts the visible rows to [start, end] line numbers;
// label is the human form shown in the header. Esc clears it.
func (m Model) SetRange(start, end int, label string) Model {
	if start < 1 || end < start {
		return m
	}
	m.RangeStart = start
	m.RangeEnd = end
	m.RangeLabel = label
	m.Offset, m.atEnd = start, false
	m.planRange()
	m.rematch()
	m.bump()
	return m
}

func (m Model) ClearRange() Model {
	m.RangeStart = 0
	m.RangeEnd = 0
	m.RangeLabel = ""
	m.rangePlan, m.atEnd = nil, false
	m.rematch()
	m.bump()
	return m
}

// rematch re-runs a committed search for the lines now in view, keeping
// the current match or the next after it in view.
func (m *Model) rematch() {
	from := m.Offset
	if m.Search.Total > 0 {
		from = m.Search.Matches[m.Search.Current-1]
	}
	m.runSearch(m.Search.Query)
	if m.Search.Total == 0 {
		return
	}
	m.Search.Current = 1
	for i, line := range m.Search.Matches {
		if line >= from {
			m.Search.Current = i + 1
			break
		}
	}
	m.gotoCurrentMatch()
}

// runSearch matches query against the lines in view: the active range,
// or the whole log.
func (m *Model) runSearch(query string) {
	m.Search, m.hits = buildSearch(m.Document, query, m.RangeStart, m.RangeEnd), nil
	if m.Search.Query == "" {
		return
	}
	m.hits = make(map[int]bool, len(m.Search.Matches))
	for _, line := range m.Search.Matches {
		m.hits[line] = true
	}
}

// VisibleRowNumbers exposes each visible row's anchor line (tests).
func (m Model) VisibleRowNumbers() []int {
	numbers := []int{}
	for _, row := range m.visibleRows() {
		numbers = append(numbers, row.line)
	}
	return numbers
}

func (m *Model) jumpToTime(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	// Zero-pad single-digit hours: runner clocks are zero-padded and
	// every comparison below is lexical.
	if idx := strings.IndexByte(query, ':'); idx == 1 {
		query = "0" + query
	}
	type stamped struct {
		number int
		day    int
		clock  string
	}
	var lines []stamped
	day := 0
	prev := ""
	for _, line := range m.Document.Lines {
		clock, ok := logs.ClockTime(line.Text)
		if !ok {
			continue
		}
		if prev != "" && clock < prev {
			day++
		}
		prev = clock
		lines = append(lines, stamped{number: line.Number, day: day, clock: clock})
	}
	if len(lines) == 0 {
		return
	}
	// The query names a wall-clock moment; resolve which day bucket it
	// falls in. Day 0 starts mid-stream at the log's first clock, later
	// days start at midnight, so a day matches only when the query is
	// within its span -- otherwise 00:01 would lexically match 23:59.
	for d := 0; d <= day; d++ {
		first := ""
		for _, line := range lines {
			if line.day == d {
				first = line.clock
				break
			}
		}
		if d == 0 && clockBefore(query, first) {
			continue
		}
		for _, line := range lines {
			if line.day == d && line.clock >= query {
				m.Offset = line.number
				m.LastJump = query
				return
			}
		}
	}
	// Single-day log with a query before its first clock: the moment
	// predates the log, so land on the first stamped line. Multi-day
	// logs reaching here mean the query is past every span -- no jump
	// (a lexically-small query there is post-midnight, not earlier).
	if day == 0 && clockBefore(query, lines[0].clock) {
		m.Offset = lines[0].number
		m.LastJump = query
	}
}

// clockBefore reports query < clock with prefix semantics: a query
// that is a prefix of the clock ("10:00" vs "10:00:00.000") is not
// before it.
func clockBefore(query, clock string) bool {
	if len(clock) > len(query) {
		clock = clock[:len(query)]
	}
	return query < clock
}

func (m Model) Collapsed(line int) bool {
	return m.collapsed[line]
}

func (m *Model) setCollapsed(value bool) {
	m.collapsed = make(map[int]bool, len(m.Document.Folds))
	for _, f := range m.Document.Folds {
		m.collapsed[f.StartLine] = value
	}
	m.bump()
}

func (m *Model) moveMatch(delta int) {
	if m.Search.Total == 0 {
		return
	}
	next := m.Search.Current + delta
	if next < 1 {
		next = m.Search.Total
	}
	if next > m.Search.Total {
		next = 1
	}
	m.Search.Current = next
	m.gotoCurrentMatch()
}

func (m *Model) gotoCurrentMatch() {
	if m.Search.Current < 1 || m.Search.Current > len(m.Search.Matches) {
		return
	}
	line := m.Search.Matches[m.Search.Current-1]
	m.reveal(line)
	forward := line > m.Offset
	m.Offset = line
	if rows := m.rows(); forward && line > 1 && m.screenLines(rows[rowIndexFor(rows, line-1)]) < m.Height {
		m.Offset = line - 1 // one line of context when it fits
	}
}

// buildSearch matches lines start..end (end 0: to the last line).
func buildSearch(doc logs.Document, query string, start, end int) SearchState {
	query = strings.TrimSpace(query)
	state := SearchState{Query: query}
	if query == "" {
		return state
	}
	lower := strings.ToLower(query)
	for _, line := range doc.Lines {
		if line.Number < start || (end > 0 && line.Number > end) {
			continue
		}
		if strings.Contains(strings.ToLower(line.Text), lower) {
			state.Matches = append(state.Matches, line.Number)
		}
	}
	state.Total = len(state.Matches)
	if state.Total > 0 {
		state.Current = 1
	}
	return state
}

// visibleRows fills the viewport: a top too close to the end is drawn from
// earlier rows, as G would, so a jump never leaves the screen half empty.
func (m Model) visibleRows() []viewRow {
	rows := m.rows()
	top, _ := m.position()
	return rows[top:min(len(rows), top+max(m.Height, 1))]
}

// Changed reports whether m would draw differently from before.
func (m Model) Changed(before Model) bool {
	return m.rev != before.rev || m.Offset != before.Offset || m.topRow != before.topRow ||
		m.skip != before.skip || m.atEnd != before.atEnd || m.Wrap != before.Wrap || m.InputMode != before.InputMode ||
		m.input != before.input || m.LastJump != before.LastJump || m.Search.Query != before.Search.Query ||
		m.Search.Current != before.Search.Current || m.Search.Total != before.Search.Total
}

type hitKind int

const (
	noHit hitKind = iota
	searchHit
	currentHit
)

func (m Model) hit(line int) hitKind {
	switch {
	case !m.hits[line]:
		return noHit
	case m.Search.Current >= 1 && m.Search.Matches[m.Search.Current-1] == line:
		return currentHit
	default:
		return searchHit
	}
}

// onScreen reports whether line is drawn in the viewport: as a line or a
// group header, not hidden in a repeat fold.
func (m Model) onScreen(line int) bool {
	_, skip := m.position()
	budget := max(m.Height, 1)
	for i, row := range m.visibleRows() {
		if budget <= 0 {
			break
		}
		if row.kind != rowRepeat && row.line == line {
			return true
		}
		n := m.screenLines(row)
		if i == 0 {
			n -= min(skip, n-1)
		}
		budget -= n
	}
	return false
}
