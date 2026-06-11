package failure

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	flakesscreen "github.com/indrasvat/gh-hound/internal/tui/screens/flakes"
)

var exitCodeRE = regexp.MustCompile(`exit code (\d+)`)
var gotWantRE = regexp.MustCompile(`^(.*?): got ("[^"]*") want ("[^"]*")(.*)$`)

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		fitANSI(header(m), width),
		annotationHeader(width),
	}
	for _, annotation := range m.Report.Annotations {
		lines = append(lines, annotationLine(annotation, width))
	}
	lines = append(lines, errorHeader(m, width))
	for _, line := range m.Excerpt {
		lines = append(lines, logLine(line, width))
	}
	lines = append(lines, flakePanel(m, width)...)
	return strings.Join(lines, "\n")
}

// flakePanel renders the scent panel once the async verdict lands.
// The focused pane is marked with the cursor glyph: tab moves focus,
// j/k drives whichever pane holds it.
func flakePanel(m Model, width int) []string {
	if m.Flake == nil {
		return nil
	}
	flake := *m.Flake
	header := colorize(sgrRun, icons.Flake+" seen this one before: ") +
		colorize(sgrFG, fmt.Sprintf("%s flaked %d of last %d runs", flake.Job, flake.FlakedRuns, m.FlakeWindow)) +
		colorize(sgrDim, fmt.Sprintf(" (%s · score %.2f)", flake.Verdict, flake.Score))
	lines := []string{
		"",
		fitANSI(focusMark(m.PanelFocus)+header, width),
	}
	for index, evidence := range flake.Evidence {
		lines = append(lines, fitANSI(flakesscreen.EvidenceRow(evidence, m.PanelFocus && index == m.PanelSelected), width))
	}
	hint := "⇥ inspect evidence"
	if m.PanelFocus {
		hint = "j/k move · ⏎ open run · ⇥ back to excerpt"
	}
	lines = append(lines, fitANSI(colorize(sgrDim, "  "+hint), width))
	return lines
}

// focusMark renders the theme focus marker for the pane that owns
// j/k.
func focusMark(focused bool) string {
	if focused {
		return colorize(sgrOK, "▌")
	}
	return " "
}

func header(m Model) string {
	parts := []string{"…", icons.Breadcrumb}
	if name := strings.TrimSpace(m.Report.Job.Name); name != "" {
		parts = append(parts, name, icons.Breadcrumb)
	} else if m.Report.Job.ID > 0 {
		parts = append(parts, fmt.Sprintf("job %d", m.Report.Job.ID), icons.Breadcrumb)
	}
	parts = append(parts, icons.Failure)
	if step, ok := failedStep(m); ok {
		if strings.TrimSpace(step.Name) != "" {
			parts = append(parts, step.Name)
		}
		if step.Number > 0 {
			parts = append(parts, "·", fmt.Sprintf("step %d", step.Number))
		}
	} else {
		parts = append(parts, "failed step unavailable")
	}
	if code := exitCode(m); code != "" {
		parts = append(parts, "·", "exit "+code)
	}
	return strings.Join(parts, " ")
}

func annotationHeader(width int) string {
	return fitANSI(colorize(sgrDim, "Annotations"), width)
}

func annotationLine(annotation model.Annotation, width int) string {
	line := annotation.StartLine
	if line == 0 {
		line = annotation.EndLine
	}
	path := underlineInfo(fmt.Sprintf("%s:%d", annotation.Path, line))
	value := colorize(sgrFail, icons.Failure) + " " + path + "  " + colorize(sgrFGSoft, annotation.Message)
	return fitANSI(value, width)
}

func errorHeader(m Model, width int) string {
	left := colorize(sgrDim, fmt.Sprintf("error window · %d of %d lines", visibleLines(m), totalLines(m)))
	if m.Flake != nil {
		// Two focusable panes exist: mark the one that owns j/k.
		left = focusMark(!m.PanelFocus) + left
	}
	right := colorize(sgrOK, "⤓ expand full log (l)")
	return backgroundSafe(joinRightANSI(left, right, width), width, sgrFG, sgrSurfaceBG)
}

func logLine(line logs.Line, width int) string {
	gutter := colorize(sgrLine2, fmt.Sprintf("%03d", line.Number))
	value := gutter + " " + renderLogText(line.Text)
	if isHitLine(line.Text) {
		return backgroundSafe(value, width, sgrFG, sgrHitBG)
	}
	return fitANSI(value, width)
}

func renderLogText(text string) string {
	text = strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(text, "17:"):
		parts := strings.Fields(text)
		if len(parts) > 1 {
			return colorize(sgrDim, parts[0]) + " " + colorize(sgrInfo, strings.Join(parts[1:], " "))
		}
	case isHitLine(text):
		return renderGotWant(text)
	case strings.Contains(text, "##[error]"):
		return strings.Replace(text, "##[error]", colorize(sgrFail, "##[error]"), 1)
	case strings.Contains(text, "--- FAIL") || strings.HasPrefix(text, "FAIL "):
		return colorize(sgrFail, text)
	case strings.HasPrefix(text, "ok "):
		return colorize(sgrOK, "ok") + strings.TrimPrefix(text, "ok")
	}
	return text
}

func renderGotWant(text string) string {
	text = strings.TrimSpace(text)
	match := gotWantRE.FindStringSubmatch(text)
	if len(match) != 5 {
		return text
	}
	return underlineInfo(match[1]) + ": got " + colorize(sgrOK, match[2]) + " want " + colorize(sgrFail, match[3]) + match[4]
}

func isHitLine(text string) bool {
	return strings.Contains(text, " got ") && strings.Contains(text, " want ")
}

func failedStep(m Model) (model.Step, bool) {
	for _, step := range m.Report.Job.Steps {
		if step.Conclusion == model.ConclusionFailure || step.Conclusion == model.ConclusionActionRequired || step.Conclusion == model.ConclusionTimedOut {
			return step, true
		}
	}
	if len(m.Report.Job.Steps) > 0 {
		return m.Report.Job.Steps[0], true
	}
	return model.Step{}, false
}

func exitCode(m Model) string {
	for _, line := range m.Report.Log.Failure.Lines {
		if match := exitCodeRE.FindStringSubmatch(line.Text); len(match) == 2 {
			return match[1]
		}
	}
	return ""
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

func underlineInfo(value string) string {
	return sgrInfo + sgrUnderline + value + sgrReset
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

const (
	sgrReset     = "\x1b[0m"
	sgrUnderline = "\x1b[4m"
	sgrOK        = "\x1b[38;2;79;211;122m"
	sgrRun       = "\x1b[38;2;224;163;62m"
	sgrFail      = "\x1b[38;2;226;86;75m"
	sgrInfo      = "\x1b[38;2;110;156;181m"
	sgrDim       = "\x1b[38;2;107;112;96m"
	sgrFG        = "\x1b[38;2;234;232;217m"
	sgrFGSoft    = "\x1b[38;2;207;205;187m"
	sgrLine2     = "\x1b[38;2;61;66;51m"
	sgrSurfaceBG = "\x1b[48;2;27;29;23m"
	sgrHitBG     = "\x1b[48;2;43;33;24m"
)
