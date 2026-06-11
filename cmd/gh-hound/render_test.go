package main

import (
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
