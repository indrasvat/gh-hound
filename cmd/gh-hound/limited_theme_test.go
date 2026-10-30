package main

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
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

var fillNames = []string{"BGElev", "Surface", "Surface2", "OKTint", "RunTint", "RunTintStrong", "FailTint", "InfoTint"}

func fills(t theme.Theme) map[string]string {
	return map[string]string{
		"BGElev": t.BGElev, "Surface": t.Surface, "Surface2": t.Surface2, "OKTint": t.OKTint, "RunTint": t.RunTint,
		"RunTintStrong": t.RunTintStrong, "FailTint": t.FailTint, "InfoTint": t.InfoTint,
	}
}

func TestPalette256FillsStayDistinct(t *testing.T) {
	for _, mode := range []theme.Mode{theme.ModeBramble, theme.ModeBone} {
		truecolor, limited := theme.ForMode(mode), theme.ForMode(mode).For(theme.Palette256)
		if mode == theme.ModeBramble && downsampled(colorprofile.ANSI256, theme.Bg(truecolor.Surface2)) != downsampled(colorprofile.ANSI256, theme.Bg(truecolor.BG)) {
			t.Fatalf("the truecolor selection fill no longer folds onto the background; is the 256 theme still needed?")
		}
		seen := map[string]string{downsampled(colorprofile.ANSI256, theme.Bg(limited.BG)): "BG"}
		for _, name := range fillNames {
			got := downsampled(colorprofile.ANSI256, theme.Bg(fills(limited)[name]))
			if prev, dup := seen[got]; dup {
				t.Errorf("%s: %s and %s both downsample to %q", mode, name, prev, got)
			}
			seen[got] = name
		}
		if limited.FG != truecolor.FG || limited.OK != truecolor.OK {
			t.Errorf("%s: the 256 theme changed the foreground colours", mode)
		}
	}
}

// xterm256 is the RGB an index shows on a standard xterm palette.
func xterm256(index int) [3]float64 {
	if index >= 232 {
		v := float64(8 + 10*(index-232))
		return [3]float64{v, v, v}
	}
	level := func(n int) float64 {
		if n == 0 {
			return 0
		}
		return float64(55 + 40*n)
	}
	i := index - 16
	return [3]float64{level(i / 36), level(i / 6 % 6), level(i % 6)}
}

func rgbOf(t *testing.T, token string) [3]float64 {
	if n, err := strconv.Atoi(token); err == nil {
		return xterm256(n)
	}
	v, err := strconv.ParseUint(token[1:], 16, 32)
	if err != nil {
		t.Fatalf("token %q", token)
	}
	return [3]float64{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)}
}

func contrast(a, b [3]float64) float64 {
	lum := func(c [3]float64) float64 {
		var l [3]float64
		for i, v := range c {
			v /= 255
			if v <= 0.03928 {
				l[i] = v / 12.92
			} else {
				l[i] = math.Pow((v+0.055)/1.055, 2.4)
			}
		}
		return 0.2126*l[0] + 0.7152*l[1] + 0.0722*l[2]
	}
	x, y := lum(a)+0.05, lum(b)+0.05
	return math.Max(x, y) / math.Min(x, y)
}

// Dim and subtle text (line numbers, timestamps, counts) sit on every
// fill: at 256 colours the cube's darkest hues were as bright as that
// text, so a search hit could not be read.
func TestPalette256DimTextReadsOnEveryFill(t *testing.T) {
	l := theme.ForMode(theme.ModeBramble).For(theme.Palette256)
	for _, text := range []string{l.Dim, l.Subtle} {
		for _, name := range fillNames {
			if c := contrast(rgbOf(t, text), rgbOf(t, fills(l)[name])); c < 2 {
				t.Errorf("text %s on %s (%s): contrast %.2f, want >= 2", text, name, fills(l)[name], c)
			}
		}
	}
}

// At 16 colours a fill shares a colour with the text drawn on it, so the
// 16-colour theme draws none: rows keep every glyph.
func TestPalette16DrawsNoFills(t *testing.T) {
	for _, mode := range []theme.Mode{theme.ModeBramble, theme.ModeBone} {
		basic := theme.ForMode(mode).For(theme.Palette16)
		for _, name := range fillNames {
			if sgr := theme.Bg(fills(basic)[name]); sgr != "" {
				t.Errorf("%s: %s still fills (%q)", mode, name, sgr)
			}
		}
	}
}
