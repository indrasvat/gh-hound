package caches

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/numfmt"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/components/textinput"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

const gaugeWidth = 30

func View(m Model, width int, now time.Time) string {
	return ViewSize(m, width, 0, now)
}

func ViewSize(m Model, width, height int, now time.Time) string {
	if width <= 0 {
		width = 80
	}
	st := stylesFor(m.theme())
	lines := []string{
		fitANSI(joinRightANSI(usageHeadline(st, m), sortLabel(st, m.SortBy), width), width),
		gaugeLine(st, m, width),
	}
	if usecase.CacheNearCap(m.Usage, m.Cap) {
		lines = append(lines, fitANSI(colorize(st.warn, "  kennel's almost full — GitHub starts evicting at 10 GB."), width))
	}
	lines = append(lines, "")

	visible := m.VisibleCaches()
	showFilter := m.InputMode || strings.TrimSpace(m.Filter) != ""
	if showFilter {
		count := numfmt.Group(len(visible)) + " matches"
		if len(visible) == 1 {
			count = "1 match"
		}
		tail := "  " + count + " · ⎋ clear"
		if m.InputMode {
			tail = "▌  " + count
		}
		filter := textinput.Tail(m.Filter, width-3-ansi.StringWidth(tail))
		lines = append(lines, dimLine(st, "  /"+filter+tail, width))
	}
	if len(m.Caches) == 0 {
		lines = append(lines, dimLine(st, "  the kennel's empty — nothing cached on this repo.", width))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, cachesHeader(st, width))
	if len(visible) == 0 {
		lines = append(lines, dimLine(st, "  no caches match /"+m.Filter, width))
		return strings.Join(lines, "\n")
	}

	selected := clampSelection(m.Selected, len(visible))
	fixedRows := len(lines) + 1
	capacity := rowCapacity(height, fixedRows, len(visible))
	start, end := viewport(selected, len(visible), capacity)
	for i, cache := range visible[start:end] {
		line := row(st, cache, start+i == selected, width, now)
		if m.Loading {
			line = dimLine(st, ansi.Strip(line), width)
		}
		lines = append(lines, line)
	}
	lines = append(lines, dimLine(st, pageLine(start, end, len(visible)), width))
	if m.Loading && m.LoadingLine != "" {
		lines = append(lines, fitANSI(m.LoadingLine, width))
	}
	return strings.Join(lines, "\n")
}

// UsageLine is the hound-voiced gauge headline shared with the app
// chrome: `kennel: 7.2/10 GB`.
func UsageLine(usage model.CacheUsage, capBytes int64) string {
	if capBytes <= 0 {
		capBytes = usecase.CacheCapFallbackBytes
	}
	return "kennel: " + humanGB(usage.ActiveSizeInBytes) + "/" + humanGB(capBytes) + " GB"
}

func usageHeadline(st styles, m Model) string {
	caches := "caches"
	if m.Usage.ActiveCount == 1 {
		caches = "cache"
	}
	return "  " + colorize(st.fgSoft, UsageLine(m.Usage, m.Cap)) +
		colorize(st.subtle, fmt.Sprintf(" · %d %s", m.Usage.ActiveCount, caches))
}

// gaugeLine renders the themed pressure bar. The fill never overflows
// the bar even when usage runs past the cap (eviction lags), but the
// color always tells the truth: ok under 50%, run to 90%, fail past
// the eviction threshold.
func gaugeLine(st styles, m Model, width int) string {
	pressure := usecase.CachePressure(m.Usage, m.Cap)
	bar := min(gaugeWidth, max(width-6, 5))
	filled := min(max(int(float64(bar)*pressure+0.5), 0), bar)
	if pressure > 0 && filled == 0 {
		filled = 1
	}
	color := st.ok
	switch {
	case pressure > 0.9:
		color = st.fail
	case pressure >= 0.5:
		color = st.run
	}
	value := "  " + colorize(color, strings.Repeat("▰", filled)) + colorize(st.line2, strings.Repeat("▱", bar-filled)) +
		colorize(st.subtle, " "+strconv.Itoa(int(pressure*100+0.5))+"%")
	return fitANSI(value, width)
}

