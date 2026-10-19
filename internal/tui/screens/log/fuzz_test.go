package log

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/logs/fold"
)

// FuzzViewport drives the log screen as a state machine: random logs, keys,
// jumps, ranges and resizes, checking the frame against a flattened oracle
// after every event.
func FuzzViewport(f *testing.F) {
	f.Add([]byte{1, 200, 0, 4, 20, 13, 30, 5, 3, 1, 0, 1, 0})
	f.Add([]byte{2, 0, 4, 25, 3, 1, 90, 0, 8, 0, 13, 12, 3, 11, 0, 0, 3, 10, 3})
	f.Add([]byte{4, 40, 4, 30, 2, 5, 3, 6, 255, 12, 13, 40, 6, 9, 10, 14, 1, 17, 6, 0})
	f.Add([]byte{1, 255, 1, 255, 13, 1, 1, 11, 3, 13, 200, 39, 0, 0, 1, 1, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		in := &fuzzInput{data: data}
		doc := in.document()
		m := NewModel(doc, 1, 1+in.next()%20)
		m.Width = 10 + in.next()%120
		if plan, err := fold.ForDocument(context.Background(), doc, fold.Default()); err == nil {
			m = m.SetPlan(plan, in.next()%4 != 0)
		}
		var events []string
		pinned := false // G, then only resizes: the end must stay on screen
		for step := 0; step < 64 && !in.done(); step++ {
			var event string
			prev, before := m, windowStart(m)
			m, event = in.event(m)
			if forward := event == "j" || event == "down" || event == "ctrl+d"; forward && windowStart(m) < before {
				t.Fatalf("after %v: %s moved the view backwards", append(events, event), event)
			}
			events = append(events, event)
			resized := strings.HasPrefix(event, "resize")
			pinned = event == "G" || pinned && resized
			checkFrame(t, m, event, events)
			if pinned && resized && !endsAtBottom(m) {
				t.Fatalf("after %v: a resize at the end lost the last line", events)
			}
			if want := buildSearch(m.Document, m.Search.Query, m.RangeStart, m.RangeEnd); m.Search.Total != want.Total {
				t.Fatalf("after %v: %d matches, %d in the lines in view", events, m.Search.Total, want.Total)
			}
			for _, line := range m.Search.Matches {
				if m.RangeLabel != "" && (line < m.RangeStart || line > m.RangeEnd) {
					t.Fatalf("after %v: match on line %d outside the range %d-%d", events, line, m.RangeStart, m.RangeEnd)
				}
			}
			if event == "esc" && prev.RangeLabel == "" && m.Search.Query != "" {
				t.Fatalf("after %v: esc kept the search %q", events, m.Search.Query)
			}
			if keyed := !resized && !strings.HasPrefix(event, "jump") && !strings.HasPrefix(event, "range"); keyed {
				drawn := drawsLine(m, m.jumpLine)
				// "/q" is three keys: the echo may go at "/" and the match land back on the line.
				single := !strings.HasPrefix(event, "/")
				if m.LastJump != "" && !drawn || single && prev.LastJump != "" && drawn && m.LastJump == "" {
					t.Fatalf("after %v: echo %q with line %d drawn=%v", events, m.LastJump, m.jumpLine, drawn)
				}
			}
		}
		checkCoverage(t, m, events)
	})
}

type fuzzInput struct {
	data []byte
	at   int
}

func (in *fuzzInput) done() bool { return in.at >= len(in.data) }

func (in *fuzzInput) next() int {
	if in.done() {
		return 0
	}
	in.at++
	return int(in.data[in.at-1])
}

func (in *fuzzInput) document() logs.Document {
	var lines []string
	depth := 0
	for n := 1 + in.next()%24; n > 0 && len(lines) < 400; n-- {
		switch in.next() % 11 {
		case 0:
			lines = append(lines, fmt.Sprintf("ok %d", len(lines)))
		case 1:
			lines = append(lines, "head "+strings.Repeat("x", in.next()*4)+" TAIL")
		case 2:
			lines = append(lines, "##[group]step")
			depth++
		case 3:
			if depth > 0 {
				lines = append(lines, "##[endgroup]")
				depth--
			}
		case 4:
			for i := range 5 + in.next()%40 {
				lines = append(lines, "waiting for db", fmt.Sprintf("state 0x%08x", i*7919))
			}
		case 5:
			lines = append(lines, strings.Repeat("日本語 ✓ ", 1+in.next()%30))
		case 6:
			lines = append(lines, "\x1b[31mred\x1b[0m "+strings.Repeat("ansi ", in.next()%40))
		case 7:
			lines = append(lines, "", "\tindented")
		case 8:
			lines = append(lines, "##[error]boom "+strings.Repeat("e", in.next()))
		case 9:
			lines = append(lines, fmt.Sprintf("10:00:%02dZ ::warning::flake", in.next()%60))
		case 10:
			junk := []string{"\t", "\r", "\x1b[1A", "\x1b[2K", "\x1b]8;;u\x07", "\x07", "\x7f", "\x1b[33m", "\x1b"}
			lines = append(lines, "a"+junk[in.next()%len(junk)]+"b\tc"+junk[in.next()%len(junk)])
		}
	}
	return logs.Parse(strings.Join(lines, "\n"))
}

var fuzzKeys = []string{"j", "k", "g", "G", "ctrl+d", "ctrl+u", "z", "Z", "c", "]", "[", "w", "n", "N", "down", "up"}

