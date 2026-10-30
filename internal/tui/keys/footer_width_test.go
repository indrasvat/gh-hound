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
	for _, screen := range []Screen{ScreenDetail, ScreenAllGreen} {
		if got, want := FooterForScreenWidth(screen, 20), FooterForScreen(screen); got != want {
			t.Fatalf("%s: %q, want %q", screen, got, want)
		}
	}
}

// The runs footer is 86 cells; at 80 columns it once ended "/ …",
// cutting filter and help.
func TestRunsFooterFitsEightyColumns(t *testing.T) {
	got := FooterForScreenWidth(ScreenRunsList, 78)
	if n := utf8.RuneCountInString(got); n > 78 {
		t.Fatalf("%d cells: %q", n, got)
	}
	if !strings.Contains(got, "/ filter") || !strings.HasSuffix(got, "· ?") || strings.Contains(got, "s scope") {
		t.Fatalf("80-column runs footer = %q, want / filter and ? kept, s scope dropped first", got)
	}
	if wide := FooterForScreenWidth(ScreenRunsList, 120); !strings.Contains(wide, "s scope") {
		t.Fatalf("wide runs footer dropped s scope: %q", wide)
	}
}
