package workflows

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func View(m Model, width int) string {
	return ViewSize(m, width, 0)
}

func ViewSize(m Model, width, height int) string {
	if width <= 0 {
		width = 80
	}
	st := stylesFor(m.Theme.OrBramble())
	nameW, fileW, stateW := columnWidths(width, m.Workflows)
	lines := []string{header(st, width, nameW, fileW, stateW)}
	rows := m.Workflows
	selected := m.Selected
	if selected >= len(rows) {
		selected = max(len(rows)-1, 0)
	}
	start, end := viewport(selected, len(rows), rowCapacity(height, len(rows)))
	for i, workflow := range rows[start:end] {
		lines = append(lines, row(st, workflow, start+i == selected, width, nameW, fileW, stateW))
	}
	lines = append(lines, summaryLine(st, m, width))
	if why := whyLine(m); why != "" {
		lines = append(lines, dimLine(st, "  "+why, width))
	}
	return strings.Join(lines, "\n")
}

// columnWidths is the ONE source of column math: the header and every
// row derive from it, so labels can never drift off their values. The
// state column grows past the longest documented badge when an
// unknown state must render verbatim.
func columnWidths(width int, workflows []model.Workflow) (nameW, fileW, stateW int) {
	stateW = stateColumnWidth
	for _, workflow := range workflows {
		stateW = max(stateW, ansi.StringWidth(BadgeText(workflow.State)))
	}
	nameW = 24
	if width < 100 {
		nameW = 16
	}
	// cursor(2) + name + gap + file + gap + state
	fileW = max(width-2-nameW-1-stateW-1, 8)
	return nameW, fileW, stateW
}

// stateColumnWidth fits the longest documented badge, "⊘ fork-disabled".
const stateColumnWidth = 15

func header(st styles, width, nameW, fileW, stateW int) string {
	line := "  " + pad("Workflow", nameW) + " " + pad("File", fileW) + " " + pad("State", stateW)
	return dimLine(st, line, width)
}

func row(st styles, workflow model.Workflow, selected bool, width, nameW, fileW, stateW int) string {
	prefix := "  "
	if selected {
		prefix = colorize(st.ok, icons.Cursor) + " "
	}
	name := colorize(st.fgSoft, truncate(displayName(workflow), nameW))
	file := colorize(st.subtle, truncate(workflow.Path, fileW))
	glyph, label, color := badge(st, workflow.State)
	state := colorize(color, truncate(glyph+" "+label, stateW))
	line := prefix + padANSI(name, nameW) + " " + padANSI(file, fileW) + " " + padANSI(state, stateW)
	return fitANSI(line, width)
}

func displayName(workflow model.Workflow) string {
	if strings.TrimSpace(workflow.Name) != "" {
		return strings.TrimSpace(workflow.Name)
	}
	if strings.TrimSpace(workflow.Path) != "" {
		return strings.TrimSpace(workflow.Path)
	}
	return fmt.Sprintf("workflow %d", workflow.ID)
}

// badge maps a workflow state to its themed glyph + hound-voiced
// label. Unknown future states render verbatim with a neutral badge —
// never rejected, never guessed at.
func badge(st styles, state string) (glyph, label, color string) {
	switch state {
	case model.WorkflowStateActive:
		return icons.Success, "active", st.ok
	case model.WorkflowStateDisabledInactivity:
		return icons.Queued, "asleep", st.run
	case model.WorkflowStateDisabledManually:
		return icons.Cancelled, "muzzled", st.warn
	case model.WorkflowStateDisabledFork:
		return icons.Cancelled, "fork-disabled", st.neutral
	case model.WorkflowStateDeleted:
		return icons.Failure, "deleted", st.fail
	default:
		return icons.Neutral, state, st.neutral
	}
}

// BadgeText is the plain badge for other surfaces (the dispatch
// picker) so every workflow listing shares one vocabulary.
func BadgeText(state string) string {
	glyph, label, _ := badge(styles{}, state)
	return glyph + " " + label
}

// StateLabel is the hound-voiced label alone (asleep, muzzled, …) for
// toasts that name a state mid-sentence.
func StateLabel(state string) string {
	_, label, _ := badge(styles{}, state)
	return label
}

func summaryLine(st styles, m Model, width int) string {
	counts := map[string]int{}
	for _, workflow := range m.Workflows {
		counts[workflow.State]++
	}
	noun := "workflows"
	if len(m.Workflows) == 1 {
		noun = "workflow"
	}
	parts := []string{fmt.Sprintf("%d %s", len(m.Workflows), noun)}
	for _, entry := range []struct {
		state string
		label string
		color string
	}{
		{model.WorkflowStateDisabledInactivity, "asleep", st.run},
		{model.WorkflowStateDisabledManually, "muzzled", st.warn},
		{model.WorkflowStateDisabledFork, "fork-disabled", st.neutral},
		{model.WorkflowStateDeleted, "deleted", st.fail},
	} {
		if counts[entry.state] > 0 {
			parts = append(parts, colorize(entry.color, fmt.Sprintf("%d %s", counts[entry.state], entry.label)))
		}
	}
	line := strings.Join(parts, " · ")
	if hint := toggleHint(m); hint != "" {
		line += " · " + colorize(st.subtle, hint)
	}
	return fitANSI(line, width)
}

// toggleHint advertises the leash only when it would actually work.
func toggleHint(m Model) string {
	workflow, ok := m.SelectedWorkflow()
	if !ok || !workflow.Toggleable() {
		return ""
	}
	if workflow.State == model.WorkflowStateActive {
		return "e muzzle"
	}
	return "e wake"
}

// whyLine explains the selected workflow's state when the toggle is
// not on offer (and the 60-day nap when it is): the "why are there no
// runs" answer, in hound voice.
func whyLine(m Model) string {
	workflow, ok := m.SelectedWorkflow()
	if !ok {
		return ""
	}
	switch workflow.State {
	case model.WorkflowStateDisabledInactivity:
		return "fell asleep after 60 quiet days — e wakes it"
	case model.WorkflowStateDisabledFork:
		return "the fork holds this leash — GitHub will not toggle it from here"
	case model.WorkflowStateDeleted:
		return "the workflow file is gone — nothing left to wake"
	case model.WorkflowStateActive, model.WorkflowStateDisabledManually:
		return ""
	default:
		return fmt.Sprintf("state %q is new to the hound — no toggle offered", workflow.State)
	}
}

func rowCapacity(height, total int) int {
	if height <= 0 {
		return total
	}
	// header + summary + why-line.
	return max(height-3, 1)
}

func viewport(selected, total, capacity int) (int, int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	if capacity >= total {
		return 0, total
	}
	start := max(selected-capacity/2, 0)
	if start+capacity > total {
		start = total - capacity
	}
	return start, start + capacity
}

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the workflows screen.
type styles struct {
	ok, fail, run, warn, neutral, subtle, fgSoft string
	dim                                          lipgloss.Style
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), run: theme.Fg(th.Run), warn: theme.Fg(th.Warn),
		neutral: theme.Fg(th.Neutral), subtle: theme.Fg(th.Subtle), fgSoft: theme.Fg(th.FGSoft),
		dim: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)),
	}
}

const sgrReset = "\x1b[0m"

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
}

func dimLine(st styles, value string, width int) string {
	return st.dim.Render(fitANSI(value, width))
}

func pad(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func padANSI(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func truncate(value string, width int) string {
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
