package flakes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func View(m Model, width int) string {
	return ViewSize(m, width, 0)
}

func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fitANSI(headerLine(m.Report), width)}
	switch m.Report.Status {
	case usecase.FlakeStatusClean:
		lines = append(lines, fitANSI(colorize(sgrDim, fmt.Sprintf("%d runs sniffed on %s · nothing wobbled.", m.Report.RunsScanned, m.Report.Workflow)), width))
	case usecase.FlakeStatusInsufficient:
		lines = append(lines,
			fitANSI(colorize(sgrDim, fmt.Sprintf("only %d completed %s on %s — the hound needs %d to call it.", m.Report.SampleSize, runsNoun(m.Report.SampleSize), m.Report.Workflow, usecase.FlakeMinSample)), width),
			fitANSI(colorize(sgrDim, "no evidence was discarded; what little there is shows below."), width),
		)
		lines = append(lines, m.jobLines(width, height)...)
	default:
		lines = append(lines, m.jobLines(width, height)...)
	}
	return strings.Join(lines, "\n")
}

func (m Model) jobLines(width, height int) []string {
	lines := []string{}
	cursor := 0
	for _, job := range m.Report.Jobs {
		lines = append(lines, fitANSI(jobLine(job), width))
		for _, evidence := range job.Evidence {
			lines = append(lines, fitANSI(EvidenceRow(evidence, cursor == m.Selected), width))
			cursor++
		}
	}
	if height > 0 && len(lines) > height-1 {
		lines = clampToSelection(lines, m.Selected, height-1)
	}
	return lines
}

// clampToSelection keeps the cursor visible when the evidence list is
// taller than the pane.
func clampToSelection(lines []string, selected, capacity int) []string {
	if capacity <= 0 {
		return nil
	}
	start := 0
	if selected >= capacity {
		start = selected - capacity + 1
	}
	if start+capacity > len(lines) {
		start = len(lines) - capacity
	}
	if start < 0 {
		start = 0
	}
	return lines[start:min(start+capacity, len(lines))]
}

func headerLine(report usecase.FlakeReport) string {
	verdict := strings.TrimSpace(report.Verdict)
	if verdict == "" {
		verdict = "the scent check"
	}
	switch report.Status {
	case usecase.FlakeStatusFlaky:
		return colorize(sgrRun, verdict)
	case usecase.FlakeStatusSuspect:
		return colorize(sgrFG, verdict)
	case usecase.FlakeStatusClean:
		return colorize(sgrOK, verdict)
	default:
		return colorize(sgrDim, verdict)
	}
}

func jobLine(job usecase.JobFlake) string {
	glyph := colorize(sgrRun, icons.Flake)
	counts := []string{}
	if job.Flips > 0 {
		counts = append(counts, plural(job.Flips, "flip"))
	}
	if job.Flaps > 0 {
		counts = append(counts, plural(job.Flaps, "flap"))
	}
	if job.Masks > 0 {
		counts = append(counts, plural(job.Masks, "masked retry", "masked retries"))
	}
	return glyph + " " + colorize(sgrFG, job.Job) +
		colorize(sgrDim, fmt.Sprintf(" · score %.2f · %s · %s", job.Score, job.Verdict, strings.Join(counts, " · ")))
}

// EvidenceRow renders one evidence line; shared with the failure
// screen's flake panel so the two surfaces cannot drift.
func EvidenceRow(evidence usecase.FlakeEvidence, selected bool) string {
	prefix := "  "
	if selected {
		prefix = colorize(sgrOK, icons.Cursor) + " "
	}
	kind := colorize(sgrInfo, string(evidence.Kind))
	return prefix + colorize(sgrFGSoft, fmt.Sprintf("#%d", evidence.Run.RunNumber)) + " " + kind + colorize(sgrDim, " · "+evidence.Detail)
}

func plural(count int, singular string, pluralForm ...string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	if len(pluralForm) > 0 {
		return fmt.Sprintf("%d %s", count, pluralForm[0])
	}
	return fmt.Sprintf("%d %ss", count, singular)
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

const (
	sgrReset  = "\x1b[0m"
	sgrOK     = "\x1b[38;2;79;211;122m"
	sgrRun    = "\x1b[38;2;224;163;62m"
	sgrInfo   = "\x1b[38;2;110;156;181m"
	sgrDim    = "\x1b[38;2;107;112;96m"
	sgrFG     = "\x1b[38;2;234;232;217m"
	sgrFGSoft = "\x1b[38;2;207;205;187m"
)

func runsNoun(n int) string {
	if n == 1 {
		return "run"
	}
	return "runs"
}
