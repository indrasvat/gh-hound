package keys

import (
	"os"
	"strings"
	"testing"
)

func TestScreenFootersMatchVisualContract(t *testing.T) {
	contract := readVisualContract(t, "../../../docs/visual-contract.md")
	expected := map[Screen]string{
		ScreenWelcome:  "enter continue · ? help · q quit",
		ScreenAllGreen: "w watch next push · D dispatch · / filter · ? help",
		ScreenRunsList: "enter open · r rerun · x cancel · l logs · w watch · / filter · ? help",
		ScreenDetail:   "enter expand · r rerun job · R rerun failed · x cancel · esc back · ?",
		ScreenFailure:  "R rerun failed · r rerun job · l full log · o browser · y copy excerpt",
		ScreenWatch:    "x cancel · f follow · d debug · esc detach",
		ScreenLog:      "j/k scroll · g/G ends · / search · n/N match · z/Z fold · w wrap · esc back",
		ScreenDispatch: "enter run · tab next · esc cancel",
		ScreenPalette:  "workflows · watch · diff (v2) · theme",
		ScreenHelp:     ": palette · ? close · esc close",
		ScreenToasts:   "esc dismiss · g dismiss all · r retry · ? help",
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
