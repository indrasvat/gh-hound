package help

import (
	"strings"

	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
)

func View(screen keys.Screen, width int) string {
	layer := keys.FooterLayer(screen)
	var nav, actions, views []string
	for _, binding := range layer.Bindings {
		key := binding.Key
		if binding.Display != "" {
			key = binding.Display
		}
		text := key + " " + binding.Help
		switch binding.Action {
		case "open", "expand", "back", "scroll", "top_bottom", "continue", "move":
			nav = append(nav, text)
		case "rerun", "rerun_job", "rerun_failed", "cancel", "dispatch", "run":
			actions = append(actions, text)
		default:
			views = append(views, text)
		}
	}
	lines := []string{"help · gh hound"}
	lines = append(lines, section("Navigate", nav, width)...)
	lines = append(lines, section("Actions", actions, width)...)
	lines = append(lines, section("View", views, width)...)
	lines = append(lines, "Legend", icons.Success+" success · "+icons.Failure+" failure · "+icons.InProgress+" running")
	return fitLines(lines, width)
}

// section renders a help heading and its wrapped entries — or nothing
// at all when the screen binds no keys in that group, so action-less
// screens (the scent check) don't show an empty "Actions" heading.
func section(heading string, entries []string, width int) []string {
	if len(entries) == 0 {
		return nil
	}
	return append([]string{heading}, wrapEntries(entries, width)...)
}

// wrapEntries flows key entries into · -separated lines that fit the
// width: a section with many bindings wraps instead of truncating its
// tail entries into unreadability. Callers guard against the empty
// slice via section; defensively it returns nothing here too.
func wrapEntries(entries []string, width int) []string {
	if width <= 0 {
		width = 80
	}
	if len(entries) == 0 {
		return nil
	}
	lines := []string{}
	current := ""
	for _, entry := range entries {
		candidate := entry
		if current != "" {
			candidate = current + " · " + entry
		}
		if current != "" && len([]rune(candidate)) > width {
			lines = append(lines, current)
			current = entry
			continue
		}
		current = candidate
	}
	lines = append(lines, current)
	return lines
}

func fitLines(lines []string, width int) string {
	if width <= 0 {
		width = 80
	}
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) > width {
			lines[i] = string(runes[:width-1]) + "…"
		}
	}
	return strings.Join(lines, "\n")
}
