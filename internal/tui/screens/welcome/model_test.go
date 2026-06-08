package welcome

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
)

func TestViewMatchesWelcomeMockContent(t *testing.T) {
	view := ansi.Strip(View(Model{Build: banner.BuildInfo{Version: "v0.1.0"}}))
	for _, want := range []string{
		"Hunt down your GitHub Actions CI",
		"WATCH",
		"DIAGNOSE",
		"RERUN",
		"⏎ Press Enter to continue",
		"v0.1.0 · github.com/indrasvat/gh-hound",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("welcome view missing %q\n%s", want, view)
		}
	}
}
