// Package diff is the trail screen: the last-green → first-red
// boundary of a workflow plus the suspect commits between them.
package diff

import (
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone         IntentKind = ""
	IntentOpenFirstBad IntentKind = "open_first_bad"
	IntentBrowser      IntentKind = "browser"
	IntentBack         IntentKind = "back"
)

type Intent struct {
	Kind IntentKind
}

type Model struct {
	Verdict  usecase.RegressionVerdict
	Selected int
	Intent   Intent
}

func NewModel(verdict usecase.RegressionVerdict) Model {
	return Model{Verdict: verdict}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	total := len(m.Verdict.SuspectCommits)
	switch msg.Key {
	case "j", "down":
		if m.Selected < total-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "enter":
		// Only a located boundary has a first-bad run to open.
		if m.Verdict.Status == usecase.RegressionLocated && m.Verdict.FirstBad.ID != 0 {
			m.Intent = Intent{Kind: IntentOpenFirstBad}
		}
	case "o":
		if m.Verdict.CompareURL != "" {
			m.Intent = Intent{Kind: IntentBrowser}
		}
	case "esc":
		m.Intent = Intent{Kind: IntentBack}
	}
	return m
}
