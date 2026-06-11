// Package workflows is the kennel: every workflow in the repo with
// its state, and the leash to wake or muzzle the ones that can be
// toggled. It answers the classic mystery — "my cron workflow stopped
// running" — without a browser.
package workflows

import (
	"strconv"

	"github.com/indrasvat/gh-hound/internal/model"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone    IntentKind = ""
	IntentToggle  IntentKind = "toggle"
	IntentBrowser IntentKind = "browser"
	IntentBack    IntentKind = "back"
)

type Intent struct {
	Kind     IntentKind
	Workflow model.Workflow
}

type Model struct {
	Repo      string
	Workflows []model.Workflow
	Selected  int
	Intent    Intent
}

func NewModel(repo string, workflows []model.Workflow) Model {
	return Model{Repo: repo, Workflows: append([]model.Workflow(nil), workflows...)}
}

func (m Model) SelectedWorkflow() (model.Workflow, bool) {
	if m.Selected < 0 || m.Selected >= len(m.Workflows) {
		return model.Workflow{}, false
	}
	return m.Workflows[m.Selected], true
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "j", "down":
		if m.Selected < len(m.Workflows)-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "g":
		m.Selected = 0
	case "G":
		if len(m.Workflows) > 0 {
			m.Selected = len(m.Workflows) - 1
		}
	case "e":
		// The toggle is offered ONLY for toggleable states; the others
		// carry a why-line in the view instead.
		if workflow, ok := m.SelectedWorkflow(); ok && workflow.Toggleable() {
			m.Intent = Intent{Kind: IntentToggle, Workflow: workflow}
		}
	case "o":
		if workflow, ok := m.SelectedWorkflow(); ok {
			m.Intent = Intent{Kind: IntentBrowser, Workflow: workflow}
		}
	case "esc":
		m.Intent = Intent{Kind: IntentBack}
	}
	return m
}

// WithToggled flips a workflow's state locally after an accepted
// toggle — the result is derived, never re-fetched, so the toggle
// stays exactly one API call. The identifier matches by path or
// numeric id, the same selectors the API accepts.
func (m Model) WithToggled(identifier string, enabled bool) Model {
	state := model.WorkflowStateDisabledManually
	if enabled {
		state = model.WorkflowStateActive
	}
	workflows := append([]model.Workflow(nil), m.Workflows...)
	for i, workflow := range workflows {
		if workflow.Path == identifier || strconv.FormatInt(workflow.ID, 10) == identifier {
			workflows[i].State = state
		}
	}
	m.Workflows = workflows
	return m
}
