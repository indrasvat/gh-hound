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
