package keys

import (
	"os"
	"strings"
	"testing"
)

func TestScreenFootersMatchVisualContract(t *testing.T) {
	contract := readVisualContract(t, "../../../docs/visual-contract.md")
	expected := map[Screen]string{
		ScreenWelcome:  "⏎ continue · ? help · q quit",
		ScreenAllGreen: "j/k move · s scope · ⏎ open · w watch · D dispatch · / filter · ? help",
		ScreenRunsList: "j/k · s scope · ⏎ open · ↻ rerun · ✗ cancel · l log · w watch · f status · / filter · ?",
		ScreenDetail:   "⏎ expand · ↻ rerun job · R rerun failed · ✗ cancel · ⎋ back · ?",
		ScreenFailure:  "↻ rerun failed · r rerun job · l full log · o browser · y copy excerpt",
		ScreenWatch:    "✗ cancel · f follow · d debug · ⎋ detach",
		ScreenLog:      "j/k scroll · g/G ends · / search · t time · n/N match · z/Z fold · w wrap · ⎋ back",
		ScreenDispatch: "⏎ run · ⇥ next · ⎋ cancel",
		ScreenDiff:     "j/k move · ⏎ first bad · o compare · ⎋ back",
		ScreenPalette:  "workflows · watch · diff · theme",
		ScreenCaches:   "j/k move · s sort · / filter · d dig up · D dig up key · ⎋ back · ?",
		ScreenHelp:     ": palette · ? close · ⎋ close",
		ScreenToasts:   "⎋ dismiss · g dismiss all · r retry · ? help",
	}

	for screen, want := range expected {
		if got := FooterForScreen(screen); got != want {
			t.Fatalf("%s footer = %q, want %q", screen, got, want)
		}
		row := "| " + string(screen) + " | " + want + " |"
		if !strings.Contains(contract, row) {
			t.Fatalf("visual contract missing footer row %q", row)
		}
	}
}

func readVisualContract(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read visual contract: %v", err)
	}
	return string(data)
}
