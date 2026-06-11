package confirm

import "strings"

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone    IntentKind = ""
	IntentConfirm IntentKind = "confirm"
	IntentCancel  IntentKind = "cancel"
)

type Intent struct {
	Kind IntentKind
}

type Model struct {
	Message string
	Intent  Intent
}

func New(message string) Model {
	return Model{Message: message}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "y", "Y":
		m.Intent = Intent{Kind: IntentConfirm}
	case "enter", "n", "N", "esc":
		m.Intent = Intent{Kind: IntentCancel}
	}
	return m
}

func View(m Model, width int) string {
	if width <= 0 {
		width = 80
	}
	// Multi-line messages carry paths (artifact destinations) that
	// must not truncate the question away: each line fits separately.
	lines := make([]string, 0, 6)
	for line := range strings.SplitSeq(m.Message, "\n") {
		lines = append(lines, fit(line, width))
	}
	lines = append(lines,
		"",
		fit("This will call the GitHub Actions API.", width),
		fit("Default is no. Press y to confirm, Enter/n/Esc to cancel.", width),
	)
	return strings.Join(lines, "\n")
}

func fit(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return "."
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	return string(runes[:width-3]) + "..."
}
