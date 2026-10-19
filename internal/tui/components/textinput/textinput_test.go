package textinput

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTailKeepsTheEndWithinRoom(t *testing.T) {
	tests := []struct {
		text string
		room int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 4, "…def"},
		{strings.Repeat("世", 10) + "末", 7, "…世世末"},
	}
	for _, tt := range tests {
		got := Tail(tt.text, tt.room)
		if got != tt.want || ansi.StringWidth(got) > tt.room {
			t.Errorf("Tail(%q, %d) = %q, want %q", tt.text, tt.room, got, tt.want)
		}
	}
}
