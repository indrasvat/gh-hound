package tui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
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
	context = fitContext(context, width-visibleLen(title)-visibleLen(right)-3)
	ctx := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Dim)).Render(context)
	right = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Dim)).Render(right)
	raw := title + " " + context + " " + right
	spacer := max(width-visibleLen(raw), 2)
	line := mark + " " + ctx + strings.Repeat(" ", spacer) + right
	return fitPlain(line, width)
}

// fitContext shortens the header's context to room, so the right side
// (counts, live) shows: the context's end gives way, except an identity
// (" · @login"), which the branch before it gives way to instead.
func fitContext(context string, room int) string {
	if visibleLen(context) <= room {
		return context
	}
	if cut := strings.LastIndex(context, " · @"); cut > 0 {
		head, tail := context[:cut], context[cut:]
		if avail := room - visibleLen(tail); avail >= 2 {
			return ansi.Truncate(head, avail, "…") + tail
		}
		return ansi.Truncate(strings.TrimPrefix(tail, " · "), max(room, 1), "…")
	}
	return ansi.Truncate(context, max(room, 1), "…")
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

// overlayBox frames body under its title. Short of room (rows, 0:
// unbounded), the title moves into the top border, then the borders go.
// With fillSelection the row under the cursor takes the surface-2 fill
// behind a green bar (the palette, contract ⑧).
func overlayBox(th theme.Theme, title, body string, width, room int, fillSelection bool) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	inner := overlayInnerWidth(width)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Bold(true).Render(title)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Render
	rows := splitLines(fitANSIBlock(body, inner))
	lines := []string{}
	for _, line := range rows {
		if fillSelection && strings.HasPrefix(ansi.Strip(line), icons.Cursor) {
			lines = append(lines, selectedBoxLine(th, line, inner))
			continue
		}
		lines = append(lines, boxLine(th, line, inner))
	}
	bottom := border("╰" + strings.Repeat("─", inner) + "╯")
	switch {
	case room <= 0 || len(rows)+4 <= room:
		lines = append([]string{
			border("╭" + strings.Repeat("─", inner) + "╮"),
			boxLine(th, header, inner),
			border("├" + strings.Repeat("─", inner) + "┤"),
		}, append(lines, bottom)...)
	case len(rows)+2 <= room:
		header = fitPlain(header, max(inner-3, 0))
		top := border("╭─ ") + header + border(" "+strings.Repeat("─", max(inner-3-ansi.StringWidth(header), 0))+"╮")
		lines = append([]string{top}, append(lines, bottom)...)
	}
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
		// A wide character the box cuts in half becomes a space in the
		// colours it was drawn in, so tinted rows keep their tint.
		left := ansi.Truncate(line, pad, "")
		if gap := pad - ansi.StringWidth(left); gap > 0 {
			// Truncate keeps line's bytes up to the cut character.
			at, _ := cellAt(line, pad)
			left = left[:at] + strings.Repeat(" ", gap) + left[at:]
		}
		right := ""
		if lineWidth := ansi.StringWidth(line); lineWidth > pad+boxWidth {
			right = ansi.TruncateLeft(line, pad+boxWidth, "")
			if ansi.StringWidth(right) > lineWidth-pad-boxWidth {
				from, to := cellAt(right, 0)
				right = right[:from] + " " + right[to:]
			}
		}
		out[i] = left + sgrReset + row + sgrReset + right
	}
	return strings.Join(out, "\n")
}

// cellAt returns the byte span of the printable cell covering column col
// of s, skipping escape sequences.
func cellAt(s string, col int) (from, to int) {
	state := byte(ansi.NormalState)
	for i := 0; i < len(s); {
		_, w, n, next := ansi.DecodeSequence(s[i:], state, nil)
		if col -= w; w > 0 && col < 0 {
			return i, i + n
		}
		i, state = i+n, next
	}
	return len(s), len(s)
}

