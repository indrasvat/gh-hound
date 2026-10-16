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

// pollLog: a step group, 300 two-line poll cycles, a rare exit code inside
// the poll, and a timeout.
func pollLog() logs.Document {
	var b strings.Builder
	b.WriteString("##[group]Run e2e\nstarting cluster\n##[endgroup]\n")
	for i := range 300 {
		fmt.Fprintf(&b, "%02d:%02d:%02dZ checking if cluster is running\n", 10+i/360, i/6%60, i%6*10)
		fmt.Fprintf(&b, "%02d:%02d:%02dZ state :: &{ 0x%012x }\n", 10+i/360, i/6%60, i%6*10+1, i*7919)
	}
	b.WriteString("ERROR timed out waiting for cluster\n")
	return logs.Parse(b.String())
}

func compactModel(t *testing.T, doc logs.Document, height int) Model {
	t.Helper()
	plan, err := fold.ForDocument(context.Background(), doc, fold.Default())
	if err != nil {
		t.Fatal(err)
	}
	return NewModel(doc, 1, height).SetPlan(plan, true)
}

func plain(s string) string { return ansi.Strip(s) }

func TestCompactViewFoldsThePoll(t *testing.T) {
	doc := pollLog()
	m := compactModel(t, doc, 20)
	if !m.Folded() || m.FoldCount() == 0 {
		t.Fatalf("expected folds, got %d", m.FoldCount())
	}
	out := plain(View(m, 120))
	if !strings.HasPrefix(out, "log · compact") {
		t.Fatalf("header: %q", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "▸ ×") || !strings.Contains(out, "2-line block") || !strings.Contains(out, " lines") {
		t.Fatalf("no repeat fold row:\n%s", out)
	}
	if !strings.Contains(out, "timed out waiting for cluster") {
		t.Fatalf("timeout line not visible:\n%s", out)
	}
	if m.CompactRows() >= len(doc.Lines)/10 {
		t.Fatalf("compact rows %d for %d lines", m.CompactRows(), len(doc.Lines))
	}
}

func TestCompactToggleKeepsTopLine(t *testing.T) {
	m := compactModel(t, pollLog(), 10)
	m = m.JumpToLine(300)
	m = m.Update(KeyMsg{Key: "c"})
	if m.Folded() || m.Offset != 300 {
		t.Fatalf("after c: folded=%v offset=%d", m.Folded(), m.Offset)
	}
	if got := m.VisibleRowNumbers()[0]; got != 300 {
		t.Fatalf("full view top row = %d, want 300", got)
	}
	m = m.Update(KeyMsg{Key: "c"})
	if !m.Folded() {
		t.Fatal("c did not return to compact")
	}
}

func TestSearchOpensTheFoldHoldingTheMatch(t *testing.T) {
	m := compactModel(t, pollLog(), 10)
	for k := range strings.SplitSeq("/10:30:20", "") {
		m = m.Update(KeyMsg{Key: k})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Search.Total == 0 {
		t.Fatal("no matches")
	}
	match := m.Search.Matches[0]
	found := false
	for _, n := range m.VisibleRowNumbers() {
		if n == match {
			found = true
		}
	}
	if !found {
		t.Fatalf("match line %d not visible: %v", match, m.VisibleRowNumbers())
	}
	if out := plain(View(m, 120)); !strings.Contains(out, "› search-hit") {
		t.Fatalf("hit not highlighted:\n%s", out)
	}
	opened := false
	for _, r := range m.rows() {
		opened = opened || (r.kind == rowRepeat && r.open && r.line <= match && match <= r.end)
	}
	if !opened {
		t.Fatal("the fold holding the match was not opened")
	}
}

func TestZClosesAndOpensEveryFold(t *testing.T) {
	m := compactModel(t, pollLog(), 40)
	m = m.Update(KeyMsg{Key: "Z"})
	if rows := len(m.rows()); rows < len(m.Document.Lines) {
		t.Fatalf("Z should open everything: rows=%d lines=%d", rows, len(m.Document.Lines))
	}
	m = m.Update(KeyMsg{Key: "z"})
	if out := plain(View(m, 120)); !strings.Contains(out, "▸ Run e2e") || strings.Contains(out, "▾") {
		t.Fatalf("z should close groups and repeats:\n%s", out)
	}
}

func TestGFillsTheScreenWithFoldsClosed(t *testing.T) {
	m := compactModel(t, pollLog(), 6)
	m = m.Update(KeyMsg{Key: "Z"}).Update(KeyMsg{Key: "z"}).Update(KeyMsg{Key: "c"})
	m = m.Update(KeyMsg{Key: "G"})
	if got := len(m.VisibleRowNumbers()); got != 6 {
		t.Fatalf("G left %d rows, want 6", got)
	}
}

func TestScrollingMovesThroughAnOpenedFold(t *testing.T) {
	m := compactModel(t, pollLog(), 5)
	m = m.Update(KeyMsg{Key: "Z"})
	seen := map[int]bool{}
	for range 20 {
		seen[m.top()] = true
		m = m.Update(KeyMsg{Key: "j"})
	}
	if len(seen) < 20 {
		t.Fatalf("j got stuck: %d distinct tops in 20 presses", len(seen))
	}
}

func TestGutterAlignsPastLine999(t *testing.T) {
	var b strings.Builder
	for i := range 1200 {
		fmt.Fprintf(&b, "unique line %d of the build\n", i)
	}
	m := NewModel(logs.Parse(b.String()), 995, 8)
	lines := strings.Split(plain(View(m, 80)), "\n")[1:]
	for _, l := range lines {
		if len(l) < 5 || l[4] != ' ' {
			t.Fatalf("gutter not 4 wide: %q", l)
		}
	}
}

func TestRangeZoomPlansJustTheRange(t *testing.T) {
	m := compactModel(t, pollLog(), 20).SetRange(200, 260, "10:16–10:21")
	out := plain(View(m, 120))
	if !strings.Contains(out, "[10:16–10:21]") || !strings.Contains(out, "▸ ×") {
		t.Fatalf("range should be folded:\n%s", out)
	}
	for _, n := range m.VisibleRowNumbers() {
		if n < 200 || n > 260 {
			t.Fatalf("row %d outside range", n)
		}
	}
}

func TestWithoutAPlanNothingChanges(t *testing.T) {
	m := NewModel(pollLog(), 1, 10)
	m = m.Update(KeyMsg{Key: "c"})
	if m.Folded() || strings.HasPrefix(plain(View(m, 80)), "log · compact") {
		t.Fatal("c without a plan must be a no-op")
	}
}

func TestRepeatRowKeepsCountWhenNarrow(t *testing.T) {
	m := compactModel(t, pollLog(), 10)
	for l := range strings.SplitSeq(plain(View(m, 50)), "\n") {
		if strings.Contains(l, "▸ ×") && !strings.HasSuffix(strings.TrimRight(l, " "), " lines") {
			t.Fatalf("count truncated: %q", l)
		}
		if ansi.StringWidth(l) > 50 {
			t.Fatalf("row wider than 50: %q", l)
		}
	}
}

func TestWrapContinuesLongLines(t *testing.T) {
	doc := logs.Parse("short\n" + strings.Repeat("abcdefghij ", 20) + "\nend")
	m := NewModel(doc, 1, 10).Update(KeyMsg{Key: "w"})
	out := plain(View(m, 40))
	if !strings.Contains(out, "↪") {
		t.Fatalf("no continuation row:\n%s", out)
	}
	for l := range strings.SplitSeq(out, "\n") {
		if ansi.StringWidth(l) > 40 {
			t.Fatalf("wrapped row too wide: %q", l)
		}
	}
}

func BenchmarkViewCompact300kViewport(b *testing.B) {
	var builder strings.Builder
	for i := range 150_000 {
		fmt.Fprintf(&builder, "%02d:%02d:%02dZ waiting for cluster\n%02d:%02d:%02dZ state 0x%08x\n", i/3600%24, i/60%60, i%60, i/3600%24, i/60%60, i%60, i)
	}
	doc := logs.Parse(builder.String())
	plan, _ := fold.ForDocument(context.Background(), doc, fold.Default())
	m := NewModel(doc, 1, 40).SetPlan(plan, true).Update(KeyMsg{Key: "Z"}).JumpToLine(200_000)
	b.ReportAllocs()
	for b.Loop() {
		_ = View(m, 120)
	}
}
