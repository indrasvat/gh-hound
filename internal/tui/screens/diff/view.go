package diff

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

const (
	shaWidth    = 7
	authorWidth = 16
)

func View(m Model, width int) string {
	return ViewSize(m, width, 0)
}

func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{fitANSI(headerLine(m), width)}
	switch m.Verdict.Status {
	case usecase.RegressionLocated:
		lines = append(lines,
			fitANSI(boundaryGood(m.Verdict.LastGood), width),
			fitANSI(boundaryBad(m.Verdict.FirstBad), width),
			fitANSI(suspectHeader(m.Verdict), width),
		)
		lines = append(lines, suspectRows(m, width, height-len(lines)-1)...)
	case usecase.RegressionGreen:
		lines = append(lines, fitANSI(colorize(sgrDim, "the yard is quiet — nothing upwind of this run."), width))
	default:
		lines = append(lines,
			fitANSI(colorize(sgrDim, fmt.Sprintf("%d runs sniffed, no clean/dirty boundary in reach.", m.Verdict.RunsScanned)), width),
			fitANSI(colorize(sgrDim, "raise diff_max_pages in config to follow the trail deeper."), width),
		)
	}
	return strings.Join(lines, "\n")
}

// headerLine carries the hound verdict: the one line both surfaces
// share.
func headerLine(m Model) string {
	verdict := strings.TrimSpace(m.Verdict.Verdict)
	if verdict == "" {
		verdict = "the trail"
	}
	switch m.Verdict.Status {
	case usecase.RegressionLocated:
		return colorize(sgrFG, verdict)
	case usecase.RegressionGreen:
		return colorize(sgrOK, verdict)
	default:
		return colorize(sgrRun, verdict)
	}
}

func boundaryGood(run model.Run) string {
	return colorize(sgrOK, icons.Success+" #"+itoa(run.RunNumber)) + colorize(sgrDim, " clean"+attemptNote(run)+" · "+shortSHA(run.HeadSHA))
}

func boundaryBad(run model.Run) string {
	return colorize(sgrFail, icons.Failure+" #"+itoa(run.RunNumber)) + colorize(sgrDim, " dirty"+attemptNote(run)+" · "+shortSHA(run.HeadSHA))
}

func attemptNote(run model.Run) string {
	if run.RunAttempt > 1 {
		return fmt.Sprintf(" · attempt %d", run.RunAttempt)
	}
	return ""
}

func suspectHeader(verdict usecase.RegressionVerdict) string {
	return colorize(sgrDim, fmt.Sprintf("suspects · %d of %d commits between them", len(verdict.SuspectCommits), verdict.TotalSuspects))
}

func suspectRows(m Model, width, maxRows int) []string {
	commits := m.Verdict.SuspectCommits
	if maxRows > 0 && len(commits) > maxRows {
		// Keep the selection visible when the list is taller than the
		// pane.
		start := 0
		if m.Selected >= maxRows {
			start = m.Selected - maxRows + 1
		}
		commits = commits[start:min(start+maxRows, len(commits))]
		return renderCommitRows(m, commits, width, m.Selected-start)
	}
	return renderCommitRows(m, commits, width, m.Selected)
}

func renderCommitRows(m Model, commits []model.Commit, width, selected int) []string {
	rows := make([]string, 0, len(commits))
	for index, commit := range commits {
		rows = append(rows, commitRow(commit, width, index == selected))
	}
	return rows
}

func commitRow(commit model.Commit, width int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = colorize(sgrOK, icons.Cursor) + " "
	}
	sha := padPlain(shortSHA(commit.SHA), shaWidth)
	author := padPlain(truncPlain(commit.Author, authorWidth), authorWidth)
	row := prefix + colorize(sgrInfo, sha) + "  " + colorize(sgrFGSoft, author) + "  " + colorize(sgrFG, commit.Message)
	return fitANSI(row, width)
}

func shortSHA(sha string) string {
	if len(sha) > shaWidth {
		return sha[:shaWidth]
	}
	return sha
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}

func padPlain(value string, width int) string {
	if pad := width - len([]rune(value)); pad > 0 {
		return value + strings.Repeat(" ", pad)
	}
	return value
}

func truncPlain(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
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
	sgrFail   = "\x1b[38;2;226;86;75m"
	sgrRun    = "\x1b[38;2;224;163;62m"
	sgrInfo   = "\x1b[38;2;110;156;181m"
	sgrDim    = "\x1b[38;2;107;112;96m"
	sgrFG     = "\x1b[38;2;234;232;217m"
	sgrFGSoft = "\x1b[38;2;207;205;187m"
)
