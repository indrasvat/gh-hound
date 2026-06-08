package welcome

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
)

func View(model Model, widths ...int) string {
	width := 78
	if len(widths) > 0 && widths[0] > 0 {
		width = widths[0]
	}
	height := 0
	if len(widths) > 1 && widths[1] > 0 {
		height = widths[1]
	}
	var out strings.Builder
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#4FD37A")).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#CFCDBB"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#66BE8A")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#8C9179"))

	for line := range strings.SplitSeq(strings.TrimRight(banner.RenderMark(), "\n"), "\n") {
		out.WriteString(center(line, width))
		out.WriteString("\n")
	}
	out.WriteString("\n")
	out.WriteString(center(title.Render("Hunt down your GitHub Actions CI")+muted.Render(" — without the click-through"), width))
	out.WriteString("\n\n")
	out.WriteString(strings.Join(cardRows(width, accent, muted), "\n"))
	out.WriteString("\n\n")
	out.WriteString(center(accent.Render("⏎")+muted.Render(" Press Enter to continue"), width))
	out.WriteString("\n")
	out.WriteString(center(dim.Render(model.Build.Version+" · github.com/indrasvat/gh-hound"), width))
	return placeVertical(out.String(), height)
}

func cardRows(width int, accent, muted lipgloss.Style) []string {
	cardWidth := 24
	gap := "  "
	if width >= 96 {
		cardWidth = min(34, max(24, (width-8)/3))
	} else if width < cardWidth*3+len(gap)*2 {
		cardWidth = max(18, (width-len(gap)*2)/3)
	}
	inner := max(cardWidth-2, 1)
	cards := cardCopy(inner)
	top := make([]string, 0, 3)
	title := make([]string, 0, 3)
	body1 := make([]string, 0, 3)
	body2 := make([]string, 0, 3)
	bottom := make([]string, 0, 3)
	for _, card := range cards {
		top = append(top, "╭"+strings.Repeat("─", inner)+"╮")
		title = append(title, "│"+fitCard(accent.Render(card.title), inner)+"│")
		body1 = append(body1, "│"+fitCard(muted.Render(card.body1), inner)+"│")
		body2 = append(body2, "│"+fitCard(muted.Render(card.body2), inner)+"│")
		bottom = append(bottom, "╰"+strings.Repeat("─", inner)+"╯")
	}
	rows := []string{
		center(strings.Join(top, gap), width),
		center(strings.Join(title, gap), width),
		center(strings.Join(body1, gap), width),
		center(strings.Join(body2, gap), width),
		center(strings.Join(bottom, gap), width),
	}
	return rows
}

func cardCopy(inner int) []struct {
	title string
	body1 string
	body2 string
} {
	if inner < 28 {
		return []struct {
			title string
			body1 string
			body2 string
		}{
			{"› WATCH", "Branch runs, live.", "Newest in focus."},
			{"✗ DIAGNOSE", "Failed step + log.", "Annotated excerpt."},
			{"↻ RERUN", "Rerun/cancel", "Dispatch one-key."},
		}
	}
	return []struct {
		title string
		body1 string
		body2 string
	}{
		{"› WATCH", "Branch-scoped runs, live.", "Newest run stays focused."},
		{"✗ DIAGNOSE", "Jump to failed step.", "Denoised annotated logs."},
		{"↻ RERUN", "Failed jobs, cancel,", "dispatch: one key each."},
	}
}

func placeVertical(value string, height int) string {
	if height <= 0 {
		return value
	}
	lines := strings.Split(value, "\n")
	if len(lines) >= height {
		return value
	}
	padTop := (height - len(lines)) / 3
	if padTop <= 0 {
		return value
	}
	return strings.Repeat("\n", padTop) + value
}

func fitCard(value string, width int) string {
	if ansi.StringWidth(value) > width {
		value = ansi.Truncate(value, width, "…")
	}
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func center(value string, width int) string {
	if width <= 0 {
		return value
	}
	visible := ansi.StringWidth(value)
	if visible >= width {
		return value
	}
	return strings.Repeat(" ", (width-visible)/2) + value
}
