package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

const minFrameWidth = 48

func frameViewSize(th theme.Theme, title, context, right, body, footer string, width, height int, focused bool) string {
	return frameLines(th, title, context, right, colorizeBody(th, body, contentWidth(width)), footer, width, height, focused)
}

// frameLines frames a body that is already coloured.
func frameLines(th theme.Theme, title, context, right, body, footer string, width, height int, focused bool) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	inner := max(width-2, 1)

	borderColor := th.Line2
	if focused {
		borderColor = th.OK
	}
	head := renderHeader(th, title, context, right, inner)
	foot := renderFooter(th, footer, inner)

	bodyLines := splitLines(body)
	if height > 0 {
		bodyRows := max(height-6, 1)
		bodyLines = fitLineCount(bodyLines, bodyRows)
	}

	border := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor)).Render
	lines := []string{
		border("╭" + strings.Repeat("─", inner) + "╮"),
		framedLine(th, head, inner, borderColor),
		border("├" + strings.Repeat("─", inner) + "┤"),
	}
	for _, line := range bodyLines {
		lines = append(lines, framedLine(th, line, inner, borderColor))
	}
	lines = append(lines,
		border("├"+strings.Repeat("─", inner)+"┤"),
		framedLine(th, foot, inner, borderColor),
		border("╰"+strings.Repeat("─", inner)+"╯"),
	)
	return strings.Join(lines, "\n")
}

func renderHeader(th theme.Theme, title, context, right string, width int) string {
	if width < 1 {
		width = 1
	}
	mark := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Bold(true).Render(title)
	ctx := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Dim)).Render(context)
	right = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Dim)).Render(right)
	raw := title + " " + context + " " + right
	spacer := max(width-visibleLen(raw), 2)
	line := mark + " " + ctx + strings.Repeat(" ", spacer) + right
	return fitPlain(line, width)
}

func renderFooter(th theme.Theme, footer string, width int) string {
	if footer == "" {
		footer = keys.FooterForScreen(keys.ScreenRunsList)
	}
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(th.Subtle)).
		Render(fitPlain(footer, width))
}

// overlayInnerWidth mirrors overlayBox's sizing so content that wraps
// to a width (the help overlay) can wrap to the width the box will
// actually render, not a guess the box then truncates.
func overlayInnerWidth(width int) int {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	boxWidth := width - 12
	if boxWidth < 36 {
		boxWidth = width - 4
	}
	if boxWidth > 82 {
		boxWidth = 82
	}
	return max(boxWidth-2, 1)
}

