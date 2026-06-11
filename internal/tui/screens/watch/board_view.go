package watch

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

// Board column geometry. Header and rows derive from the SAME math so
// they can never drift, and the elapsed column is fixed-width so
// repaints change digits, not layout.
const (
	boardMarkerWidth  = 2
	boardGlyphWidth   = 2
	boardNumberWidth  = 8
	boardStateWidth   = 12
	boardElapsedWidth = 8
)

// boardColumnWidths is the single source of column truth for the
// header and every row: only the workflow column flexes.
func boardColumnWidths(width int) (workflowWidth int) {
	fixed := boardMarkerWidth + boardGlyphWidth + boardNumberWidth + boardStateWidth + boardElapsedWidth + 4
	return max(width-fixed, 8)
}

func BoardView(b Board, width int, now time.Time) string {
	return BoardViewSize(b, width, 0, now)
}

func BoardViewSize(b Board, width, height int, now time.Time) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		fitANSI(boardHeadline(b, width), width),
		"",
		boardHeader(width),
	}
	if len(b.Runs) == 0 {
		lines = append(lines, fitANSI(colorize(sgrDim, "  the pack is empty — nothing to watch on this scent."), width))
		return strings.Join(lines, "\n")
	}
	for index, run := range b.Runs {
		line := boardRow(run, index == b.Selected, width, now)
		if b.Loading {
			line = fitANSI(colorize(sgrDim, ansi.Strip(line)), width)
		}
		lines = append(lines, line)
	}
	if b.Loading && b.LoadingLine != "" {
		lines = append(lines, fitANSI(b.LoadingLine, width))
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// boardHeadline is the aggregate header in the hound voice:
// `the pack: 3 running · 1 home · 0 lost` with the scent and follow
// state on the right.
func boardHeadline(b Board, width int) string {
	summary := b.Summary()
	left := colorize(sgrFGSoft, "the pack: ") +
		colorize(sgrRun, fmt.Sprintf("%d running", summary.Running)) +
		colorize(sgrDim, " · ") +
		colorize(sgrOK, fmt.Sprintf("%d home", summary.Home)) +
		colorize(sgrDim, " · ") +
		colorize(boardLostColor(summary.Lost), fmt.Sprintf("%d lost", summary.Lost))
	follow := "follow ○"
	if b.Follow {
		follow = "follow ●"
	}
	right := colorize(sgrDim, shortBoardSHA(b.HeadSHA)+" "+b.Event+" · "+follow)
	return joinRightANSI(left, right, width)
}

func boardLostColor(lost int) string {
	if lost > 0 {
		return sgrFail
	}
	return sgrDim
}

func boardHeader(width int) string {
	workflowWidth := boardColumnWidths(width)
	header := strings.Repeat(" ", boardMarkerWidth) +
		padPlain("St", boardGlyphWidth) + " " +
		padPlain("Workflow", workflowWidth) + " " +
		padPlain("#", boardNumberWidth) + " " +
		padPlain("State", boardStateWidth) + " " +
		padLeftPlain("Elapsed", boardElapsedWidth)
	return fitANSI(colorize(sgrDim, header), width)
}

func boardRow(run model.Run, selected bool, width int, now time.Time) string {
	workflowWidth := boardColumnWidths(width)
	marker := strings.Repeat(" ", boardMarkerWidth)
	if selected {
		marker = colorize(sgrOK, icons.Cursor) + " "
	}
	line := marker +
		padANSI(colorize(boardStatusColor(run), boardGlyph(run)), boardGlyphWidth) + " " +
		padANSI(colorize(sgrFGSoft, truncatePlain(boardWorkflowName(run), workflowWidth)), workflowWidth) + " " +
		padANSI(colorize(sgrDim, fmt.Sprintf("#%d", run.RunNumber)), boardNumberWidth) + " " +
		padANSI(colorize(boardStatusColor(run), boardStateLabel(run)), boardStateWidth) + " " +
		padLeftANSI(colorize(sgrDim, boardElapsed(run, now)), boardElapsedWidth)
	return fitANSI(line, width)
}

func boardWorkflowName(run model.Run) string {
	for _, value := range []string{run.Name, run.Path, run.DisplayTitle} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return fmt.Sprintf("run %d", run.ID)
}

func boardGlyph(run model.Run) string {
	if run.Status == model.StatusCompleted {
		return icons.ForConclusion(run.Conclusion)
	}
	if run.Status == model.StatusWaiting {
		return icons.Gate
	}
	return icons.ForStatus(run.Status)
}

func boardStateLabel(run model.Run) string {
	if run.Status == model.StatusCompleted {
		if run.Conclusion == model.ConclusionNone {
			return string(run.Status)
		}
		return string(run.Conclusion)
	}
	return string(run.Status)
}

func boardStatusColor(run model.Run) string {
	if run.Status == model.StatusCompleted {
		switch run.Conclusion {
		case model.ConclusionSuccess, model.ConclusionSkipped, model.ConclusionNeutral:
			return sgrOK
		case model.ConclusionFailure, model.ConclusionActionRequired, model.ConclusionTimedOut:
			return sgrFail
		default:
			return sgrDim
		}
	}
	switch run.Status {
	case model.StatusInProgress:
		return sgrRun
	default:
		return sgrInfo
	}
}

// boardElapsed renders a fixed-width clock (`12m08s`): completed runs
// pin start→finish, live runs tick start→now. Width never changes
// between repaints, so the column cannot wobble.
func boardElapsed(run model.Run, now time.Time) string {
	start := run.RunStartedAt
	if start.IsZero() {
		return "—"
	}
	end := now
	if run.Status == model.StatusCompleted && !run.UpdatedAt.IsZero() {
		end = run.UpdatedAt
	}
	if end.Before(start) {
		return "—"
	}
	elapsed := end.Sub(start)
	if elapsed >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(elapsed.Hours()), int(elapsed.Minutes())%60)
	}
	return fmt.Sprintf("%dm%02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
}

func shortBoardSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func padPlain(value string, width int) string {
	value = truncatePlain(value, width)
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func padLeftPlain(value string, width int) string {
	value = truncatePlain(value, width)
	return strings.Repeat(" ", max(width-ansi.StringWidth(value), 0)) + value
}

func padLeftANSI(value string, width int) string {
	return strings.Repeat(" ", max(width-ansi.StringWidth(value), 0)) + value
}

func truncatePlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}

func joinRightANSI(left, right string, width int) string {
	leftWidth := ansi.StringWidth(left)
	rightWidth := ansi.StringWidth(right)
	if leftWidth+rightWidth+1 > width {
		return left
	}
	return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
}

// sgrFail matches the runs list's failure tone; the rest of the board
// palette reuses this package's existing SGR constants.
const sgrFail = "\x1b[38;2;226;86;75m"
