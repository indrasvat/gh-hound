package textinput

import (
	"strings"
	"testing"
	"unicode/utf8"

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

// A cut inside a wide rune must not leave the result a cell too wide.
func TestTailNeverExceedsRoom(t *testing.T) {
	text := strings.Repeat("a漢", 30)
	for n := range len([]rune(text)) {
		prefix := string([]rune(text)[:n+1])
		for room := 1; room <= 20; room++ {
			if got := Tail(prefix, room); ansi.StringWidth(got) > room {
				t.Fatalf("Tail(%q, %d) = %q, %d cells", prefix, room, got, ansi.StringWidth(got))
			}
		}
	}
}

func FuzzTail(f *testing.F) {
	f.Add("a漢a漢", 5)
	f.Add("é\U0001F415世", 3)
	f.Fuzz(func(t *testing.T, text string, room int) {
		room = room%120 + 1
		if room < 1 || !utf8.ValidString(text) {
			return
		}
		got := Tail(text, room)
		if ansi.StringWidth(got) > room {
			t.Fatalf("Tail(%q, %d) = %q, %d cells", text, room, got, ansi.StringWidth(got))
		}
		if !strings.HasSuffix(text, strings.TrimPrefix(got, "…")) {
			t.Fatalf("Tail(%q, %d) = %q is not the text's end", text, room, got)
		}
	})
}
