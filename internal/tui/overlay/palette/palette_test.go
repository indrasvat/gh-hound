package palette

import (
	"strings"
	"testing"
)

func TestPaletteFiltersMovesAndSelectsRoute(t *testing.T) {
	m := New(DefaultItems())
	m = m.Update(KeyMsg{Key: "r"})
	m = m.Update(KeyMsg{Key: "u"})
	if got := m.Visible(); len(got) != 3 || got[0].Name != "runs" {
		t.Fatalf("visible = %#v", got)
	}
	m = m.Update(KeyMsg{Key: "j"})
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Route != "runs --all" {
		t.Fatalf("intent = %#v", m.Intent)
	}
	view := View(m, 80)
	for _, want := range []string{"❯ ru", "▌runs --all", "workflows · watch · diff (v2) · theme"} {
		if !strings.Contains(view, want) {
			t.Fatalf("palette view missing %q\n%s", want, view)
		}
	}
}
