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

func TestHelpOmitsEmptyActionsSection(t *testing.T) {
	// The scent check binds no rerun/cancel/dispatch keys: the help
	// overlay must not render an "Actions" heading over a blank line.
	view := View(keys.ScreenFlakes, 80)
	if strings.Contains(view, "Actions") {
		t.Fatalf("action-less screen rendered an empty Actions section:\n%s", view)
	}
	// The sections it DOES have still render.
	for _, want := range []string{"Navigate", "Legend"} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view missing %q:\n%s", want, view)
		}
	}
	// No stray blank section lines (heading immediately followed by a
	// blank then another heading).
	for line := range strings.SplitSeq(view, "\n") {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("help view has a blank line — empty section leaked:\n%s", view)
		}
	}
}
