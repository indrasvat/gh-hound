package log

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(out, "⋯ ×") || !strings.Contains(out, "2-line block") || !strings.Contains(out, " lines") {
		t.Fatalf("no repeat fold row:\n%s", out)
	}
	if !strings.Contains(out, "timed out waiting for cluster") {
		t.Fatalf("timeout line not visible:\n%s", out)
	}
	if m.Rows() >= len(doc.Lines)/10 {
		t.Fatalf("compact rows %d for %d lines", m.Rows(), len(doc.Lines))
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
	if _, cur := hitRows(View(m, 120)); !strings.HasPrefix(cur, gutterText(match, gutterWidth(len(m.Document.Lines)))) {
		t.Fatalf("hit not highlighted, current row %q:\n%s", cur, plain(View(m, 120)))
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
	if !strings.Contains(out, "[10:16–10:21]") || !strings.Contains(out, "⋯ ×") {
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
		if strings.Contains(l, "⋯ ×") && !strings.HasSuffix(strings.TrimRight(l, " "), " lines hidden") {
			t.Fatalf("count truncated: %q", l)
		}
		if ansi.StringWidth(l) > 50 {
			t.Fatalf("row wider than 50: %q", l)
		}
	}
}

func TestRepeatRowKeepsWarningAndCountFirst(t *testing.T) {
	title := "×1,234 2-line block · 10:00 → 11:59 · region us-east-1a → us-west-2b (40 values) · 12 changing ids · ⚠ 18 error-like inside"
	row := plain(renderRepeat(42, 3, title, 1234, true, 80))
	if !strings.Contains(row, "⚠ 18 error-like") || !strings.HasSuffix(strings.TrimRight(row, " "), "1,234 lines hidden") {
		t.Fatalf("80 cols must keep the warning and count: %q", row)
	}
	for _, width := range []int{40, 30, 24} {
		row := plain(renderRepeat(42, 3, title, 1234, true, width))
		if w := ansi.StringWidth(row); w > width {
			t.Fatalf("width %d: row is %d wide: %q", width, w, row)
		}
		if !strings.Contains(row, "1,234") || !strings.Contains(row, "×1,234") {
			t.Fatalf("width %d lost the count: %q", width, row)
		}
	}
}

func TestOpenRepeatHeaderLeavesTheNumberToItsLine(t *testing.T) {
	m := compactModel(t, pollLog(), 40).Update(KeyMsg{Key: "Z"})
	lines := strings.Split(plain(View(m, 100)), "\n")
	for i, l := range lines {
		if strings.Contains(l, "▾ ×") {
			if strings.TrimSpace(l[:4]) != "" || strings.Contains(l, "hidden") || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "00") {
				t.Fatalf("open header gutter %q, next %q", l, lines[min(i+1, len(lines)-1)])
			}
			return
		}
	}
	t.Fatal("no open repeat header")
}

func TestTheEndStaysOnScreenAcrossResizes(t *testing.T) {
	m := compactModel(t, pollLog(), 30).Update(KeyMsg{Key: "Z"}).Update(KeyMsg{Key: "G"})
	m.Width = 120
	for _, h := range []int{8, 40, 3, 17} {
		m.Height = h
		if out := plain(View(m, 120)); !strings.Contains(out, "timed out waiting for cluster") {
			t.Fatalf("height %d lost the last line:\n%s", h, out)
		}
	}
	m = m.Update(KeyMsg{Key: "k"})
	m.Height = 4
	if out := plain(View(m, 120)); strings.Contains(out, "timed out waiting for cluster") {
		t.Fatalf("after k the view is no longer pinned to the end:\n%s", out)
	}
}

func groupedPolls() logs.Document {
	var b strings.Builder
	for g := range 2 {
		fmt.Fprintf(&b, "##[group]wait %d\n", g)
		for i := range 40 {
			fmt.Fprintf(&b, "waiting for db\nstate 0x%08x\n", g*1000+i)
		}
		b.WriteString("##[endgroup]\n")
	}
	b.WriteString("done\n")
	return logs.Parse(b.String())
}

