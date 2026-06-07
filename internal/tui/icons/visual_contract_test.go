package icons

import (
	"os"
	"strings"
	"testing"
)

func TestGlyphsMatchVisualContract(t *testing.T) {
	contract := readVisualContract(t, "../../../docs/visual-contract.md")
	expected := map[string]string{
		"success":         Success,
		"failure":         Failure,
		"in_progress":     InProgress,
		"queued":          Queued,
		"cancelled":       Cancelled,
		"skipped":         Skipped,
		"action_required": ActionRequired,
		"timed_out":       TimedOut,
		"neutral":         Neutral,
		"selection_bar":   Cursor,
		"branch":          Branch,
		"breadcrumb":      Breadcrumb,
		"fold_open":       FoldOpen,
		"fold_closed":     FoldClosed,
		"prompt":          Prompt,
		"rerun":           Rerun,
		"dispatch":        Dispatch,
		"enter":           Enter,
		"escape":          Escape,
	}
	for name, glyph := range expected {
		row := "| " + name + " | " + glyph + " |"
		if !strings.Contains(contract, row) {
			t.Fatalf("visual contract missing glyph row %q", row)
		}
	}
	if strings.Contains(contract, "\ufe0f") {
		t.Fatal("visual contract contains emoji variation selector")
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
