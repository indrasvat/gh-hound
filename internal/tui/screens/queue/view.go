package queue

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	return ViewSize(m, width, 0)
}

func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	st := stylesFor(m.Theme.OrBramble())
	lines := []string{
		fitANSI(headline(st, m.Report, width), width),
		fitANSI(summaryLine(st, m.Report), width),
	}
	if len(m.Report.Groups) == 0 {
		lines = append(lines, fitANSI(colorize(st.dim, "  no GitHub queue evidence for this target."), width))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "")
	lines = append(lines, fitANSI(groupsLine(st, m.Report.Groups), width))

	rows := m.Rows()
	if len(rows) == 0 {
		lines = append(lines, fitANSI(colorize(st.dim, "  group is configured, but no active members are exposed."), width))
		return strings.Join(lines, "\n")
	}
	selected := max(min(m.Selected, len(rows)-1), 0)
	header := memberHeader(st, width)
	if header != "" {
		lines = append(lines, header)
	}
	var details []string
	if width >= 110 {
		if row, ok := m.SelectedRow(); ok {
			details = detailBlock(st, m.Report, row, width)
		}
	}
	if len(details) > 0 && height > 0 {
		detailRows := 1 + len(details)
		if height < len(lines)+detailRows+2 {
			details = nil
		}
	}
	fixedRows := len(lines) + 1
	if len(details) > 0 {
		fixedRows += 1 + len(details)
	}
	capacity := rowCapacity(height, fixedRows, len(rows))
	start, end := viewport(selected, len(rows), capacity)
	for index, row := range rows[start:end] {
		lines = append(lines, memberRow(st, row, start+index == selected, width))
	}
	if len(rows) > end-start {
		lines = append(lines, fitANSI(colorize(st.dim, fmt.Sprintf("  rows %d-%d/%d", start+1, end, len(rows))), width))
	}
	if len(details) > 0 {
		lines = append(lines, "")
		lines = append(lines, details...)
	}
	return strings.Join(lines, "\n")
}

func headline(st styles, report model.ConcurrencyReport, width int) string {
	glyph, color := statusGlyph(st, report.Status)
	left := colorize(color, glyph+" GitHub queue") + colorize(st.dim, " · "+string(report.Status))
	right := colorize(st.subtle, targetLabel(report.Target))
	return joinRightANSI(left, right, width)
}

func summaryLine(st styles, report model.ConcurrencyReport) string {
	text := strings.TrimSpace(report.Summary)
	if text == "" {
		text = strings.TrimSpace(report.Decision.Reason)
	}
	if text == "" {
		text = "queue state unknown"
	}
	if report.Decision.Action != "" {
		return "  " + colorize(st.fgSoft, text) + colorize(st.dim, " · "+string(report.Decision.Action))
	}
	return "  " + colorize(st.fgSoft, text)
}

func groupsLine(st styles, groups []model.ConcurrencyGroup) string {
	active, blockers := 0, 0
	scopes := map[model.ConcurrencyScope]struct{}{}
	for _, group := range groups {
		count := group.ActiveMemberCount
		if count == 0 {
			count = len(group.Members)
		}
		active += count
		blockers += group.BlockedByCount()
		scopes[group.Scope()] = struct{}{}
	}
	scope := "mixed"
	if len(scopes) == 1 {
		for value := range scopes {
			scope = string(value)
		}
	}
	return "  " + colorize(st.info, fmt.Sprintf("%d group(s)", len(groups))) +
		colorize(st.dim, fmt.Sprintf(" · %s · %d active · %d blocker(s)", scope, active, blockers))
}

func memberHeader(st styles, width int) string {
	if width < 92 {
		return ""
	}
	line := "  St " +
		padANSI("Run", 10) +
		padANSI("Pos", 5) +
		padANSI("Name", 30)
	if width >= 110 {
		line += padANSI("Group", 22)
	}
	line += "Verdict"
	return fitANSI(colorize(st.dim, line), width)
}

func memberRow(st styles, row Row, selected bool, width int) string {
	member := row.Member
	prefix := "  "
	if selected {
		prefix = colorize(st.ok, icons.Cursor) + " "
	}
	glyph, color := memberGlyph(st, member)
	run := fmt.Sprintf("#%d", member.RunID)
	position := "-"
	if member.Position != nil {
		position = fmt.Sprintf("%d", *member.Position)
	}
	name := member.RunName
	if member.JobName != "" {
		name += " / " + member.JobName
	}
	if name == "" {
		name = "workflow run"
	}
	verdict := verdictText(member)
	if width < 92 {
		line := prefix + colorize(color, glyph) + " " + colorize(st.fgSoft, run) + colorize(st.dim, " pos "+position+" · ") + colorize(st.fg, name) + colorize(st.dim, " · "+row.GroupName+" · "+verdict)
		return fitANSI(line, width)
	}
	line := prefix + colorize(color, glyph) + "  " +
		padANSI(colorize(st.fgSoft, run), 10) +
		padANSI(colorize(st.subtle, position), 5) +
		padANSI(colorize(st.fg, truncate(name, 28)), 30)
	if width >= 110 {
		line += padANSI(colorize(st.subtle, truncate(row.GroupName, 20)), 22)
	}
	line += colorize(st.dim, verdict)
	return fitANSI(line, width)
}

