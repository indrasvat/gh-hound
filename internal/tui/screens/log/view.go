package log

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(header(m)+"  scrollbar ▌", width)}
	for _, row := range m.visibleRows() {
		if row.IsFold {
			lines = append(lines, renderFold(row.Line.Number, row.Fold.Title, row.Fold.CollapsedCount, row.Collapsed, width))
			continue
		}
		lines = append(lines, renderLine(row.Line, m.isMatch(row.Line.Number), width))
	}
	return strings.Join(lines, "\n")
}

func header(m Model) string {
	if m.InputMode && m.TimeInput {
		return fmt.Sprintf("log · t→%s▌", m.input)
	}
	if m.InputMode {
		return fmt.Sprintf("log · /%s▌", m.input)
	}
	if m.LastJump != "" && m.Search.Query == "" {
		return fmt.Sprintf("log · t→%s", m.LastJump)
	}
	if m.Search.Query != "" {
		return fmt.Sprintf("log · /%s · match %d/%d", m.Search.Query, m.Search.Current, m.Search.Total)
	}
	return "log"
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

func renderFold(number int, title string, count int, collapsed bool, width int) string {
	glyph := "▾"
	if collapsed {
		glyph = "▸"
	}
	gutter := colorize(sgrLineNo, fmt.Sprintf("%03d", number))
	label := colorize(sgrFold, fmt.Sprintf("%s %s", glyph, title))
	countText := colorize(sgrDim, fmt.Sprintf("%d lines", count))
	return backgroundSafe(gutter+" "+label+" "+countText, width, sgrFG, sgrFoldBG)
}

func renderLine(line logs.Line, searchHit bool, width int) string {
	gutter := colorize(sgrLineNo, fmt.Sprintf("%03d", line.Number))
	value := gutter + " " + renderLogText(line)
	if searchHit {
		value = colorize(sgrSearch, "› search-hit") + " " + value
		return backgroundSafe(value, width, sgrFG, sgrSearchBG)
	}
	return fitANSI(value, width)
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
	sgrSearchBG   = "\x1b[48;2;43;33;24m"
)
