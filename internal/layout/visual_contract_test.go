package layout

import (
	"os"
	"strings"
	"testing"
)

func TestBreakpointsMatchVisualContract(t *testing.T) {
	contract := readVisualContract(t, "../../docs/visual-contract.md")
	expected := map[string]string{
		"80x24":  "single column; detail is full-screen push; keep status workflow number age",
		"120x40": "master-detail side-by-side; full runs columns",
		"200x60": "side-by-side with extra padding; add actor and sha",
	}

	for geometry, rule := range expected {
		row := "| " + geometry + " | " + rule + " |"
		if !strings.Contains(contract, row) {
			t.Fatalf("visual contract missing breakpoint row %q", row)
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
