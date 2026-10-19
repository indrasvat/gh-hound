package palette

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPaletteFiltersMovesAndSelectsRoute(t *testing.T) {
	m := New(DefaultItems())
	m = m.Update(KeyMsg{Key: "r"})
	m = m.Update(KeyMsg{Key: "u"})
	if got := m.Visible(); len(got) != 4 || got[0].Name != "runs" {
		t.Fatalf("visible = %#v", got)
	}
	m = m.Update(KeyMsg{Key: "down"})
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Route != "runs --all" {
		t.Fatalf("intent = %#v", m.Intent)
	}
	view := View(m, 80)
	for _, want := range []string{"❯ ru", "▌runs --all", "workflows · watch · diff · theme"} {
		if !strings.Contains(view, want) {
			t.Fatalf("palette view missing %q\n%s", want, view)
		}
	}
}

func TestPaletteSelectionCarriesStableRouteValue(t *testing.T) {
	m := New([]Item{
		{Name: "dispatch: Release", Description: "workflow_dispatch", Route: "dispatch", Value: "release.yml"},
	})
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Route != "dispatch" || m.Intent.Value != "release.yml" {
		t.Fatalf("intent = %#v", m.Intent)
	}
}

// TestQueryTypingAcceptsJAndK pins the round-13 P1 fix: the query is
// a TEXT INPUT, so every printable letter — including j and k, which
// once doubled as navigation and were silently swallowed — must
// append. Reintroducing `case "j", "down":` turns this red.
func TestQueryTypingAcceptsJAndK(t *testing.T) {
	m := New(DefaultItems())
	for _, key := range []string{"w", "o", "r", "k", "f", "l", "o", "w", "s"} {
		m = m.Update(KeyMsg{Key: key})
	}
	if m.Query != "workflows" {
		t.Fatalf("query = %q, want %q (j/k must type, not navigate)", m.Query, "workflows")
	}
	m = New(DefaultItems())
	for _, key := range []string{"j", "u", "m", "k"} {
		m = m.Update(KeyMsg{Key: key})
	}
	if m.Query != "jumk" {
		t.Fatalf("query = %q, want %q", m.Query, "jumk")
	}
}

func TestViewSizeWindowsLongListsAroundSelection(t *testing.T) {
	items := []Item{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		items = append(items, Item{Name: name, Description: name + " item"})
	}
	m := New(items)
	view := ViewSize(m, 60, 4)
	if !strings.Contains(view, "▌a · a item") || strings.Contains(view, "d · d item") || !strings.Contains(view, "… ↓ 4 more") {
		t.Fatalf("first window:\n%s", view)
	}
	for range 6 {
		m = m.Update(KeyMsg{Key: "down"})
	}
	view = ViewSize(m, 60, 4)
	if !strings.Contains(view, "▌g · g item") || strings.Contains(view, "a · a item") || !strings.Contains(view, "… ↑ 4 more") {
		t.Fatalf("selection must stay visible:\n%s", view)
	}
	if got := strings.Count(ViewSize(m, 60, 0), " item"); got != 7 {
		t.Fatalf("unbounded view shows %d items", got)
	}
}

func TestNamePrefixMatchesLead(t *testing.T) {
	m := New([]Item{{Name: "runs --all", Description: "runs across all branches"}, {Name: "All actors", Description: "clear the actor filter"}})
	m.Query = "all"
	if got := m.Visible(); len(got) != 2 || got[0].Name != "All actors" {
		t.Fatalf("visible = %+v", got)
	}
}

func TestBackspaceRemovesAWholeRune(t *testing.T) {
	m := New(nil)
	for _, key := range []string{"c", "é", "backspace"} {
		m = m.Update(KeyMsg{Key: key})
	}
	if m.Query != "c" {
		t.Fatalf("query = %q, want %q", m.Query, "c")
	}
}

func TestLongQueryKeepsItsEndInView(t *testing.T) {
	m := New(nil)
	for key := range strings.SplitSeq(strings.Repeat("x", 90)+"END", "") {
		m = m.Update(KeyMsg{Key: key})
	}
	view := View(m, 60)
	for line := range strings.SplitSeq(view, "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Fatalf("line wider than the box: %q", line)
		}
	}
	if !strings.Contains(view, "END") {
		t.Fatalf("typed end scrolled off:\n%s", view)
	}
}
