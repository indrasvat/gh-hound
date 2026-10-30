package detail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
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
	head := headLines(m, width)
	if height > 0 {
		// breadcrumb row(s) + pane header + divider + trailing divider + hint.
		fixed := len(head) + 4 + artifactsBlockLines(m) + gateBlockLines(m)
		stepsBudget = max(height-fixed, 3)
	}
	st := stylesFor(m.Theme.OrBramble())
	lines := head
	if width < 100 {
		lines = append(lines, renderStepsPane(m, st, width, stepsBudget, true)...)
	} else {
		lines = append(lines, renderWide(m, st, width, stepsBudget)...)
	}
	return strings.Join(lines, "\n")
}

// artifactsBlockLines mirrors renderArtifactsBlock's window math
// exactly — the height budget and the rendered block must never
// disagree, in any selection or download state.
func artifactsBlockLines(m Model) int {
	if len(m.Artifacts) == 0 {
		return 0
	}
	start := 0
	if m.SelectedArtifact >= artifactsWindow {
		start = m.SelectedArtifact - artifactsWindow + 1
	}
	end := min(start+artifactsWindow, len(m.Artifacts))
	lines := 2 + (end - start)
	if len(m.Artifacts)-end > 0 {
		lines++
	}
	// The selected downloaded artifact carries a path subline.
	if _, ok := m.selectedDownloadedPath(); ok {
		lines++
	}
	return lines
}

func renderWide(m Model, st styles, width, stepsBudget int) []string {
	leftWidth := clampInt(width/3, 30, 38)
	rightWidth := width - leftWidth - 1
	jobs := renderJobsPane(m, st, leftWidth)
	steps := renderStepsPane(m, st, rightWidth, stepsBudget, false)
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
		sep := colorize(st.line, "│")
		lines = append(lines, fitANSI(padANSI(left, leftWidth)+sep+right, width))
	}
	return lines
}

func renderJobsPane(m Model, st styles, width int) []string {
	lines := []string{
		paneHeader(st, "Jobs", "", width, m.Focus == FocusJobs),
		divider(st, width),
	}
	if len(m.Jobs) == 0 {
		if m.Loading {
			line := m.LoadingLine
			if line == "" {
				line = colorize(st.dim, "  fetching jobs…")
			}
			return append(lines, fitANSI(line, width))
		}
		return append(lines, fitANSI(colorize(st.dim, "No jobs returned by GitHub"), width))
	}
	for i, job := range m.Jobs {
		lines = append(lines, jobRow(st, job, i == m.SelectedJob, width, m.clock()))
	}
	return lines
}

func renderStepsPane(m Model, st styles, width, stepsBudget int, standalone bool) []string {
	job := m.selectedJob()
	if len(m.Jobs) == 0 {
		hint := "Select a job after GitHub returns job data"
		if m.Loading {
			// In the stacked layout (width < 100) this pane is the whole
			// screen, so it must carry the shared loading line itself —
			// the jobs pane that normally hosts it is not rendered.
			if standalone && m.LoadingLine != "" {
				return []string{
					paneHeader(st, "Steps", "", width, m.Focus == FocusSteps),
					divider(st, width),
					fitANSI(m.LoadingLine, width),
				}
			}
			hint = "the hound is on its way back…"
		}
		return append([]string{
			paneHeader(st, "Steps", "", width, m.Focus == FocusSteps),
			divider(st, width),
			fitANSI(colorize(st.dim, hint), width),
		}, renderGateBlock(m, st, width)...)
	}
	header := stepHeader(st, job)
	lines := []string{
		paneHeader(st, header, duration(job.Status, job.StartedAt, job.CompletedAt, m.clock()), width, m.Focus == FocusSteps),
		divider(st, width),
	}
	start, end := stepWindow(len(job.Steps), m.SelectedStep, stepsBudget)
	if start > 0 {
		lines = append(lines, fitANSI("    "+colorize(st.dim, fmt.Sprintf("+%d more above", start)), width))
	}
	for i := start; i < end; i++ {
		lines = append(lines, stepRow(st, job.Steps[i], i == m.SelectedStep, width, m.clock()))
	}
	if remaining := len(job.Steps) - end; remaining > 0 {
		lines = append(lines, fitANSI("    "+colorize(st.dim, fmt.Sprintf("+%d more", remaining)), width))
	}
	if len(job.Steps) == 0 && job.Status != model.StatusCompleted {
		// GitHub lists a job's steps once a runner picks it up.
		lines = append(lines, fitANSI("  "+colorize(st.dim, "waiting for a runner · steps appear once it starts"), width))
	}
	lines = append(lines, renderGateBlock(m, st, width)...)
	lines = append(lines, renderArtifactsBlock(m, st, width)...)
	lines = append(lines, divider(st, width))
	lines = append(lines, hintLine(m, st, width))
	return lines
}

