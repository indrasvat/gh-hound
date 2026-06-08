package failure

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

var exitCodeRE = regexp.MustCompile(`exit code (\d+)`)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		fit(header(m), width),
		fit("[exit "+exitCode(m)+"]  expand full log (l)", width),
		fit("Annotations", width),
	}
	for _, annotation := range m.Report.Annotations {
		lines = append(lines, fit(annotationLine(annotation), width))
	}
	lines = append(lines, fit(fmt.Sprintf("╭─ error window · %d of %d lines · denoised ─", visibleLines(m), totalLines(m)), width))
	for _, line := range m.Excerpt {
		lines = append(lines, fit(fmt.Sprintf("%03d │ %s", line.Number, decorate(line.Text)), width))
	}
	lines = append(lines, fit("╰─ l opens full log at this offset · y copies excerpt", width))
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
	return fmt.Sprintf("%s _%s:%d_ — %s", icons.Failure, annotation.Path, line, annotation.Message)
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
	if m.TotalLines > 0 {
		return m.TotalLines
	}
	total := len(m.Report.Log.Lines) - 2
	if total < len(m.Excerpt) {
		return len(m.Excerpt)
	}
	return total
}

func visibleLines(m Model) int {
	if m.TotalLines > 1000 && len(m.Excerpt) < 12 {
		return 12
	}
	return len(m.Excerpt)
}

func decorate(text string) string {
	switch {
	case strings.Contains(text, "##[error]"):
		return "FAIL " + text
	case strings.Contains(text, "--- FAIL") || strings.HasPrefix(text, "FAIL "):
		return "FAIL " + text
	case strings.Contains(text, " got ") && strings.Contains(text, " want "):
		return "HIT  " + text
	case strings.Contains(text, "go test"):
		return "CMD  " + text
	default:
		return text
	}
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
