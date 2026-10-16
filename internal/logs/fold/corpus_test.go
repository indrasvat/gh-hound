package fold

import (
	"slices"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs/fold/internal/corpus"
)

// TestCorpusQuality is the quality gate: on every labelled scenario the
// default engine is lossless, hides no signal line and folds nothing in the
// negative controls; across scenarios it hides nearly all achievable noise.
func TestCorpusQuality(t *testing.T) {
	e := Default()
	hiddenNoise, achievable := 0, 0
	for _, log := range corpus.All(1) {
		p, err := e.SafePlan(log.Lines, DefaultLimits())
		if err != nil {
			t.Fatalf("%s: %v", log.Name, err)
		}
		if !slices.Equal(p.Expand(), log.Lines) {
			t.Fatalf("%s: not lossless", log.Name)
		}
		for i, hidden := range p.HiddenMask() {
			if !hidden {
				continue
			}
			switch log.Labels[i] {
			case corpus.Signal:
				t.Errorf("%s: signal line %d hidden: %q", log.Name, i+1, log.Lines[i])
			case corpus.Noise:
				hiddenNoise++
			}
		}
		if log.Class == "negative" && p.Rows() != len(log.Lines) {
			t.Errorf("%s: negative control folded %d rows", log.Name, len(log.Lines)-p.Rows())
		}
		for _, b := range log.Blocks {
			if hc := b.Count - 2; hc >= 1 && hc*b.Period >= 2 {
				achievable += hc * b.Period
			}
		}
	}
	if recall := float64(hiddenNoise) / float64(achievable); recall < 0.95 {
		t.Errorf("noise recall %.3f < 0.95 (%d of %d)", recall, hiddenNoise, achievable)
	}
}
