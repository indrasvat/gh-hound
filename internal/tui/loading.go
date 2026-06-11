package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
)

// The Task 220 invariant: no keystroke may block on the network. The
// UI repaints within 50ms; if a fetch is still in flight after the
// grace window, the shared loading indicator appears. There is exactly
// one loading indicator in gh-hound — this file. Per-screen spinners
// are a review-blocking defect (docs/visual-contract.md).
const (
	loadGraceDelay    = 100 * time.Millisecond
	loadFrameInterval = 120 * time.Millisecond
	loadBarWidth      = 5
)

type loadKind string

const (
	loadKindRuns      loadKind = "runs"
	loadKindDetail    loadKind = "detail"
	loadKindFailure   loadKind = "failure"
	loadKindLog       loadKind = "log"
	loadKindDispatch  loadKind = "dispatch"
	loadKindWatch     loadKind = "watch"
	loadKindApprovals loadKind = "approvals"
	loadKindDiff      loadKind = "diff"
)

// pendingLoad is the app's single in-flight fetch. Supersession and
// cancellation work by pointer identity: replacing or clearing
// App.load orphans the old goroutine's result, so stale responses can
// never apply. The goroutine only writes its own state and exits — no
// leak is possible.
type pendingLoad struct {
	mu      sync.Mutex
	kind    loadKind
	label   string
	started time.Time

	// Byte progress for fetches with a knowable size (logs). total <= 0
	// renders the indeterminate spinner-only line.
	read  int64
	total int64

	done  bool
	apply func(App) App

	// cancel stops the underlying work (network included) when the
	// load is superseded or esc-cancelled — orphaning the result is
	// not enough, the serial queue must be freed too.
	cancel context.CancelFunc
}

func (p *pendingLoad) finish(apply func(App) App) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apply = apply
	p.done = true
}

func (p *pendingLoad) progress(read, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.read = read
	p.total = total
}

func (p *pendingLoad) snapshot() (done bool, apply func(App) App, read, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done, p.apply, p.read, p.total
}

// loadingBody is the shared whole-pane loading state for screens that
// have nothing useful to show until the fetch lands (failure, log).
// Same component, same geometry — only the label differs.
func loadingBody(th theme.Theme, load *pendingLoad, width int, now time.Time) string {
	line := loadingLine(th, load, width, now)
	if line == "" {
		// Inside the grace window the pane stays blank rather than
		// flashing a frame.
		return ""
	}
	return "\n" + line
}

// spinnerGlyph derives the frame purely from elapsed time so renders
// stay stateless and fixtures can pin a frame via elapsed.
func spinnerGlyph(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	idx := int(elapsed/loadFrameInterval) % len(icons.SpinnerFrames)
	return icons.SpinnerFrames[idx]
}

// loadingLine renders the shared loading indicator: one line, spinner
// + hound-voiced label, plus a determinate byte bar when the total is
// known. Empty inside the grace window so fast paths never flash.
func loadingLine(th theme.Theme, load *pendingLoad, width int, now time.Time) string {
	if load == nil || width <= 0 {
		return ""
	}
	elapsed := now.Sub(load.started)
	if elapsed < loadGraceDelay {
		return ""
	}
	_, _, read, total := load.snapshot()
	glyph := spinnerGlyph(elapsed)
	label := load.label + "…"
	suffix := ""
	if total > 0 {
		filled := min(max(int(float64(loadBarWidth)*float64(read)/float64(total)+0.5), 0), loadBarWidth)
		bar := strings.Repeat("▰", filled) + strings.Repeat("▱", loadBarWidth-filled)
		suffix = fmt.Sprintf(" %s %s/%s", bar, humanBytes(read), humanBytes(total))
	}
	plain := " " + glyph + " " + label + suffix
	if ansi.StringWidth(plain) > width {
		plain = ansi.Truncate(plain, width-1, "…")
	}
	spin := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Run)).Render
	text := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted)).Render
	// Re-split styled parts on the truncated plain text so styling never
	// changes geometry: glyph in the run color, the rest muted.
	if before, after, ok := strings.Cut(plain, glyph); ok {
		return before + spin(glyph) + text(after)
	}
	return text(plain)
}

func humanBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
