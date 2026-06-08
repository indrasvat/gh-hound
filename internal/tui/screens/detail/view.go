package detail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(breadcrumb(m), width)}
	if width < 100 {
		lines = append(lines, renderStepsPane(m, width)...)
	} else {
		lines = append(lines, renderWide(m, width)...)
	}
	return strings.Join(lines, "\n")
}

func renderWide(m Model, width int) []string {
	leftWidth := clampInt(width/3, 30, 38)
	rightWidth := width - leftWidth - 1
	jobs := renderJobsPane(m, leftWidth)
	steps := renderStepsPane(m, rightWidth)
	height := max(len(jobs), len(steps))
	lines := make([]string, 0, height)
	for i := range height {
		left, right := "", ""
		if i < len(jobs) {
			left = jobs[i]
		}
		if i < len(steps) {
			right = steps[i]
		}
		sep := colorize(sgrLine, "│")
		lines = append(lines, fitANSI(padANSI(left, leftWidth)+sep+right, width))
	}
	return lines
}

func renderJobsPane(m Model, width int) []string {
	lines := []string{
		paneHeader("Jobs", "", width, m.Focus == FocusJobs),
		divider(width),
	}
	for i, job := range m.Jobs {
		lines = append(lines, jobRow(job, i == m.SelectedJob, width))
	}
	return lines
}

func renderStepsPane(m Model, width int) []string {
	job := m.selectedJob()
	header := stepHeader(job)
	lines := []string{
		paneHeader(header, duration(job.StartedAt, job.CompletedAt), width, m.Focus == FocusSteps),
		divider(width),
	}
	for i, step := range job.Steps {
		lines = append(lines, stepRow(step, i == m.SelectedStep, width))
	}
	lines = append(lines, divider(width))
	lines = append(lines, hintLine(width))
	return lines
}

func paneHeader(left, right string, width int, focused bool) string {
	prefix := ""
	if focused {
		prefix = colorize(sgrOK, "●") + " "
	}
	if !strings.Contains(left, "\x1b[") {
		left = colorize(sgrDim, left)
	}
	left = prefix + left
	if right == "" {
		return fitANSI(left, width)
	}
	return joinRightANSI(left, colorize(sgrSubtle, right), width)
}

func stepHeader(job model.Job) string {
	name := first(job.Name, "job")
	status := first(string(job.Conclusion), string(job.Status))
	return colorize(sgrFGSoft, name) + " " + pill(status, job.Conclusion) + " " + chip(firstLabel(job))
}

func pill(label string, conclusion model.Conclusion) string {
	color := statusSGR(model.StatusCompleted, conclusion)
	if label == "" {
		label = "status"
	}
	return colorize(color, "["+label+"]")
}

func chip(label string) string {
	return colorize(sgrSubtle, "["+label+"]")
}

func jobRow(job model.Job, selected bool, width int) string {
	bar := " "
	if selected {
		bar = colorize(sgrOK, icons.Cursor)
	}
	leftWidth := max(width-12, 1)
	left := fitANSI(bar+" "+colorize(statusSGR(job.Status, job.Conclusion), jobGlyph(job))+" "+colorize(sgrFGSoft, job.Name), leftWidth)
	right := colorize(sgrDim, duration(job.StartedAt, job.CompletedAt))
	row := joinRightANSI(left, right, width)
	if selected {
		return backgroundSafe(row, width, sgrFG, sgrSurface2BG)
	}
	return row
}

func stepRow(step model.Step, selected bool, width int) string {
	bar := " "
	isFailure := step.Conclusion == model.ConclusionFailure || step.Conclusion == model.ConclusionActionRequired || step.Conclusion == model.ConclusionTimedOut
	if isFailure {
		bar = colorize(sgrFail, icons.Cursor)
	} else if selected {
		bar = colorize(sgrOK, icons.Cursor)
	}
	number := colorize(sgrDim, fmt.Sprintf("%d", step.Number))
	nameColor := sgrFGSoft
	if isFailure {
		nameColor = sgrFail
	}
	leftWidth := max(width-12, 1)
	left := fitANSI(bar+" "+colorize(statusSGR(step.Status, step.Conclusion), stepGlyph(step))+" "+padANSI(number, 2)+" "+colorize(nameColor, step.Name), leftWidth)
	right := colorize(durationSGR(step), duration(step.StartedAt, step.CompletedAt))
	row := joinRightANSI(left, right, width)
	switch {
	case isFailure:
		return backgroundSafe(row, width, sgrFG, sgrFailTintBG)
	case selected:
		return backgroundSafe(row, width, sgrFG, sgrSurface2BG)
	default:
		return row
	}
}

