// Package flakes is the scent check screen: the scored flake verdict
// for one workflow+branch window, with evidence rows that drill down
// into the runs that wobbled.
package flakes

import (
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone    IntentKind = ""
	IntentOpenRun IntentKind = "open_run"
	IntentBack    IntentKind = "back"
)

type Intent struct {
	Kind IntentKind
	Run  model.Run
}

// evidenceRef addresses one evidence row inside the flattened
// job→evidence listing the cursor moves over.
type evidenceRef struct {
	job      int
	evidence int
}

type Model struct {
	Report   usecase.FlakeReport
	Selected int
	Intent   Intent
}

func NewModel(report usecase.FlakeReport) Model {
	return Model{Report: report}
}

// rows flattens the per-job evidence into the selectable row list.
func (m Model) rows() []evidenceRef {
	refs := []evidenceRef{}
	for jobIndex, job := range m.Report.Jobs {
		for evidenceIndex := range job.Evidence {
			refs = append(refs, evidenceRef{job: jobIndex, evidence: evidenceIndex})
		}
	}
	return refs
}

// SelectedEvidence returns the evidence row under the cursor.
func (m Model) SelectedEvidence() (usecase.FlakeEvidence, bool) {
	refs := m.rows()
	if len(refs) == 0 || m.Selected < 0 || m.Selected >= len(refs) {
		return usecase.FlakeEvidence{}, false
	}
	ref := refs[m.Selected]
	return m.Report.Jobs[ref.job].Evidence[ref.evidence], true
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	total := len(m.rows())
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
		if evidence, ok := m.SelectedEvidence(); ok && evidence.Run.ID != 0 {
			m.Intent = Intent{Kind: IntentOpenRun, Run: evidence.Run}
		}
	case "esc":
		m.Intent = Intent{Kind: IntentBack}
	}
	return m
}
