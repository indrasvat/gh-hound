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
	return ViewSize(m, width, 0)
}

// ViewSize renders the detail screen within a height budget. With many
// steps the steps list windows around the selection so the artifacts
// block and hint line are never clipped into invisible-but-interactive
// state; height <= 0 means unbudgeted.
func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	stepsBudget := 0
	if height > 0 {
		// breadcrumb + pane header + divider + trailing divider + hint.
		fixed := 5 + artifactsBlockLines(m)
		stepsBudget = max(height-fixed, 3)
	}
	lines := []string{fit(breadcrumb(m), width)}
	if width < 100 {
		lines = append(lines, renderStepsPane(m, width, stepsBudget, true)...)
	} else {
		lines = append(lines, renderWide(m, width, stepsBudget)...)
	}
	return strings.Join(lines, "\n")
}

func artifactsBlockLines(m Model) int {
	if len(m.Artifacts) == 0 {
		return 0
	}
	visible := min(len(m.Artifacts), artifactsWindow)
	lines := 2 + visible
	if len(m.Artifacts) > visible {
		lines++
	}
	return lines
}

func renderWide(m Model, width, stepsBudget int) []string {
	leftWidth := clampInt(width/3, 30, 38)
	rightWidth := width - leftWidth - 1
	jobs := renderJobsPane(m, leftWidth)
	steps := renderStepsPane(m, rightWidth, stepsBudget, false)
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
	if len(m.Jobs) == 0 {
		if m.Loading {
			line := m.LoadingLine
			if line == "" {
				line = colorize(sgrDim, "  fetching jobs…")
			}
			return append(lines, fitANSI(line, width))
		}
		return append(lines, fitANSI(colorize(sgrDim, "No jobs returned by GitHub"), width))
	}
	for i, job := range m.Jobs {
		lines = append(lines, jobRow(job, i == m.SelectedJob, width))
	}
	return lines
}

func renderStepsPane(m Model, width, stepsBudget int, standalone bool) []string {
	job := m.selectedJob()
	if len(m.Jobs) == 0 {
		hint := "Select a job after GitHub returns job data"
		if m.Loading {
			// In the stacked layout (width < 100) this pane is the whole
			// screen, so it must carry the shared loading line itself —
			// the jobs pane that normally hosts it is not rendered.
			if standalone && m.LoadingLine != "" {
				return []string{
					paneHeader("Steps", "", width, m.Focus == FocusSteps),
					divider(width),
					fitANSI(m.LoadingLine, width),
				}
			}
			hint = "the hound is on its way back…"
		}
		return []string{
			paneHeader("Steps", "", width, m.Focus == FocusSteps),
			divider(width),
			fitANSI(colorize(sgrDim, hint), width),
		}
	}
	header := stepHeader(job)
	lines := []string{
		paneHeader(header, duration(job.StartedAt, job.CompletedAt), width, m.Focus == FocusSteps),
		divider(width),
	}
	start, end := stepWindow(len(job.Steps), m.SelectedStep, stepsBudget)
	if start > 0 {
		lines = append(lines, fitANSI("    "+colorize(sgrDim, fmt.Sprintf("+%d more above", start)), width))
	}
	for i := start; i < end; i++ {
		lines = append(lines, stepRow(job.Steps[i], i == m.SelectedStep, width))
	}
	if remaining := len(job.Steps) - end; remaining > 0 {
		lines = append(lines, fitANSI("    "+colorize(sgrDim, fmt.Sprintf("+%d more", remaining)), width))
	}
	lines = append(lines, renderArtifactsBlock(m, width)...)
	lines = append(lines, divider(width))
	lines = append(lines, hintLine(m, width))
	return lines
}

// stepWindow returns the visible [start, end) slice of steps for the
// given budget, keeping the selection inside the window. Overflow
// indicator lines are paid for out of the same budget.
func stepWindow(total, selected, budget int) (int, int) {
	if budget <= 0 || total <= budget {
		return 0, total
	}
	visible := max(budget-2, 1)
	start := selected - visible/2
	start = max(start, 0)
	start = min(start, total-visible)
	return start, start + visible
}

const artifactsWindow = 5

