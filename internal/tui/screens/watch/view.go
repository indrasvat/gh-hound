package watch

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(header(m), width)}
	notes := noLogNotes(m.State.NoLog)
	for _, note := range notes {
		lines = append(lines, fitANSI(colorize(sgrDim, note), width))
	}
	live := liveJobs(m.State.Jobs)
	if !m.Finished() && len(live) > 0 {
		now := m.State.Now
		if now.IsZero() {
			now = time.Now()
		}
		shown := live
		if len(live) > maxProgressLines {
			// The block is bounded so the log tail below keeps its rows:
			// a 50-shard matrix reads as two jobs and a count.
			shown = live[:maxProgressLines-1]
		}
		for _, job := range shown {
			lines = append(lines, jobProgress(job, now, width))
		}
		if more := len(live) - len(shown); more > 0 {
			lines = append(lines, fitANSI(colorize(sgrDim, fmt.Sprintf("+%d more jobs not finished", more)), width))
		}
		lines = append(lines, fitANSI(colorize(sgrDim, "GitHub serves a job's log once it finishes · step progress until then"), width))
	}
	if len(m.State.Lines) == 0 {
		switch {
		case !m.Finished() && len(live) > 0:
		case !m.Finished():
			lines = append(lines, fitANSI(colorize(sgrDim, "waiting for completed job logs from GitHub Actions"), width))
		case len(notes) == 0:
			lines = append(lines, fitANSI(colorize(sgrDim, "no job logs to show"), width))
		}
		return strings.Join(lines, "\n")
	}
	lines = append(lines, foldLine(width))
	start := 0
	if m.Follow && len(m.State.Lines) > 6 {
		start = len(m.State.Lines) - 6
	}
	visible := m.State.Lines[start:]
	for index, line := range visible {
		lines = append(lines, logLine(line, width, !m.Finished() && index == len(visible)-1))
	}
	return strings.Join(lines, "\n")
}

func header(m Model) string {
	follow := "follow ○"
	if m.Follow {
		follow = "follow ●"
	}
	elapsed := m.State.Elapsed
	if elapsed == "" {
		elapsed = "0m00s"
	}
	debug := ""
	if m.Debug {
		debug = " · debug"
	}
	state := "streaming " + icons.InProgress
	if m.Finished() {
		state = icons.ForConclusion(m.State.Run.Conclusion) + " " + m.Outcome()
	}
	return fmt.Sprintf("watch · %s #%d · %s %s %s %s%s",
		m.State.Run.Name,
		m.State.Run.RunNumber,
		m.State.Branch,
		state,
		elapsed,
		follow,
		debug,
	)
}

// maxProgressLines bounds the step-progress block.
const maxProgressLines = 3

// liveJobs are the jobs not yet finished, running ones first, each
// group in job order.
func liveJobs(jobs []model.Job) []model.Job {
	var running, waiting []model.Job
	for _, job := range jobs {
		switch job.Status {
		case model.StatusCompleted:
		case model.StatusInProgress:
			running = append(running, job)
		default:
			waiting = append(waiting, job)
		}
	}
	return append(running, waiting...)
}

// jobProgress reads "⠿ bdd › 29 Run tests 12m40s": the job, the step it
// is on, and how long that step has run. A job not yet started reads
// "○ deploy queued".
func jobProgress(job model.Job, now time.Time, width int) string {
	name := oneLine(job.Name)
	if name == "" {
		name = "job"
	}
	glyph, color := icons.ForStatus(job.Status), sgrDim
	if job.Status == model.StatusInProgress {
		color = sgrRun
	}
	value := colorize(color, glyph) + " " + colorize(sgrFGSoft, name)
	step, ok := currentStep(job)
	switch {
	case ok:
		value += colorize(sgrDim, " › ") + colorize(sgrDim, fmt.Sprintf("%d", step.Number)) + " " + colorize(sgrFG, oneLine(step.Name))
		if !step.StartedAt.IsZero() {
			value += " " + colorize(sgrInfo, elapsedSince(step.StartedAt, now))
		}
	case job.Status == model.StatusInProgress:
		if !job.StartedAt.IsZero() {
			value += " " + colorize(sgrInfo, elapsedSince(job.StartedAt, now))
		}
	default:
		value += " " + colorize(sgrDim, string(job.Status))
	}
	return fitANSI(value, width)
}

// currentStep is the step a running job is on: the one in progress,
// or failing that the next one not yet finished.
func currentStep(job model.Job) (model.Step, bool) {
	for _, step := range job.Steps {
		if step.Status == model.StatusInProgress {
			return step, true
		}
	}
	if job.Status != model.StatusInProgress {
		return model.Step{}, false
	}
	for _, step := range job.Steps {
		if step.Status != model.StatusCompleted {
			return step, true
		}
	}
	return model.Step{}, false
}

// oneLine folds a GitHub name onto one line: newlines and tabs would
// break the row or its measured width.
func oneLine(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

func elapsedSince(start, now time.Time) string {
	d := max(now.Sub(start), 0).Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// noLogNotes reads "log expired · build, test" and "log not available
// yet · deploy": the status leads, so a cut line keeps it.
func noLogNotes(missing []NoLog) []string {
	var expired, notYet []string
	for _, job := range missing {
		name := strings.Join(strings.Fields(job.Job), " ")
		if name == "" {
			name = "job"
		}
		if job.Expired {
			expired = append(expired, name)
		} else {
			notYet = append(notYet, name)
		}
	}
	var notes []string
	if len(expired) > 0 {
		notes = append(notes, "log expired · "+strings.Join(expired, ", "))
	}
	if len(notYet) > 0 {
		notes = append(notes, "log not available yet · "+strings.Join(notYet, ", "))
	}
	return notes
}

func foldLine(width int) string {
	value := colorize(sgrOK, "▾") + " " + colorize(sgrFGSoft, "completed job logs")
	return backgroundSafe(value, width, sgrFG, sgrSurfaceBG)
}

func logLine(line logs.Line, width int, active bool) string {
	gutter := colorize(sgrLine2, fmt.Sprintf("%03d", line.Number))
	return fitANSI(gutter+" "+renderLogText(line.Text, active), width)
}

func renderLogText(text string, active bool) string {
	text = strings.TrimSpace(text)
	rendered := text
	switch {
	case strings.HasPrefix(text, "17:"):
		parts := strings.Fields(text)
		if len(parts) > 1 {
			rendered = colorize(sgrDim, parts[0]) + " " + colorize(sgrInfo, strings.Join(parts[1:], " "))
		}
	case strings.HasPrefix(text, "ok "):
		rendered = colorize(sgrOK, "ok") + strings.TrimPrefix(text, "ok")
	case strings.Contains(text, "--- PASS"):
		rendered = colorize(sgrRun, text)
	case strings.HasPrefix(text, "=== RUN"):
		rendered = strings.Replace(text, "Test", colorize(sgrInfo, "Test"), 1)
	}
	if active {
		rendered += colorize(sgrOK, "█")
	}
	return rendered
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

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
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

const (
	sgrReset     = "\x1b[0m"
	sgrOK        = "\x1b[38;2;79;211;122m"
	sgrRun       = "\x1b[38;2;224;163;62m"
	sgrInfo      = "\x1b[38;2;110;156;181m"
	sgrDim       = "\x1b[38;2;107;112;96m"
	sgrFG        = "\x1b[38;2;234;232;217m"
	sgrFGSoft    = "\x1b[38;2;207;205;187m"
	sgrLine2     = "\x1b[38;2;61;66;51m"
	sgrSurfaceBG = "\x1b[48;2;27;29;23m"
)
