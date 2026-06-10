package runs

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone        IntentKind = ""
	IntentOpenDetail  IntentKind = "open_detail"
	IntentOpenLogs    IntentKind = "open_logs"
	IntentWatch       IntentKind = "watch"
	IntentDispatch    IntentKind = "dispatch"
	IntentBrowser     IntentKind = "browser"
	IntentCopy        IntentKind = "copy"
	IntentRerun       IntentKind = "rerun"
	IntentRerunFailed IntentKind = "rerun_failed"
	IntentCancel      IntentKind = "cancel"
	IntentForceCancel IntentKind = "force_cancel"
	IntentFilter      IntentKind = "filter"
	IntentLoadMore    IntentKind = "load_more"
)

type Intent struct {
	Kind   IntentKind
	RunID  int64
	Filter string
}

type Summary struct {
	Failing int
	Running int
	Passed  int
}

type Model struct {
	Context        usecase.LaunchContext
	Selected       int
	Filter         string
	InputMode      bool
	ServerFiltered bool
	Intent         Intent
}

func NewModel(ctx usecase.LaunchContext) Model {
	if ctx.Page == 0 {
		ctx.Page = 1
	}
	if !ctx.HasMore && ctx.PerPage > 0 && len(activeRuns(ctx)) >= ctx.PerPage {
		ctx.HasMore = true
	}
	return Model{Context: ctx}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	if m.InputMode {
		return m.updateInput(msg)
	}
	total := len(m.filteredRuns())
	switch msg.Key {
	case "j", "down":
		if m.Selected < total-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "ctrl+d":
		m.Selected = min(m.Selected+10, max(total-1, 0))
	case "ctrl+u":
		m.Selected = max(m.Selected-10, 0)
	case "g":
		m.Selected = 0
	case "G":
		if total > 0 {
			m.Selected = total - 1
		}
		if m.Context.HasMore && total > 0 {
			m.Intent = Intent{Kind: IntentLoadMore}
		}
	case "s":
		m = m.toggleScope()
	case "esc":
		if m.Filter != "" {
			m.Filter = ""
			m.ServerFiltered = false
			m.Selected = 0
			m.Intent = Intent{Kind: IntentFilter, Filter: ""}
		}
	case "/":
		m.InputMode = true
		m.Filter = ""
	case "enter":
		m.Intent = m.intentFor(IntentOpenDetail)
	case "l":
		m.Intent = m.intentFor(IntentOpenLogs)
	case "w":
		m.Intent = m.intentFor(IntentWatch)
	case "D":
		m.Intent = Intent{Kind: IntentDispatch}
	case "o":
		m.Intent = m.intentFor(IntentBrowser)
	case "y":
		m.Intent = m.intentFor(IntentCopy)
	case "r":
		m.Intent = m.intentFor(IntentRerun)
	case "R":
		m.Intent = m.intentFor(IntentRerunFailed)
	case "x":
		m.Intent = m.intentFor(IntentCancel)
	case "X":
		m.Intent = m.intentFor(IntentForceCancel)
	}
	return m
}

func (m Model) updateInput(msg KeyMsg) Model {
	switch msg.Key {
	case "esc":
		m.InputMode = false
	case "enter":
		m.InputMode = false
		m.Selected = 0
		m.Intent = Intent{Kind: IntentFilter, Filter: m.Filter}
	case "backspace":
		if len(m.Filter) > 0 {
			m.Filter = m.Filter[:len(m.Filter)-1]
		}
	default:
		if len([]rune(msg.Key)) == 1 {
			m.Filter += msg.Key
		}
	}
	return m
}

func (m Model) Summary() Summary {
	var summary Summary
	for _, run := range m.filteredRuns() {
		if isRunning(run) {
			summary.Running++
			continue
		}
		if isFailing(run) {
			summary.Failing++
			continue
		}
		if isPassing(run) {
			summary.Passed++
		}
	}
	return summary
}

// FilteredRuns exposes the rows the list will render after filtering.
func (m Model) FilteredRuns() []model.Run {
	return m.filteredRuns()
}

func (m Model) AllGreen() bool {
	summary := m.Summary()
	runs := m.filteredRuns()
	if len(runs) == 0 || summary.Failing > 0 {
		return false
	}
	for _, run := range runs {
		if run.Status != model.StatusCompleted {
			return false
		}
	}
	return true
}

