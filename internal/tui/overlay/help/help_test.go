package help

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

func TestHelpUsesActiveKeymapAndLegend(t *testing.T) {
	view := View(keys.ScreenRunsList, 80)
	for _, want := range []string{"help · gh hound", "Navigate", "Actions", "View", "⏎ open", "↻ rerun", "Legend", "✔ success", "✗ failure"} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view missing %q\n%s", want, view)
		}
	}
}

func TestHelpWrapsLongSectionsInsteadOfTruncating(t *testing.T) {
	// The detail screen's View section overflows 60 cols; every entry
	// must survive on some line, none truncated into an ellipsis.
	view := View(keys.ScreenDetail, 60)
	for _, want := range []string{"a artifacts", "d download artifact", "o open artifact folder / browser", "y copy artifact path / run URL"} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view lost %q after wrapping\n%s", want, view)
		}
	}
	for line := range strings.SplitSeq(view, "\n") {
		if len([]rune(line)) > 60 {
			t.Fatalf("help line overflows width: %q", line)
		}
	}
}
