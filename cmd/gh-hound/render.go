package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

// Terminal control sequences the renderer speaks. The contract (pinned
// by tests and the vqa stream audit): after the first paint, a frame
// NEVER erases the whole screen — erasing then repainting is exactly
// the blank-flash flicker users see while scrolling.
const (
	beginSync  = "\x1b[?2026h" // synchronized update begin (atomic apply on supporting terminals)
	endSync    = "\x1b[?2026l" // synchronized update end
	enterAlt   = "\x1b[?1049h" // alternate screen buffer
	leaveAlt   = "\x1b[?1049l"
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	eraseToEnd = "\x1b[K" // erase to end of line — per-line clearing instead of 2J
	eraseBelow = "\x1b[J" // erase below — only when a frame shrinks
	cursorHome = "\x1b[H"
	moveRowOne = "\x1b[%d;1H" // absolute row, column one
)

// frameRenderer paints frames flicker-free, the way Bubble Tea's
// standard renderer (and v2's cellbuf screen) does: keep the previous
// frame, write ONLY the lines that changed with absolute positioning
// and erase-to-end-of-line, and flush the whole update as ONE write
// wrapped in synchronized-output guards. The screen is overwritten in
// place — it never blanks between frames.
type frameRenderer struct {
	out  io.Writer
	prev []string
	buf  bytes.Buffer
}

func newFrameRenderer(out io.Writer) *frameRenderer {
	return &frameRenderer{out: out}
}

// Invalidate forgets the previous frame so the next Render repaints
// everything — required after a resize, when the terminal's notion of
// rows has shifted under the diff.
func (r *frameRenderer) Invalidate() {
	r.prev = nil
}

// Render diffs view against the previous frame and flushes the update
// in a single write. The first frame (and the first after Invalidate)
// paints every line; steady-state scrolling typically rewrites only
// the rows that moved.
func (r *frameRenderer) Render(view string) error {
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	r.buf.Reset()
	r.buf.WriteString(beginSync)
	if r.prev == nil {
		r.buf.WriteString(cursorHome)
		for i, line := range lines {
			if i > 0 {
				r.buf.WriteString("\r\n")
			}
			r.buf.WriteString(line)
			r.buf.WriteString(eraseToEnd)
		}
		r.buf.WriteString(eraseBelow)
	} else {
		for i, line := range lines {
			if i < len(r.prev) && r.prev[i] == line {
				continue
			}
			fmt.Fprintf(&r.buf, moveRowOne, i+1)
			r.buf.WriteString(line)
			r.buf.WriteString(eraseToEnd)
		}
		if len(lines) < len(r.prev) {
			// The frame shrank: clear the rows the old frame still
			// owns below the new last line.
			fmt.Fprintf(&r.buf, moveRowOne, len(lines)+1)
			r.buf.WriteString(eraseBelow)
		}
	}
	r.buf.WriteString(endSync)
	r.prev = lines
	_, err := r.out.Write(r.buf.Bytes())
	return err
}

// tuiOutput downsamples the TUI's truecolor styling on terminals that
// only speak 256 or 16 colours. A terminal advertising truecolor
// (COLORTERM) is never downgraded, and anything detection cannot place
// passes through untouched: only SGR colours are rewritten, never the
// cursor and screen sequences the renderer depends on.
func tuiOutput(w io.Writer, lookup func(string) (string, bool)) io.Writer {
	if colorTerm, _ := lookup("COLORTERM"); strings.EqualFold(colorTerm, "truecolor") || strings.EqualFold(colorTerm, "24bit") {
		return w
	}
	var environ []string
	for _, key := range []string{"TERM", "COLORTERM", "TMUX", "TTY_FORCE"} {
		if value, ok := lookup(key); ok {
			environ = append(environ, key+"="+value)
		}
	}
	switch profile := colorprofile.Detect(w, environ); profile {
	case colorprofile.ANSI256, colorprofile.ANSI:
		return &colorprofile.Writer{Forward: w, Profile: profile}
	default:
		return w
	}
}
