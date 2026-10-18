package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/approvals"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
)

// Bramble: bg #0E0F0C. Worked by hand from brightness(.46) toward the bg,
// then saturate(.65) with the CSS weights .213/.715/.072.
const (
	dimFGBramble = "38;2;115;115;115" // fg #EAE8D9 → 115,115,109, as a neutral grey
	dimOKBramble = "38;2;60;100;72"   // ok #4FD37A
)

func TestDimBlendsTowardTheBackground(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	got := dimANSI(th, "\x1b[38;2;79;211;122mok\x1b[0m plain")
	want := "\x1b[" + dimFGBramble + "m\x1b[" + dimOKBramble + "mok\x1b[0;" + dimFGBramble + "m plain"
	if got != want {
		t.Fatalf("dim:\n got %q\nwant %q", got, want)
	}
}

func TestDimKeepsAttributesAndDimsBackgrounds(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	got := dimANSI(th, "\x1b[1;4;38;2;79;211;122;48;2;14;15;12mX\x1b[39mY\x1b[mZ")
	want := "\x1b[" + dimFGBramble + "m\x1b[1;4;" + dimOKBramble + ";48;2;14;15;13mX\x1b[" + dimFGBramble + "mY\x1b[0;" + dimFGBramble + "mZ"
	if got != want {
		t.Fatalf("dim:\n got %q\nwant %q", got, want)
	}
}

func TestDimOnBoneFadesTowardTheLightBackground(t *testing.T) {
	th := theme.ForMode(theme.ModeBone)
	fg, bg := hexRGB(th.FG), hexRGB(th.BG)
	dimmed := truecolors(dimANSI(th, "text"))
	if len(dimmed) != 1 {
		t.Fatalf("plain text takes the dimmed fg, got %v", dimmed)
	}
	for i := range 3 {
		if d := dimmed[0][i]; d <= fg[i] || d >= bg[i] {
			t.Fatalf("channel %d: %d is not between fg %d and bg %d", i, d, fg[i], bg[i])
		}
	}
}

func TestLayerKeepsTheBaseAroundTheBox(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	base := strings.Repeat("0123456789\n", 3)
	got := strings.Split(layerOver(th, base, "AB\nCD", 10), "\n")
	plainWant := []string{"0123AB6789", "0123CD6789", "0123456789"}
	if len(got) != 3 {
		t.Fatalf("rows: %q", got)
	}
	for i, line := range got {
		if ansi.Strip(line) != plainWant[i] {
			t.Fatalf("row %d = %q, want %q", i, ansi.Strip(line), plainWant[i])
		}
	}
	if !strings.Contains(got[0], "\x1b[0mAB\x1b[0m") || !strings.HasPrefix(got[2], "\x1b["+dimFGBramble+"m") {
		t.Fatalf("box rows stay undimmed, base rows dim: %q", got)
	}
}

func TestLayerCarriesColourAcrossTheBox(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	base := "\x1b[48;2;43;33;24m" + strings.Repeat("x", 10) + "\x1b[0m"
	got := layerOver(th, base, "B", 10)
	right := got[strings.Index(got, "B")+1:]
	if !strings.Contains(right[:strings.Index(right, "x")], "48;2;") {
		t.Fatalf("the tint must continue right of the box: %q", got)
	}
}

func TestLayerKeepsColumnsAcrossWideCharacters(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	got := ansi.Strip(layerOver(th, "あいうえお", "X", 10))
	if got != "あい"+"X"+" "+"えお" || ansi.StringWidth(got) != 10 {
		t.Fatalf("got %q (%d cols)", got, ansi.StringWidth(got))
	}
	if got := ansi.Strip(layerOver(th, "あいうえお", "XY", 9)); got != "あ "+"XY"+" えお" {
		t.Fatalf("a wide char cut on the left keeps the box column: %q", got)
	}
}

func TestLayerOverAShortBase(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	got := strings.Split(ansi.Strip(layerOver(th, "ab", "XY\nZW", 6)), "\n")
	if len(got) != 2 || got[0] != "abXY" || got[1] != "  ZW" {
		t.Fatalf("rows: %q", got)
	}
}

