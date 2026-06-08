package runs

import (
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
	Context   usecase.LaunchContext
	Selected  int
	Filter    string
	InputMode bool
	Intent    Intent
}

func NewModel(ctx usecase.LaunchContext) Model {
	return Model{Context: ctx}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	if m.InputMode {
		return m.updateInput(msg)
	}
	switch msg.Key {
	case "j", "down":
		if m.Selected < len(m.Context.Runs)-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "g":
		m.Selected = 0
	case "G":
		if len(m.Context.Runs) > 0 {
			m.Selected = len(m.Context.Runs) - 1
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
	for _, run := range m.Context.Runs {
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

func (m Model) AllGreen() bool {
	summary := m.Summary()
	if len(m.Context.Runs) == 0 || summary.Failing > 0 {
		return false
	}
	for _, run := range m.Context.Runs {
		if run.Status != model.StatusCompleted {
			return false
		}
	}
	return true
}

func (m Model) selectedRun() (model.Run, bool) {
	if len(m.Context.Runs) == 0 || m.Selected < 0 || m.Selected >= len(m.Context.Runs) {
		return model.Run{}, false
	}
	return m.Context.Runs[m.Selected], true
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

func isRunning(run model.Run) bool {
	return run.Status == model.StatusInProgress
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
