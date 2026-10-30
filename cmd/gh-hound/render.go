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
	eraseLine  = "\x1b[2K" // erase the whole line before repainting it — per-line clearing instead of 2J
	eraseBelow = "\x1b[J"  // erase below — only when a frame shrinks
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
	// width and height are the terminal's, when known: the erase below
	// a frame must start on a row that exists.
	width, height int
}

func newFrameRenderer(out io.Writer) *frameRenderer {
	return &frameRenderer{out: out}
}

// SetSize records the terminal geometry the frames are painted into.
func (r *frameRenderer) SetSize(width, height int) {
	r.width, r.height = width, height
}

// paintLine rewrites one row: erase it, then draw. Erasing first never
// lands in the pending-wrap cell a full-width line leaves behind (which
// used to wipe the frame's right border), and it clears leftovers even
// when a glyph's measured width differs from the terminal's.
func (r *frameRenderer) paintLine(row int, line string) {
	fmt.Fprintf(&r.buf, moveRowOne, row+1)
	r.buf.WriteString(eraseLine)
	r.buf.WriteString(line)
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
	for i, line := range lines {
		if r.prev != nil && i < len(r.prev) && r.prev[i] == line {
			continue
		}
		r.paintLine(i, line)
	}
	if shrank := r.prev != nil && len(lines) < len(r.prev); r.prev == nil || shrank {
		// Clear rows the frame no longer owns — from the row below its
		// last line, and only if the screen has one.
		if r.height <= 0 || len(lines) < r.height {
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
	if profile := colorProfile(w, lookup); limitedProfile(profile) {
		return &colorprofile.Writer{Forward: w, Profile: profile}
	}
	return w
}

// colorProfile is what the terminal can show; tuiOutput downsamples to it
// and the TUI picks its fills for it (theme.Limited).
func colorProfile(w io.Writer, lookup func(string) (string, bool)) colorprofile.Profile {
	if colorTerm, _ := lookup("COLORTERM"); strings.EqualFold(colorTerm, "truecolor") || strings.EqualFold(colorTerm, "24bit") {
		return colorprofile.TrueColor
	}
	var environ []string
	for _, key := range []string{"TERM", "COLORTERM", "TMUX", "TTY_FORCE"} {
		if value, ok := lookup(key); ok {
			environ = append(environ, key+"="+value)
		}
	}
	return colorprofile.Detect(w, environ)
}

func limitedProfile(profile colorprofile.Profile) bool {
	return profile == colorprofile.ANSI256 || profile == colorprofile.ANSI
}
