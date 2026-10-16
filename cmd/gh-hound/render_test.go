package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

// recordingWriter captures every individual Write call so tests can
// assert the one-flush-per-frame contract, not just the byte totals.
type recordingWriter struct {
	writes []string
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

func frame(lines ...string) string {
	return strings.Join(lines, "\n")
}

// The flicker contract, pinned (root cause of the 2026-06-11 scroll
// flicker report): a frame is exactly ONE write, wrapped in
// synchronized-output guards, and after the first paint the renderer
// must NEVER erase the whole screen — \x1b[2J between frames is the
// blank flash users see.
func TestRendererNeverErasesTheScreenAfterFirstPaint(t *testing.T) {
	out := &recordingWriter{}
	r := newFrameRenderer(out)
	if err := r.Render(frame("alpha", "beta", "gamma")); err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		if err := r.Render(frame("alpha", "beta", "row "+strings.Repeat("x", i))); err != nil {
			t.Fatal(err)
		}
	}
	if len(out.writes) != 11 {
		t.Fatalf("writes = %d, want exactly one per frame", len(out.writes))
	}
	for i, w := range out.writes {
		if !strings.HasPrefix(w, beginSync) || !strings.HasSuffix(w, endSync) {
			t.Fatalf("frame %d is not wrapped in synchronized-output guards: %q", i, w)
		}
		if i > 0 && strings.Contains(w, "\x1b[2J") {
			t.Fatalf("frame %d erases the whole screen — that blank flash IS the flicker: %q", i, w)
		}
	}
}

func TestRendererRewritesOnlyChangedLines(t *testing.T) {
	out := &recordingWriter{}
	r := newFrameRenderer(out)
	if err := r.Render(frame("one", "two", "three", "four")); err != nil {
		t.Fatal(err)
	}
	if err := r.Render(frame("one", "two", "CHANGED", "four")); err != nil {
		t.Fatal(err)
	}
	update := out.writes[1]
	if strings.Contains(update, "one") || strings.Contains(update, "two") || strings.Contains(update, "four") {
		t.Fatalf("unchanged lines were rewritten:\n%q", update)
	}
	if !strings.Contains(update, "\x1b[3;1H") || !strings.Contains(update, "CHANGED"+eraseToEnd) {
		t.Fatalf("changed line must reposition to its row and erase its tail:\n%q", update)
	}
}

func TestRendererClearsBelowWhenTheFrameShrinks(t *testing.T) {
	out := &recordingWriter{}
	r := newFrameRenderer(out)
	if err := r.Render(frame("a", "b", "c", "d")); err != nil {
		t.Fatal(err)
	}
	if err := r.Render(frame("a", "b")); err != nil {
		t.Fatal(err)
	}
	update := out.writes[1]
	if !strings.Contains(update, "\x1b[3;1H"+eraseBelow) {
		t.Fatalf("shrinking frame must erase below the new last row:\n%q", update)
	}
}

func TestRendererInvalidateForcesAFullRepaint(t *testing.T) {
	out := &recordingWriter{}
	r := newFrameRenderer(out)
	if err := r.Render(frame("a", "b")); err != nil {
		t.Fatal(err)
	}
	r.Invalidate()
	if err := r.Render(frame("a", "b")); err != nil {
		t.Fatal(err)
	}
	update := out.writes[1]
	// Identical content, but after Invalidate every line repaints from
	// home — resize reflow invalidated the diff's row mapping.
	if !strings.Contains(update, cursorHome) || !strings.Contains(update, "a"+eraseToEnd) || !strings.Contains(update, "b"+eraseToEnd) {
		t.Fatalf("post-invalidate frame must repaint fully:\n%q", update)
	}
}

func TestRendererIdenticalFrameWritesNoRowContent(t *testing.T) {
	out := &recordingWriter{}
	r := newFrameRenderer(out)
	view := frame("same", "same again")
	if err := r.Render(view); err != nil {
		t.Fatal(err)
	}
	if err := r.Render(view); err != nil {
		t.Fatal(err)
	}
	update := out.writes[1]
	want := beginSync + endSync
	if update != want {
		t.Fatalf("identical frame should flush only the empty sync guards, got %q", update)
	}
}

func TestTUIOutputDownsamplesOnlyKnownLowColorTerminals(t *testing.T) {
	frame := beginSync + "\x1b[2;1H\x1b[38;2;79;211;122mok\x1b[0m" + eraseToEnd + endSync
	tests := []struct {
		name      string
		env       map[string]string
		truecolor bool
	}{
		{"256-colour terminal", map[string]string{"TERM": "xterm-256color", "TTY_FORCE": "1"}, false},
		{"truecolor advertised", map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor", "TTY_FORCE": "1"}, true},
		{"multiplexer advertising truecolor", map[string]string{"TERM": "tmux-256color", "COLORTERM": "truecolor", "TTY_FORCE": "1"}, true},
		{"unknown terminal passes through", map[string]string{"TTY_FORCE": "1"}, true},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if _, err := io.WriteString(tuiOutput(&out, mapEnv(tt.env)), frame); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if strings.Contains(got, "38;2;") != tt.truecolor {
			t.Fatalf("%s: truecolor kept = %v, want %v: %q", tt.name, !tt.truecolor, tt.truecolor, got)
		}
		for _, keep := range []string{beginSync, "\x1b[2;1H", eraseToEnd, endSync, "ok"} {
			if !strings.Contains(got, keep) {
				t.Fatalf("%s: renderer sequence %q lost: %q", tt.name, keep, got)
			}
		}
	}
}

// A line filling the last column leaves the cursor in the pending-wrap
// cell; an erase there wiped the frame's right border on live
// terminals (QA: the border showed only in fixture captures).
func TestRendererNeverErasesTheLastColumnOfAFullLine(t *testing.T) {
	var out recordingWriter
	r := newFrameRenderer(&out)
	r.SetSize(6, 3)
	if err := r.Render("╭────╮\n│ ok │\n╰────╯"); err != nil {
		t.Fatal(err)
	}
	first := out.writes[0]
	if strings.Contains(first, "╮"+eraseToEnd) || strings.Contains(first, "╯"+eraseToEnd) || strings.Contains(first, eraseBelow) {
		t.Fatalf("full-width frame must not erase its last column or below a full screen: %q", first)
	}
	if err := r.Render("╭────╮\n│ no │\n╰────╯"); err != nil {
		t.Fatal(err)
	}
	if diff := out.writes[1]; strings.Contains(diff, eraseToEnd) || !strings.Contains(diff, "│ no │") {
		t.Fatalf("diff repaint of a full-width line must not erase: %q", diff)
	}
	r.SetSize(10, 5)
	r.Invalidate()
	if err := r.Render("short\nlines"); err != nil {
		t.Fatal(err)
	}
	if last := out.writes[2]; !strings.Contains(last, "short"+eraseToEnd) || !strings.Contains(last, fmt.Sprintf(moveRowOne, 3)+eraseBelow) {
		t.Fatalf("shorter lines and a shorter frame still clear their tails: %q", last)
	}
}
