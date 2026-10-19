package logview

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/numfmt"
)

func Line(number int, text string, width int) string {
	return fit(fmt.Sprintf("%03d %s", number, text), width)
}

func Fold(number int, title string, count int, collapsed bool, width int) string {
	glyph := "▾"
	if collapsed {
		glyph = "▸"
	}
	return fit(fmt.Sprintf("%03d %s %s %s lines", number, glyph, title, numfmt.Group(count)), width)
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
