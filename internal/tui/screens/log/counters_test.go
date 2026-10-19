package log

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func headerLine(m Model) string {
	return strings.SplitN(plain(View(m, 120)), "\n", 2)[0]
}

var foldedRE = regexp.MustCompile(`([\d,]+) lines folded in (\d+) place`)

func foldedCounts(t *testing.T, m Model) (lines, places int) {
	t.Helper()
	match := foldedRE.FindStringSubmatch(headerLine(m))
	if match == nil {
		return 0, 0
	}
	lines, _ = strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	places, _ = strconv.Atoi(match[2])
	return lines, places
}

// The compact header counts the folds still closed: opening one (a time
// jump or a search hit lands inside it) takes it out of the count.
func TestFoldedHeaderCountsOnlyClosedFolds(t *testing.T) {
	m := compactModel(t, pollLog(), 20)
	lines, places := foldedCounts(t, m)
	if places == 0 {
		t.Fatalf("no folded count in %q", headerLine(m))
	}
	jumped := m.JumpToLine(300)
	if l, p := foldedCounts(t, jumped); p != places-1 || l >= lines {
		t.Fatalf("after a jump into a fold: %q, was %d lines in %d places", headerLine(jumped), lines, places)
	}
	if l, p := foldedCounts(t, jumped.Update(KeyMsg{Key: "z"})); p != places || l != lines {
		t.Fatalf("z closes every fold again: %d lines in %d places, want %d in %d", l, p, lines, places)
	}
	if head := headerLine(m.Update(KeyMsg{Key: "Z"})); !strings.HasPrefix(head, "log · compact") || strings.Contains(head, "folded") {
		t.Fatalf("every fold open: header %q", head)
	}
}

// While a time range is active, search matches only inside it, and the
// header shows both.
func TestSearchStaysInsideTheRange(t *testing.T) {
	m := NewModel(pollLog(), 1, 10).SetRange(20, 40, "10:01:30-10:03:10")
	for k := range strings.SplitSeq("/checking", "") {
		m = m.Update(KeyMsg{Key: k})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Search.Total == 0 || m.Search.Total >= 300 {
		t.Fatalf("matches = %d, want only the range's", m.Search.Total)
	}
	for _, line := range m.Search.Matches {
		if line < 20 || line > 40 {
			t.Fatalf("match on line %d outside the range 20-40", line)
		}
	}
	want := fmt.Sprintf("log · [10:01:30-10:03:10] · /checking · match 1/%d", m.Search.Total)
	if head := headerLine(m); !strings.HasPrefix(head, want) {
		t.Fatalf("header %q, want prefix %q", head, want)
	}
	cleared := m.Update(KeyMsg{Key: "esc"})
	if cleared.RangeLabel != "" || cleared.Search.Total != 300 {
		t.Fatalf("clearing the range must search the whole log again: %d matches", cleared.Search.Total)
	}
}

func TestMatchCounterIsDigitGrouped(t *testing.T) {
	m := NewModel(logs.Parse(strings.Repeat("hit\n", 1234)), 1, 10)
	for k := range strings.SplitSeq("/hit", "") {
		m = m.Update(KeyMsg{Key: k})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if head := headerLine(m); !strings.HasPrefix(head, "log · /hit · match 1/1,234") {
		t.Fatalf("header %q", head)
	}
}

func needles() logs.Document { // "needle" on lines 10, 50, 60, 200
	var b strings.Builder
	for i := 1; i <= 300; i++ {
		w := "hay"
		if i == 10 || i == 50 || i == 60 || i == 200 {
			w = "needle"
		}
		fmt.Fprintf(&b, "line %d %s\n", i, w)
	}
	return logs.Parse(b.String())
}

func searched(m Model, q string) Model {
	for k := range strings.SplitSeq("/"+q, "") {
		m = m.Update(KeyMsg{Key: k})
	}
	return m.Update(KeyMsg{Key: "enter"})
}

// A range set under a committed search shows the current match.
func TestSetRangeShowsTheCurrentMatch(t *testing.T) {
	m := searched(NewModel(needles(), 1, 5), "needle").SetRange(40, 100, "R")
	cur := m.Search.Matches[m.Search.Current-1]
	if !m.onScreen(cur) {
		t.Fatalf("header says match %d (line %d) but rows %v", m.Search.Current, cur, m.VisibleRowNumbers())
	}
}

// Clearing the range keeps the current match, on screen, so n goes on
// from it.
func TestClearRangeKeepsTheCurrentMatchInView(t *testing.T) {
	m := searched(NewModel(needles(), 1, 5), "needle").SetRange(40, 100, "R").Update(KeyMsg{Key: "n"})
	before := m.Search.Matches[m.Search.Current-1] // line 60
	c := m.Update(KeyMsg{Key: "esc"})
	if got := c.Search.Matches[c.Search.Current-1]; got != before || !c.onScreen(got) {
		t.Fatalf("after clearing the range: current line %d (was %d), rows %v", got, before, c.VisibleRowNumbers())
	}
	if n := c.Update(KeyMsg{Key: "n"}); n.Offset < m.Offset {
		t.Fatalf("n after clearing the range went backwards: %d -> %d", m.Offset, n.Offset)
	}
}
