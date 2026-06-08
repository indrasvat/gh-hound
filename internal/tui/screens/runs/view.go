package runs

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/components/sparkline"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int, now time.Time) string {
	return ViewSize(m, width, 0, now)
}

func ViewSize(m Model, width, height int, now time.Time) string {
	if width <= 0 {
		width = 80
	}
	if m.AllGreen() {
		return renderAllGreen(m, width, height, now)
	}
	return renderRuns(m, width, height, now)
}

func renderRuns(m Model, width, height int, now time.Time) string {
	runs := m.filteredRuns()
	selected := clampSelection(m.Selected, len(runs))
	rowCapacity := runRowCapacity(height, 2, m.InputMode, len(runs))
	start, end := viewport(selected, len(runs), rowCapacity)
	lines := []string{
		fit("  Workflow           Event             #     Duration  Age", width),
	}
	if m.InputMode {
		lines = append(lines, filterLine(m.Filter, len(runs), width))
	}
	if len(runs) == 0 {
		lines = append(lines, dimLine("  no runs match /"+m.Filter, width))
	}
	for i, run := range runs[start:end] {
		index := start + i
		lines = append(lines, row(run, index == selected, width, now))
	}
	summary := m.Summary()
	lines = append(lines, fitANSI(joinRightANSI(summaryLine(summary), pageLine(start, end, len(runs)), width), width))
	return strings.Join(lines, "\n")
}

func renderAllGreen(m Model, width, height int, now time.Time) string {
	summary := m.Summary()
	runs := m.filteredRuns()
	selected := clampSelection(m.Selected, len(runs))
	latestTitle := ""
	latestMeta := ""
	if len(runs) > 0 {
		run := runs[0]
		latestTitle = fmt.Sprintf("%s #%d", run.Name, run.RunNumber)
		latestMeta = fmt.Sprintf("%s · success", duration(run))
	}
	branch := first(m.Context.Branch, "all branches")
	rowCapacity := runRowCapacity(height, 5, m.InputMode, len(runs))
	start, end := viewport(selected, len(runs), rowCapacity)
	lines := []string{
		allGreenBandLine("", width),
		allGreenBandLine(joinRightANSI(successLead("All checks passing on "+branch), latestTitle, width), width),
		allGreenBandLine(joinRightANSI("     "+fmt.Sprintf("%d recent runs · %d failing · last finished %s ago", len(runs), summary.Failing, age(runs[0], now)), latestMeta, width), width),
		allGreenBandLine("", width),
		allGreenHeader(width),
	}
	if m.InputMode {
		lines = append(lines, filterLine(m.Filter, len(runs), width))
	}
	if len(runs) == 0 {
		lines = append(lines, dimLine("  no runs match /"+m.Filter, width))
	}
	for i, run := range runs[start:end] {
		lines = append(lines, allGreenRow(run, start+i == selected, width, now))
	}
	if len(runs) > 0 {
		lines = append(lines, dimLine(pageLine(start, end, len(runs)), width))
	}
	return strings.Join(lines, "\n")
}

func row(run model.Run, selected bool, width int, now time.Time) string {
	prefix := " "
	if selected {
		prefix = colorize(sgrOK, icons.Cursor)
	}
	status := colorize(statusColor(run), glyph(run))
	name := colorize(sgrFGSoft, truncate(run.Name, 16))
	event := colorize(eventColor(run), truncate(run.Event, 16))
	number := colorize(sgrMuted, fmt.Sprintf("#%d", run.RunNumber))
	duration := colorize(durationColor(run), sparkline.Render(sparkValues(run), 5))
	runAge := colorize(sgrSubtle, age(run, now))
	line := prefix + status + " " +
		padANSI(name, 16) + " " +
		padANSI(event, 16) + " " +
		padANSI(number, 5) + " " +
		padANSI(duration, 8) + " " +
		runAge
	return fitANSI(line, width)
}

func glyph(run model.Run) string {
	if run.Status == model.StatusCompleted {
		return icons.ForConclusion(run.Conclusion)
	}
	return icons.ForStatus(run.Status)
}

func age(run model.Run, now time.Time) string {
	base := run.UpdatedAt
	if base.IsZero() {
		base = run.RunStartedAt
	}
	if base.IsZero() || now.Before(base) {
		return "now"
	}
	duration := now.Sub(base)
	if duration < time.Second {
		return "now"
	}
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	return fmt.Sprintf("%dh", int(duration.Hours()))
}

