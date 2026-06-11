package caches

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
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
	lines := []string{
		fitANSI(joinRightANSI(usageHeadline(m), sortLabel(m.SortBy), width), width),
		gaugeLine(m, width),
	}
	if usecase.CacheNearCap(m.Usage, m.Cap) {
		lines = append(lines, fitANSI(colorize(sgrWarn, "  kennel's almost full — GitHub starts evicting at 10 GB."), width))
	}
	lines = append(lines, "")

	visible := m.VisibleCaches()
	showFilter := m.InputMode || strings.TrimSpace(m.Filter) != ""
	if showFilter {
		lines = append(lines, dimLine(fmt.Sprintf("  /%s  %d matches", m.Filter, len(visible)), width))
	}
	if len(m.Caches) == 0 {
		lines = append(lines, dimLine("  the kennel's empty — nothing cached on this repo.", width))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, cachesHeader(width))
	if len(visible) == 0 {
		lines = append(lines, dimLine("  no caches match /"+m.Filter, width))
		return strings.Join(lines, "\n")
	}

	selected := clampSelection(m.Selected, len(visible))
	fixedRows := len(lines) + 1
	capacity := rowCapacity(height, fixedRows, len(visible))
	start, end := viewport(selected, len(visible), capacity)
	for i, cache := range visible[start:end] {
		line := row(cache, start+i == selected, width, now)
		if m.Loading {
			line = dimLine(ansi.Strip(line), width)
		}
		lines = append(lines, line)
	}
	lines = append(lines, dimLine(pageLine(start, end, len(visible)), width))
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

func usageHeadline(m Model) string {
	caches := "caches"
	if m.Usage.ActiveCount == 1 {
		caches = "cache"
	}
	return "  " + colorize(sgrFGSoft, UsageLine(m.Usage, m.Cap)) +
		colorize(sgrSubtle, fmt.Sprintf(" · %d %s", m.Usage.ActiveCount, caches))
}

// gaugeLine renders the themed pressure bar. The fill never overflows
// the bar even when usage runs past the cap (eviction lags), but the
// color always tells the truth: ok under 50%, run to 90%, fail past
// the eviction threshold.
func gaugeLine(m Model, width int) string {
	pressure := usecase.CachePressure(m.Usage, m.Cap)
	bar := min(gaugeWidth, max(width-6, 5))
	filled := min(max(int(float64(bar)*pressure+0.5), 0), bar)
	if pressure > 0 && filled == 0 {
		filled = 1
	}
	color := sgrOK
	switch {
	case pressure > 0.9:
		color = sgrFail
	case pressure >= 0.5:
		color = sgrRun
	}
	value := "  " + colorize(color, strings.Repeat("▰", filled)) + colorize(sgrLine2, strings.Repeat("▱", bar-filled)) +
		colorize(sgrSubtle, " "+strconv.Itoa(int(pressure*100+0.5))+"%")
	return fitANSI(value, width)
}

func sortLabel(by usecase.CacheSort) string {
	if by == usecase.CacheSortLastUsed {
		return colorize(sgrSubtle, "s sort: last used · stalest first")
	}
	return colorize(sgrSubtle, "s sort: size · biggest first")
}

func cachesHeader(width int) string {
	if width >= 100 {
		return dimLine("  Key                                                          Ref                     Size  Last used", width)
	}
	return dimLine("  Key                                      Ref               Size  Last used", width)
}

func row(cache model.Cache, selected bool, width int, now time.Time) string {
	prefix := " "
	if selected {
		prefix = colorize(sgrOK, icons.Cursor)
	}
	keyWidth, refWidth := 40, 16
	if width >= 100 {
		keyWidth, refWidth = 60, 22
	}
	key := colorize(sgrFGSoft, truncate(cache.Key, keyWidth))
	ref := colorize(sgrSubtle, truncate(shortRef(cache.Ref), refWidth))
	size := colorize(sgrMuted, fmt.Sprintf("%8s", humanSize(cache.SizeInBytes)))
	used := colorize(sgrSubtle, age(cache.LastAccessedAt, now))
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

func dimLine(value string, width int) string {
	return colorize(sgrSubtle, padANSI(fitANSI(value, width), width))
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

const (
	sgrOK     = "\x1b[38;2;79;211;122m"
	sgrFail   = "\x1b[38;2;226;86;75m"
	sgrRun    = "\x1b[38;2;224;163;62m"
	sgrWarn   = "\x1b[38;2;232;137;90m"
	sgrMuted  = "\x1b[38;2;174;179;155m"
	sgrSubtle = "\x1b[38;2;140;145;121m"
	sgrFGSoft = "\x1b[38;2;207;205;187m"
	sgrLine2  = "\x1b[38;2;61;66;51m"
	sgrReset  = "\x1b[0m"
)
