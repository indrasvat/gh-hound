package runs

import (
	"fmt"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/components/sparkline"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

func View(m Model, width int, now time.Time) string {
	if width <= 0 {
		width = 80
	}
	if m.AllGreen() {
		return renderAllGreen(m, width, now)
	}
	return renderRuns(m, width, now)
}

func renderRuns(m Model, width int, now time.Time) string {
	lines := []string{
		fit(header(m, true), width),
		fit("  Workflow           Event             #     Duration  Age      Actor/SHA", width),
	}
	for i, run := range m.Context.Runs {
		lines = append(lines, row(run, i == m.Selected, width, now))
	}
	summary := m.Summary()
	lines = append(lines,
		fit(fmt.Sprintf("%d failing · %d running · %d passed", summary.Failing, summary.Running, summary.Passed), width),
		fit(keys.FooterForScreen(keys.ScreenRunsList), width),
	)
	return strings.Join(lines, "\n")
}

func renderAllGreen(m Model, width int, now time.Time) string {
	summary := m.Summary()
	latest := ""
	if len(m.Context.Runs) > 0 {
		run := m.Context.Runs[0]
		latest = fmt.Sprintf("%s #%d · success", run.Name, run.RunNumber)
	}
	lines := []string{
		fit(header(m, false), width),
		fit("╭─ all clear ─────────────────────────────────────────────╮", width),
		fit("│ ✔  All checks passing on "+first(m.Context.Branch, "all branches"), width),
		fit(fmt.Sprintf("│ %d recent runs · %d failing · last finished %s", len(m.Context.Runs), summary.Failing, age(m.Context.Runs[0], now)), width),
		fit("│ latest "+latest, width),
		fit("╰──────────────────────────────────────────────────────────╯", width),
	}
	for _, run := range m.Context.Runs {
		lines = append(lines, fit(fmt.Sprintf("%s %s #%d %s", glyph(run), truncate(run.Name, 22), run.RunNumber, age(run, now)), width))
	}
	lines = append(lines, fit(keys.FooterForScreen(keys.ScreenAllGreen), width))
	return strings.Join(lines, "\n")
}

func header(m Model, focused bool) string {
	branch := first(m.Context.Branch, "all branches")
	right := "◔ 4,981/5k live"
	if focused {
		right += " 304"
	}
	return fmt.Sprintf("hound %s %s · @%s %s", icons.Branch, branch, first(m.Context.Actor, "indrasvat"), right)
}

func row(run model.Run, selected bool, width int, now time.Time) string {
	prefix := " "
	if selected {
		prefix = icons.Cursor
	}
	line := fmt.Sprintf("%s%s %-16s %-16s #%3d  %-8s %s",
		prefix,
		glyph(run),
		truncate(run.Name, 16),
		truncate(run.Event, 16),
		run.RunNumber,
		sparkline.Render([]int{30, 55, 90, 65, 40}, 5),
		age(run, now),
	)
	if width >= 110 {
		line = fmt.Sprintf("%s  @indrasvat a1b2c3d", line)
	}
	return fit(line, width)
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
	if duration < time.Minute {
		return "now"
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	return fmt.Sprintf("%dh", int(duration.Hours()))
}

func fit(value string, width int) string {
	return truncate(value, width)
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