func duration(run model.Run) string {
	if run.RunStartedAt.IsZero() || run.UpdatedAt.IsZero() || run.UpdatedAt.Before(run.RunStartedAt) {
		return "queued"
	}
	d := run.UpdatedAt.Sub(run.RunStartedAt)
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func fit(value string, width int) string {
	return fitANSI(value, width)
}

func summaryLine(summary Summary) string {
	return colorize(sgrFail, fmt.Sprintf("%d failing", summary.Failing)) +
		" · " +
		colorize(sgrRun, fmt.Sprintf("%d running", summary.Running)) +
		" · " +
		colorize(sgrSubtle, fmt.Sprintf("%d passed", summary.Passed))
}

func statusColor(run model.Run) string {
	if run.Status == model.StatusCompleted {
		switch run.Conclusion {
		case model.ConclusionSuccess:
			return sgrOK
		case model.ConclusionFailure:
			return sgrFail
		case model.ConclusionCancelled, model.ConclusionSkipped, model.ConclusionNeutral, model.ConclusionNone:
			return sgrNeutral
		case model.ConclusionActionRequired, model.ConclusionTimedOut:
			return sgrWarn
		default:
			return sgrNeutral
		}
	}
	switch run.Status {
	case model.StatusInProgress:
		return sgrRun
	case model.StatusQueued, model.StatusPending, model.StatusRequested, model.StatusWaiting:
		return sgrInfo
	default:
		return sgrNeutral
	}
}

func eventColor(run model.Run) string {
	if run.Status == model.StatusInProgress {
		return sgrRun
	}
	if run.Status != model.StatusCompleted {
		return sgrInfo
	}
	if run.Conclusion == model.ConclusionSuccess {
		return sgrOK
	}
	return sgrSubtle
}

func durationColor(run model.Run) string {
	if run.Status == model.StatusInProgress {
		return sgrRun
	}
	if run.Status != model.StatusCompleted {
		return sgrInfo
	}
	if run.Conclusion == model.ConclusionSuccess {
		return sgrOK
	}
	return sgrFGSoft
}

func sparkValues(run model.Run) []int {
	switch {
	case run.Status == model.StatusInProgress:
		return []int{40, 55, 80, 30, 20}
	case run.Status != model.StatusCompleted:
		return []int{15, 15}
	case run.Conclusion == model.ConclusionSuccess:
		return []int{30, 35, 50, 65, 50}
	case run.Conclusion == model.ConclusionCancelled:
		return []int{25, 25}
	default:
		return []int{35, 60, 100, 70, 45}
	}
}

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
}

func allGreenBandLine(value string, width int) string {
	value = fitANSI(value, width)
	value = padANSI(value, width)
	value = strings.ReplaceAll(value, sgrReset, sgrReset+sgrBandFG+sgrBandBG)
	return sgrBandFG + sgrBandBG + value + sgrReset
}

func successLead(title string) string {
	return "  " +
		sgrOK + sgrBold + icons.Success + sgrNoBold +
		sgrBandFG + "  " +
		sgrTitleFG + sgrBold + title + sgrNoBold +
		sgrBandFG
}

func allGreenHeader(width int) string {
	if width >= 70 {
		return dimLine("  Status  Workflow                                      #      Age", width)
	}
	return dimLine("  Status  Workflow                         #    Age", width)
}

func allGreenRow(run model.Run, selected bool, width int, now time.Time) string {
	prefix := "  "
	if selected {
		prefix = icons.Cursor + " "
	}
	icon := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4FD37A")).
		Render(glyph(run))
	name := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#CFCDBB")).
		Render(truncate(run.Name, 28))
	num := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7060")).
		Render(fmt.Sprintf("%d", run.RunNumber))
	runAge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8C9179")).
		Render(age(run, now))
	if width >= 70 {
		return fitANSI(joinRightANSI(prefix+icon+"       "+name, num+"      "+runAge, width), width)
	}
	return fitANSI(joinRightANSI(prefix+icon+"       "+name, num+"    "+runAge, width), width)
}

func filterLine(filter string, count int, width int) string {
	return dimLine(fmt.Sprintf("  /%s  %d matches", filter, count), width)
}

func runRowCapacity(height, fixedRows int, inputMode bool, total int) int {
	if height <= 0 {
		return total
	}
	capacity := height - fixedRows
	if inputMode {
		capacity--
	}
	if total > capacity {
		capacity--
	}
	return max(capacity, 1)
}

func viewport(selected, total, capacity int) (int, int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
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

func clampSelection(selected, total int) int {
	if total <= 0 || selected < 0 {
		return 0
	}
	if selected >= total {
		return total - 1
	}
	return selected
}

func pageLine(start, end, total int) string {
	if total == 0 {
		return "0/0"
	}
	return fmt.Sprintf("rows %d-%d/%d", start+1, end, total)
}

func dimLine(value string, width int) string {
	value = fitANSI(value, width)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8C9179")).
		Render(padANSI(value, width))
}

func joinRightANSI(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = fitANSI(right, width)
	leftWidth := max(width-ansi.StringWidth(right)-2, 1)
	left = fitANSI(left, leftWidth)
	spaces := max(width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", spaces) + right
}

const (
	sgrBold    = "\x1b[1m"
	sgrNoBold  = "\x1b[22m"
	sgrBandFG  = "\x1b[38;2;207;205;187m"
	sgrBandBG  = "\x1b[48;2;20;35;24m"
	sgrTitleFG = "\x1b[38;2;234;232;217m"
	sgrOK      = "\x1b[38;2;79;211;122m"
	sgrFail    = "\x1b[38;2;226;86;75m"
	sgrRun     = "\x1b[38;2;224;163;62m"
	sgrInfo    = "\x1b[38;2;110;156;181m"
	sgrWarn    = "\x1b[38;2;232;137;90m"
	sgrNeutral = "\x1b[38;2;107;112;96m"
	sgrMuted   = "\x1b[38;2;174;179;155m"
	sgrSubtle  = "\x1b[38;2;140;145;121m"
	sgrFGSoft  = "\x1b[38;2;207;205;187m"
	sgrReset   = "\x1b[0m"
)

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
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
