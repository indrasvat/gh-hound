package log

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/logs/fold"
	"github.com/indrasvat/gh-hound/internal/numfmt"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/components/textinput"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	m.Width = width
	st := stylesFor(m.Theme.OrBramble())
	lines := []string{fitANSI(strings.TrimSpace(colorize(st.header, header(m, width))+"  "+scrollIndicator(st, m)), width)}
	budget := max(m.Height, 1)
	_, skip := m.position()
	for i, row := range m.visibleRows() {
		if budget == 0 {
			break
		}
		rendered := m.renderRow(st, row)
		if i == 0 {
			rendered = rendered[min(skip, len(rendered)-1):]
		}
		rendered = rendered[:min(len(rendered), budget)]
		lines = append(lines, rendered...)
		budget -= len(rendered)
	}
	return strings.Join(lines, "\n")
}

// renderRow draws one row at m.Width; a wrapped line takes several.
func (m Model) renderRow(st styles, row viewRow) []string {
	gutter := gutterWidth(len(m.Document.Lines))
	switch row.kind {
	case rowGroup:
		f := m.groups[row.line]
		return []string{renderFold(st, row.line, gutter, TerminalSafe(f.Title), f.CollapsedCount, !row.open, m.Width)}
	case rowRepeat:
		return []string{renderRepeat(st, row.line, gutter, TerminalSafe(m.repeatTitle(row)), row.end-row.line+1, !row.open, m.Width)}
	}
	line := m.Document.Lines[row.line-1]
	return renderLine(st, line, gutter, m.hit(line.Number), m.wrapping(), m.Width)
}

func gutterWidth(lines int) int {
	return max(3, len(fmt.Sprint(lines)))
}

func gutterText(number, width int) string {
	return fmt.Sprintf("%0*d", width, number)
}

func header(m Model, width int) string {
	if m.InputMode {
		return "log · /" + textinput.Tail(m.input, width-8) + "▌"
	}
	if m.RangeLabel != "" {
		head := fmt.Sprintf("log · [%s]", m.RangeLabel)
		if m.Search.Query != "" {
			head += " · " + matchCounter(m)
		}
		if m.Folded() {
			head += " · compact"
		}
		return head + " · ⎋ clear"
	}
	if m.LastJump != "" && m.Search.Query == "" {
		head := fmt.Sprintf("log · t→%s", m.LastJump)
		if m.Folded() {
			head += " · compact"
		}
		return head
	}
	if m.Search.Query != "" {
		head := "log · " + matchCounter(m)
		if m.Folded() {
			head += " · compact"
		}
		return head + " · ⎋ clear"
	}
	if m.Folded() {
		folds, hidden := m.folds(true)
		if folds == 0 {
			return "log · compact"
		}
		return fmt.Sprintf("log · compact · %s lines folded in %s place%s", numfmt.Group(hidden), numfmt.Group(folds), plural(folds))
	}
	return "log"
}

func matchCounter(m Model) string {
	return fmt.Sprintf("/%s · match %s/%s", m.Search.Query, numfmt.Group(m.Search.Current), numfmt.Group(m.Search.Total))
}

// scrollIndicator is a compact viewport-position gauge for the header.
func scrollIndicator(st styles, m Model) string {
	total := len(m.rows())
	if total <= 0 || m.Height <= 0 || total <= m.Height {
		return ""
	}
	cells := 8
	top, _ := m.position()
	pos := top * (cells - 1) / max(total-m.Height, 1)
	if pos >= cells {
		pos = cells - 1
	}
	bar := []rune(strings.Repeat("─", cells))
	bar[pos] = '▮'
	return colorize(st.line2, string(bar))
}

func foldGlyph(collapsed bool) string {
	if collapsed {
		return "▸"
	}
	return "▾"
}

func renderFold(st styles, number, gutterW int, title string, count int, collapsed bool, width int) string {
	gutter := colorize(st.line2, gutterText(number, gutterW))
	label := colorize(st.ok, foldGlyph(collapsed)) + " " + colorize(st.fgSoft, title)
	countText := colorize(st.dim, numfmt.Group(count)+" line"+plural(count))
	return backgroundSafe(gutter+" "+label+" "+countText, width, st.fg, st.surfaceBG)
}