func overlayBox(th theme.Theme, title, body string, width int) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	inner := overlayInnerWidth(width)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Bold(true).Render(title)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Render
	lines := []string{
		border("╭" + strings.Repeat("─", inner) + "╮"),
		boxLine(th, header, inner),
		border("├" + strings.Repeat("─", inner) + "┤"),
	}
	for _, line := range splitLines(fitANSIBlock(body, inner)) {
		lines = append(lines, boxLine(th, line, inner))
	}
	lines = append(lines, border("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(lines, "\n")
}

// layerOver centres box in width over base, which is dimmed as the mock's
// .dimbg. Each box row replaces the base columns it covers; the base keeps
// its colours either side of it.
func layerOver(th theme.Theme, base, box string, width int) string {
	baseLines, boxLines := splitLines(base), splitLines(box)
	boxWidth := 0
	for _, line := range boxLines {
		boxWidth = max(boxWidth, ansi.StringWidth(line))
	}
	pad := max((width-boxWidth)/2, 0)
	out := make([]string, max(len(baseLines), len(boxLines)))
	for i := range out {
		if i < len(baseLines) {
			out[i] = dimANSI(th, baseLines[i])
		}
		if i >= len(boxLines) {
			continue
		}
		line, row := out[i], boxLines[i]
		row += strings.Repeat(" ", boxWidth-ansi.StringWidth(row))
		left := ansi.Truncate(line, pad, "")
		left += strings.Repeat(" ", max(pad-ansi.StringWidth(left), 0))
		right := ""
		if width := ansi.StringWidth(line); width > pad+boxWidth {
			// TruncateLeft keeps the colours set before the cut; a wide
			// character the box cuts in half becomes a space.
			want := width - pad - boxWidth
			right = ansi.TruncateLeft(line, pad+boxWidth, "")
			if ansi.StringWidth(right) > want {
				right = ansi.TruncateLeft(line, pad+boxWidth+1, "")
			}
			right = strings.Repeat(" ", max(want-ansi.StringWidth(right), 0)) + right
		}
		out[i] = left + sgrReset + row + sgrReset + right
	}
	return strings.Join(out, "\n")
}

// dimANSI dims a line as CSS brightness(.46) saturate(.65) would over the
// theme background: truecolor colours are rewritten, and text with no
// colour takes the dimmed foreground.
func dimANSI(th theme.Theme, line string) string {
	if line == "" {
		return line
	}
	bg := hexRGB(th.BG)
	fg := "38;2;" + rgbParams(dimRGB(hexRGB(th.FG), bg))
	var b strings.Builder
	b.WriteString("\x1b[" + fg + "m")
	for line != "" {
		at := strings.Index(line, "\x1b[")
		if at < 0 {
			b.WriteString(line)
			break
		}
		b.WriteString(line[:at])
		line = line[at:]
		end := 2
		for end < len(line) && (line[end] < 0x40 || line[end] > 0x7e) {
			end++
		}
		if end == len(line) || line[end] != 'm' {
			b.WriteString(line[:min(end+1, len(line))])
			line = line[min(end+1, len(line)):]
			continue
		}
		b.WriteString("\x1b[" + dimSGR(line[2:end], fg, bg) + "m")
		line = line[end+1:]
	}
	return b.String()
}

func dimSGR(params, fg string, bg [3]int) string {
	ps := strings.Split(params, ";")
	out := make([]string, 0, len(ps)+1)
	for k := 0; k < len(ps); k++ {
		switch p := ps[k]; {
		case p == "" || p == "0":
			out = append(out, "0", fg)
		case p == "39":
			out = append(out, fg)
		case (p == "38" || p == "48") && k+4 < len(ps) && ps[k+1] == "2":
			var c [3]int
			for i := range 3 {
				v, _ := strconv.Atoi(ps[k+2+i])
				c[i] = v
			}
			out = append(out, p, "2", rgbParams(dimRGB(c, bg)))
			k += 4
		case (p == "38" || p == "48") && k+2 < len(ps) && ps[k+1] == "5":
			out = append(out, ps[k:k+3]...)
			k += 2
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, ";")
}

// dimRGB blends c 46% of the way from bg, then keeps 65% of its saturation
// (CSS saturate weights).
func dimRGB(c, bg [3]int) [3]int {
	var f [3]float64
	for i := range 3 {
		f[i] = float64(bg[i]) + float64(c[i]-bg[i])*0.46
	}
	lum := 0.213*f[0] + 0.715*f[1] + 0.072*f[2]
	var out [3]int
	for i := range 3 {
		out[i] = min(max(int(math.Round(lum+0.65*(f[i]-lum))), 0), 255)
	}
	return out
}

func rgbParams(c [3]int) string {
	return fmt.Sprintf("%d;%d;%d", c[0], c[1], c[2])
}

func selectedLine(th theme.Theme, value string, width int) string {
	if width < 2 {
		return value
	}
	return backgroundSafeLine(value, width, th.FG, th.Surface2)
}

func fitANSIBlock(value string, width int) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = fitPlain(line, width)
	}
	return strings.Join(lines, "\n")
}

func framedLine(th theme.Theme, value string, width int, borderColor string) string {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor)).Render
	value = fitPlain(value, width)
	padding := max(width-ansi.StringWidth(value), 0)
	return border("│") + value + strings.Repeat(" ", padding) + border("│")
}

