// Package textinput keeps what is being typed in view.
package textinput

import "github.com/charmbracelet/x/ansi"

// Tail returns text, or its end behind "…" when it is wider than room
// cells: the characters being typed are the ones that must stay visible.
func Tail(text string, room int) string {
	width := ansi.StringWidth(text)
	if room < 1 || width <= room {
		return text
	}
	return "…" + ansi.TruncateLeft(text, width-room+1, "")
}
