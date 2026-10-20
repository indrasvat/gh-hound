package watch

import (
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

type State struct {
	Repo   string
	Branch string
	Run    model.Run
	// Jobs are the run's jobs; one still running reads as its current
	// step, since GitHub serves its log only once it finishes.
	Jobs  []model.Job
	Lines []logs.Line
	// NoLog names the finished jobs shown without their log.
	NoLog   []NoLog
	Elapsed string
	// Now is the clock a running step's elapsed time reads against;
	// zero means the wall clock.
	Now time.Time
}

type NoLog struct {
	Job     string
	Expired bool
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

// Finished reports a run that has ended: nothing streams, nothing to
// cancel.
func (m Model) Finished() bool {
	return m.State.Run.Status == model.StatusCompleted
}

// Outcome names how a finished run ended.
func (m Model) Outcome() string {
	if m.State.Run.Conclusion == model.ConclusionNone {
		return "completed"
	}
	return string(m.State.Run.Conclusion)
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "f":
		m.Follow = !m.Follow
	case "d":
		m.Debug = !m.Debug
	case "x":
		if !m.Finished() {
			m.Intent = Intent{Kind: IntentCancel, RunID: m.State.Run.ID}
		}
	case "esc":
		m.Intent = Intent{Kind: IntentDetach, RunID: m.State.Run.ID}
	}
	return m
}
