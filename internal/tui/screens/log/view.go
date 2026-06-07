package log

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/tui/components/logview"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(header(m), width)}
	for _, row := range m.visibleRows() {
		if row.IsFold {
			lines = append(lines, logview.Fold(row.Line.Number, row.Fold.Title, row.Fold.CollapsedCount, row.Collapsed, width))
			continue
		}
		text := row.Line.Text
		if m.isMatch(row.Line.Number) {
			text = "› " + text
		}
		lines = append(lines, logview.Line(row.Line.Number, text, width))
	}
	lines = append(lines, fit(keys.FooterForScreen(keys.ScreenLog), width))
	return strings.Join(lines, "\n")
}

func header(m Model) string {
	if m.Search.Query != "" {
		return fmt.Sprintf("log · /%s · match %d/%d", m.Search.Query, m.Search.Current, m.Search.Total)
	}
	return "log"
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
