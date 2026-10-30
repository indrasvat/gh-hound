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
	if ansi.StringWidth(head) > 77 {
		t.Fatalf("a cut header must keep the cell before the border, like every header: %d cells, %q", ansi.StringWidth(head), head)
	}
	if !strings.HasSuffix(strings.TrimRight(head, " "), right) || !strings.Contains(head, "… · @indrasvat") {
		t.Fatalf("header %q lost its right side or identity", head)
	}
	if short := ansi.Strip(renderHeader(th, "hound", "⎇ branch main · @indrasvat", right, 78)); !strings.Contains(short, "⎇ branch main · @indrasvat") {
		t.Fatalf("a header that fits changed: %q", short)
	}
}

// Only an identity is kept at the context's end: other contexts (the
// scent check's workflow · branch) keep their start and give way at the end.
func TestHeaderCutKeepsTheStartOfOtherContexts(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	head := ansi.Strip(renderHeader(th, "hound", "the scent check › Security Audit · feature/very-long-authentication-refactoring", "flaky · score 0.82", 78))
	if !strings.Contains(head, "the scent check › Security Audit") || !strings.Contains(head, "flaky · score 0.82") {
		t.Fatalf("header %q lost the workflow or its right side", head)
	}
	// A long login with a wide right side keeps the identity, not the branch.
	login := "@" + strings.Repeat("a", 39)
	head = ansi.Strip(renderHeader(th, "hound", "⎇ branch fix/parser · "+login, "6 runs loaded · live · 4999/5k · 304", 78))
	if !strings.Contains(head, login[:20]) || ansi.StringWidth(head) > 78 {
		t.Fatalf("header %q dropped the identity", head)
	}
}