func sortLabel(st styles, by usecase.CacheSort) string {
	if by == usecase.CacheSortLastUsed {
		return colorize(st.subtle, "s sort: last used · stalest first")
	}
	return colorize(st.subtle, "s sort: size · biggest first")
}

// cachesHeader derives its columns from the SAME width math as row()
// so headers can never drift from the values beneath them.
func cachesHeader(st styles, width int) string {
	keyWidth, refWidth := columnWidths(width)
	header := "  " +
		pad("Key", keyWidth) + "  " +
		pad("Ref", refWidth) + " " +
		fmt.Sprintf("%8s", "Size") + "  " +
		"Last used"
	return dimLine(st, header, width)
}

// columnWidths is the single source of column truth for header and rows.
func columnWidths(width int) (keyWidth, refWidth int) {
	if width >= 100 {
		return 60, 22
	}
	return 40, 16
}

func pad(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func row(st styles, cache model.Cache, selected bool, width int, now time.Time) string {
	prefix := " "
	if selected {
		prefix = colorize(st.ok, icons.Cursor)
	}
	keyWidth, refWidth := columnWidths(width)
	key := colorize(st.fgSoft, truncate(cache.Key, keyWidth))
	ref := colorize(st.subtle, truncate(shortRef(cache.Ref), refWidth))
	size := colorize(st.muted, fmt.Sprintf("%8s", humanSize(cache.SizeInBytes)))
	used := colorize(st.subtle, age(cache.LastAccessedAt, now))
	line := prefix + " " +
		padANSI(key, keyWidth) + "  " +
		padANSI(ref, refWidth) + " " +
		size + "  " +
		used
	return fitANSI(line, width)
}

// shortRef drops the refs/heads/ noise; PR merge refs stay explicit.
func shortRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func age(at time.Time, now time.Time) string {
	if at.IsZero() || now.Before(at) {
		return "now"
	}
	elapsed := now.Sub(at)
	switch {
	case elapsed < time.Minute:
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	default:
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	}
}

// humanGB renders GiB with one decimal, trimming the trailing .0 so
// the cap reads `10`.
func humanGB(bytes int64) string {
	value := float64(bytes) / float64(1<<30)
	return strings.TrimSuffix(strconv.FormatFloat(value, 'f', 1, 64), ".0")
}

// HumanSize is the row size label (also used by the app's delete
// confirms): 1024-based with one decimal.
func HumanSize(bytes int64) string {
	return humanSize(bytes)
}

func humanSize(bytes int64) string {
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

func rowCapacity(height, fixedRows, total int) int {
	if height <= 0 {
		return total
	}
	capacity := height - fixedRows
	if total > capacity {
		capacity--
	}
	return max(capacity, 1)
}

func viewport(selected, total, capacity int) (int, int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	if capacity >= total {
		return 0, total
	}
	start := max(selected-capacity/2, 0)
	if start+capacity > total {
		start = total - capacity
	}
	return start, start + capacity
}

func pageLine(start, end, total int) string {
	if total == 0 {
		return "0/0"
	}
	return fmt.Sprintf("  rows %d-%d/%d", start+1, end, total)
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func dimLine(st styles, value string, width int) string {
	return colorize(st.subtle, padANSI(fitANSI(value, width), width))
}

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
}

func fitANSI(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}

func padANSI(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func joinRightANSI(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = fitANSI(right, width)
	leftWidth := max(width-ansi.StringWidth(right)-2, 1)
	left = fitANSI(left, leftWidth)
	spaces := max(width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", spaces) + right
}

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours the kennel.
type styles struct {
	ok, fail, run, warn, muted, subtle, fgSoft, line2 string
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), run: theme.Fg(th.Run), warn: theme.Fg(th.Warn),
		muted: theme.Fg(th.Muted), subtle: theme.Fg(th.Subtle), fgSoft: theme.Fg(th.FGSoft), line2: theme.Fg(th.Line2),
	}
}

func (m Model) theme() theme.Theme {
	if m.Theme.Mode == "" {
		return theme.ForMode(theme.ModeBramble)
	}
	return m.Theme
}

const sgrReset = "\x1b[0m"
