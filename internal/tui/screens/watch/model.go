package watch

import (
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

type State struct {
	Repo    string
	Branch  string
	Run     model.Run
	Lines   []logs.Line
	Elapsed string
}

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone   IntentKind = ""
	IntentCancel IntentKind = "cancel"
	IntentDetach IntentKind = "detach"
)

type Intent struct {
	Kind  IntentKind
	RunID int64
}

type Model struct {
	State  State
	Follow bool
	Debug  bool
	Intent Intent
}

func NewModel(state State) Model {
	return Model{State: state, Follow: true}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "f":
		m.Follow = !m.Follow
	case "d":
		m.Debug = !m.Debug
	case "x":
		m.Intent = Intent{Kind: IntentCancel, RunID: m.State.Run.ID}
	case "esc":
		m.Intent = Intent{Kind: IntentDetach, RunID: m.State.Run.ID}
	}
	return m
}