func detailBlock(st styles, report model.ConcurrencyReport, row Row, width int) []string {
	member := row.Member
	lines := []string{
		fitANSI(colorize(st.dim, "selected"), width),
		fitANSI("  "+colorize(st.fgSoft, fmt.Sprintf("run %d", member.RunID))+colorize(st.dim, " · "+string(member.Status)), width),
	}
	if row.GroupName != "" {
		lines = append(lines, fitANSI("  "+colorize(st.fgSoft, row.GroupName)+colorize(st.dim, " · concurrency group"), width))
	}
	if member.JobID != nil {
		lines = append(lines, fitANSI("  "+colorize(st.fgSoft, fmt.Sprintf("job %d", *member.JobID))+colorize(st.dim, " · "+member.JobName), width))
	}
	if member.BlocksTarget {
		lines = append(lines, fitANSI("  "+colorize(st.run, "blocks target"), width))
	}
	if member.Target {
		lines = append(lines, fitANSI("  "+colorize(st.info, "target item"), width))
	}
	if len(report.Blockers) > 0 {
		lines = append(lines, fitANSI("  "+colorize(st.dim, fmt.Sprintf("%d item(s) ahead", len(report.Blockers))), width))
	}
	return lines
}

func targetLabel(target model.ConcurrencyTarget) string {
	switch target.Kind {
	case model.ConcurrencyTargetRun:
		return fmt.Sprintf("run %d", target.RunID)
	case model.ConcurrencyTargetJob:
		return fmt.Sprintf("job %d", target.JobID)
	case model.ConcurrencyTargetGroup:
		return target.GroupName
	case model.ConcurrencyTargetList:
		return "groups"
	default:
		return "target"
	}
}

func statusGlyph(st styles, status model.ConcurrencyStatus) (string, string) {
	switch status {
	case model.ConcurrencyStatusBlocked:
		return "◌", st.info
	case model.ConcurrencyStatusNotConcurrency:
		return icons.Success, st.ok
	case model.ConcurrencyStatusInactive, model.ConcurrencyStatusUnsupported:
		return "◇", st.dim
	case model.ConcurrencyStatusError:
		return icons.Failure, st.fail
	default:
		return "◇", st.dim
	}
}

func memberGlyph(st styles, member model.ConcurrencyMember) (string, string) {
	switch {
	case member.Target && member.Status == model.StatusPending:
		return "◌", st.info
	case member.Target:
		return "◌", st.info
	case member.Status == model.StatusInProgress:
		return icons.InProgress, st.run
	case member.BlocksTarget:
		return icons.InProgress, st.run
	default:
		return "◇", st.dim
	}
}

func verdictText(member model.ConcurrencyMember) string {
	switch {
	case member.Target && member.BlocksTarget:
		return "target and blocker"
	case member.Target:
		return "target"
	case member.BlocksTarget:
		return "ahead of target"
	default:
		return string(member.Status)
	}
}

func rowCapacity(height, fixedRows, total int) int {
	if height <= 0 {
		return total
	}
	capacity := height - fixedRows
	if total > capacity {
		capacity--
	}
	return max(capacity, 1)
}

func viewport(selected, total, capacity int) (int, int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	if capacity >= total {
		return 0, total
	}
	start := max(selected-capacity/2, 0)
	if start+capacity > total {
		start = total - capacity
	}
	return start, start + capacity
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
	value = fitANSI(value, width)
	pad := width - ansi.StringWidth(value)
	if pad > 0 {
		return value + strings.Repeat(" ", pad)
	}
	return value
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func joinRightANSI(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = fitANSI(right, width)
	rightWidth := ansi.StringWidth(right)
	leftWidth := max(width-rightWidth-1, 0)
	left = fitANSI(left, leftWidth)
	spacer := max(width-ansi.StringWidth(left)-rightWidth, 1)
	return left + strings.Repeat(" ", spacer) + right
}

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
}

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the queue screen.
type styles struct {
	ok, fail, run, info, dim, subtle, fg, fgSoft string
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), run: theme.Fg(th.Run), info: theme.Fg(th.Info),
		dim: theme.Fg(th.Dim), subtle: theme.Fg(th.Subtle), fg: theme.Fg(th.FG), fgSoft: theme.Fg(th.FGSoft),
	}
}

const sgrReset = "\x1b[0m"
