// Package textinput keeps what is being typed in view.
package textinput

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Tail returns text, or its end behind "…" when it is wider than room
// cells: the characters being typed are the ones that must stay visible.
func Tail(text string, room int) string {
	if room < 1 || ansi.StringWidth(text) <= room {
		return text
	}
	cut := len(text)
	for used := 0; cut > 0; {
		r, size := utf8.DecodeLastRuneInString(text[:cut])
		width := ansi.StringWidth(string(r))
		if used+width > room-1 {
			break
		}
		used += width
		cut -= size
	}
	return "…" + text[cut:]
}