func TestBracketsWalkEveryRepeatWhenTheLogFits(t *testing.T) {
	m := compactModel(t, groupedPolls(), 30).Update(KeyMsg{Key: "z"}).Update(KeyMsg{Key: "g"})
	m = m.Update(KeyMsg{Key: "]"}).Update(KeyMsg{Key: "]"})
	if out := plain(View(m, 100)); !strings.Contains(out, "▾ wait 0") || !strings.Contains(out, "▾ wait 1") {
		t.Fatalf("two ] presses should open both groups:\n%s", out)
	}
}

func TestBracketsReachRepeatsInsideClosedGroups(t *testing.T) {
	m := compactModel(t, groupedPolls(), 4).Update(KeyMsg{Key: "z"}).Update(KeyMsg{Key: "g"})
	for _, want := range []string{"004 ⋯ ×", "085 ⋯ ×"} {
		m = m.Update(KeyMsg{Key: "]"})
		if first := strings.Split(plain(View(m, 100)), "\n")[1]; !strings.HasPrefix(first, want) {
			t.Fatalf("] should land on %q, got %q", want, first)
		}
	}
	m = m.Update(KeyMsg{Key: "["})
	if first := strings.Split(plain(View(m, 100)), "\n")[1]; !strings.HasPrefix(first, "004 ⋯ ×") {
		t.Fatalf("[ should land back on 004, got %q", first)
	}
}

func TestWrapDoesNotIndentContinuationsTwice(t *testing.T) {
	m := NewModel(logs.Parse("[worker-b] lease held by "+strings.Repeat("w", 30)+" w-a4d71b"), 1, 5)
	m = m.Update(KeyMsg{Key: "w"})
	for l := range strings.SplitSeq(plain(View(m, 60)), "\n") {
		if strings.Contains(l, "↪  ") {
			t.Fatalf("continuation starts with a double space: %q", l)
		}
	}
}

func TestRangeHeaderSaysCompact(t *testing.T) {
	m := compactModel(t, pollLog(), 10).SetRange(4, 300, "10:00-10:30")
	if !m.Folded() {
		t.Skip("range has no folds")
	}
	if out := plain(View(m, 100)); !strings.Contains(out, "log · [10:00-10:30] · compact · ⎋ clear") {
		t.Fatalf("range header: %s", strings.Split(out, "\n")[0])
	}
}

func TestControlBytesNeverReachTheTerminal(t *testing.T) {
	raw := strings.Join([]string{
		"hint:\tgit config --global init.defaultBranch <name>",
		"Updating files:   9%\rUpdating files:  10%",
		"\x1b[1A\x1b[2Kprogress \x1b]8;;https://x.io\x07link\x1b]8;;\x07 \x1b[32mok\x1b[0m",
		"bell\x07 and del\x7f",
		"##[group]step\twith tab",
		"##[endgroup]",
	}, "\n")
	m := NewModel(logs.Parse(raw), 1, 10)
	for _, width := range []int{40, 80} {
		for l := range strings.SplitSeq(View(m, width), "\n") {
			for _, c := range []string{"\t", "\r", "\x07", "\x7f", "\x1b[1A", "\x1b[2K", "\x1b]"} {
				if strings.Contains(l, c) {
					t.Fatalf("width %d: %q reaches the terminal in %q", width, c, l)
				}
			}
			if w := ansi.StringWidth(l); w > width {
				t.Fatalf("width %d: row is %d wide: %q", width, w, plain(l))
			}
		}
	}
	if out := plain(View(m, 80)); !strings.Contains(out, "hint:   git config") || !strings.Contains(out, "\x1b") && !strings.Contains(out, "ok") {
		t.Fatalf("tab should expand to the next stop:\n%s", out)
	}
}

