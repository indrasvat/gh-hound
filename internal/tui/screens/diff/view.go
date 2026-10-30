package diff

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
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
	st := stylesFor(m.theme())
	lines := []string{fitANSI(headerLine(m, st), width)}
	switch m.Verdict.Status {
	case usecase.RegressionLocated:
		lines = append(lines,
			fitANSI(boundaryGood(st, m.Verdict.LastGood), width),
			fitANSI(boundaryBad(st, m.Verdict.FirstBad), width),
			fitANSI(suspectHeader(st, m.Verdict), width),
		)
		lines = append(lines, suspectRows(m, st, width, height-len(lines)-1)...)
	case usecase.RegressionGreen:
		lines = append(lines, fitANSI(colorize(st.dim, "the yard is quiet — nothing upwind of this run."), width))
	default:
		lines = append(lines,
			fitANSI(colorize(st.dim, fmt.Sprintf("%d runs sniffed, no clean/dirty boundary in reach.", m.Verdict.RunsScanned)), width),
			fitANSI(colorize(st.dim, "raise diff_max_pages in config to follow the trail deeper."), width),
		)
	}
	return strings.Join(lines, "\n")
}

// headerLine carries the hound verdict: the one line both surfaces
// share.
func headerLine(m Model, st styles) string {
	verdict := strings.TrimSpace(m.Verdict.Verdict)
	if verdict == "" {
		verdict = "the trail"
	}
	switch m.Verdict.Status {
	case usecase.RegressionLocated:
		return colorize(st.fg, verdict)
	case usecase.RegressionGreen:
		return colorize(st.ok, verdict)
	default:
		return colorize(st.run, verdict)
	}
}

func boundaryGood(st styles, run model.Run) string {
	return colorize(st.ok, icons.Success+" #"+itoa(run.RunNumber)) + colorize(st.dim, " clean"+attemptNote(run)+" · "+shortSHA(run.HeadSHA))
}

func boundaryBad(st styles, run model.Run) string {
	return colorize(st.fail, icons.Failure+" #"+itoa(run.RunNumber)) + colorize(st.dim, " dirty"+attemptNote(run)+" · "+shortSHA(run.HeadSHA))
}

func attemptNote(run model.Run) string {
	if run.RunAttempt > 1 {
		return fmt.Sprintf(" · attempt %d", run.RunAttempt)
	}
	return ""
}

func suspectHeader(st styles, verdict usecase.RegressionVerdict) string {
	return colorize(st.dim, fmt.Sprintf("suspects · %d of %d commits between them", len(verdict.SuspectCommits), verdict.TotalSuspects))
}

func suspectRows(m Model, st styles, width, maxRows int) []string {
	commits := m.Verdict.SuspectCommits
	if maxRows > 0 && len(commits) > maxRows {
		// Keep the selection visible when the list is taller than the
		// pane.
		start := 0
		if m.Selected >= maxRows {
			start = m.Selected - maxRows + 1
		}
		commits = commits[start:min(start+maxRows, len(commits))]
		return renderCommitRows(st, commits, width, m.Selected-start)
	}
	return renderCommitRows(st, commits, width, m.Selected)
}

func renderCommitRows(st styles, commits []model.Commit, width, selected int) []string {
	rows := make([]string, 0, len(commits))
	for index, commit := range commits {
		rows = append(rows, commitRow(st, commit, width, index == selected))
	}
	return rows
}

func commitRow(st styles, commit model.Commit, width int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = colorize(st.ok, icons.Cursor) + " "
	}
	sha := padPlain(shortSHA(commit.SHA), shaWidth)
	author := padPlain(truncPlain(commit.Author, authorWidth), authorWidth)
	row := prefix + colorize(st.info, sha) + "  " + colorize(st.fgSoft, author) + "  " + colorize(st.fg, commit.Message)
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

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the trail screen.
type styles struct {
	ok, fail, run, info, dim, fg, fgSoft string
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), run: theme.Fg(th.Run),
		info: theme.Fg(th.Info), dim: theme.Fg(th.Dim), fg: theme.Fg(th.FG),
		fgSoft: theme.Fg(th.FGSoft),
	}
}

func (m Model) theme() theme.Theme {
	if m.Theme.Mode == "" {
		return theme.ForMode(theme.ModeBramble)
	}
	return m.Theme
}

const sgrReset = "\x1b[0m"
