package layout

type Breakpoint string

const (
	BreakpointNarrow  Breakpoint = "narrow"
	BreakpointDefault Breakpoint = "default"
	BreakpointWide    Breakpoint = "wide"
)

func BreakpointFor(width, height int) Breakpoint {
	switch {
	case width >= 200 && height >= 60:
		return BreakpointWide
	case width >= 120 && height >= 40:
		return BreakpointDefault
	default:
		return BreakpointNarrow
	}
}

func RunsColumns(breakpoint Breakpoint) []string {
	switch breakpoint {
	case BreakpointWide, BreakpointDefault:
		return []string{"number", "status", "workflow", "actor", "event", "duration", "age"}
	default:
		return []string{"number", "status", "workflow", "actor", "age"}
	}
}
