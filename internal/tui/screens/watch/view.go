package watch

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/tui/components/logview"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(header(m), width)}
	start := 0
	if m.Follow && len(m.State.Lines) > 6 {
		start = len(m.State.Lines) - 6
	}
	for _, line := range m.State.Lines[start:] {
		lines = append(lines, logview.Line(line.Number+40, line.Text, width))
	}
	lines = append(lines, fit("incoming ▾ active step tail █", width))
	lines = append(lines, fit(keys.FooterForScreen(keys.ScreenWatch), width))
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
	return fmt.Sprintf("watch · %s #%d · %s streaming %s %s %s",
		m.State.Run.Name,
		m.State.Run.RunNumber,
		m.State.Branch,
		icons.InProgress,
		elapsed,
		follow,
	)
}

func fit(value string, width int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
