// Package approvals is the deployment-gate overlay: pick the pending
// environments of a waiting run, then approve or reject them. Both
// verdicts are confirm-gated by the app, like rerun and cancel.
package approvals

import (
	"fmt"
	"maps"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone    IntentKind = ""
	IntentApprove IntentKind = "approve"
	IntentReject  IntentKind = "reject"
)

type Intent struct {
	Kind         IntentKind
	Environments []string
	Comment      string
}

// Environment is one pending gate as the overlay shows it.
type Environment struct {
	ID         int64
	Name       string
	WaitTimer  int
	CanApprove bool
	Reviewers  []string
}

type Model struct {
	Run          model.Run
	Environments []Environment
	Selected     int
	Picked       map[int]bool
	Comment      string
	CommentMode  bool
	Notice       string
	Intent       Intent
}

// NewModel builds the overlay state. Every approvable environment
// starts picked so y/n act on the whole gate by default.
func NewModel(run model.Run, pending []model.PendingDeployment) Model {
	environments := make([]Environment, 0, len(pending))
	picked := map[int]bool{}
	for i, gate := range pending {
		reviewers := make([]string, 0, len(gate.Reviewers))
		for _, reviewer := range gate.Reviewers {
			name := reviewer.Name
			if reviewer.Type == "Team" {
				name += " (team)"
			}
			reviewers = append(reviewers, name)
		}
		environments = append(environments, Environment{
			ID:         gate.EnvironmentID,
			Name:       gate.EnvironmentName,
			WaitTimer:  gate.WaitTimer,
			CanApprove: gate.CurrentUserCanApprove,
			Reviewers:  reviewers,
		})
		if gate.CurrentUserCanApprove {
			picked[i] = true
		}
	}
	return Model{Run: run, Environments: environments, Picked: picked}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	m.Notice = ""
	if m.CommentMode {
		return m.updateComment(msg)
	}
	switch msg.Key {
	case "j", "down":
		if m.Selected < len(m.Environments)-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "space":
		m = m.togglePicked()
	case "c":
		m.CommentMode = true
	case "y":
		m = m.review(IntentApprove)
	case "n":
		m = m.review(IntentReject)
	}
	return m
}

func (m Model) updateComment(msg KeyMsg) Model {
	switch msg.Key {
	case "esc", "enter":
		m.CommentMode = false
	case "backspace":
		if len(m.Comment) > 0 {
			m.Comment = m.Comment[:len(m.Comment)-1]
		}
	case "space":
		m.Comment += " "
	default:
		if len([]rune(msg.Key)) == 1 {
			m.Comment += msg.Key
		}
	}
	return m
}

func (m Model) togglePicked() Model {
	if len(m.Environments) == 0 {
		return m
	}
	environment := m.Environments[m.Selected]
	if !environment.CanApprove {
		m.Notice = "not yours to open — " + environment.Name + " needs another reviewer"
		return m
	}
	picked := make(map[int]bool, len(m.Picked))
	maps.Copy(picked, m.Picked)
	picked[m.Selected] = !picked[m.Selected]
	m.Picked = picked
	return m
}

func (m Model) review(kind IntentKind) Model {
	names := m.PickedEnvironments()
	if len(names) == 0 {
		m.Notice = "pick a gate first — space marks an environment"
		return m
	}
	m.Intent = Intent{Kind: kind, Environments: names, Comment: strings.TrimSpace(m.Comment)}
	return m
}

// PickedEnvironments lists the names currently marked for review, in
// display order.
func (m Model) PickedEnvironments() []string {
	names := []string{}
	for i, environment := range m.Environments {
		if m.Picked[i] {
			names = append(names, environment.Name)
		}
	}
	return names
}

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		fit(fmt.Sprintf("run #%d holds at the gate — pick, then act", m.Run.RunNumber), width),
		"",
	}
	for i, environment := range m.Environments {
		lines = append(lines, fit(environmentRow(m, i, environment), width))
		lines = append(lines, fit("      reviewers: "+strings.Join(environment.Reviewers, ", "), width))
	}
	lines = append(lines, "")
	comment := strings.TrimSpace(m.Comment)
	commentLine := "comment: " + comment
	if comment == "" {
		commentLine = `comment: reviewed from gh-hound (default) · c to edit`
	}
	if m.CommentMode {
		commentLine = "comment" + icons.Prompt + " " + m.Comment + "▏(enter done · esc cancel)"
	}
	lines = append(lines, fit(commentLine, width))
	if m.Notice != "" {
		lines = append(lines, fit(m.Notice, width))
	}
	return strings.Join(lines, "\n")
}

func environmentRow(m Model, index int, environment Environment) string {
	cursor := " "
	if index == m.Selected {
		cursor = icons.Cursor
	}
	check := "[ ]"
	if m.Picked[index] {
		check = "[x]"
	}
	state := "you can open this gate"
	if !environment.CanApprove {
		check = "[-]"
		state = "not yours to open"
	}
	wait := ""
	if environment.WaitTimer > 0 {
		wait = fmt.Sprintf("wait %dm · ", environment.WaitTimer/60)
	}
	return fmt.Sprintf("%s %s %s %s  %s%s", cursor, check, icons.Gate, environment.Name, wait, state)
}

func fit(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
