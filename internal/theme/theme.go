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

// Limited is the theme for a terminal without truecolor. Downsampled to
// 256 colours, every dark grey below #303030 lands on the background's
// index (232) and the Bone fills share 253, so selection, input shading
// and bands vanish. Its fills are xterm-256 indices, sent as they are.
func (t Theme) Limited() Theme {
	if t.Mode == ModeBone {
		t.BGElev, t.Surface, t.Surface2 = "254", "253", "251"
		t.OKTint, t.RunTint, t.RunTintStrong, t.FailTint, t.InfoTint = "194", "230", "223", "224", "195"
		return t
	}
	t.BGElev, t.Surface, t.Surface2 = "234", "236", "238"
	t.OKTint, t.RunTint, t.RunTintStrong, t.FailTint, t.InfoTint = "22", "58", "94", "52", "23"
	return t
}
