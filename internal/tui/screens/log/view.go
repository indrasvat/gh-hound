package log

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/logs/fold"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(header(m)+"  "+scrollIndicator(m), width)}
	gutter := gutterWidth(len(m.Document.Lines))
	budget := max(m.Height, 1)
	for _, row := range m.visibleRows() {
		if budget == 0 {
			break
		}
		switch row.kind {
		case rowGroup:
			f := m.groups[row.line]
			lines = append(lines, renderFold(row.line, gutter, f.Title, f.CollapsedCount, !row.open, width))
			budget--
		case rowRepeat:
			lines = append(lines, renderRepeat(row.line, gutter, m.repeatTitle(row), row.end-row.line+1, !row.open, width))
			budget--
		default:
			line := m.Document.Lines[row.line-1]
			rendered := renderLine(line, gutter, m.isMatch(line.Number), m.Wrap, width)
			if len(rendered) > budget {
				rendered = rendered[:budget]
			}
			lines = append(lines, rendered...)
			budget -= len(rendered)
		}
	}
	return strings.Join(lines, "\n")
}

func gutterWidth(lines int) int {
	return max(3, len(fmt.Sprint(lines)))
}

func gutterText(number, width int) string {
	return fmt.Sprintf("%0*d", width, number)
}

func header(m Model) string {
	if m.InputMode {
		return fmt.Sprintf("log · /%s▌", m.input)
	}
	if m.RangeLabel != "" {
		return fmt.Sprintf("log · [%s] · ⎋ clear", m.RangeLabel)
	}
	if m.LastJump != "" && m.Search.Query == "" {
		head := fmt.Sprintf("log · t→%s", m.LastJump)
		if m.Folded() {
			head += " · compact"
		}
		return head
	}
	if m.Search.Query != "" {
		head := fmt.Sprintf("log · /%s · match %d/%d", m.Search.Query, m.Search.Current, m.Search.Total)
		if m.Folded() {
			head += " · compact"
		}
		return head
	}
	if m.Folded() {
		folds, hidden := m.foldTotals()
		return fmt.Sprintf("log · compact · %s lines folded in %s place%s", groupDigits(hidden), groupDigits(folds), plural(folds))
	}
	return "log"
}

// scrollIndicator is a compact viewport-position gauge for the header.
func scrollIndicator(m Model) string {
	total := len(m.rows())
	if total <= 0 || m.Height <= 0 || total <= m.Height {
		return ""
	}
	cells := 8
	pos := min(m.top(), m.maxTopRow()) * (cells - 1) / max(total-m.Height, 1)
	if pos >= cells {
		pos = cells - 1
	}
	bar := []rune(strings.Repeat("─", cells))
	bar[pos] = '▮'
	return colorize(sgrLineNo, string(bar))
}

func fit(value string, width int) string {
	value = strings.TrimSpace(value)
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}

func foldGlyph(collapsed bool) string {
	if collapsed {
		return "▸"
	}
	return "▾"
}

func renderFold(number, gutterW int, title string, count int, collapsed bool, width int) string {
	gutter := colorize(sgrLineNo, gutterText(number, gutterW))
	label := colorize(sgrFold, fmt.Sprintf("%s %s", foldGlyph(collapsed), title))
	countText := colorize(sgrDim, groupDigits(count)+" line"+plural(count))
	return backgroundSafe(gutter+" "+label+" "+countText, width, sgrFG, sgrFoldBG)
}

