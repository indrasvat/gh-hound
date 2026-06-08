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
	lines := []string{"help · gh hound", "Navigate", strings.Join(nav, " · "), "Actions", strings.Join(actions, " · "), "View", strings.Join(views, " · "), "Legend", icons.Success + " success · " + icons.Failure + " failure · " + icons.InProgress + " running"}
	return fitLines(lines, width)
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
