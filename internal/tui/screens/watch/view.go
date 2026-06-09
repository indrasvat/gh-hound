package watch

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(header(m), width)}
	if len(m.State.Lines) == 0 {
		lines = append(lines, fitANSI(colorize(sgrDim, "waiting for completed job logs from GitHub Actions"), width))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, foldLine(width))
	start := 0
	if m.Follow && len(m.State.Lines) > 6 {
		start = len(m.State.Lines) - 6
	}
	visible := m.State.Lines[start:]
	for index, line := range visible {
		lines = append(lines, logLine(line, width, index == len(visible)-1))
	}
	return strings.Join(lines, "\n")
}

func header(m Model) string {
	follow := "follow ○"
	if m.Follow {
		follow = "follow ●"
	}
	elapsed := m.State.Elapsed
	if elapsed == "" {
		elapsed = "0m00s"
	}
	debug := ""
	if m.Debug {
		debug = " · debug"
	}
	return fmt.Sprintf("watch · %s #%d · %s streaming %s %s %s%s",
		m.State.Run.Name,
		m.State.Run.RunNumber,
		m.State.Branch,
		icons.InProgress,
		elapsed,
		follow,
		debug,
	)
}

func foldLine(width int) string {
	value := colorize(sgrOK, "▾") + " " + colorize(sgrFGSoft, "completed job logs")
	return backgroundSafe(value, width, sgrFG, sgrSurfaceBG)
}

func logLine(line logs.Line, width int, active bool) string {
	gutter := colorize(sgrLine2, fmt.Sprintf("%03d", line.Number))
	return fitANSI(gutter+" "+renderLogText(line.Text, active), width)
}

func renderLogText(text string, active bool) string {
	text = strings.TrimSpace(text)
	rendered := text
	switch {
	case strings.HasPrefix(text, "17:"):
		parts := strings.Fields(text)
		if len(parts) > 1 {
			rendered = colorize(sgrDim, parts[0]) + " " + colorize(sgrInfo, strings.Join(parts[1:], " "))
		}
	case strings.HasPrefix(text, "ok "):
		rendered = colorize(sgrOK, "ok") + strings.TrimPrefix(text, "ok")
	case strings.Contains(text, "--- PASS"):
		rendered = colorize(sgrRun, text)
	case strings.HasPrefix(text, "=== RUN"):
		rendered = strings.Replace(text, "Test", colorize(sgrInfo, "Test"), 1)
	}
	if active {
		rendered += colorize(sgrOK, "█")
	}
	return rendered
}

func fit(value string, width int) string {
	return fitANSI(strings.TrimSpace(value), width)
}

func fitANSI(value string, width int) string {
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

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
}

func padANSI(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func backgroundSafe(value string, width int, fg string, bg string) string {
	value = padANSI(fitANSI(value, width), width)
	style := fg + bg
	value = strings.ReplaceAll(value, sgrReset, sgrReset+style)
	return style + value + sgrReset
}

const (
	sgrReset     = "\x1b[0m"
	sgrOK        = "\x1b[38;2;79;211;122m"
	sgrRun       = "\x1b[38;2;224;163;62m"
	sgrInfo      = "\x1b[38;2;110;156;181m"
	sgrDim       = "\x1b[38;2;107;112;96m"
	sgrFG        = "\x1b[38;2;234;232;217m"
	sgrFGSoft    = "\x1b[38;2;207;205;187m"
	sgrLine2     = "\x1b[38;2;61;66;51m"
	sgrSurfaceBG = "\x1b[48;2;27;29;23m"
)
