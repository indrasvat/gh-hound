package theme

import (
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestBrambleAndBoneTokensMatchHTMLMock(t *testing.T) {
	bramble := ForMode(ModeBramble)
	bone := ForMode(ModeBone)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"bramble bg", bramble.BG, "#0E0F0C"},
		{"bramble bg elevated", bramble.BGElev, "#141512"},
		{"bramble surface", bramble.Surface, "#1B1D17"},
		{"bramble surface2", bramble.Surface2, "#24271E"},
		{"bramble line", bramble.Line, "#2E3227"},
		{"bramble line2", bramble.Line2, "#3D4233"},
		{"bramble fg", bramble.FG, "#EAE8D9"},
		{"bramble dim", bramble.Dim, "#6B7060"},
		{"bramble ok", bramble.OK, "#4FD37A"},
		{"bramble fail", bramble.Fail, "#E2564B"},
		{"bramble run", bramble.Run, "#E0A33E"},
		{"bramble info", bramble.Info, "#6E9CB5"},
		{"bramble warn", bramble.Warn, "#E8895A"},
		{"bramble neutral", bramble.Neutral, "#6B7060"},
		{"bone bg", bone.BG, "#EFEDE1"},
		{"bone fg", bone.FG, "#23241C"},
		{"bone ok", bone.OK, "#1F9E55"},
		{"bone fail", bone.Fail, "#C24033"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestSemanticMappingMatchesPRD(t *testing.T) {
	bramble := ForMode(ModeBramble)

	if got := bramble.SemanticForStatus(model.StatusInProgress); got != bramble.Run {
		t.Fatalf("in_progress color = %q, want run %q", got, bramble.Run)
	}
	if got := bramble.SemanticForStatus(model.StatusQueued); got != bramble.Info {
		t.Fatalf("queued color = %q, want info %q", got, bramble.Info)
	}
	if got := bramble.SemanticForConclusion(model.ConclusionSuccess); got != bramble.OK {
		t.Fatalf("success color = %q, want ok %q", got, bramble.OK)
	}
	if got := bramble.SemanticForConclusion(model.ConclusionActionRequired); got != bramble.Warn {
		t.Fatalf("action_required color = %q, want warn %q", got, bramble.Warn)
	}
	if got := bramble.SemanticForConclusion(model.ConclusionSkipped); got != bramble.Neutral {
		t.Fatalf("skipped color = %q, want neutral %q", got, bramble.Neutral)
	}
}
