package theme

import "github.com/indrasvat/gh-hound/internal/model"

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
		}
	}
	return Theme{
		Mode:     ModeBramble,
		BG:       "#0E0F0C",
		BGElev:   "#141512",
		Surface:  "#1B1D17",
		Surface2: "#24271E",
		Line:     "#2E3227",
		Line2:    "#3D4233",
		Dim:      "#6B7060",
		Subtle:   "#8C9179",
		Muted:    "#AEB39B",
		FGSoft:   "#CFCDBB",
		FG:       "#EAE8D9",
		OK:       "#4FD37A",
		OKDeep:   "#2E8E55",
		Fail:     "#E2564B",
		Run:      "#E0A33E",
		Info:     "#6E9CB5",
		Warn:     "#E8895A",
		Neutral:  "#6B7060",
		Glow:     "rgba(79,211,122,.10)",
		TermBG:   "#0B0C0A",
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