func hintLine(width int) string {
	return fitANSI("  "+colorize(sgrInfo, "n")+" jump to failure · "+colorize(sgrInfo, "l")+" full log · "+colorize(sgrInfo, "y")+" copy", width)
}

func divider(width int) string {
	return colorize(sgrLine, strings.Repeat("─", max(width, 1)))
}

func breadcrumb(m Model) string {
	sha := m.Run.HeadSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return fmt.Sprintf("indrasvat/gh-hound %s %s #%d %s %s · @%s", icons.Breadcrumb, m.Run.Name, m.Run.RunNumber, icons.Breadcrumb, first(m.Run.HeadBranch, "branch"), first(sha, "sha"))
}

func jobGlyph(job model.Job) string {
	if job.Status == model.StatusCompleted {
		return icons.ForConclusion(job.Conclusion)
	}
	return icons.ForStatus(job.Status)
}

func stepGlyph(step model.Step) string {
	if step.Status == model.StatusCompleted {
		return icons.ForConclusion(step.Conclusion)
	}
	return icons.ForStatus(step.Status)
}

func duration(start, end time.Time) string {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return "queued"
	}
	d := end.Sub(start)
	if d < time.Second {
		return fmt.Sprintf("%.1fs", float64(d.Milliseconds())/1000)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func statusSGR(status model.Status, conclusion model.Conclusion) string {
	if status == model.StatusCompleted {
		switch conclusion {
		case model.ConclusionSuccess:
			return sgrOK
		case model.ConclusionFailure:
			return sgrFail
		case model.ConclusionActionRequired, model.ConclusionTimedOut:
			return sgrWarn
		case model.ConclusionCancelled, model.ConclusionSkipped, model.ConclusionNeutral, model.ConclusionNone:
			return sgrNeutral
		default:
			return sgrNeutral
		}
	}
	switch status {
	case model.StatusInProgress:
		return sgrRun
	case model.StatusQueued, model.StatusPending, model.StatusRequested, model.StatusWaiting:
		return sgrInfo
	default:
		return sgrNeutral
	}
}

func durationSGR(step model.Step) string {
	if step.Status != model.StatusCompleted {
		return sgrInfo
	}
	switch step.Conclusion {
	case model.ConclusionSuccess:
		return sgrOK
	case model.ConclusionFailure:
		return sgrFail
	default:
		return sgrSubtle
	}
}

func firstLabel(job model.Job) string {
	if len(job.Labels) == 0 {
		return "runner"
	}
	return job.Labels[0]
}

func fit(value string, width int) string {
	return fitANSI(strings.TrimSpace(value), width)
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

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
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

func padANSI(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func backgroundSafe(value string, width int, fg string, bg string) string {
	value = padANSI(fitANSI(value, width), width)
	style := fg + bg
	value = strings.ReplaceAll(value, sgrReset, sgrReset+style)
	return style + value + sgrReset
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

const (
	sgrReset      = "\x1b[0m"
	sgrOK         = "\x1b[38;2;79;211;122m"
	sgrFail       = "\x1b[38;2;226;86;75m"
	sgrRun        = "\x1b[38;2;224;163;62m"
	sgrInfo       = "\x1b[38;2;110;156;181m"
	sgrWarn       = "\x1b[38;2;232;137;90m"
	sgrNeutral    = "\x1b[38;2;107;112;96m"
	sgrDim        = "\x1b[38;2;107;112;96m"
	sgrSubtle     = "\x1b[38;2;140;145;121m"
	sgrFG         = "\x1b[38;2;234;232;217m"
	sgrFGSoft     = "\x1b[38;2;207;205;187m"
	sgrLine       = "\x1b[38;2;46;50;39m"
	sgrSurface2BG = "\x1b[48;2;36;39;30m"
	sgrFailTintBG = "\x1b[48;2;40;19;18m"
)
