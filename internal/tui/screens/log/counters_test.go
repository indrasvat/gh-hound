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
