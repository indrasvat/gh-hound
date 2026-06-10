package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

const minFrameWidth = 48

func frameViewSize(th theme.Theme, title, context, right, body, footer string, width, height int, focused bool) string {
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
	body = colorizeBody(th, body, inner)

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
	spacer := max(width-visibleLen(raw), 1)
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

func overlayBox(th theme.Theme, title, body string, width int) string {
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
	inner := max(boxWidth-2, 1)
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

	pad := max((width-boxWidth)/2, 0)
	prefix := strings.Repeat(" ", pad)
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
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
	color = strings.TrimPrefix(color, "#")
	if len(color) != 6 {
		return ""
	}
	r, errR := strconv.ParseUint(color[0:2], 16, 8)
	g, errG := strconv.ParseUint(color[2:4], 16, 8)
	b, errB := strconv.ParseUint(color[4:6], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return ""
	}
	code := 38
	if background {
		code = 48
	}
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", code, r, g, b)
}

const sgrReset = "\x1b[0m"
