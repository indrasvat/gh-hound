package log

import (
	"slices"
	"strings"

	"github.com/indrasvat/gh-hound/internal/logs"
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
	Document  logs.Document
	Offset    int
	Height    int
	Wrap      bool
	InputMode bool
	LastJump  string
	input     string
	collapsed map[int]bool
	Search    SearchState
}

func NewModel(doc logs.Document, offset, height int) Model {
	if offset < 1 {
		offset = 1
	}
	if height <= 0 {
		height = 10
	}
	return Model{
		Document:  doc,
		Offset:    offset,
		Height:    height,
		collapsed: map[int]bool{},
	}
}

func (m Model) Update(msg KeyMsg) Model {
	if m.InputMode {
		return m.updateInput(msg)
	}
	switch msg.Key {
	case "j", "down":
		m.Offset = min(m.Offset+1, max(1, len(m.Document.Lines)))
	case "k", "up":
		m.Offset = max(1, m.Offset-1)
	case "g":
		m.Offset = 1
	case "G":
		m.Offset = max(1, len(m.Document.Lines)-m.Height+1)
	case "ctrl+d":
		m.Offset = min(m.Offset+m.Height/2, max(1, len(m.Document.Lines)))
	case "ctrl+u":
		m.Offset = max(1, m.Offset-m.Height/2)
	case "/":
		m.InputMode = true
		m.input = ""
	case "n":
		m.moveMatch(1)
	case "N":
		m.moveMatch(-1)
	case "z":
		m.setCollapsed(true)
	case "Z":
		m.setCollapsed(false)
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
		m.Search = buildSearch(m.Document, m.input)
		m.gotoCurrentMatch()
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
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
	return m
}

func (m *Model) jumpToTime(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
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
		if d == 0 && query < first {
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
	if day == 0 && query < lines[0].clock {
		m.Offset = lines[0].number
		m.LastJump = query
	}
}

func (m Model) Collapsed(line int) bool {
	return m.collapsed[line]
}

func (m *Model) setCollapsed(value bool) {
	if m.collapsed == nil {
		m.collapsed = map[int]bool{}
	}
	for _, fold := range m.Document.Folds {
		m.collapsed[fold.StartLine] = value
	}
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
	if line > m.Offset {
		m.Offset = max(1, line-1)
	} else {
		m.Offset = line
	}
}

func buildSearch(doc logs.Document, query string) SearchState {
	query = strings.TrimSpace(query)
	state := SearchState{Query: query}
	if query == "" {
		return state
	}
	lower := strings.ToLower(query)
	for _, line := range doc.Lines {
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

func (m Model) visibleRows() []row {
	rows := make([]row, 0, m.Height)
	folds := map[int]logs.Fold{}
	for _, fold := range m.Document.Folds {
		folds[fold.StartLine] = fold
	}
	for i := max(0, m.Offset-1); i < len(m.Document.Lines) && len(rows) < m.Height; i++ {
		line := m.Document.Lines[i]
		if fold, ok := folds[line.Number]; ok {
			rows = append(rows, row{Line: line, Fold: fold, IsFold: true, Collapsed: m.Collapsed(line.Number)})
			if m.Collapsed(line.Number) {
				i = max(i, fold.EndLine-1)
			}
			continue
		}
		rows = append(rows, row{Line: line})
	}
	return rows
}

type row struct {
	Line      logs.Line
	Fold      logs.Fold
	IsFold    bool
	Collapsed bool
}

func (m Model) isMatch(line int) bool {
	return slices.Contains(m.Search.Matches, line)
}
