// Package queue is the GitHub Actions concurrency queue screen.
package queue

import (
	"encoding/json"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone    IntentKind = ""
	IntentOpenRun IntentKind = "open_run"
	IntentCopy    IntentKind = "copy"
	IntentBack    IntentKind = "back"
)

type Intent struct {
	Kind IntentKind
	Run  model.Run
	Text string
}

type Row struct {
	GroupName string
	Member    model.ConcurrencyMember
}

type Model struct {
	Report   model.ConcurrencyReport
	Selected int
	Intent   Intent
}

func NewModel(report model.ConcurrencyReport) Model {
	return Model{Report: report}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	total := len(m.Members())
	switch msg.Key {
	case "j", "down":
		if m.Selected < total-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "g":
		m.Selected = 0
	case "G":
		if total > 0 {
			m.Selected = total - 1
		}
	case "enter":
		if member, ok := m.SelectedMember(); ok && member.RunID != 0 {
			m.Intent = Intent{Kind: IntentOpenRun, Run: runFromMember(member)}
		}
	case "y":
		m.Intent = Intent{Kind: IntentCopy, Text: m.CopyJSON()}
	case "esc":
		m.Intent = Intent{Kind: IntentBack}
	}
	return m
}

func (m Model) Members() []model.ConcurrencyMember {
	rows := m.Rows()
	members := make([]model.ConcurrencyMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, row.Member)
	}
	return members
}

func (m Model) Rows() []Row {
	var rows []Row
	for _, group := range m.Report.Groups {
		for _, member := range group.Members {
			rows = append(rows, Row{GroupName: group.GroupName, Member: member})
		}
	}
	return rows
}

func (m Model) SelectedMember() (model.ConcurrencyMember, bool) {
	rows := m.Rows()
	if len(rows) == 0 {
		return model.ConcurrencyMember{}, false
	}
	selected := max(min(m.Selected, len(rows)-1), 0)
	return rows[selected].Member, true
}

func (m Model) SelectedRow() (Row, bool) {
	rows := m.Rows()
	if len(rows) == 0 {
		return Row{}, false
	}
	selected := max(min(m.Selected, len(rows)-1), 0)
	return rows[selected], true
}

func (m Model) CopyJSON() string {
	payload := render.QueueResultFromReport(m.Report)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func runFromMember(member model.ConcurrencyMember) model.Run {
	return model.Run{
		ID:      member.RunID,
		Name:    member.RunName,
		Status:  member.Status,
		HTMLURL: member.RunHTMLURL,
	}
}
