package detail

import (
	"fmt"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fit(breadcrumb(m), width)}
	if width < 100 {
		lines = append(lines, renderSteps(m, width)...)
	} else {
		lines = append(lines, renderWide(m, width)...)
	}
	lines = append(lines, fit(keys.FooterForScreen(keys.ScreenDetail), width))
	return strings.Join(lines, "\n")
}

func renderWide(m Model, width int) []string {
	leftWidth := width / 3
	rightWidth := width - leftWidth - 3
	jobs := renderJobs(m, leftWidth)
	steps := renderSteps(m, rightWidth)
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
		lines = append(lines, fit(pad(left, leftWidth)+" | "+right, width))
	}
	return lines
}

func renderJobs(m Model, width int) []string {
	title := "Jobs"
	if m.Focus == FocusJobs {
		title = "Jobs *"
	}
	lines := []string{fit(title, width)}
	for i, job := range m.Jobs {
		prefix := " "
		if i == m.SelectedJob {
			prefix = icons.Cursor
		}
		lines = append(lines, fit(fmt.Sprintf("%s%s %-18s %s", prefix, jobGlyph(job), job.Name, duration(job.StartedAt, job.CompletedAt)), width))
	}
	return lines
}

func renderSteps(m Model, width int) []string {
	job := m.selectedJob()
	title := "Steps"
	if m.Focus == FocusSteps {
		title = "Steps *"
	}
	header := fmt.Sprintf("%s %s %s %s", job.Name, job.Conclusion, firstLabel(job), duration(job.StartedAt, job.CompletedAt))
	lines := []string{fit(title, width), fit(header, width)}
	for i, step := range job.Steps {
		prefix := " "
		if i == m.SelectedStep {
			prefix = icons.Cursor
		}
		lines = append(lines, fit(fmt.Sprintf("%s%s %d %-28s %s", prefix, stepGlyph(step), step.Number, step.Name, duration(step.StartedAt, step.CompletedAt)), width))
	}
	return lines
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

func firstLabel(job model.Job) string {
	if len(job.Labels) == 0 {
		return "runner"
	}
	return job.Labels[0]
}

func fit(value string, width int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func pad(value string, width int) string {
	runes := []rune(value)
	if len(runes) >= width {
		return fit(value, width)
	}
	return value + strings.Repeat(" ", width-len(runes))
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
