package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/indrasvat/gh-hound/internal/theme"
)

func downsampled(profile colorprofile.Profile, sgr string) string {
	var b bytes.Buffer
	w := &colorprofile.Writer{Forward: &b, Profile: profile}
	_, _ = fmt.Fprint(w, sgr+"x")
	return b.String()
}

func TestLimitedThemeFillsStayDistinctAt256(t *testing.T) {
	for _, mode := range []theme.Mode{theme.ModeBramble, theme.ModeBone} {
		truecolor, limited := theme.ForMode(mode), theme.ForMode(mode).Limited()
		if bg := downsampled(colorprofile.ANSI256, theme.Bg(truecolor.BG)); downsampled(colorprofile.ANSI256, theme.Bg(truecolor.Surface2)) != bg && mode == theme.ModeBramble {
			t.Fatalf("%s: the truecolor selection fill no longer folds onto the background; is Limited still needed?", mode)
		}
		seen := map[string]string{downsampled(colorprofile.ANSI256, theme.Bg(limited.BG)): "BG"}
		for name, fill := range map[string]string{
			"BGElev": limited.BGElev, "Surface": limited.Surface, "Surface2": limited.Surface2,
			"OKTint": limited.OKTint, "RunTint": limited.RunTint, "RunTintStrong": limited.RunTintStrong,
			"FailTint": limited.FailTint, "InfoTint": limited.InfoTint,
		} {
			got := downsampled(colorprofile.ANSI256, theme.Bg(fill))
			if prev, dup := seen[got]; dup {
				t.Errorf("%s: %s and %s both downsample to %q", mode, name, prev, got)
			}
			seen[got] = name
		}
		if limited.FG != truecolor.FG || limited.OK != truecolor.OK {
			t.Errorf("%s: Limited changed text colours", mode)
		}
	}
}

// At 16 colours only the strongest fills survive; the selection must.
func TestLimitedSelectionSurvivesSixteenColours(t *testing.T) {
	for _, mode := range []theme.Mode{theme.ModeBramble, theme.ModeBone} {
		l := theme.ForMode(mode).Limited()
		if bg, sel := downsampled(colorprofile.ANSI, theme.Bg(l.BG)), downsampled(colorprofile.ANSI, theme.Bg(l.Surface2)); bg == sel {
			t.Errorf("%s: the selection fill downsamples onto the background at 16 colours (%q)", mode, sel)
		}
	}
}