// dimANSI dims a line as CSS brightness(.46) saturate(.65) would over the
// theme background: truecolor colours are rewritten, and text with no
// colour takes the dimmed foreground.
func dimANSI(th theme.Theme, line string) string {
	if line == "" {
		return line
	}
	bg := hexRGB(th.BG)
	fg := "38;2;" + rgbParams(dimText(th))
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

// dimSGR rewrites every colour an SGR sets (truecolor, 256 and basic
// palettes, ';' or ':' separated) as a dimmed truecolor.
func dimSGR(params, fg string, bg [3]int) string {
	ps := strings.Split(params, ";")
	out := make([]string, 0, len(ps)+1)
	dim := func(target string, c [3]int) { out = append(out, target, "2", rgbParams(dimRGB(c, bg))) }
	for k := 0; k < len(ps); k++ {
		p := ps[k]
		n, err := strconv.Atoi(p)
		switch {
		case strings.Contains(p, ":"):
			sub := strings.Split(p, ":")
			switch {
			case len(sub) >= 5 && sub[1] == "2" && isColourTarget(sub[0]):
				dim(sub[0], subRGB(sub[len(sub)-3:]))
			case len(sub) == 3 && sub[1] == "5" && isColourTarget(sub[0]):
				i, _ := strconv.Atoi(sub[2])
				dim(sub[0], paletteRGB(ansi.ExtendedColor(i)))
			default:
				out = append(out, p)
			}
		case p == "" || err == nil && n == 0:
			out = append(out, "0", fg)
		case err != nil:
			out = append(out, p)
		case n == 39:
			out = append(out, fg)
		case isColourTarget(p) && k+1 < len(ps) && ps[k+1] == "2":
			if k+4 >= len(ps) { // truncated: keep the rest as it is
				return strings.Join(append(out, ps[k:]...), ";")
			}
			dim(p, subRGB(ps[k+2:k+5]))
			k += 4
		case isColourTarget(p) && k+2 < len(ps) && ps[k+1] == "5":
			i, _ := strconv.Atoi(ps[k+2])
			dim(p, paletteRGB(ansi.ExtendedColor(i)))
			k += 2
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			dim("38", paletteRGB(ansi.BasicColor(n%10+(n/90)*8)))
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			dim("48", paletteRGB(ansi.BasicColor(n%10+(n/100)*8)))
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, ";")
}

func isColourTarget(p string) bool { return p == "38" || p == "48" || p == "58" }

func subRGB(ps []string) [3]int {
	var c [3]int
	for i := range 3 {
		c[i], _ = strconv.Atoi(ps[i])
	}
	return c
}

// paletteRGB is a palette colour's RGB in the default xterm palette.
func paletteRGB(c color.Color) [3]int {
	r, g, b, _ := c.RGBA()
	return [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
}

// dimText is plain text's dimmed colour: a neutral grey, since a warm one
// maps to yellow or olive when the writer downsamples to 16 or 256 colours.
func dimText(th theme.Theme) [3]int {
	d := dimRGB(hexRGB(th.FG), hexRGB(th.BG))
	grey := int(math.Round(0.213*float64(d[0]) + 0.715*float64(d[1]) + 0.072*float64(d[2])))
	return [3]int{grey, grey, grey}
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

func selectedBoxLine(th theme.Theme, value string, width int) string {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(th.OK)).Render
	value = fitPlain(value, width)
	rest := strings.TrimPrefix(value, icons.Cursor) + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
	fill := theme.Bg(th.Surface2)
	return border("│") + fill + theme.Fg(th.OK) + icons.Cursor + theme.Fg(th.FG) + rest + sgrReset + border("│")
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
	style := theme.Fg(fg) + theme.Bg(bg)
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

// hexRGB is BG's or FG's colour, always #RRGGBB (Limited's fills are
// palette indices and must not come here).
func hexRGB(color string) [3]int {
	v, err := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	if err != nil {
		return [3]int{}
	}
	return [3]int{int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)}
}

const sgrReset = "\x1b[0m"
