package theme

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

type Mode string

const (
	ModeBramble Mode = "bramble"
	ModeBone    Mode = "bone"
)

type Theme struct {
	Mode     Mode
	BG       string
	BGElev   string
	Surface  string
	Surface2 string
	Line     string
	Line2    string
	Dim      string
	Subtle   string
	Muted    string
	FGSoft   string
	FG       string
	OK       string
	OKDeep   string
	Fail     string
	Run      string
	Info     string
	Warn     string
	Neutral  string
	Glow     string
	TermBG   string
	// OKTint fills the all-green summary band.
	OKTint string
	// Tints fill rows that carry a state: a search hit (RunTint), the
	// current hit (RunTintStrong), a failed step (FailTint), a folded
	// repeat (InfoTint).
	RunTint       string
	RunTintStrong string
	FailTint      string
	InfoTint      string
	// Mark is the banner's top-to-bottom gradient; Mark[1] is also the
	// welcome screen's accent.
	Mark [6]string
}

func ForMode(mode Mode) Theme {
	if mode == ModeBone {
		return Theme{
			Mode:     ModeBone,
			BG:       "#EFEDE1",
			BGElev:   "#E7E4D5",
			Surface:  "#DEDACB",
			Surface2: "#D2CDBA",
			Line:     "#CBC6B2",
			Line2:    "#B5B09A",
			Dim:      "#8A8773",
			Subtle:   "#6E6B58",
			Muted:    "#565442",
			FGSoft:   "#33342A",
			FG:       "#23241C",
			OK:       "#1F9E55",
			OKDeep:   "#2E8E55",
			Fail:     "#C24033",
			Run:      "#B57A1E",
			Info:     "#3E7491",
			Warn:     "#C2632F",
			Neutral:  "#8A8773",
			Glow:     "rgba(31,158,85,.10)",
			TermBG:   "#1B1D17",
			OKTint:   "#D6E3D0",
			// About 15/30/14/12% of the semantic colour over BG.
			RunTint:       "#E6DCC4",
			RunTintStrong: "#DECAA7",
			FailTint:      "#E9D5C9",
			InfoTint:      "#DADED7",
			Mark:          [6]string{"#1F9E55", "#2E8E55", "#3E7491", "#4D6466", "#33342A", "#23241C"},
		}
	}
	return Theme{
		Mode:          ModeBramble,
		BG:            "#0E0F0C",
		BGElev:        "#141512",
		Surface:       "#1B1D17",
		Surface2:      "#24271E",
		Line:          "#2E3227",
		Line2:         "#3D4233",
		Dim:           "#6B7060",
		Subtle:        "#8C9179",
		Muted:         "#AEB39B",
		FGSoft:        "#CFCDBB",
		FG:            "#EAE8D9",
		OK:            "#4FD37A",
		OKDeep:        "#2E8E55",
		Fail:          "#E2564B",
		Run:           "#E0A33E",
		Info:          "#6E9CB5",
		Warn:          "#E8895A",
		Neutral:       "#6B7060",
		Glow:          "rgba(79,211,122,.10)",
		TermBG:        "#0B0C0A",
		OKTint:        "#142318",
		RunTint:       "#2B2118",
		RunTintStrong: "#4C371E",
		FailTint:      "#281312",
		InfoTint:      "#141C21",
		Mark:          [6]string{"#4FD37A", "#66BE8A", "#6E9CB5", "#97AFA9", "#CFCDBB", "#EAE8D9"},
	}
}

func (t Theme) SemanticForStatus(status model.Status) string {
	switch status {
	case model.StatusInProgress:
		return t.Run
	case model.StatusQueued, model.StatusPending, model.StatusWaiting, model.StatusRequested:
		return t.Info
	case model.StatusCompleted:
		return t.Neutral
	default:
		return t.Neutral
	}
}

func (t Theme) SemanticForConclusion(conclusion model.Conclusion) string {
	switch conclusion {
	case model.ConclusionSuccess:
		return t.OK
	case model.ConclusionFailure:
		return t.Fail
	case model.ConclusionActionRequired, model.ConclusionTimedOut:
		return t.Warn
	case model.ConclusionCancelled, model.ConclusionStale, model.ConclusionSkipped, model.ConclusionNeutral, model.ConclusionNone:
		return t.Neutral
	default:
		return t.Neutral
	}
}

// OrBramble is the theme a screen draws with: a zero theme (a model built
// outside the app, as fixtures and tests do) is Bramble.
func (t Theme) OrBramble() Theme {
	if t.Mode == "" {
		return ForMode(ModeBramble)
	}
	return t
}

// Fg is a token as a foreground SGR: #RRGGBB as truecolor, which the TTY
// writer downsamples on terminals without it, or a decimal xterm-256
// index (Limited's fills) as is. Screens colour through these and never
// write an SGR colour of their own.
func Fg(color string) string { return sgr(38, color) }

// Bg is Fg for the background.
func Bg(color string) string { return sgr(48, color) }

func sgr(code int, color string) string {
	if index, err := strconv.ParseUint(color, 10, 8); err == nil {
		return fmt.Sprintf("\x1b[%d;5;%dm", code, index)
	}
	color = strings.TrimPrefix(color, "#")
	if len(color) != 6 {
		return ""
	}
	value, err := strconv.ParseUint(color, 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", code, value>>16&0xff, value>>8&0xff, value&0xff)
}

// Palette is what a terminal can show: truecolor, the xterm 256-colour
// palette, or the 16 basic colours.
type Palette int

const (
	PaletteTrue Palette = iota
	Palette256
	Palette16
)

// For is the theme to draw with on palette.
func (t Theme) For(palette Palette) Theme {
	switch palette {
	case Palette256:
		return t.limited()
	case Palette16:
		return t.basic()
	}
	return t
}

// limited is the 256-colour theme. Downsampled, every dark grey below
// #303030 lands on the background's index (232) and the Bone fills
// share 253, so selection, shading and bands vanish. Its fills are
// xterm-256 indices sent as they are: Bramble's on the dark grey ramp,
// as the cube's darkest hues (#5F) are as bright as dim text, with dim
// and subtle text a step lighter so line numbers and timestamps stay
// readable on every fill.
func (t Theme) limited() Theme {
	if t.Mode == ModeBone {
		t.BGElev, t.Surface, t.Surface2 = "254", "253", "251"
		t.OKTint, t.RunTint, t.RunTintStrong, t.FailTint, t.InfoTint = "194", "230", "223", "224", "195"
		return t
	}
	t.BGElev, t.Surface, t.InfoTint, t.RunTint, t.Surface2, t.RunTintStrong = "233", "235", "234", "236", "238", "239"
	t.OKTint, t.FailTint = "22", "52"
	t.Dim, t.Subtle = "245", "248"
	return t
}

// basic is the 16-colour theme: a fill would share a colour with the
// text on it (red on red, cyan on cyan), so it draws none; the cursor
// bar marks the selection.
func (t Theme) basic() Theme {
	t.BGElev, t.Surface, t.Surface2 = "", "", ""
	t.OKTint, t.RunTint, t.RunTintStrong, t.FailTint, t.InfoTint = "", "", "", "", ""
	return t
}
