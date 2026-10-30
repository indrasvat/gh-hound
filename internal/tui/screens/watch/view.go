package watch

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	return ViewSize(m, width, 0)
}

// ViewSize renders the watch within a height budget: the step-progress
// block takes the rows the header, notes and log tail leave, so every
// running job shows when there is room; height <= 0 means unbudgeted,
// which bounds the block to three lines.
func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	st := stylesFor(m.Theme)
	lines := []string{fit(header(m), width)}
	notes := noLogNotes(m.State.NoLog)
	for _, note := range notes {
		lines = append(lines, fitANSI(colorize(st.dim, note), width))
	}
	live := liveJobs(m.State.Jobs)
	if !m.Finished() && len(live) > 0 {
		now := m.State.Now
		if now.IsZero() {
			now = time.Now()
		}
		room := maxProgressLines
		if height > 0 {
			// The header, the no-log notes, the note under the block, and
			// the fold plus log tail keep their rows.
			reserved := len(lines) + 1
			if tail := len(m.State.Lines); tail > 0 {
				// Follow shows the last six lines; paused, it shows all.
				if m.Follow {
					tail = min(tail, followTail)
				}
				reserved += 1 + tail
			}
			room = max(height-reserved, 1)
		}
		shown := live
		if len(live) > room {
			// Bounded so the log tail keeps its rows: a 50-shard matrix
			// reads as the jobs that fit and a count.
			shown = live[:max(room-1, 0)]
		}
		for _, job := range shown {
			lines = append(lines, jobProgress(st, job, now, width))
		}
		if more := len(live) - len(shown); more > 0 {
			lines = append(lines, fitANSI(colorize(st.dim, fmt.Sprintf("+%d more jobs not finished", more)), width))
		}
		lines = append(lines, fitANSI(colorize(st.dim, "GitHub serves a job's log once it finishes · step progress until then"), width))
	}
	if len(m.State.Lines) == 0 {
		switch {
		case !m.Finished() && len(live) > 0:
		case !m.Finished():
			lines = append(lines, fitANSI(colorize(st.dim, "waiting for completed job logs from GitHub Actions"), width))
		case len(notes) == 0:
			lines = append(lines, fitANSI(colorize(st.dim, "no job logs to show"), width))
		}
		return strings.Join(lines, "\n")
	}
	lines = append(lines, foldLine(st, width))
	start := 0
	if m.Follow && len(m.State.Lines) > followTail {
		start = len(m.State.Lines) - followTail
	}
	visible := m.State.Lines[start:]
	for index, line := range visible {
		lines = append(lines, logLine(st, line, width, !m.Finished() && index == len(visible)-1))
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

// maxProgressLines bounds the step-progress block when the view has no
// height budget; followTail is how many log lines follow shows.
const (
	maxProgressLines = 3
	followTail       = 6
)

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
func jobProgress(st styles, job model.Job, now time.Time, width int) string {
	name := oneLine(job.Name)
	if name == "" {
		name = "job"
	}
	glyph, color := icons.ForStatus(job.Status), st.dim
	if job.Status == model.StatusInProgress {
		color = st.run
	}
	value := colorize(color, glyph) + " " + colorize(st.fgSoft, name)
	step, ok := currentStep(job)
	switch {
	case ok:
		value += colorize(st.dim, " › ") + colorize(st.dim, fmt.Sprintf("%d", step.Number)) + " " + colorize(st.fg, oneLine(step.Name))
		if step.Status == model.StatusInProgress && !step.StartedAt.IsZero() {
			value += " " + colorize(st.info, elapsedSince(step.StartedAt, now))
		}
	case job.Status == model.StatusInProgress:
		if !job.StartedAt.IsZero() {
			value += " " + colorize(st.info, elapsedSince(job.StartedAt, now))
		}
	default:
		value += " " + colorize(st.dim, string(job.Status))
	}
	return fitANSI(value, width)
}

// currentStep is the step a running job is on: the one in progress,
// or failing that the next one not yet finished.
func currentStep(job model.Job) (model.Step, bool) {
	if job.Status != model.StatusInProgress {
		return model.Step{}, false
	}
	for _, step := range job.Steps {
		if step.Status == model.StatusInProgress {
			return step, true
		}
	}
	for _, step := range job.Steps {
		if step.Status != model.StatusCompleted {
			return step, true
		}
	}
	return model.Step{}, false
}

// oneLine folds a GitHub name onto one line: newlines and tabs would
// break the row or its measured width, and any other control rune is
// dropped so it never reaches the terminal.
func oneLine(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, name)
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

func foldLine(st styles, width int) string {
	value := colorize(st.ok, "▾") + " " + colorize(st.fgSoft, "completed job logs")
	return backgroundSafe(value, width, st.fg, st.surfaceBG)
}

func logLine(st styles, line logs.Line, width int, active bool) string {
	gutter := colorize(st.line2, fmt.Sprintf("%03d", line.Number))
	return fitANSI(gutter+" "+renderLogText(st, line.Text, active), width)
}

func renderLogText(st styles, text string, active bool) string {
	text = strings.TrimSpace(text)
	rendered := text
	switch {
	case strings.HasPrefix(text, "17:"):
		parts := strings.Fields(text)
		if len(parts) > 1 {
			rendered = colorize(st.dim, parts[0]) + " " + colorize(st.info, strings.Join(parts[1:], " "))
		}
	case strings.HasPrefix(text, "ok "):
		rendered = colorize(st.ok, "ok") + strings.TrimPrefix(text, "ok")
	case strings.Contains(text, "--- PASS"):
		rendered = colorize(st.run, text)
	case strings.HasPrefix(text, "=== RUN"):
		rendered = strings.Replace(text, "Test", colorize(st.info, "Test"), 1)
	}
	if active {
		rendered += colorize(st.ok, "█")
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

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the watch and the board.
type styles struct {
	ok, run, info, fail, dim, fg, fgSoft, line2 string
	surfaceBG                                   string
}

// stylesFor falls back to Bramble for a zero theme.
func stylesFor(th theme.Theme) styles {
	if th.Mode == "" {
		th = theme.ForMode(theme.ModeBramble)
	}
	return styles{
		ok: theme.Fg(th.OK), run: theme.Fg(th.Run), info: theme.Fg(th.Info), fail: theme.Fg(th.Fail),
		dim: theme.Fg(th.Dim), fg: theme.Fg(th.FG), fgSoft: theme.Fg(th.FGSoft), line2: theme.Fg(th.Line2),
		surfaceBG: theme.Bg(th.Surface),
	}
}

const sgrReset = "\x1b[0m"
