package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
)

// A long branch name once pushed the header's right side (run count,
// live) off an 80-column frame; the branch now gives way, the identity
// and the right side stay.
func TestHeaderKeepsItsRightSideBesideALongBranch(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	context := "⎇ branch indrasvat/honour-config-file-flag-precedence-theme-tokens · @indrasvat"
	right := "6 runs loaded · live"
	head := ansi.Strip(renderHeader(th, "hound", context, right, 78))
	if ansi.StringWidth(head) > 78 {
		t.Fatalf("header is %d cells: %q", ansi.StringWidth(head), head)
	}
	if !strings.HasSuffix(strings.TrimRight(head, " "), right) || !strings.Contains(head, "… · @indrasvat") {
		t.Fatalf("header %q lost its right side or identity", head)
	}
	if short := ansi.Strip(renderHeader(th, "hound", "⎇ branch main · @indrasvat", right, 78)); !strings.Contains(short, "⎇ branch main · @indrasvat") {
		t.Fatalf("a header that fits changed: %q", short)
	}
}
