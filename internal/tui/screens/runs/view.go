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
	"github.com/indrasvat/gh-hound/internal/usecase"
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
	fixedRows := 2
	notice := visibleNotice(m)
	if notice != "" {
		fixedRows++
	}
	showFilter := m.InputMode || strings.TrimSpace(m.Filter) != ""
	rowCapacity := runRowCapacity(height, fixedRows, showFilter, len(runs))
	start, end := viewport(selected, len(runs), rowCapacity)
	lines := []string{
		runsHeader(width),
	}
	if showFilter {
		lines = append(lines, filterLine(m.Filter, len(runs), width))
	}
	if len(runs) == 0 {
		lines = append(lines, dimLine("  no runs match /"+m.Filter, width))
	}
	for i, run := range runs[start:end] {
		index := start + i
		lines = append(lines, row(run, index == selected, width, now))
	}
	if notice != "" {
		lines = append(lines, dimLine("  "+notice, width))
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
		latestTitle = plainRunTitle(run)
		latestMeta = fmt.Sprintf("%s · success", duration(run))
	}
	branch := scopeTitle(m.Context.Scope, m.Context.Branch)
	fixedRows := 5
	notice := visibleNotice(m)
	if notice != "" {
		fixedRows++
	}
	showFilter := m.InputMode || strings.TrimSpace(m.Filter) != ""
	rowCapacity := runRowCapacity(height, fixedRows, showFilter, len(runs))
	start, end := viewport(selected, len(runs), rowCapacity)
	lines := []string{
		allGreenBandLine("", width),
		allGreenBandLine(joinRightANSI(successLead(nonFailingHeadline(runs, branch)), latestTitle, width), width),
		allGreenBandLine(joinRightANSI("     "+fmt.Sprintf("%d recent runs · %d failing · last finished %s ago", len(runs), summary.Failing, age(runs[0], now)), latestMeta, width), width),
		allGreenBandLine("", width),
		allGreenHeader(width),
	}
	if notice != "" {
		lines = append(lines, dimLine("  "+notice, width))
	}
	if showFilter {
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
	number := colorize(sgrMuted, fmt.Sprintf("#%d", run.RunNumber))
	status := colorize(statusColor(run), glyph(run))
	label := runLabel(run, 30)
	event := colorize(eventColor(run), truncate(run.Event, 16))
	duration := colorize(durationColor(run), sparkline.Render(sparkValues(run), 5))
	runAge := colorize(sgrSubtle, age(run, now))
	line := prefix +
		padANSI(number, 6) + " " +
		status + " " +
		padANSI(label, 30) + " " +
		padANSI(event, 16) + " " +
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

func runsHeader(width int) string {
	if width >= 92 {
		return dimLine("  #      Status  Workflow / detail              Event             Duration  Age", width)
	}
	return dimLine("  #      St  Workflow / detail              Event             Dur.  Age", width)
}

func runLabel(run model.Run, width int) string {
	name := runName(run)
	detail := runDetail(run)
	if detail == "" {
		return colorize(sgrFGSoft, truncate(name, width))
	}
	nameWidth := max(min(ansi.StringWidth(name), width/2), 6)
	if ansi.StringWidth(name)+3+ansi.StringWidth(detail) <= width {
		nameWidth = ansi.StringWidth(name)
	}
	detailWidth := max(width-nameWidth-3, 1)
	return colorize(sgrFGSoft, truncate(name, nameWidth)) +
		colorize(sgrSubtle, " · "+truncate(detail, detailWidth))
}

func runName(run model.Run) string {
	name := first(strings.TrimSpace(run.Name), strings.TrimSpace(run.Path), strings.TrimSpace(run.DisplayTitle))
	if name != "" {
		return name
	}
	switch {
	case run.RunNumber > 0:
		return fmt.Sprintf("#%d", run.RunNumber)
	case run.ID > 0:
		return fmt.Sprintf("run %d", run.ID)
	default:
		return ""
	}
}

func runDetail(run model.Run) string {
	for _, value := range []string{run.DisplayTitle, run.HeadBranch, run.Event} {
		value = strings.TrimSpace(value)
		if value != "" && !strings.EqualFold(value, strings.TrimSpace(run.Name)) {
			return value
		}
	}
	return ""
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
		return dimLine("  #      Status  Workflow / detail                                  Age", width)
	}
	return dimLine("  #      St  Workflow / detail                         Age", width)
}

func allGreenRow(run model.Run, selected bool, width int, now time.Time) string {
	prefix := "  "
	if selected {
		prefix = icons.Cursor + " "
	}
	icon := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4FD37A")).
		Render(glyph(run))
	num := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7060")).
		Render(fmt.Sprintf("#%d", run.RunNumber))
	label := runLabel(run, 36)
	runAge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8C9179")).
		Render(age(run, now))
	if width >= 70 {
		return fitANSI(joinRightANSI(prefix+padANSI(num, 6)+" "+icon+"       "+label, runAge, width), width)
	}
	return fitANSI(joinRightANSI(prefix+padANSI(num, 6)+" "+icon+" "+label, runAge, width), width)
}

func scopeTitle(scope usecase.LaunchScope, branch string) string {
	if scope == usecase.LaunchScopeRepo || strings.TrimSpace(branch) == "" {
		return "repo all branches"
	}
	return "branch " + strings.TrimSpace(branch)
}

func plainRunTitle(run model.Run) string {
	name := first(strings.TrimSpace(run.Name), strings.TrimSpace(run.Path), strings.TrimSpace(run.DisplayTitle))
	switch {
	case name != "" && run.RunNumber > 0:
		return fmt.Sprintf("%s #%d", name, run.RunNumber)
	case name != "":
		return name
	case run.RunNumber > 0:
		return fmt.Sprintf("#%d", run.RunNumber)
	case run.ID > 0:
		return fmt.Sprintf("run %d", run.ID)
	default:
		return ""
	}
}

func visibleNotice(m Model) string {
	if m.Context.Scope == usecase.LaunchScopeRepo {
		return ""
	}
	return m.Context.Notice
}

func nonFailingHeadline(runs []model.Run, scope string) string {
	for _, run := range runs {
		if run.Status != model.StatusCompleted || run.Conclusion != model.ConclusionSuccess {
			return "No failing checks on " + scope
		}
	}
	return "All checks passing on " + scope
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
