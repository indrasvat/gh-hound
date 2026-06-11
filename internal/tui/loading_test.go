package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

func TestSpinnerGlyphCyclesFrames(t *testing.T) {
	if len(icons.SpinnerFrames) == 0 {
		t.Fatal("icons.SpinnerFrames is empty")
	}
	if got := spinnerGlyph(0); got != icons.SpinnerFrames[0] {
		t.Fatalf("frame at 0 = %q, want %q", got, icons.SpinnerFrames[0])
	}
	if got := spinnerGlyph(loadFrameInterval); got != icons.SpinnerFrames[1] {
		t.Fatalf("frame at one interval = %q, want %q", got, icons.SpinnerFrames[1])
	}
	full := loadFrameInterval * time.Duration(len(icons.SpinnerFrames))
	if got := spinnerGlyph(full); got != icons.SpinnerFrames[0] {
		t.Fatalf("frame wraps: at %v = %q, want %q", full, got, icons.SpinnerFrames[0])
	}
}

func TestSpinnerFramesAreTextPresentation(t *testing.T) {
	for i, frame := range icons.SpinnerFrames {
		if strings.Contains(frame, "\ufe0f") {
			t.Fatalf("frame %d contains VS16 emoji selector", i)
		}
	}
}

func TestLoadingLineHiddenWithinGrace(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	now := time.Now()
	load := &pendingLoad{kind: loadKindRuns, label: "sniffing out failing runs", started: now.Add(-50 * time.Millisecond)}
	if line := loadingLine(th, load, 80, now); line != "" {
		t.Fatalf("loading line visible inside the %v grace window: %q", loadGraceDelay, line)
	}
	load.started = now.Add(-150 * time.Millisecond)
	line := loadingLine(th, load, 80, now)
	if line == "" {
		t.Fatal("loading line hidden after grace expired")
	}
	plain := ansi.Strip(line)
	if !strings.Contains(plain, "sniffing out failing runs") {
		t.Fatalf("loading line missing label: %q", plain)
	}
	hasFrame := false
	for _, frame := range icons.SpinnerFrames {
		if strings.Contains(plain, frame) {
			hasFrame = true
			break
		}
	}
	if !hasFrame {
		t.Fatalf("loading line missing spinner glyph: %q", plain)
	}
}

func TestLoadingLineIndeterminateOmitsBar(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	now := time.Now()
	load := &pendingLoad{kind: loadKindLog, label: "fetching log", started: now.Add(-time.Second)}
	plain := ansi.Strip(loadingLine(th, load, 80, now))
	if strings.Contains(plain, "▰") || strings.Contains(plain, "▱") {
		t.Fatalf("indeterminate load rendered a progress bar: %q", plain)
	}
}

func TestLoadingLineDeterminateProgress(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	now := time.Now()
	load := &pendingLoad{
		kind:    loadKindLog,
		label:   "fetching log",
		started: now.Add(-time.Second),
		read:    2202009, // ~2.1 MB
		total:   5033165, // ~4.8 MB
	}
	plain := ansi.Strip(loadingLine(th, load, 80, now))
	if !strings.Contains(plain, "▰") || !strings.Contains(plain, "▱") {
		t.Fatalf("determinate load missing progress bar: %q", plain)
	}
	if !strings.Contains(plain, "2.1 MB") || !strings.Contains(plain, "4.8 MB") {
		t.Fatalf("determinate load missing byte counts: %q", plain)
	}
	filled := strings.Count(plain, "▰")
	empty := strings.Count(plain, "▱")
	if filled+empty != loadBarWidth {
		t.Fatalf("bar width = %d, want %d", filled+empty, loadBarWidth)
	}
	// ~44%% of the bar should be filled.
	if filled < loadBarWidth*35/100 || filled > loadBarWidth*55/100 {
		t.Fatalf("bar fill %d/%d does not reflect 2.1/4.8 MB", filled, loadBarWidth)
	}
}

func TestLoadingLineClampsToWidth(t *testing.T) {
	th := theme.ForMode(theme.ModeBramble)
	now := time.Now()
	load := &pendingLoad{
		kind:    loadKindDetail,
		label:   strings.Repeat("a very long label ", 20),
		started: now.Add(-time.Second),
	}
	for _, width := range []int{20, 40, 78} {
		plain := ansi.Strip(loadingLine(th, load, width, now))
		if got := ansi.StringWidth(plain); got > width {
			t.Fatalf("loading line width %d exceeds %d: %q", got, width, plain)
		}
	}
}

func TestLoadingLineBothThemesShareGeometry(t *testing.T) {
	now := time.Now()
	load := &pendingLoad{kind: loadKindRuns, label: "sniffing out failing runs", started: now.Add(-time.Second)}
	bramble := ansi.Strip(loadingLine(theme.ForMode(theme.ModeBramble), load, 80, now))
	bone := ansi.Strip(loadingLine(theme.ForMode(theme.ModeBone), load, 80, now))
	if bramble != bone {
		t.Fatalf("themes diverge in geometry:\nbramble %q\nbone    %q", bramble, bone)
	}
}