// renderRepeat draws a repeat fold on its own band; the summary yields to
// width, the hidden-line count never does.
func renderRepeat(number, gutterW int, title string, count int, collapsed bool, width int) string {
	marker := "⋯"
	if !collapsed {
		marker = "▾"
	}
	countText := groupDigits(count) + " line" + plural(count) + " hidden"
	head, rest, _ := strings.Cut(title, " · ")
	main, severe, _ := strings.Cut(rest, fold.SevereGlyph+" ")
	main = strings.TrimSuffix(main, " · ")
	lead := gutterW + 3 + ansi.StringWidth(head)
	room := width - lead - 2 - ansi.StringWidth(countText) - 1
	label := colorize(sgrRepeatHead, head)
	if main != "" && room > 3 {
		label += colorize(sgrFold, " · "+fitPlainWidth(main, room-3))
	}
	if severe != "" && ansi.StringWidth(label)+ansi.StringWidth(severe)+5 <= width-gutterW-3-ansi.StringWidth(countText)-1 {
		label += colorize(sgrFold, " · ") + colorize(sgrFail, fold.SevereGlyph+" "+severe)
	}
	left := colorize(sgrLineNo, gutterText(number, gutterW)) + " " + colorize(sgrRepeat, marker) + " " + label
	gap := max(1, width-ansi.StringWidth(left)-ansi.StringWidth(countText)-1)
	return backgroundSafe(left+strings.Repeat(" ", gap)+colorize(sgrDim, countText), width, sgrFG, sgrRepeatBG)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func fitPlainWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

func groupDigits(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// renderLine returns one screen line, or several when wrapping.
func renderLine(line logs.Line, gutterW int, searchHit, wrap bool, width int) []string {
	gutter := colorize(sgrLineNo, gutterText(line.Number, gutterW))
	text := renderLogText(line)
	prefix := ""
	if searchHit {
		prefix = colorize(sgrSearch, "› search-hit") + " "
	}
	if !wrap {
		value := prefix + gutter + " " + text
		if searchHit {
			return []string{backgroundSafe(value, width, sgrFG, sgrSearchBG)}
		}
		return []string{fitANSI(value, width)}
	}
	lead := ansi.StringWidth(prefix) + gutterW + 1
	parts := strings.Split(ansi.Hardwrap(text, max(1, width-lead), true), "\n")
	out := make([]string, 0, len(parts))
	for i, part := range parts {
		head := prefix + gutter + " "
		if i > 0 {
			head = strings.Repeat(" ", lead-2) + colorize(sgrDim, "↪") + " "
		}
		value := head + part
		if searchHit {
			out = append(out, backgroundSafe(value, width, sgrFG, sgrSearchBG))
		} else {
			out = append(out, fitANSI(value, width))
		}
	}
	return out
}

func renderLogText(line logs.Line) string {
	text := line.Text
	if strings.TrimSpace(text) == "" {
		return ""
	}
	tokens := normalizedTokens(text, line.Tokens)
	baseStyle := baseStyle(line.Tokens)
	var out strings.Builder
	cursor := 0
	wantStart := strings.Index(text, " want ")
	for _, token := range tokens {
		if token.Start < cursor || token.Start > len(text) || token.End > len(text) {
			continue
		}
		writeStyled(&out, text[cursor:token.Start], baseStyle)
		style := tokenStyle(token.Class)
		if token.Class == logs.ClassString && wantStart >= 0 {
			if token.Start > wantStart {
				style = sgrStringFail
			} else {
				style = sgrStringOK
			}
		}
		writeStyled(&out, text[token.Start:token.End], style)
		cursor = token.End
	}
	writeStyled(&out, text[cursor:], baseStyle)
	return out.String()
}

func normalizedTokens(text string, tokens []logs.Token) []logs.Token {
	out := make([]logs.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Start < 0 || token.End <= token.Start || token.End > len(text) {
			continue
		}
		switch token.Class {
		case logs.ClassFail, logs.ClassWarn:
			continue
		case logs.ClassWant:
			continue
		}
		out = append(out, token)
	}
	slices.SortFunc(out, func(a, b logs.Token) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		return tokenRank(a.Class) - tokenRank(b.Class)
	})
	filtered := out[:0]
	cursor := 0
	for _, token := range out {
		if token.Start < cursor {
			continue
		}
		filtered = append(filtered, token)
		cursor = token.End
	}
	return filtered
}

func baseStyle(tokens []logs.Token) string {
	for _, token := range tokens {
		switch token.Class {
		case logs.ClassFail:
			return sgrFail
		case logs.ClassWarn:
			return sgrWarn
		}
	}
	return ""
}

func tokenRank(class logs.TokenClass) int {
	switch class {
	case logs.ClassTimestamp:
		return 0
	case logs.ClassPath:
		return 1
	case logs.ClassString:
		return 2
	case logs.ClassNumber:
		return 3
	case logs.ClassOK:
		return 4
	case logs.ClassCommand:
		return 5
	default:
		return 9
	}
}

func tokenStyle(class logs.TokenClass) string {
	switch class {
	case logs.ClassTimestamp:
		return sgrTimestamp
	case logs.ClassCommand:
		return sgrCommand
	case logs.ClassOK:
		return sgrOK
	case logs.ClassPath:
		return sgrPath
	case logs.ClassString:
		return sgrString
	case logs.ClassNumber:
		return sgrNumber
	default:
		return ""
	}
}

func writeStyled(out *strings.Builder, value, style string) {
	if value == "" {
		return
	}
	if style == "" {
		out.WriteString(value)
		return
	}
	out.WriteString(style)
	out.WriteString(value)
	out.WriteString(sgrReset)
}

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
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

func padANSI(value string, width int) string {
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}

func backgroundSafe(value string, width int, fg string, bg string) string {
	value = padANSI(fitANSI(value, width), width)
	style := fg + bg
	value = strings.ReplaceAll(value, sgrReset, sgrReset+style)
	return style + value + sgrReset
}

const (
	sgrReset      = "\x1b[0m"
	sgrFG         = "\x1b[38;2;234;232;217m"
	sgrDim        = "\x1b[38;2;107;112;96m"
	sgrLineNo     = "\x1b[38;2;61;66;51m"
	sgrTimestamp  = "\x1b[38;2;107;112;96m"
	sgrCommand    = "\x1b[38;2;110;156;181m"
	sgrOK         = "\x1b[38;2;79;211;122m"
	sgrFail       = "\x1b[38;2;226;86;75m"
	sgrWarn       = "\x1b[38;2;232;137;90m"
	sgrPath       = "\x1b[38;2;110;156;181m\x1b[4m"
	sgrString     = "\x1b[38;2;207;205;187m"
	sgrStringOK   = "\x1b[38;2;79;211;122m"
	sgrStringFail = "\x1b[38;2;226;86;75m"
	sgrNumber     = "\x1b[38;2;224;163;62m"
	sgrFold       = "\x1b[38;2;207;205;187m"
	sgrSearch     = "\x1b[38;2;224;163;62m"
	sgrFoldBG     = "\x1b[48;2;27;29;23m"
	sgrRepeat     = "\x1b[38;2;110;156;181m"
	sgrRepeatHead = "\x1b[1;38;2;110;156;181m"
	sgrRepeatBG   = "\x1b[48;2;20;28;33m"
	sgrSearchBG   = "\x1b[48;2;43;33;24m"
)
