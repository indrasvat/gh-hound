package tui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/theme"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
)

// Bramble: bg #0E0F0C. Worked by hand from brightness(.46) toward the bg,
// then saturate(.65) with the CSS weights .213/.715/.072.
const (
	dimFGBramble = "38;2;115;115;109" // fg #EAE8D9
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
	fg, bg := rgbOf(t, th.FG), rgbOf(t, th.BG)
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
		width = 1 + abs(width)%120
		// a box is drawn UI: ASCII and box drawing (no marks that join the
		// base's characters); the base is any log text
		base, box = sanitizeFuzz(base), strings.Map(func(r rune) rune {
			if r > '~' && (r < '─' || r > '╿') {
				return '?'
			}
			return r
		}, sanitizeFuzz(box))
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
			// exact columns are promised for ordinary text; any input must
			// still draw without panicking
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

// sanitizeFuzz keeps colours but nothing that moves the cursor, as screens
// draw them.
func sanitizeFuzz(s string) string { return logscreen.TerminalSafe(s) }

// ordinary text: printable ASCII, CJK and kana, box drawing.
func ordinary(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool {
		return (r < ' ' || r > '~') && (r < '─' || r > '╿') && !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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

func rgbOf(t *testing.T, hex string) [3]int {
	t.Helper()
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		t.Fatal(err)
	}
	return [3]int{int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)}
}

func TestEveryModalDimsTheBase(t *testing.T) {
	timeJump := func(w, h int) string {
		return typeKeys(t, openLog(t, logApp(t, config.Default(), noisyRaw())), "t").ViewSize(w, h)
	}
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 60}} {
		w, h := size[0], size[1]
		for name, view := range map[string]string{
			"help":          RenderFixtureSize("help", w, h),
			"palette":       RenderFixtureSize("palette", w, h),
			"approvals":     RenderFixtureSize("approvals", w, h),
			"rerun-confirm": RenderFixtureSize("rerun-confirm", w, h),
			"time-jump":     timeJump(w, h),
		} {
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
				t.Fatalf("%s %dx%d: no box", name, w, h)
			}
			inner := frameInner(body[top])
			edge := ansi.StringWidth(inner[:strings.Index(inner, "╭")])
			if strings.TrimSpace(ansi.Truncate(ansi.Strip(inner), edge, "")) == "" {
				t.Fatalf("%s %dx%d: nothing of the base beside the box: %q", name, w, h, ansi.Strip(body[top]))
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
			for _, c := range truecolors(strings.Join(outside, "\n")) {
				if c[0] > 125 || c[1] > 125 || c[2] > 125 {
					t.Fatalf("%s %dx%d: undimmed colour %v outside the box", name, w, h, c)
				}
			}
		}
	}
}