func FuzzLayerOver(f *testing.F) {
	f.Add("0123456789\nabc", "AB\nCD", 10)
	f.Add("\x1b[48;2;1;2;3mあいう\x1b[0m\n\x1b[38;2;9;9;9mz", "X", 7)
	f.Fuzz(func(t *testing.T, base, box string, width int) {
		width = 1 + int(uint(width)%120)
		// a box is drawn UI: ASCII and box drawing (no marks that join the
		// base's characters); the base is any log text
		base, box = logscreen.TerminalSafe(base), strings.Map(func(r rune) rune {
			if r > '~' && (r < '─' || r > '╿') {
				return '?'
			}
			return r
		}, logscreen.TerminalSafe(box))
		th := theme.ForMode(theme.ModeBramble)
		out := strings.Split(layerOver(th, base, box, width), "\n")
		baseLines, boxLines := splitLines(base), splitLines(box)
		boxW := 0
		for _, l := range boxLines {
			boxW = max(boxW, ansi.StringWidth(l))
		}
		pad := max((width-boxW)/2, 0)
		for i, line := range out {
			plain := ansi.Strip(line)
			if i >= len(boxLines) {
				if i < len(baseLines) && plain != ansi.Strip(baseLines[i]) {
					t.Fatalf("row %d below the box changed: %q -> %q", i, ansi.Strip(baseLines[i]), plain)
				}
				continue
			}
			// exact columns are promised for ordinary text: a lone combining
			// or zero-width character just before the box joins its border
			// (cosmetic, exotic). Any input must still draw without panicking.
			if i < len(baseLines) && !ordinary(ansi.Strip(baseLines[i])) {
				continue
			}
			boxPlain := ansi.Strip(boxLines[i])
			if got := ansi.Cut(plain, pad, pad+ansi.StringWidth(boxPlain)); got != boxPlain {
				t.Fatalf("row %d: box %q not at column %d in %q", i, boxPlain, pad, plain)
			}
		}
		for _, c := range truecolors(strings.Join(out[min(len(boxLines), len(out)):], "\n")) {
			if c[0] > 125 || c[1] > 125 || c[2] > 125 {
				t.Fatalf("an undimmed colour below the box: %v", c)
			}
		}
	})
}

// ordinary text: printable ASCII, CJK and kana, box drawing.
func ordinary(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool {
		return (r < ' ' || r > '~') && (r < '─' || r > '╿') && !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
	})
}

var (
	truecolorRE = regexp.MustCompile(`[34]8;2;(\d+);(\d+);(\d+)`)
	trailingSGR = regexp.MustCompile(`(\x1b\[[0-9;]*m)+$`)
)

// frameInner drops the frame's coloured side borders from a body row.
func frameInner(line string) string {
	start := strings.Index(line, "│") + len("│")
	end := strings.LastIndex(line, "│")
	if j := strings.LastIndex(line[:end], "\x1b["); j >= start {
		end = j
	}
	return line[start:end]
}

func truecolors(s string) [][3]int {
	var out [][3]int
	for _, m := range truecolorRE.FindAllStringSubmatch(s, -1) {
		var c [3]int
		for i := range 3 {
			c[i], _ = strconv.Atoi(m[i+1])
		}
		out = append(out, c)
	}
	return out
}

func TestEveryModalDimsTheBase(t *testing.T) {
	coloured := strings.Join([]string{
		"10:00:00Z \x1b[31mERROR boom\x1b[0m",
		"10:00:01Z \x1b[1;36mRun make ci\x1b[0m",
		"10:00:02Z \x1b[38;5;196mred from a 256 palette\x1b[0m",
		"10:00:03Z \x1b[38:2::255:0:0mcolon truecolor\x1b[0m",
		"10:00:04Z \x1b[42mgreen background\x1b[0m",
	}, "\n")
	type scene struct {
		name string
		app  func(*testing.T) App
		open string
	}
	scenes := []scene{
		{"help", func(*testing.T) App { return NewScenarioApp("failure", BuildInfo{}) }, "?"},
		{"palette", func(*testing.T) App { return NewScenarioApp("failure", BuildInfo{}) }, ":"},
		{"confirm", func(*testing.T) App { return NewScenarioApp("failure", BuildInfo{}) }, "r"},
		{"approvals", func(*testing.T) App {
			app := NewScenarioApp("failure", BuildInfo{})
			app.runs = sampleWaitingRunsModel()
			return app
		}, ""},
		{"time-jump", func(t *testing.T) App { return openLog(t, logApp(t, config.Default(), noisyRaw())) }, "t"},
		{"time-jump over ANSI log colours", func(t *testing.T) App { return openLog(t, logApp(t, config.Default(), coloured)) }, "t"},
	}
	for _, mode := range []theme.Mode{theme.ModeBramble, theme.ModeBone} {
		for _, sc := range scenes {
			for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 60}} {
				w, h := size[0], size[1]
				app := sc.app(t)
				app.theme = theme.ForMode(mode)
				base := app.ViewSize(w, h)
				if sc.open == "" {
					run, _ := app.runs.SelectedRun()
					app.approvals = approvals.NewModel(run, samplePendingDeployments())
					app.overlays = append(app.overlays, OverlayApprovals)
				} else {
					app = typeKeys(t, app, sc.open)
				}
				where := fmt.Sprintf("%s %s %dx%d", mode, sc.name, w, h)
				checkDimmedBase(t, where, app.theme, base, app.ViewSize(w, h))
			}
		}
	}
}