func boxLine(th theme.Theme, value string, width int) string {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Render
	value = fitPlain(value, width)
	padding := max(width-ansi.StringWidth(value), 0)
	return border("│") + value + strings.Repeat(" ", padding) + border("│")
}

func splitLines(value string) []string {
	if value == "" {
		return []string{""}
	}
	return strings.Split(strings.TrimRight(value, "\n"), "\n")
}

func fitLineCount(lines []string, count int) []string {
	if count <= 0 {
		return nil
	}
	out := append([]string(nil), lines...)
	if len(out) > count {
		out = out[:count]
		if count > 0 {
			out[count-1] = "…"
		}
		return out
	}
	for len(out) < count {
		out = append(out, "")
	}
	return out
}

func colorizeBody(th theme.Theme, value string, width int) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		line = fitPlain(line, width)
		plain := ansi.Strip(line)
		trimmed := strings.TrimSpace(plain)
		if trimmed == "" {
			lines[i] = line
			continue
		}
		if strings.HasPrefix(trimmed, "▌") {
			lines[i] = selectedLine(th, line, width)
			continue
		}
		if strings.Contains(line, "\x1b[") {
			lines[i] = line
			continue
		}
		switch {
		case strings.Contains(trimmed, "✗") || strings.Contains(trimmed, "FAIL") || strings.Contains(strings.ToLower(trimmed), "error"):
			lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Fail)).Render(line)
		case strings.Contains(trimmed, "✔") || strings.Contains(trimmed, "PASS") || strings.Contains(trimmed, "All checks"):
			lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Render(line)
		case strings.Contains(trimmed, ">") || strings.Contains(strings.ToLower(trimmed), "streaming") || strings.Contains(strings.ToLower(trimmed), "running"):
			lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Run)).Render(line)
		case looksLikeSectionHeader(trimmed):
			lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Info)).Bold(true).Render(line)
		case strings.HasPrefix(trimmed, "0") || strings.HasPrefix(trimmed, "1") || strings.HasPrefix(trimmed, "2") || strings.HasPrefix(trimmed, "3") || strings.HasPrefix(trimmed, "4") || strings.HasPrefix(trimmed, "5") || strings.HasPrefix(trimmed, "6") || strings.HasPrefix(trimmed, "7") || strings.HasPrefix(trimmed, "8") || strings.HasPrefix(trimmed, "9"):
			lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)).Render(line)
		default:
			lines[i] = line
		}
	}
	return strings.Join(lines, "\n")
}

func looksLikeSectionHeader(value string) bool {
	if strings.Contains(value, "Workflow / detail") {
		return true
	}
	for _, prefix := range []string{"Jobs", "Steps", "Annotations", "log", "dispatch", "watch", "hound "} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func fitPlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = strings.TrimRight(value, "\n")
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}

func visibleLen(value string) int {
	return ansi.StringWidth(value)
}

func backgroundSafeLine(value string, width int, fg string, bg string) string {
	value = fitPlain(value, width)
	value += strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
	style := sgrHex(fg, false) + sgrHex(bg, true)
	if style == "" {
		return lipgloss.NewStyle().
			Width(width).
			Foreground(lipgloss.Color(fg)).
			Background(lipgloss.Color(bg)).
			Render(value)
	}
	value = strings.ReplaceAll(value, sgrReset, sgrReset+style)
	return style + value + sgrReset
}

func sgrHex(color string, background bool) string {
	c, ok := parseHex(color)
	if !ok {
		return ""
	}
	code := 38
	if background {
		code = 48
	}
	return fmt.Sprintf("\x1b[%d;2;%sm", code, rgbParams(c))
}

func parseHex(color string) ([3]int, bool) {
	v, err := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	if err != nil || len(strings.TrimPrefix(color, "#")) != 6 {
		return [3]int{}, false
	}
	return [3]int{int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)}, true
}

// hexRGB is a theme token's colour; tokens are always #RRGGBB.
func hexRGB(color string) [3]int {
	c, _ := parseHex(color)
	return c
}

const sgrReset = "\x1b[0m"