func (m Model) selectedRun() (model.Run, bool) {
	runs := m.filteredRuns()
	if len(runs) == 0 {
		return model.Run{}, false
	}
	selected := max(m.Selected, 0)
	if selected >= len(runs) {
		selected = len(runs) - 1
	}
	return runs[selected], true
}

func (m Model) SelectedRun() (model.Run, bool) {
	return m.selectedRun()
}

func (m Model) intentFor(kind IntentKind) Intent {
	run, ok := m.selectedRun()
	if !ok {
		return Intent{}
	}
	return Intent{Kind: kind, RunID: run.ID}
}

func (m Model) filteredRuns() []model.Run {
	query := strings.ToLower(strings.TrimSpace(m.Filter))
	if query == "" || m.ServerFiltered {
		// Server-tagged queries (branch:x, running, ...) already came
		// back filtered; re-applying the raw tag as a substring match
		// shows 0 results for data the API just returned.
		return m.activeRuns()
	}
	runs := m.activeRuns()
	filtered := make([]model.Run, 0, len(runs))
	for _, run := range runs {
		if runMatchesQuery(run, query) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func runMatchesQuery(run model.Run, query string) bool {
	for _, alias := range queryAliases(query) {
		if strings.Contains(strings.ToLower(run.Name), alias) ||
			strings.Contains(strings.ToLower(run.DisplayTitle), alias) ||
			strings.Contains(strings.ToLower(run.Event), alias) ||
			strings.Contains(strings.ToLower(run.HeadBranch), alias) ||
			strings.Contains(strings.ToLower(string(run.Status)), alias) ||
			strings.Contains(strings.ToLower(string(run.Conclusion)), alias) ||
			strings.Contains(strings.ToLower(run.Actor), alias) ||
			strings.Contains(fmt.Sprintf("%d", run.RunNumber), alias) {
			return true
		}
	}
	return false
}

func queryAliases(query string) []string {
	switch query {
	case "failed", "failing", "red":
		return []string{query, "failure"}
	case "passed", "passing", "green":
		return []string{query, "success"}
	case "canceled":
		return []string{query, "cancelled"}
	case "running", "live":
		return []string{query, "in_progress"}
	default:
		return []string{query}
	}
}

func (m Model) activeRuns() []model.Run {
	return activeRuns(m.Context)
}

func activeRuns(ctx usecase.LaunchContext) []model.Run {
	switch ctx.Scope {
	case usecase.LaunchScopeRepo:
		if len(ctx.RepoRuns) > 0 {
			return ctx.RepoRuns
		}
	case usecase.LaunchScopeBranch:
		if len(ctx.BranchRuns) > 0 {
			return ctx.BranchRuns
		}
	}
	return ctx.Runs
}

func (m Model) toggleScope() Model {
	switch m.Context.Scope {
	case usecase.LaunchScopeBranch:
		if len(m.Context.RepoRuns) > 0 {
			m.Context.Scope = usecase.LaunchScopeRepo
			m.Context.Runs = m.Context.RepoRuns
			m.Selected = 0
		}
	case usecase.LaunchScopeRepo:
		if len(m.Context.BranchRuns) > 0 {
			m.Context.Scope = usecase.LaunchScopeBranch
			m.Context.Runs = m.Context.BranchRuns
			m.Selected = 0
		}
	default:
		if len(m.Context.BranchRuns) > 0 && len(m.Context.RepoRuns) > 0 {
			m.Context.Scope = usecase.LaunchScopeRepo
			m.Context.Runs = m.Context.RepoRuns
			m.Selected = 0
		}
	}
	return m
}

func isRunning(run model.Run) bool {
	switch run.Status {
	case model.StatusInProgress, model.StatusQueued, model.StatusWaiting, model.StatusPending, model.StatusRequested:
		return true
	default:
		return false
	}
}

func isFailing(run model.Run) bool {
	switch run.Conclusion {
	case model.ConclusionFailure, model.ConclusionActionRequired, model.ConclusionTimedOut:
		return true
	default:
		return false
	}
}

func isPassing(run model.Run) bool {
	if run.Status != model.StatusCompleted {
		return false
	}
	switch run.Conclusion {
	case model.ConclusionSuccess, model.ConclusionSkipped, model.ConclusionNeutral:
		return true
	default:
		return false
	}
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