// checkDimmedBase: beside and below the box every colour is truecolor and
// the dimmed image of a colour the undimmed frame draws.
func checkDimmedBase(t *testing.T, where string, th theme.Theme, base, view string) {
	t.Helper()
	bg := hexRGB(th.BG)
	allowed := map[[3]int]bool{dimText(th): true}
	for _, c := range colours(base) {
		allowed[dimRGB(c.rgb, bg)] = true
	}
	lines := strings.Split(view, "\n")
	body := lines[3 : len(lines)-3] // between the frame's header and footer
	top, bottom := -1, -1
	for i, line := range body {
		if strings.Contains(line, "╭") && top < 0 {
			top = i
		}
		if strings.Contains(line, "╰") {
			bottom = i
		}
	}
	if top < 0 {
		t.Fatalf("%s: no box", where)
	}
	inner := frameInner(body[top])
	edge := ansi.StringWidth(inner[:strings.Index(inner, "╭")])
	if strings.TrimSpace(ansi.Truncate(ansi.Strip(inner), edge, "")) == "" {
		t.Fatalf("%s: nothing of the base beside the box: %q", where, ansi.Strip(body[top]))
	}
	var outside []string
	for _, line := range body[top : max(bottom, top)+1] { // left of the box on every box row
		outside = append(outside, trailingSGR.ReplaceAllString(ansi.Truncate(frameInner(line), edge, ""), ""))
	}
	if bottom >= 0 {
		for _, line := range body[bottom+1:] {
			outside = append(outside, frameInner(line))
		}
	}
	for _, c := range colours(strings.Join(outside, "\n")) {
		if !c.truecolor || !allowed[c.rgb] {
			t.Fatalf("%s: colour %s %v outside the box is not a dimmed base colour", where, c.form, c.rgb)
		}
	}
}

type colourUse struct {
	form      string
	rgb       [3]int
	truecolor bool
}

var sgrRE = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// colours lists every colour an SGR sets, palette colours by their RGB.
func colours(s string) []colourUse {
	var out []colourUse
	for _, m := range sgrRE.FindAllStringSubmatch(s, -1) {
		ps := strings.FieldsFunc(m[1], func(r rune) bool { return r == ';' || r == ':' })
		for k := 0; k < len(ps); k++ {
			n, _ := strconv.Atoi(ps[k])
			switch {
			case (n == 38 || n == 48 || n == 58) && k+4 < len(ps) && ps[k+1] == "2":
				var c [3]int
				for i := range 3 {
					c[i], _ = strconv.Atoi(ps[k+2+i])
				}
				out = append(out, colourUse{m[1], c, true})
				k += 4
			case (n == 38 || n == 48) && k+2 < len(ps) && ps[k+1] == "5":
				i, _ := strconv.Atoi(ps[k+2])
				out = append(out, colourUse{m[1], paletteRGB(ansi.ExtendedColor(i)), false})
				k += 2
			case n >= 30 && n <= 37 || n >= 40 && n <= 47:
				out = append(out, colourUse{m[1], paletteRGB(ansi.BasicColor(n % 10)), false})
			case n >= 90 && n <= 97 || n >= 100 && n <= 107:
				out = append(out, colourUse{m[1], paletteRGB(ansi.BasicColor(n%10 + 8)), false})
			}
		}
	}
	return out
}

func TestEscRestoresTheUndimmedBase(t *testing.T) {
	for _, open := range []string{"?", ":", "r"} {
		app := NewScenarioApp("failure", BuildInfo{Version: "v0.1.0"})
		before := app.ViewSize(120, 40)
		app = typeKeys(t, app, open)
		if app.TopOverlay() == OverlayNone {
			t.Fatalf("%q opened no overlay", open)
		}
		if app = typeKeys(t, app, "esc"); app.ViewSize(120, 40) != before {
			t.Fatalf("esc after %q must restore the frame exactly", open)
		}
	}
}