// renderRepeat draws a repeat fold on its own band. Width goes to the fold's
// size, the hidden count, the error warning, then the varying values.
func renderRepeat(st styles, number, gutterW int, title string, count int, collapsed bool, width int) string {
	marker := "⋯"
	if !collapsed {
		marker = "▾"
	}
	head, rest, _ := strings.Cut(title, " · ")
	main, severe, _ := strings.Cut(rest, fold.SevereGlyph+" ")
	main = strings.TrimSuffix(main, " · ")
	avail := width - gutterW - 3 - 1 // gutter, marker, right padding
	digits := numfmt.Group(count)
	full, short := digits+" line"+plural(count)+" hidden", digits+" hidden"
	if !collapsed {
		full, short = digits+" line"+plural(count), digits
	}
	countText := digits
	for _, c := range []string{full, short} {
		if ansi.StringWidth(head)+1+ansi.StringWidth(c) <= avail {
			countText = c
			break
		}
	}
	head = fitPlainWidth(head, avail-1-ansi.StringWidth(countText))
	room := avail - ansi.StringWidth(head) - 1 - ansi.StringWidth(countText)
	warning := ""
	if severe != "" && room >= 6 {
		warning = fitPlainWidth(fold.SevereGlyph+" "+severe, room-3)
		room -= 3 + ansi.StringWidth(warning)
	}
	label := colorize(st.header, head)
	if main != "" && room > 4 {
		label += colorize(st.fgSoft, " · "+fitPlainWidth(main, room-3))
	}
	if warning != "" {
		label += colorize(st.fgSoft, " · ") + colorize(st.fail, warning)
	}
	gutter := strings.Repeat(" ", gutterW) // an open fold's first line carries the number
	if collapsed {
		gutter = colorize(st.line2, gutterText(number, gutterW))
	}
	left := gutter + " " + colorize(st.info, marker) + " " + label
	gap := max(1, width-ansi.StringWidth(left)-ansi.StringWidth(countText)-1)
	return backgroundSafe(left+strings.Repeat(" ", gap)+colorize(st.dim, countText), width, st.fg, st.infoTintBG)
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

// renderLine returns one screen line, or several when wrapping. A search
// hit is tinted, the current one more strongly with a warn line number;
// neither changes the row's width.
func renderLine(st styles, line logs.Line, gutterW int, hit hitKind, wrap bool, width int) []string {
	gutterStyle, bg := st.line2, ""
	switch hit {
	case searchHit:
		bg = st.runTintBG
	case currentHit:
		gutterStyle, bg = st.run, st.runTintStrongBG
	}
	gutter := colorize(gutterStyle, gutterText(line.Number, gutterW))
	text := renderLogText(st, line)
	if !wrap {
		value := gutter + " " + text
		if bg != "" {
			return []string{backgroundSafe(value, width, st.fg, bg)}
		}
		return []string{fitANSI(value, width)}
	}
	lead := gutterW + 1
	parts := strings.Split(ansi.Hardwrap(text, max(1, width-lead), true), "\n")
	out := make([]string, 0, len(parts))
	base := baseStyle(st, line.Tokens)
	for i, part := range parts {
		head := gutter + " "
		if i > 0 {
			head = strings.Repeat(" ", lead-2) + colorize(st.dim, "↪") + " " + base
			part = trimLeadingSpace(part)
		}
		value := head + part
		if bg != "" {
			out = append(out, backgroundSafe(value, width, st.fg, bg))
		} else {
			out = append(out, fitANSI(value, width))
		}
	}
	return out
}

// trimLeadingSpace drops one space a wrap left at the start of a
// continuation, skipping any SGR sequences before it.
func trimLeadingSpace(s string) string {
	i := 0
	for i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '[' {
		j := strings.IndexByte(s[i:], 'm')
		if j < 0 {
			return s
		}
		i += j + 1
	}
	if i < len(s) && s[i] == ' ' {
		return s[:i] + s[i+1:]
	}
	return s
}

func renderLogText(st styles, line logs.Line) string {
	text := line.Text
	if strings.TrimSpace(text) == "" {
		return ""
	}
	tokens := normalizedTokens(text, line.Tokens)
	baseStyle := baseStyle(st, line.Tokens)
	var out strings.Builder
	cursor := 0
	wantStart := strings.Index(text, " want ")
	for _, token := range tokens {
		if token.Start < cursor || token.Start > len(text) || token.End > len(text) {
			continue
		}
		writeStyled(&out, text[cursor:token.Start], baseStyle)
		style := tokenStyle(st, token.Class)
		if token.Class == logs.ClassString && wantStart >= 0 {
			if token.Start > wantStart {
				style = st.fail
			} else {
				style = st.ok
			}
		}
		writeStyled(&out, text[token.Start:token.End], style)
		cursor = token.End
	}
	writeStyled(&out, text[cursor:], baseStyle)
	return TerminalSafe(out.String())
}

// TerminalSafe keeps a log's colours but nothing that moves the cursor:
// other escape sequences, C0/C1 controls and invalid UTF-8 are dropped or
// replaced, and tabs expand to 8-column stops, so every row is exactly as
// wide as it measures.
func TerminalSafe(s string) string {
	if !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	var b strings.Builder
	col := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b:
			n := escapeLen(s[i:])
			if isSGR(s[i : i+n]) {
				b.WriteString(s[i : i+n])
			}
			i += n
		case c == '\t':
			pad := 8 - col%8
			b.WriteString(strings.Repeat(" ", pad))
			col += pad
			i++
		case c < 0x20 || c == 0x7f:
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			i += size
			if r >= 0x80 && r <= 0x9f {
				continue // C1 controls, e.g. U+009B is an 8-bit CSI
			}
			b.WriteRune(r) // an invalid byte decodes to U+FFFD
			col += ansi.StringWidth(string(r))
		}
	}
	return b.String()
}

