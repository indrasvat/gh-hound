package flakes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
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
	st := stylesFor(m.Theme.OrBramble())
	lines := []string{fitANSI(headerLine(st, m.Report), width)}
	switch m.Report.Status {
	case usecase.FlakeStatusClean:
		lines = append(lines, fitANSI(colorize(st.dim, fmt.Sprintf("%d runs sniffed on %s · nothing wobbled.", m.Report.RunsScanned, m.Report.Workflow)), width))
	case usecase.FlakeStatusInsufficient:
		lines = append(lines,
			fitANSI(colorize(st.dim, fmt.Sprintf("only %d completed %s on %s — the hound needs %d to call it.", m.Report.SampleSize, runsNoun(m.Report.SampleSize), m.Report.Workflow, usecase.FlakeMinSample)), width),
			fitANSI(colorize(st.dim, "no evidence was discarded; what little there is shows below."), width),
		)
		lines = append(lines, m.jobLines(st, width, height)...)
	default:
		lines = append(lines, m.jobLines(st, width, height)...)
	}
	return strings.Join(lines, "\n")
}

func (m Model) jobLines(st styles, width, height int) []string {
	lines := []string{}
	cursor := 0
	for _, job := range m.Report.Jobs {
		lines = append(lines, fitANSI(jobLine(st, job), width))
		for _, evidence := range job.Evidence {
			lines = append(lines, fitANSI(evidenceRow(st, evidence, cursor == m.Selected), width))
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

func headerLine(st styles, report usecase.FlakeReport) string {
	verdict := strings.TrimSpace(report.Verdict)
	if verdict == "" {
		verdict = "the scent check"
	}
	switch report.Status {
	case usecase.FlakeStatusFlaky:
		return colorize(st.run, verdict)
	case usecase.FlakeStatusSuspect:
		return colorize(st.fg, verdict)
	case usecase.FlakeStatusClean:
		return colorize(st.ok, verdict)
	default:
		return colorize(st.dim, verdict)
	}
}

func jobLine(st styles, job usecase.JobFlake) string {
	glyph := colorize(st.run, icons.Flake)
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
	return glyph + " " + colorize(st.fg, job.Job) +
		colorize(st.dim, fmt.Sprintf(" · score %.2f · %s · %s", job.Score, job.Verdict, strings.Join(counts, " · ")))
}

// EvidenceRows renders evidence for the failure screen's flake panel, the
// same lines as here so the two cannot drift; selected is the cursor's
// row, or -1.
func EvidenceRows(th theme.Theme, evidence []usecase.FlakeEvidence, selected int) []string {
	st := stylesFor(th)
	rows := make([]string, len(evidence))
	for i, e := range evidence {
		rows[i] = evidenceRow(st, e, i == selected)
	}
	return rows
}

func evidenceRow(st styles, evidence usecase.FlakeEvidence, selected bool) string {
	prefix := "  "
	if selected {
		prefix = colorize(st.ok, icons.Cursor) + " "
	}
	kind := colorize(st.info, string(evidence.Kind))
	return prefix + colorize(st.fgSoft, fmt.Sprintf("#%d", evidence.Run.RunNumber)) + " " + kind + colorize(st.dim, " · "+evidence.Detail)
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

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the scent check screen.
type styles struct {
	ok, run, info, dim, fg, fgSoft string
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), run: theme.Fg(th.Run), info: theme.Fg(th.Info),
		dim: theme.Fg(th.Dim), fg: theme.Fg(th.FG), fgSoft: theme.Fg(th.FGSoft),
	}
}

const sgrReset = "\x1b[0m"

func runsNoun(n int) string {
	if n == 1 {
		return "run"
	}
	return "runs"
}
