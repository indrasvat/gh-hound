package keys

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLogFooterFitsNarrowWidths(t *testing.T) {
	full := FooterForScreen(ScreenLog)
	if !strings.Contains(full, "c compact") {
		t.Fatalf("log footer lacks c compact: %q", full)
	}
	if got := FooterForScreenWidth(ScreenLog, 200); got != full {
		t.Fatalf("wide footer = %q, want %q", got, full)
	}
	for _, width := range []int{78, 60, 40} {
		got := FooterForScreenWidth(ScreenLog, width)
		if n := utf8.RuneCountInString(got); n > width {
			t.Fatalf("width %d: %d runes: %q", width, n, got)
		}
		if !strings.HasSuffix(got, "⎋ back") {
			t.Fatalf("width %d lost back: %q", width, got)
		}
	}
	if got := FooterForScreenWidth(ScreenLog, 78); !strings.Contains(got, "c compact") || !strings.Contains(got, "/ search") {
		t.Fatalf("78 dropped a primary hint: %q", got)
	}
}

func TestFooterWithoutPrioritiesIsUnchanged(t *testing.T) {
	for _, screen := range []Screen{ScreenRunsList, ScreenDetail} {
		if got, want := FooterForScreenWidth(screen, 20), FooterForScreen(screen); got != want {
			t.Fatalf("%s: %q, want %q", screen, got, want)
		}
	}
}