func (in *fuzzInput) event(m Model) (Model, string) {
	switch op := in.next() % 20; {
	case op < 16:
		return m.Update(KeyMsg{Key: fuzzKeys[op]}), fuzzKeys[op]
	case op == 16:
		m.Width, m.Height = 10+in.next()%150, 1+in.next()%40
		return m, fmt.Sprintf("resize %dx%d", m.Width, m.Height)
	case op == 17:
		q := string("xaTo"[in.next()%4])
		for _, k := range []string{"/", q, "enter"} {
			m = m.Update(KeyMsg{Key: k})
		}
		return m, "/" + q
	case op == 18:
		line := 1 + in.next()%max(1, len(m.Document.Lines))
		m = m.JumpToLine(line)
		m.LastJump = "t" // as the app does after a jump
		return m, fmt.Sprintf("jump %d", line)
	default:
		if m.RangeLabel != "" || m.Search.Query != "" {
			return m.Update(KeyMsg{Key: "esc"}), "esc"
		}
		a := 1 + in.next()%max(1, len(m.Document.Lines))
		b := a + in.next()%60
		return m.SetRange(a, b, "r"), fmt.Sprintf("range %d-%d", a, b)
	}
}

// flat renders every row on its own: the frame must be a window of it.
func flat(m Model) []string {
	var out []string
	for _, r := range m.rows() {
		for _, l := range m.renderRow(r) {
			out = append(out, ansi.Strip(l))
		}
	}
	return out
}

func checkFrame(t *testing.T, m Model, event string, events []string) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("after %v (%dx%d wrap=%v compact=%v): %s", events, m.Width, m.Height, m.Wrap, m.Compact, fmt.Sprintf(format, args...))
	}
	out := strings.Split(View(m, m.Width), "\n")
	if len(out)-1 > max(m.Height, 1) {
		fail("%d body lines for height %d", len(out)-1, m.Height)
	}
	for _, l := range out {
		if w := ansi.StringWidth(l); w > m.Width {
			fail("line %d cols wide at width %d: %q", w, m.Width, ansi.Strip(l))
		}
		if TerminalSafe(l) != l {
			fail("control bytes reach the terminal: %q", l)
		}
	}
	all := flat(m)
	row, skip := m.position()
	k := skip
	for _, r := range m.rows()[:row] {
		k += m.screenLines(r)
	}
	body := out[1:]
	if want := min(max(m.Height, 1), len(all)); len(body) != want {
		fail("viewport has %d lines, want %d (of %d)", len(body), want, len(all))
	}
	for i, l := range body {
		if k+i >= len(all) || ansi.Strip(l) != all[k+i] {
			fail("body line %d is %q, oracle has window at %d", i, ansi.Strip(l), k)
		}
	}
	switch event {
	case "g":
		if k != 0 && m.RangeLabel == "" {
			fail("g starts at screen line %d", k)
		}
	case "G":
		if k+len(body) != len(all) {
			fail("G ends at %d of %d", k+len(body), len(all))
		}
	}
	checkProjection(t, m, fail)
}

// drawsLine reports whether the frame shows line: a row whose rendered
// gutter carries its number (a collapsed repeat's ⋯ row hides it), counted
// on rendered rows rather than the cached heights.
func drawsLine(m Model, line int) bool {
	head := gutterText(line, gutterWidth(len(m.Document.Lines))) + " "
	start, n := windowStart(m), 0
	for _, r := range m.rows() {
		out := m.renderRow(r)
		rest, ok := strings.CutPrefix(ansi.Strip(out[0]), head)
		if ok && !strings.HasPrefix(rest, "⋯") && n+len(out) > start && n < start+max(m.Height, 1) {
			return true
		}
		n += len(out)
	}
	return false
}

// windowStart is the first drawn screen line in the flattened oracle.
func windowStart(m Model) int {
	row, skip := m.position()
	k := skip
	for _, r := range m.rows()[:row] {
		k += m.screenLines(r)
	}
	return k
}

func endsAtBottom(m Model) bool {
	return windowStart(m)+min(max(m.Height, 1), len(flat(m))) >= len(flat(m))
}

func checkProjection(t *testing.T, m Model, fail func(string, ...any)) {
	t.Helper()
	first, last := 1, len(m.Document.Lines)
	if m.RangeLabel != "" {
		first, last = max(1, m.RangeStart), min(last, m.RangeEnd)
	}
	next := first
	for _, r := range m.rows() {
		if r.line < next && r.kind == rowLine || r.line > next || r.line < first || r.line > last {
			fail("row %+v breaks the projection at line %d (range %d-%d)", r, next, first, last)
		}
		switch {
		case r.kind == rowLine:
			next = r.line + 1
		case !r.open:
			next = min(r.end, last) + 1
		default:
			next = r.line
			if r.kind == rowGroup {
				next = r.line + 1
			}
		}
	}
	if len(m.rows()) > 0 && next != last+1 {
		fail("projection stops at line %d of %d", next-1, last)
	}
}

func checkCoverage(t *testing.T, m Model, events []string) {
	t.Helper()
	m = m.Update(KeyMsg{Key: "g"})
	rows := m.rows()
	starts := make([]int, len(rows)+1)
	for i, r := range rows {
		starts[i+1] = starts[i] + m.screenLines(r)
	}
	all, seen := starts[len(rows)], 0
	for range all + 2 {
		row, skip := m.position()
		k := starts[row] + skip
		if k > seen {
			t.Fatalf("after %v then g/j: screen lines %d-%d never shown", events, seen, k-1)
		}
		seen = max(seen, k+max(m.Height, 1))
		m = m.Update(KeyMsg{Key: "j"})
	}
	if seen < all {
		t.Fatalf("after %v then g/j: only %d of %d screen lines reachable", events, seen, all)
	}
}