func TestDimEveryColourForm(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	bg := hexRGB(th.BG)
	dim := func(c [3]int) string { return rgbParams(dimRGB(c, bg)) }
	basic := func(n int) [3]int { return paletteRGB(ansi.BasicColor(n)) }
	ext := func(n int) [3]int { return paletteRGB(ansi.ExtendedColor(n)) }
	for _, tc := range []struct{ in, want string }{
		{"31", "38;2;" + dim(basic(1))},
		{"1;36", "1;38;2;" + dim(basic(6))},
		{"91", "38;2;" + dim(basic(9))},
		{"41", "48;2;" + dim(basic(1))},
		{"101", "48;2;" + dim(basic(9))},
		{"38;5;196", "38;2;" + dim(ext(196))},
		{"48;5;0", "48;2;" + dim(ext(0))}, // 0 here is a colour, not a reset
		{"38:2::255:0:0", "38;2;" + dim([3]int{255, 0, 0})},
		{"38:2:255:0:0", "38;2;" + dim([3]int{255, 0, 0})},
		{"38:5:196", "38;2;" + dim(ext(196))},
		{"58;2;255;0;0", "58;2;" + dim([3]int{255, 0, 0})},
		{"38;2;255;0", "38;2;255;0"}, // truncated: left as is, no reset read into it
	} {
		got := dimANSI(th, "\x1b["+tc.in+"mX")
		if want := "\x1b[" + dimFGBramble + "m\x1b[" + tc.want + "mX"; got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.in, got, want)
		}
	}
}

func TestDimmedTextStaysGreyWhenDownsampled(t *testing.T) {
	for _, mode := range []theme.Mode{theme.ModeBramble, theme.ModeBone} {
		dimmed := dimANSI(theme.ForMode(mode), "text")
		for prof, ok := range map[colorprofile.Profile]func(string) bool{
			colorprofile.ANSI: func(s string) bool { return strings.Contains(s, "[90m") || strings.Contains(s, "[37m") },
			colorprofile.ANSI256: func(s string) bool {
				c := colours(s)
				return len(c) == 1 && c[0].rgb[0] == c[0].rgb[1] && c[0].rgb[1] == c[0].rgb[2]
			},
		} {
			var b strings.Builder
			if _, err := (&colorprofile.Writer{Forward: &b, Profile: prof}).Write([]byte(dimmed)); err != nil {
				t.Fatal(err)
			}
			if !ok(b.String()) {
				t.Fatalf("%s on %v: dimmed text should stay grey, got %q", mode, prof, b.String())
			}
		}
	}
}

func TestLayerKeepsTheTintWhereTheBoxSplitsAWideCharacter(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	tint := "\x1b[48;2;43;33;24m"
	for _, tc := range []struct {
		name, base, box string
		width           int
		left            bool   // the cut is left of the box
		like, unlike    string // the space's background is that of like, not unlike
	}{
		{"right", tint + "あいうえお\x1b[0m", "X", 10, false, "え", ""},
		{"left", tint + "あいうえお\x1b[0m", "XY", 9, true, "あ", ""},
		{"right, tint ends on the cut", tint + "abcあ\x1b[0m   ", "X", 7, false, "c", ""},
		{"left, tint ends after the cut", tint + "あいう\x1b[0m", "X", 4, true, "い", ""},
		{"right, colour changes after the cut", "\x1b[41mabあ\x1b[42mいう", "X", 6, false, "a", "い"},
	} {
		out := layerOver(th, tc.base, tc.box, tc.width)
		boxAt := strings.Index(out, tc.box)
		space := boxAt + strings.Index(out[boxAt:], " ")
		if tc.left {
			space = strings.LastIndex(out[:boxAt], " ")
		}
		got := bgBefore(out, space)
		if want := bgBefore(out, strings.Index(out, tc.like)); got == "" || got != want {
			t.Errorf("%s: the space for the cut character has background %q, want %q: %q", tc.name, got, want, out)
		}
		if tc.unlike != "" && got == bgBefore(out, strings.Index(out, tc.unlike)) {
			t.Errorf("%s: the space took the colour drawn after the cut: %q", tc.name, out)
		}
	}
}

// bgBefore is the background colour the SGRs in s[:at] leave set.
func bgBefore(s string, at int) string {
	bg := ""
	for _, m := range regexp.MustCompile(`\x1b\[([0-9;:]*)m`).FindAllStringSubmatch(s[:at], -1) {
		params := strings.Split(m[1], ";")
		for i := 0; i < len(params); i++ {
			switch params[i] {
			case "", "0", "49":
				bg = ""
			case "38", "58":
				i += 4
			case "48":
				bg = strings.Join(params[i:min(i+5, len(params))], ";")
				i += 4
			}
		}
	}
	return bg
}
