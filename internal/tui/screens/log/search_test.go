package log

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

// hitRows returns the plain text of a view's tinted rows; current is the
// first row with the current-hit tint.
func hitRows(view string) (hits []string, current string) {
	for l := range strings.SplitSeq(view, "\n") {
		text := strings.TrimRight(plain(l), " ")
		switch {
		case strings.HasPrefix(l, bramble.fg+bramble.runTintStrongBG):
			if current == "" {
				current = text
			}
			hits = append(hits, text)
		case strings.HasPrefix(l, bramble.fg+bramble.runTintBG):
			hits = append(hits, text)
		}
	}
	return hits, current
}

func search(m Model, query string) Model {
	m = m.Update(KeyMsg{Key: "/"})
	for _, r := range query {
		m = m.Update(KeyMsg{Key: string(r)})
	}
	return m.Update(KeyMsg{Key: "enter"})
}

func body(view string) []string {
	lines := strings.Split(plain(view), "\n")[1:]
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

func TestSearchHitsAreTintedWithoutMovingTheGutter(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		for _, wrap := range []bool{false, true} {
			base := NewModel(document(), 1, 12)
			base.Width = width
			if wrap {
				base = base.Update(KeyMsg{Key: "w"})
			}
			found := search(base, "trail").Update(KeyMsg{Key: "g"})
			if got, want := body(View(found, width)), body(View(base, width)); !slices.Equal(got, want) {
				t.Fatalf("width %d wrap %v: a search changed the text:\n%s\nwant:\n%s", width, wrap, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			hits, current := hitRows(View(found, width))
			if len(hits) != 2 || !strings.HasPrefix(hits[1], "007 ") || !strings.HasPrefix(current, "005 ") {
				t.Fatalf("width %d wrap %v: hits %q current %q", width, wrap, hits, current)
			}
		}
	}
}

func TestAWrappedHitIsTintedOnEveryRow(t *testing.T) {
	doc := logs.Parse("first\nneedle " + strings.Repeat("y", 150) + "\nlast")
	m := NewModel(doc, 1, 10)
	m.Width = 60
	m = search(m.Update(KeyMsg{Key: "w"}), "needle").Update(KeyMsg{Key: "g"})
	hits, current := hitRows(View(m, 60))
	if len(hits) != 3 || !strings.HasPrefix(current, "002 needle") || !strings.Contains(hits[1], "↪") {
		t.Fatalf("all three rows of the wrapped hit are tinted, the first is current: %q current %q", hits, current)
	}
}

func TestTheCurrentHitIsMarkedApart(t *testing.T) {
	m := search(NewModel(document(), 1, 12), "trail").Update(KeyMsg{Key: "g"})
	view := View(m, 80)
	if !strings.Contains(view, bramble.run+"005") || !strings.Contains(view, bramble.line2+"007") {
		t.Fatalf("only the current hit's line number is warn:\n%q", view)
	}
	m = m.Update(KeyMsg{Key: "n"}).Update(KeyMsg{Key: "g"})
	if _, current := hitRows(View(m, 80)); !strings.HasPrefix(current, "007 ") {
		t.Fatalf("n moves the current hit, got %q", current)
	}
}

func TestEscClearsOneLayerAtATime(t *testing.T) {
	esc := KeyMsg{Key: "esc"}
	m := search(NewModel(document(), 1, 12), "trail").SetRange(1, 9, "17:42-17:44")
	if hits, _ := hitRows(View(m, 80)); len(hits) != 2 {
		t.Fatalf("setup: two tinted hits, got %q", hits)
	}
	m = m.Update(esc)
	if m.RangeLabel != "" || m.Search.Query != "trail" {
		t.Fatalf("the first esc clears only the range: range %q search %q", m.RangeLabel, m.Search.Query)
	}
	before := m
	m = m.Update(esc)
	if m.Search.Query != "" || m.Search.Total != 0 || !m.Changed(before) {
		t.Fatalf("the second esc clears the search: %#v", m.Search)
	}
	if hits, _ := hitRows(View(m, 80)); len(hits) > 0 || firstLine(View(m, 80)) != "log" {
		t.Fatalf("a cleared search leaves no tint and a plain header: %q\n%s", hits, plain(View(m, 80)))
	}
	m = search(m, "trail").Update(KeyMsg{Key: "/"}).Update(esc)
	if m.InputMode || m.Search.Query != "trail" {
		t.Fatalf("esc in the input only closes the input: input %v search %q", m.InputMode, m.Search.Query)
	}
}

func firstLine(s string) string { l, _, _ := strings.Cut(plain(s), "\n"); return strings.TrimSpace(l) }

func clockLog(n int) logs.Document {
	var b strings.Builder
	for i := range n {
		text := "work"
		if i == 4 {
			text = "marker"
		}
		fmt.Fprintf(&b, "10:%02d:%02dZ %s %d\n", i/60, i%60, text, i+1)
	}
	return logs.Parse(b.String())
}

func TestJumpEchoClearsOnceTheLineLeavesTheScreen(t *testing.T) {
	for _, tc := range []struct {
		keys string
		keep bool
	}{
		{"k", true},
		{"k k k k k k k k k", true}, // line 100 is the last row on screen
		{"k k k k k k k k k k", false},
		{"ctrl+u", true},
		{"j", false},
		{"G", false},
		{"g", false},
		{"c", true},
		{"/ m a r k e r enter", false},
	} {
		m := NewModel(clockLog(200), 1, 10).JumpToLine(100)
		m.LastJump = "10:01:39"
		for k := range strings.FieldsSeq(tc.keys) {
			m = m.Update(KeyMsg{Key: k})
		}
		if kept := m.LastJump != ""; kept != tc.keep {
			t.Fatalf("after %q echo kept=%v, want %v (rows %v)", tc.keys, kept, tc.keep, m.VisibleRowNumbers())
		}
		if shown := strings.Contains(firstLine(View(m, 80)), "t→10:01:39"); shown != tc.keep && !strings.Contains(tc.keys, "/") {
			t.Fatalf("after %q the header shows the echo=%v", tc.keys, shown)
		}
	}
}

func TestJumpEchoClearsWhenZFoldsTheLineAway(t *testing.T) {
	m := compactModel(t, pollLog(), 10).JumpToLine(300)
	m.LastJump = "10:24:20"
	if m = m.Update(KeyMsg{Key: "Z"}); m.LastJump == "" {
		t.Fatal("Z keeps the jumped line on screen")
	}
	if m = m.Update(KeyMsg{Key: "z"}); m.LastJump != "" {
		t.Fatalf("z folds line 300 away, the echo must go: rows %v", m.VisibleRowNumbers())
	}
}

func TestJumpEchoStaysWhileStepping(t *testing.T) {
	m := NewModel(oversizedLog(), 1, 5)
	m.Width = 60
	m = m.Update(KeyMsg{Key: "w"}).JumpToLine(2)
	m.LastJump = "10:00"
	if m = m.Update(KeyMsg{Key: "j"}); m.LastJump == "" {
		t.Fatal("j inside a wrapped line keeps it on screen")
	}
}

func TestJumpEchoStaysOnAGroupHeader(t *testing.T) {
	var b strings.Builder
	for i := range 60 {
		fmt.Fprintf(&b, "10:00:%02dZ work %d\n", i, i+1)
	}
	b.WriteString("10:01:00Z ##[group]Run make check\n10:01:01Z make check\n10:01:02Z ##[endgroup]\n10:01:03Z done\n")
	m := NewModel(logs.Parse(b.String()), 1, 10).JumpTo("10:01:00")
	if m.Offset != 61 || m.LastJump == "" {
		t.Fatalf("setup: the jump lands on the group header, offset %d", m.Offset)
	}
	if m = m.Update(KeyMsg{Key: "k"}); m.LastJump == "" {
		t.Fatalf("k keeps the group header on screen: rows %v", m.VisibleRowNumbers())
	}
}

func TestAClearedEchoRepaints(t *testing.T) {
	m := NewModel(clockLog(200), 1, 10).JumpToLine(100)
	m.LastJump = "10:01:39"
	m.Height = 3 // a resize leaves line 100 below the screen
	m.Offset = 90
	before := m
	if m = m.Update(KeyMsg{Key: "x"}); m.LastJump != "" || !m.Changed(before) {
		t.Fatalf("a key that only clears the echo must still repaint: echo %q changed %v", m.LastJump, m.Changed(before))
	}
}

func TestASearchHeaderSaysEscClearsIt(t *testing.T) {
	m := search(NewModel(document(), 1, 12), "trail")
	if head := firstLine(View(m, 80)); head != "log · /trail · match 1/2 · ⎋ clear" {
		t.Fatalf("header %q", head)
	}
	c := search(compactModel(t, pollLog(), 10), "10:30:20")
	if head := firstLine(View(c, 120)); !strings.HasPrefix(head, "log · /10:30:20 · match 1/1 · compact · ⎋ clear") {
		t.Fatalf("compact header %q", head)
	}
}
