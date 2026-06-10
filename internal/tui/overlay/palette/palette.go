package palette

import (
	"strings"

	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

type Item struct {
	Name        string
	Description string
	Tag         string
	Route       string
	Value       string
}

type KeyMsg struct {
	Key string
}

type Intent struct {
	Route string
	Value string
}

type Model struct {
	Items    []Item
	Query    string
	Selected int
	Intent   Intent
}

func DefaultItems() []Item {
	return []Item{
		{Name: "runs", Description: "workflow runs · this branch", Tag: "default"},
		{Name: "runs --all", Description: "runs across all branches"},
		{Name: "run:failed", Description: "filtered to failures"},
		{Name: "artifacts", Description: "selected run's artifacts"},
		{Name: "dispatch", Description: "trigger workflow_dispatch"},
	}
}

func New(items []Item) Model {
	return Model{Items: append([]Item(nil), items...)}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "j", "down":
		if m.Selected < len(m.Visible())-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "enter":
		visible := m.Visible()
		if len(visible) > 0 {
			item := visible[m.Selected]
			route := item.Route
			if route == "" {
				route = item.Name
			}
			m.Intent = Intent{Route: route, Value: item.Value}
		}
	case "backspace":
		if len(m.Query) > 0 {
			m.Query = m.Query[:len(m.Query)-1]
			m.Selected = 0
		}
	default:
		if len([]rune(msg.Key)) == 1 {
			m.Query += msg.Key
			m.Selected = 0
		}
	}
	return m
}

func (m Model) Visible() []Item {
	if m.Query == "" {
		return append([]Item(nil), m.Items...)
	}
	query := strings.ToLower(m.Query)
	out := []Item{}
	for _, item := range m.Items {
		haystack := strings.ToLower(item.Name + " " + item.Description)
		if strings.Contains(haystack, query) {
			out = append(out, item)
		}
	}
	return out
}

func View(m Model, width int) string {
	lines := []string{": jump to…", icons.Prompt + " " + m.Query}
	for i, item := range m.Visible() {
		prefix := " "
		if i == m.Selected {
			prefix = icons.Cursor
		}
		row := prefix + item.Name + " · " + item.Description
		if item.Tag != "" {
			row += " · " + item.Tag
		}
		lines = append(lines, fit(row, width))
	}
	lines = append(lines, "workflows · watch · diff (v2) · theme")
	return strings.Join(lines, "\n")
}

func fit(value string, width int) string {
	if width <= 0 {
		width = 80
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}