func TestTerminalSafeOnlyPassesPlainSGR(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[1;31mred\x1b[0m": "\x1b[1;31mred\x1b[0m",
		"\x1b[38:2::1:2:3mx":   "\x1b[38:2::1:2:3mx",
		"a\x1b[?1049mb":        "ab",
		"a\x1b[1 mb":           "ab",
		"a\x1b[>4;2mb":         "ab",
		"a\u009b2Jb\u0085c":    "a2Jbc",
		"a\xffb\xc2":           "a\ufffdb\ufffd",
		"keep \ufffd and 日本 ✓": "keep \ufffd and 日本 ✓",
	} {
		if got := TerminalSafe(in); got != want {
			t.Errorf("TerminalSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHalfPagesMoveThroughAnOversizedLine(t *testing.T) {
	m := NewModel(logs.Parse("first\nhead "+strings.Repeat("x", 900)+" TAIL-END"), 1, 6)
	m.Width = 60
	m = m.Update(KeyMsg{Key: "w"})
	before := plain(View(m, 60))
	m = m.Update(KeyMsg{Key: "ctrl+d"})
	if plain(View(m, 60)) == before {
		t.Fatal("ctrl+d did not move through the wrapped line")
	}
	for range 10 {
		m = m.Update(KeyMsg{Key: "ctrl+d"})
	}
	end := plain(View(m, 60))
	if !strings.Contains(end, "TAIL-E") {
		t.Fatalf("ctrl+d never reached the end:\n%s", end)
	}
	if out := plain(View(m.Update(KeyMsg{Key: "j"}), 60)); out != end {
		t.Fatalf("j at the end must stay at the end:\n%s", out)
	}
	for range 10 {
		m = m.Update(KeyMsg{Key: "ctrl+u"})
	}
	if plain(View(m, 60)) != before {
		t.Fatal("ctrl+u did not climb back to the top")
	}
}

func TestRangeTitlesAreBuiltBeforeDrawing(t *testing.T) {
	m := compactModel(t, pollLog(), 10).SetRange(4, 600, "10:00-10:50")
	if m.rangePlan == nil {
		t.Skip("range planning ran out of budget")
	}
	for i, s := range m.rangePlan.Segments {
		if shownFold(s) && !m.rangeTitles.has(i) {
			t.Fatalf("range fold %d has no title before View", i)
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

func BenchmarkViewCompact300kClosed(b *testing.B) {
	var builder strings.Builder
	for i := range 150_000 {
		fmt.Fprintf(&builder, "%02d:%02d:%02dZ waiting for cluster\n%02d:%02d:%02dZ state 0x%08x\n", i/3600%24, i/60%60, i%60, i/3600%24, i/60%60, i%60, i)
	}
	doc := logs.Parse(builder.String())
	plan, _ := fold.ForDocument(context.Background(), doc, fold.Default())
	m := NewModel(doc, 1, 40).SetPlan(plan, true)
	b.ReportAllocs()
	for b.Loop() {
		_ = View(m, 120)
	}
}

func TestClosedFoldRendersWithoutRescanning(t *testing.T) {
	var builder strings.Builder
	for i := range 150_000 {
		fmt.Fprintf(&builder, "waiting for cluster\nstate 0x%08x\n", i)
	}
	doc := logs.Parse(builder.String())
	m := compactModel(t, doc, 40)
	View(m, 120)
	start := time.Now()
	for range 10 {
		View(m, 120)
	}
	if took := time.Since(start) / 10; took > 50*time.Millisecond {
		t.Fatalf("closed 300k fold renders in %v per frame", took)
	}
}

func TestSiblingRangesKeepTheirOwnRows(t *testing.T) {
	m := compactModel(t, pollLog(), 10)
	a, b := m.SetRange(2, 4, "a"), m.SetRange(20, 22, "b")
	_, _ = a.VisibleRowNumbers(), b.VisibleRowNumbers()
	if got := b.VisibleRowNumbers(); len(got) == 0 || got[0] < 20 {
		t.Fatalf("range b shows %v", got)
	}
	if got := a.VisibleRowNumbers(); len(got) == 0 || got[0] != 2 {
		t.Fatalf("range a shows %v", got)
	}
}

func TestFoldChangesStayInTheirCopy(t *testing.T) {
	m := compactModel(t, pollLog(), 10)
	before := plain(View(m, 80))
	opened := m.Update(KeyMsg{Key: "Z"})
	if plain(View(opened, 80)) == before {
		t.Fatal("Z changed nothing")
	}
	if after := plain(View(m, 80)); after != before {
		t.Fatalf("Z on a copy changed the original:\n%s", after)
	}
}

func TestZClosesTheMatchFoldAndNReopensIt(t *testing.T) {
	doc := logs.Parse("go test\n##[group]Run tests\nok a\n=== RUN TestTrail\nfail trail one\nok b\nfail trail two\nok c\n##[endgroup]")
	m := NewModel(doc, 1, 10)
	for k := range strings.SplitSeq("/trail", "") {
		m = m.Update(KeyMsg{Key: k})
	}
	m = m.Update(KeyMsg{Key: "enter"}).Update(KeyMsg{Key: "z"})
	if hits, _ := hitRows(View(m, 80)); !strings.Contains(plain(View(m, 80)), "002 ▸ Run tests") || len(hits) > 0 {
		t.Fatalf("z should close the group holding the match:\n%s", plain(View(m, 80)))
	}
	m = m.Update(KeyMsg{Key: "n"})
	if _, cur := hitRows(View(m, 80)); !strings.HasPrefix(cur, "005 ") {
		t.Fatalf("n should reopen the fold at the next match:\n%s", plain(View(m, 80)))
	}
}

func TestBracketsJumpBetweenRepeatFolds(t *testing.T) {
	m := compactModel(t, pollLog(), 10)
	m = m.Update(KeyMsg{Key: "]"})
	if r := m.rows()[m.top()]; r.kind != rowRepeat {
		t.Fatalf("] should land on a repeat fold, got kind %d at line %d", r.kind, r.line)
	}
	at := m.Offset
	m = m.Update(KeyMsg{Key: "]"})
	if m.Offset != at {
		t.Fatalf("] past the last fold should stay, moved to %d", m.Offset)
	}
	m = m.Update(KeyMsg{Key: "g"}).Update(KeyMsg{Key: "["})
	if m.Offset != 1 {
		t.Fatalf("[ before the first fold should stay at the top, got %d", m.Offset)
	}
}

func TestSmallRepeatRunsStayAsLines(t *testing.T) {
	doc := logs.Parse("a\nwait 1\nwait 2\nwait 3\nwait 4\nb\n")
	m := compactModel(t, doc, 10)
	if m.FoldCount() != 0 || strings.Contains(plain(View(m, 80)), "⋯") {
		t.Fatalf("a run hiding fewer than %d lines must not fold:\n%s", minFoldHidden, plain(View(m, 80)))
	}
}

func TestJumpsAfterZOpenTheRepeatFoldInsideAClosedGroup(t *testing.T) {
	var b strings.Builder
	b.WriteString("##[group]Run e2e\n")
	for i := range 200 {
		fmt.Fprintf(&b, "10:%02d:%02dZ waiting\n", i/60, i%60)
	}
	b.WriteString("##[endgroup]\ndone\n")
	doc := logs.Parse(b.String())
	m := compactModel(t, doc, 10).Update(KeyMsg{Key: "z"})
	for k := range strings.SplitSeq("/10:02:00", "") {
		m = m.Update(KeyMsg{Key: k})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if _, cur := hitRows(View(m, 80)); !strings.Contains(cur, "10:02:00Z waiting") {
		t.Fatalf("search after z must reveal the match:\n%s", plain(View(m, 80)))
	}
	m = m.Update(KeyMsg{Key: "z"}).JumpTo("10:01:30")
	found := false
	for _, n := range m.VisibleRowNumbers() {
		found = found || n == m.Offset
	}
	if !found || m.rows()[m.top()].kind != rowLine {
		t.Fatalf("time jump after z must land on the line, top=%v", m.VisibleRowNumbers())
	}
}

func TestWrappedGReachesTheLastLine(t *testing.T) {
	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "step %d %s\n", i, strings.Repeat("long output ", 1+i%3*6))
	}
	b.WriteString("##[error]Process completed with exit code 1.\n")
	m := NewModel(logs.Parse(b.String()), 1, 8)
	m.Width = 60
	m = m.Update(KeyMsg{Key: "w"}).Update(KeyMsg{Key: "G"})
	out := plain(View(m, 60))
	if !strings.Contains(out, "exit code 1.") {
		t.Fatalf("wrapped G must show the last line:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	if len(lines) > 9 {
		t.Fatalf("view overflowed: %d lines", len(lines))
	}
}

func oversizedLog() logs.Document {
	return logs.Parse("first\nhead " + strings.Repeat("x", 600) + " TAIL-OF-LONG\nafter")
}

func TestWrappedGReachesTheTailOfAnOversizedLastLine(t *testing.T) {
	doc := logs.Parse("first\nsecond\nhead " + strings.Repeat("x", 600) + " TAIL-OF-LONG")
	m := NewModel(doc, 1, 5)
	m.Width = 60
	m = m.Update(KeyMsg{Key: "w"}).Update(KeyMsg{Key: "G"})
	if out := plain(View(m, 60)); !strings.Contains(out, "TAIL-OF-LO") {
		t.Fatalf("G must reach the end of the last line:\n%s", out)
	}
}

func TestJStepsThroughAnOversizedWrappedLine(t *testing.T) {
	m := NewModel(oversizedLog(), 1, 5)
	m.Width = 60
	m = m.Update(KeyMsg{Key: "w"})
	seen := false
	for range 20 {
		m = m.Update(KeyMsg{Key: "j"})
		out := plain(View(m, 60))
		if lines := strings.Split(out, "\n"); len(lines) > 6 {
			t.Fatalf("view overflowed: %d lines", len(lines))
		}
		seen = seen || strings.Contains(out, "TAIL-OF-LO")
	}
	if !seen || !strings.Contains(plain(View(m, 60)), "after") {
		t.Fatalf("j never reached the long line's tail (%v) or the next line:\n%s", seen, plain(View(m, 60)))
	}
	for range 20 {
		m = m.Update(KeyMsg{Key: "k"})
	}
	if out := plain(View(m, 60)); !strings.Contains(out, "001 first") {
		t.Fatalf("k must climb back to the top:\n%s", out)
	}
}

func TestWrappedSearchKeepsTheMatchOnScreen(t *testing.T) {
	m := NewModel(oversizedLog(), 1, 5)
	m.Width = 60
	m = m.Update(KeyMsg{Key: "w"})
	for _, k := range []string{"/", "a", "f", "t", "e", "r", "enter"} {
		m = m.Update(KeyMsg{Key: k})
	}
	if _, cur := hitRows(View(m, 60)); !strings.HasPrefix(cur, "003 after") {
		t.Fatalf("the match must be on screen:\n%s", plain(View(m, 60)))
	}
}

func TestWrappedContinuationKeepsFailColour(t *testing.T) {
	doc := logs.Parse("--- FAIL: " + strings.Repeat("TestVeryLongName ", 10))
	m := NewModel(doc, 1, 6).Update(KeyMsg{Key: "w"})
	lines := strings.Split(View(m, 40), "\n")
	if len(lines) < 3 || !strings.Contains(lines[2], sgrFail) {
		t.Fatalf("continuation row lost the fail colour: %q", lines)
	}
}