func unsafeRune(r rune) bool {
	return r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f || r == utf8.RuneError
}

// isSGR reports a plain colour sequence: ESC [ params m, params only 0-9 ; :.
func isSGR(seq string) bool {
	if len(seq) < 3 || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return false
	}
	for i := 2; i < len(seq)-1; i++ {
		if seq[i] < '0' || seq[i] > ';' {
			return false
		}
	}
	return true
}

// escapeLen is the byte length of the escape sequence at the start of s:
// CSI up to its final byte, OSC up to BEL or ST, else ESC and one byte.
func escapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	default:
		return 2
	}
	return len(s)
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

func baseStyle(st styles, tokens []logs.Token) string {
	for _, token := range tokens {
		switch token.Class {
		case logs.ClassFail:
			return st.fail
		case logs.ClassWarn:
			return st.warn
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

func tokenStyle(st styles, class logs.TokenClass) string {
	switch class {
	case logs.ClassTimestamp:
		return st.dim
	case logs.ClassCommand:
		return st.info
	case logs.ClassOK:
		return st.ok
	case logs.ClassPath:
		return st.path
	case logs.ClassString:
		return st.fgSoft
	case logs.ClassNumber:
		return st.run
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

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the log.
type styles struct {
	fg, fgSoft, dim, line2, ok, fail, warn, run, info string
	path, header                                      string
	surfaceBG, infoTintBG, runTintBG, runTintStrongBG string
}

func stylesFor(th theme.Theme) styles {
	info := theme.Fg(th.Info)
	return styles{
		fg: theme.Fg(th.FG), fgSoft: theme.Fg(th.FGSoft), dim: theme.Fg(th.Dim), line2: theme.Fg(th.Line2),
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), warn: theme.Fg(th.Warn), run: theme.Fg(th.Run), info: info,
		path: info + sgrUnderline, header: sgrBold + info,
		surfaceBG: theme.Bg(th.Surface), infoTintBG: theme.Bg(th.InfoTint),
		runTintBG: theme.Bg(th.RunTint), runTintStrongBG: theme.Bg(th.RunTintStrong),
	}
}

const (
	sgrReset     = "\x1b[0m"
	sgrBold      = "\x1b[1m"
	sgrUnderline = "\x1b[4m"
)
