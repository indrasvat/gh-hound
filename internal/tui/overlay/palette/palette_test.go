package palette

import (
	"strings"
	"testing"
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