func renderArtifactsBlock(m Model, width int) []string {
	if len(m.Artifacts) == 0 {
		return nil
	}
	lines := []string{
		divider(width),
		paneHeader(fmt.Sprintf("Artifacts (%d)", len(m.Artifacts)), "", width, m.Focus == FocusArtifacts),
	}
	start := 0
	if m.SelectedArtifact >= artifactsWindow {
		start = m.SelectedArtifact - artifactsWindow + 1
	}
	end := min(start+artifactsWindow, len(m.Artifacts))
	for i := start; i < end; i++ {
		lines = append(lines, artifactRow(m.Artifacts[i], i == m.SelectedArtifact && m.Focus == FocusArtifacts, width))
	}
	if remaining := len(m.Artifacts) - end; remaining > 0 {
		lines = append(lines, fitANSI("    "+colorize(sgrDim, fmt.Sprintf("+%d more", remaining)), width))
	}
	return lines
}

func artifactRow(artifact model.Artifact, selected bool, width int) string {
	bar := " "
	if selected {
		bar = colorize(sgrOK, icons.Cursor)
	}
	nameColor := sgrFGSoft
	iconColor := sgrInfo
	if artifact.Expired {
		nameColor = sgrDim
		iconColor = sgrDim
	}
	leftWidth := max(width-22, 1)
	left := fitANSI(bar+" "+colorize(iconColor, icons.Artifact)+" "+colorize(nameColor, artifact.Name), leftWidth)
	right := colorize(sgrDim, humanSize(artifact.SizeInBytes))
	if artifact.Expired {
		right = colorize(sgrWarn, "[expired]") + " " + right
	}
	row := joinRightANSI(left, right, width)
	if selected {
		return backgroundSafe(row, width, sgrFG, sgrSurface2BG)
	}
	return row
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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
	name := jobTitle(job)
	status := first(string(job.Conclusion), string(job.Status))
	parts := []string{}
	if name != "" {
		parts = append(parts, colorize(sgrFGSoft, name))
	}
	if status != "" {
		parts = append(parts, pill(status, job.Conclusion))
	}
	if label := firstLabel(job); label != "" {
		parts = append(parts, chip(label))
	}
	return strings.Join(parts, " ")
}

func pill(label string, conclusion model.Conclusion) string {
	color := statusSGR(model.StatusCompleted, conclusion)
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
	left := fitANSI(bar+" "+colorize(statusSGR(job.Status, job.Conclusion), jobGlyph(job))+" "+colorize(sgrFGSoft, jobTitle(job)), leftWidth)
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

func hintLine(m Model, width int) string {
	hint := "  " + colorize(sgrInfo, "n") + " jump to failure · " + colorize(sgrInfo, "l") + " full log"
	if len(m.Artifacts) > 0 {
		hint += " · " + colorize(sgrInfo, "a") + " artifacts · " + colorize(sgrInfo, "d") + " download"
	} else {
		hint += " · " + colorize(sgrInfo, "y") + " copy"
	}
	return fitANSI(hint, width)
}

func divider(width int) string {
	return colorize(sgrLine, strings.Repeat("─", max(width, 1)))
}

func breadcrumb(m Model) string {
	parts := []string{}
	if repo := strings.TrimSpace(m.Repo); repo != "" {
		parts = append(parts, repo)
	}
	if title := runTitle(m.Run); title != "" {
		parts = append(parts, title)
	}
	if branch := strings.TrimSpace(m.Run.HeadBranch); branch != "" {
		parts = append(parts, branch)
	}
	line := strings.Join(parts, " "+icons.Breadcrumb+" ")
	if sha := shortSHA(m.Run.HeadSHA); sha != "" {
		if line != "" {
			line += " · "
		}
		line += "@" + sha
	}
	return line
}

func runTitle(run model.Run) string {
	name := first(strings.TrimSpace(run.Name), strings.TrimSpace(run.DisplayTitle), strings.TrimSpace(run.Path))
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

func jobTitle(job model.Job) string {
	if name := strings.TrimSpace(job.Name); name != "" {
		return name
	}
	if job.ID > 0 {
		return fmt.Sprintf("job %d", job.ID)
	}
	return ""
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
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
		return ""
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
