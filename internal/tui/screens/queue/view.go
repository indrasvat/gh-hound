package queue

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	return ViewSize(m, width, 0)
}

func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		fitANSI(headline(m.Report, width), width),
		fitANSI(summaryLine(m.Report), width),
	}
	if len(m.Report.Groups) == 0 {
		lines = append(lines, fitANSI(colorize(sgrDim, "  no GitHub queue evidence for this target."), width))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "")
	lines = append(lines, fitANSI(groupsLine(m.Report.Groups), width))

	rows := m.Rows()
	if len(rows) == 0 {
		lines = append(lines, fitANSI(colorize(sgrDim, "  group is configured, but no active members are exposed."), width))
		return strings.Join(lines, "\n")
	}
	selected := max(min(m.Selected, len(rows)-1), 0)
	header := memberHeader(width)
	if header != "" {
		lines = append(lines, header)
	}
	var details []string
	if width >= 110 {
		if row, ok := m.SelectedRow(); ok {
			details = detailBlock(m.Report, row, width)
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
		lines = append(lines, memberRow(row, start+index == selected, width))
	}
	if len(rows) > end-start {
		lines = append(lines, fitANSI(colorize(sgrDim, fmt.Sprintf("  rows %d-%d/%d", start+1, end, len(rows))), width))
	}
	if len(details) > 0 {
		lines = append(lines, "")
		lines = append(lines, details...)
	}
	return strings.Join(lines, "\n")
}

func headline(report model.ConcurrencyReport, width int) string {
	glyph, color := statusGlyph(report.Status)
	left := colorize(color, glyph+" GitHub queue") + colorize(sgrDim, " · "+string(report.Status))
	right := colorize(sgrSubtle, targetLabel(report.Target))
	return joinRightANSI(left, right, width)
}

func summaryLine(report model.ConcurrencyReport) string {
	text := strings.TrimSpace(report.Summary)
	if text == "" {
		text = strings.TrimSpace(report.Decision.Reason)
	}
	if text == "" {
		text = "queue state unknown"
	}
	if report.Decision.Action != "" {
		return "  " + colorize(sgrFGSoft, text) + colorize(sgrDim, " · "+string(report.Decision.Action))
	}
	return "  " + colorize(sgrFGSoft, text)
}

func groupsLine(groups []model.ConcurrencyGroup) string {
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
	return "  " + colorize(sgrInfo, fmt.Sprintf("%d group(s)", len(groups))) +
		colorize(sgrDim, fmt.Sprintf(" · %s · %d active · %d blocker(s)", scope, active, blockers))
}

func memberHeader(width int) string {
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
	return fitANSI(colorize(sgrDim, line), width)
}

func memberRow(row Row, selected bool, width int) string {
	member := row.Member
	prefix := "  "
	if selected {
		prefix = colorize(sgrOK, icons.Cursor) + " "
	}
	glyph, color := memberGlyph(member)
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
		line := prefix + colorize(color, glyph) + " " + colorize(sgrFGSoft, run) + colorize(sgrDim, " pos "+position+" · ") + colorize(sgrFG, name) + colorize(sgrDim, " · "+row.GroupName+" · "+verdict)
		return fitANSI(line, width)
	}
	line := prefix + colorize(color, glyph) + "  " +
		padANSI(colorize(sgrFGSoft, run), 10) +
		padANSI(colorize(sgrSubtle, position), 5) +
		padANSI(colorize(sgrFG, truncate(name, 28)), 30)
	if width >= 110 {
		line += padANSI(colorize(sgrSubtle, truncate(row.GroupName, 20)), 22)
	}
	line += colorize(sgrDim, verdict)
	return fitANSI(line, width)
}

func detailBlock(report model.ConcurrencyReport, row Row, width int) []string {
	member := row.Member
	lines := []string{
		fitANSI(colorize(sgrDim, "selected"), width),
		fitANSI("  "+colorize(sgrFGSoft, fmt.Sprintf("run %d", member.RunID))+colorize(sgrDim, " · "+string(member.Status)), width),
	}
	if row.GroupName != "" {
		lines = append(lines, fitANSI("  "+colorize(sgrFGSoft, row.GroupName)+colorize(sgrDim, " · concurrency group"), width))
	}
	if member.JobID != nil {
		lines = append(lines, fitANSI("  "+colorize(sgrFGSoft, fmt.Sprintf("job %d", *member.JobID))+colorize(sgrDim, " · "+member.JobName), width))
	}
	if member.BlocksTarget {
		lines = append(lines, fitANSI("  "+colorize(sgrRun, "blocks target"), width))
	}
	if member.Target {
		lines = append(lines, fitANSI("  "+colorize(sgrInfo, "target item"), width))
	}
	if len(report.Blockers) > 0 {
		lines = append(lines, fitANSI("  "+colorize(sgrDim, fmt.Sprintf("%d item(s) ahead", len(report.Blockers))), width))
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

func statusGlyph(status model.ConcurrencyStatus) (string, string) {
	switch status {
	case model.ConcurrencyStatusBlocked:
		return "◌", sgrInfo
	case model.ConcurrencyStatusNotConcurrency:
		return icons.Success, sgrOK
	case model.ConcurrencyStatusInactive, model.ConcurrencyStatusUnsupported:
		return "◇", sgrDim
	case model.ConcurrencyStatusError:
		return icons.Failure, sgrFail
	default:
		return "◇", sgrDim
	}
}

func memberGlyph(member model.ConcurrencyMember) (string, string) {
	switch {
	case member.Target && member.Status == model.StatusPending:
		return "◌", sgrInfo
	case member.Target:
		return "◌", sgrInfo
	case member.Status == model.StatusInProgress:
		return icons.InProgress, sgrRun
	case member.BlocksTarget:
		return icons.InProgress, sgrRun
	default:
		return "◇", sgrDim
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

const (
	sgrReset  = "\x1b[0m"
	sgrOK     = "\x1b[38;2;79;211;122m"
	sgrFail   = "\x1b[38;2;226;86;75m"
	sgrRun    = "\x1b[38;2;224;163;62m"
	sgrInfo   = "\x1b[38;2;110;156;181m"
	sgrDim    = "\x1b[38;2;107;112;96m"
	sgrSubtle = "\x1b[38;2;150;145;128m"
	sgrFG     = "\x1b[38;2;234;232;217m"
	sgrFGSoft = "\x1b[38;2;207;205;187m"
)
