package icons

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestStatusAndConclusionGlyphsMatchHTMLMock(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"success", ForConclusion(model.ConclusionSuccess), "✔"},
		{"failure", ForConclusion(model.ConclusionFailure), "✗"},
		{"in progress", ForStatus(model.StatusInProgress), "⠹"},
		{"queued", ForStatus(model.StatusQueued), "◌"},
		{"cancelled", ForConclusion(model.ConclusionCancelled), "⊘"},
		{"skipped", ForConclusion(model.ConclusionSkipped), "⊝"},
		{"action required", ForConclusion(model.ConclusionActionRequired), "▲"},
		{"timed out", ForConclusion(model.ConclusionTimedOut), "⧗"},
		{"neutral", ForConclusion(model.ConclusionNeutral), "◇"},
		{"cursor", Cursor, "▌"},
		{"branch", Branch, "⎇"},
		{"breadcrumb", Breadcrumb, "›"},
		{"fold open", FoldOpen, "▾"},
		{"fold closed", FoldClosed, "▸"},
		{"prompt", Prompt, "❯"},
		{"rerun", Rerun, "↻"},
		{"dispatch", Dispatch, "▶"},
		{"enter", Enter, "⏎"},
		{"escape", Escape, "⎋"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s glyph = %q, want %q", tt.name, tt.got, tt.want)
		}
		if strings.Contains(tt.got, "\ufe0f") {
			t.Fatalf("%s glyph contains VS16 emoji selector", tt.name)
		}
	}
}