func gateBlockLines(m Model) int {
	if !showGateBlock(m) {
		return 0
	}
	return 2 + len(m.PendingDeployments)
}

func showGateBlock(m Model) bool {
	return m.Run.Status == model.StatusWaiting && len(m.PendingDeployments) > 0
}

// renderGateBlock is the pending-environments panel for a waiting run:
// each gate, its reviewers, the wait timer, and whether this user can
// open it.
func renderGateBlock(m Model, st styles, width int) []string {
	if !showGateBlock(m) {
		return nil
	}
	lines := []string{
		divider(st, width),
		paneHeader(st, fmt.Sprintf("Deploy gate (%d)", len(m.PendingDeployments)), "A review", width, false),
	}
	for _, gate := range m.PendingDeployments {
		lines = append(lines, gateRow(st, gate, width))
	}
	return lines
}

func gateRow(st styles, gate model.PendingDeployment, width int) string {
	state := colorize(st.ok, "you can open this gate")
	iconColor := st.info
	if !gate.CurrentUserCanApprove {
		state = colorize(st.warn, "not yours to open")
		iconColor = st.dim
	}
	left := "  " + colorize(iconColor, icons.Gate) + " " + colorize(st.fgSoft, gate.EnvironmentName) + " " + state
	if gate.WaitTimer > 0 {
		left += colorize(st.dim, fmt.Sprintf(" · wait %dm", gate.WaitTimer/60))
	}
	reviewers := make([]string, 0, len(gate.Reviewers))
	for _, reviewer := range gate.Reviewers {
		reviewers = append(reviewers, reviewer.Name)
	}
	right := colorize(st.dim, strings.Join(reviewers, ", "))
	return joinRightANSI(left, right, width)
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

func renderArtifactsBlock(m Model, st styles, width int) []string {
	if len(m.Artifacts) == 0 {
		return nil
	}
	header := fmt.Sprintf("Artifacts (%d)", len(m.Artifacts))
	if done := m.DownloadedCount(); done > 0 {
		header = fmt.Sprintf("Artifacts (%d · %d %s)", len(m.Artifacts), done, icons.Success)
	}
	lines := []string{
		divider(st, width),
		paneHeader(st, header, "", width, m.Focus == FocusArtifacts),
	}
	start := 0
	if m.SelectedArtifact >= artifactsWindow {
		start = m.SelectedArtifact - artifactsWindow + 1
	}
	end := min(start+artifactsWindow, len(m.Artifacts))
	for i := start; i < end; i++ {
		artifact := m.Artifacts[i]
		selected := i == m.SelectedArtifact && m.Focus == FocusArtifacts
		lines = append(lines, artifactRow(st, artifact, m.Download(artifact.ID), selected, m.SpinnerFrame, width))
		if selected {
			if subline, ok := artifactPathSubline(st, m.Download(artifact.ID), width); ok {
				lines = append(lines, subline)
			}
		}
	}
	if remaining := len(m.Artifacts) - end; remaining > 0 {
		lines = append(lines, fitANSI("    "+colorize(st.dim, fmt.Sprintf("+%d more", remaining)), width))
	}
	return lines
}

// artifactRow annotates the right edge with the download lifecycle:
// live byte count while transferring, extraction marker, done/failed
// verdicts — and the plain size when nothing ever started.
func artifactRow(st styles, artifact model.Artifact, status DownloadStatus, selected bool, frame int, width int) string {
	bar := " "
	if selected {
		bar = colorize(st.ok, icons.Cursor)
	}
	nameColor := st.fgSoft
	iconColor := st.info
	if artifact.Expired {
		nameColor = st.dim
		iconColor = st.dim
	}
	leftWidth := max(width-22, 1)
	left := fitANSI(bar+" "+colorize(iconColor, icons.Artifact)+" "+colorize(nameColor, artifact.Name), leftWidth)
	var right string
	switch status.State {
	case DownloadStateDownloading:
		right = colorize(st.info, spinnerGlyph(frame)+" ↓ "+humanSize(status.Bytes)+"…")
	case DownloadStateExtracting:
		right = colorize(st.info, spinnerGlyph(frame)+" extracting…")
	case DownloadStateDone:
		files := "files"
		if status.FileCount == 1 {
			files = "file"
		}
		right = colorize(st.ok, icons.Success) + " " + colorize(st.dim, fmt.Sprintf("%d %s", status.FileCount, files))
	case DownloadStateFailed:
		right = colorize(st.fail, icons.Failure+" failed")
	default:
		right = colorize(st.dim, humanSize(artifact.SizeInBytes))
	}
	if artifact.Expired {
		right = colorize(st.warn, "[expired]") + " " + colorize(st.dim, humanSize(artifact.SizeInBytes))
	}
	row := joinRightANSI(left, right, width)
	if selected {
		return backgroundSafe(row, width, st.fg, st.surface2BG)
	}
	return row
}

// artifactPathSubline shows where the selected artifact landed and the
// keys that act on it — the discoverability anchor for o/y.
func artifactPathSubline(st styles, status DownloadStatus, width int) (string, bool) {
	if status.State != DownloadStateDone || status.Path == "" {
		return "", false
	}
	line := "    " + colorize(st.dim, "↳ "+status.Path) + "  " +
		colorize(st.info, "o") + colorize(st.dim, " open · ") +
		colorize(st.info, "y") + colorize(st.dim, " copy path")
	return fitANSI(line, width), true
}

func spinnerGlyph(frame int) string {
	frames := icons.SpinnerFrames
	index := frame % len(frames)
	if index < 0 {
		index += len(frames)
	}
	return frames[index]
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

func paneHeader(st styles, left, right string, width int, focused bool) string {
	prefix := ""
	if focused {
		prefix = colorize(st.ok, "●") + " "
	}
	if !strings.Contains(left, "\x1b[") {
		left = colorize(st.dim, left)
	}
	left = prefix + left
	if right == "" {
		return fitANSI(left, width)
	}
	return joinRightANSI(left, colorize(st.subtle, right), width)
}

func stepHeader(st styles, job model.Job) string {
	name := jobTitle(job)
	status := first(string(job.Conclusion), string(job.Status))
	parts := []string{}
	if name != "" {
		parts = append(parts, colorize(st.fgSoft, name))
	}
	if status != "" {
		parts = append(parts, pill(st, status, job.Conclusion))
	}
	if label := firstLabel(job); label != "" {
		parts = append(parts, chip(st, label))
	}
	return strings.Join(parts, " ")
}

func pill(st styles, label string, conclusion model.Conclusion) string {
	color := statusSGR(st, model.StatusCompleted, conclusion)
	return colorize(color, "["+label+"]")
}

func chip(st styles, label string) string {
	return colorize(st.subtle, "["+label+"]")
}

func jobRow(st styles, job model.Job, selected bool, width int, now time.Time) string {
	bar := " "
	if selected {
		bar = colorize(st.ok, icons.Cursor)
	}
	leftWidth := max(width-12, 1)
	left := fitANSI(bar+" "+colorize(statusSGR(st, job.Status, job.Conclusion), jobGlyph(job))+" "+colorize(st.fgSoft, jobTitle(job)), leftWidth)
	right := colorize(st.dim, duration(job.Status, job.StartedAt, job.CompletedAt, now))
	row := joinRightANSI(left, right, width)
	if selected {
		return backgroundSafe(row, width, st.fg, st.surface2BG)
	}
	return row
}

func stepRow(st styles, step model.Step, selected bool, width int, now time.Time) string {
	bar := " "
	isFailure := step.Conclusion == model.ConclusionFailure || step.Conclusion == model.ConclusionActionRequired || step.Conclusion == model.ConclusionTimedOut
	if isFailure {
		bar = colorize(st.fail, icons.Cursor)
	} else if selected {
		bar = colorize(st.ok, icons.Cursor)
	}
	number := colorize(st.dim, fmt.Sprintf("%d", step.Number))
	nameColor := st.fgSoft
	if isFailure {
		nameColor = st.fail
	}
	leftWidth := max(width-12, 1)
	left := fitANSI(bar+" "+colorize(statusSGR(st, step.Status, step.Conclusion), stepGlyph(step))+" "+padANSI(number, 2)+" "+colorize(nameColor, step.Name), leftWidth)
	right := colorize(durationSGR(st, step), duration(step.Status, step.StartedAt, step.CompletedAt, now))
	row := joinRightANSI(left, right, width)
	switch {
	case isFailure:
		return backgroundSafe(row, width, st.fg, st.failTintBG)
	case selected:
		return backgroundSafe(row, width, st.fg, st.surface2BG)
	default:
		return row
	}
}

func hintLine(m Model, st styles, width int) string {
	// A downloaded selection owns the hint line: o/y change meaning
	// there and the footer must teach it.
	if _, ok := m.selectedDownloadedPath(); ok {
		hint := "  " + colorize(st.info, "o") + " open folder · " +
			colorize(st.info, "y") + " copy path · " +
			colorize(st.info, "d") + " re-download"
		return fitANSI(hint, width)
	}
	hint := "  " + colorize(st.info, "n") + " jump to failure · " + colorize(st.info, "l") + " full log"
	if len(m.Artifacts) > 0 {
		hint += " · " + colorize(st.info, "a") + " artifacts · " + colorize(st.info, "d") + " download"
	} else {
		hint += " · " + colorize(st.info, "y") + " copy"
	}
	return fitANSI(hint, width)
}

func divider(st styles, width int) string {
	return colorize(st.line, strings.Repeat("─", max(width, 1)))
}

// headLines is the breadcrumb with the run's attribution right-aligned,
// or on its own row when both do not fit — logins are never truncated.
func headLines(m Model, width int) []string {
	crumb, attribution := breadcrumb(m), Attribution(m.Run)
	if ansi.StringWidth(crumb)+2+ansi.StringWidth(attribution) <= width {
		return []string{joinRightANSI(crumb, attribution, width)}
	}
	lines := []string{fit(crumb, width)}
	current := ""
	for part := range strings.SplitSeq(attribution, " · ") {
		if current != "" && ansi.StringWidth(current)+3+ansi.StringWidth(part) > width {
			lines = append(lines, fit(current, width))
			current = part
			continue
		}
		if current != "" {
			current += " · "
		}
		current += part
	}
	return append(lines, fit(current, width))
}

// Attribution names who started the run and, after a re-run by someone
// else, who started the latest attempt. A missing actor stays missing.
func Attribution(run model.Run) string {
	actor := strings.TrimSpace(run.Actor)
	line := "actor unavailable"
	if actor != "" {
		line = "Started by @" + actor
	}
	if run.RunAttempt > 1 {
		line += fmt.Sprintf(" · Attempt %d", run.RunAttempt)
	}
	if trigger := strings.TrimSpace(run.TriggeringActor); trigger != "" && !strings.EqualFold(trigger, actor) {
		verb := "Triggered by"
		if run.RunAttempt > 1 {
			verb = "Re-run by"
		}
		line += " · " + verb + " @" + trigger
	}
	return line
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

// duration reads a finished job or step's run time, a running one's
// time so far, and "queued"/"waiting" for one not yet running — GitHub
// stamps started_at on a queued job too, so the status decides, not
// the timestamp. A finished one GitHub never timed (skipped) reads "—".
func duration(status model.Status, start, end, now time.Time) string {
	switch status {
	case model.StatusCompleted:
		if start.IsZero() || end.IsZero() || end.Before(start) {
			return "—"
		}
		return formatDuration(end.Sub(start))
	case model.StatusInProgress:
		if start.IsZero() {
			return "running"
		}
		return elapsed(max(now.Sub(start), 0))
	case model.StatusWaiting:
		return "waiting"
	default:
		return "queued"
	}
}

// elapsed reads a running clock in whole seconds (`41s`, `12m40s`),
// as the watch does.
func elapsed(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return formatDuration(d)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", float64(d.Milliseconds())/1000)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func statusSGR(st styles, status model.Status, conclusion model.Conclusion) string {
	if status == model.StatusCompleted {
		switch conclusion {
		case model.ConclusionSuccess:
			return st.ok
		case model.ConclusionFailure:
			return st.fail
		case model.ConclusionActionRequired, model.ConclusionTimedOut:
			return st.warn
		case model.ConclusionCancelled, model.ConclusionSkipped, model.ConclusionNeutral, model.ConclusionNone:
			return st.neutral
		default:
			return st.neutral
		}
	}
	switch status {
	case model.StatusInProgress:
		return st.run
	case model.StatusQueued, model.StatusPending, model.StatusRequested, model.StatusWaiting:
		return st.info
	default:
		return st.neutral
	}
}

func durationSGR(st styles, step model.Step) string {
	if step.Status != model.StatusCompleted {
		return st.info
	}
	switch step.Conclusion {
	case model.ConclusionSuccess:
		return st.ok
	case model.ConclusionFailure:
		return st.fail
	default:
		return st.subtle
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

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the detail screen.
type styles struct {
	ok, fail, run, info, warn, neutral string
	dim, subtle, fg, fgSoft, line      string
	surface2BG, failTintBG             string
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), run: theme.Fg(th.Run),
		info: theme.Fg(th.Info), warn: theme.Fg(th.Warn), neutral: theme.Fg(th.Neutral),
		dim: theme.Fg(th.Dim), subtle: theme.Fg(th.Subtle), fg: theme.Fg(th.FG),
		fgSoft: theme.Fg(th.FGSoft), line: theme.Fg(th.Line),
		surface2BG: theme.Bg(th.Surface2), failTintBG: theme.Bg(th.FailTint),
	}
}

const sgrReset = "\x1b[0m"
