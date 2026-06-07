package failure

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

var exitCodeRE = regexp.MustCompile(`exit code (\d+)`)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		fit(header(m), width),
		"Annotations",
	}
	for _, annotation := range m.Report.Annotations {
		lines = append(lines, fit(annotationLine(annotation), width))
	}
	lines = append(lines, fit(fmt.Sprintf("error window · %d of %d lines", len(m.Excerpt), totalLines(m)), width))
	for _, line := range m.Excerpt {
		lines = append(lines, fit(fmt.Sprintf("%03d %s", line.Number, line.Text), width))
	}
	lines = append(lines, fit(keys.FooterForScreen(keys.ScreenFailure), width))
	return strings.Join(lines, "\n")
}

func header(m Model) string {
	step := failedStep(m)
	return fmt.Sprintf("… %s %s %s %s %s · step %d · exit %s",
		icons.Breadcrumb,
		m.Report.Job.Name,
		icons.Breadcrumb,
		icons.Failure,
		step.Name,
		step.Number,
		exitCode(m),
	)
}

func annotationLine(annotation model.Annotation) string {
	line := annotation.StartLine
	if line == 0 {
		line = annotation.EndLine
	}
	return fmt.Sprintf("%s %s:%d — %s", icons.Failure, annotation.Path, line, annotation.Message)
}

func failedStep(m Model) model.Step {
	for _, step := range m.Report.Job.Steps {
		if step.Conclusion == model.ConclusionFailure || step.Conclusion == model.ConclusionActionRequired || step.Conclusion == model.ConclusionTimedOut {
			return step
		}
	}
	if len(m.Report.Job.Steps) > 0 {
		return m.Report.Job.Steps[0]
	}
	return model.Step{Name: "unknown", Number: 0}
}

func exitCode(m Model) string {
	for _, line := range m.Report.Log.Failure.Lines {
		if match := exitCodeRE.FindStringSubmatch(line.Text); len(match) == 2 {
			return match[1]
		}
	}
	return "1"
}

func totalLines(m Model) int {
	total := len(m.Report.Log.Lines) - 2
	if total < len(m.Excerpt) {
		return len(m.Excerpt)
	}
	return total
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
