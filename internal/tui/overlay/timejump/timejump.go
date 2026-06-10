// Package timejump is the log screen's time-navigation modal: a picker
// over the document's interesting moments (steps, the failure window,
// the slowest gaps) plus typed absolute, relative, and range queries.
package timejump

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/indrasvat/gh-hound/internal/logs"
)

type EntryKind string

const (
	EntryStep    EntryKind = "step"
	EntryFailure EntryKind = "failure"
	EntryGap     EntryKind = "gap"
)

type Entry struct {
	Kind  EntryKind
	Label string
	Clock string
	Line  int
}

type ActionKind string

const (
	ActionNone     ActionKind = ""
	ActionJump     ActionKind = "jump"
	ActionRelative ActionKind = "relative"
	ActionRange    ActionKind = "range"
	ActionInvalid  ActionKind = "invalid"
)

type Action struct {
	Kind         ActionKind
	Line         int
	EndLine      int
	DeltaSeconds float64
	Message      string
}

type Model struct {
	Entries  []Entry
	Selected int
	Input    string
	Feedback string
	timeline []logs.Stamp
}

const (
	maxGapEntries = 3
	minGapSeconds = 5
)

func New(doc logs.Document) Model {
	timeline := logs.Timeline(doc)
	clockAt := func(line int) string {
		for _, stamp := range timeline {
			if stamp.LineNumber >= line {
				return stamp.Clock
			}
		}
		return ""
	}
	var entries []Entry
	for _, fold := range doc.Folds {
		entries = append(entries, Entry{
			Kind:  EntryStep,
			Label: fold.Title,
			Clock: shortClock(clockAt(fold.StartLine)),
			Line:  fold.StartLine,
		})
	}
	if doc.Failure.Found {
		entries = append(entries, Entry{
			Kind:  EntryFailure,
			Label: "failure window",
			Clock: shortClock(clockAt(doc.Failure.AnchorLine)),
			Line:  doc.Failure.AnchorLine,
		})
	}
	entries = append(entries, gapEntries(timeline)...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Line < entries[j].Line })
	return Model{Entries: entries, timeline: timeline}
}

// gapEntries ranks the largest pauses between consecutive stamped
// lines: the "where did the time go?" affordance.
func gapEntries(timeline []logs.Stamp) []Entry {
	type gap struct {
		delta float64
		stamp logs.Stamp
	}
	var gaps []gap
	for i := 1; i < len(timeline); i++ {
		delta := timeline[i].Seconds - timeline[i-1].Seconds
		if delta >= minGapSeconds {
			gaps = append(gaps, gap{delta: delta, stamp: timeline[i]})
		}
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].delta > gaps[j].delta })
	if len(gaps) > maxGapEntries {
		gaps = gaps[:maxGapEntries]
	}
	entries := make([]Entry, 0, len(gaps))
	for _, g := range gaps {
		entries = append(entries, Entry{
			Kind:  EntryGap,
			Label: fmt.Sprintf("+%s gap", humanSeconds(g.delta)),
			Clock: shortClock(g.stamp.Clock),
			Line:  g.stamp.LineNumber,
		})
	}
	return entries
}

func (m Model) Update(key string) Model {
	switch key {
	case "j", "down":
		if m.Input == "" && m.Selected < len(m.Entries)-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Input == "" && m.Selected > 0 {
			m.Selected--
		}
	case "backspace":
		if len(m.Input) > 0 {
			m.Input = m.Input[:len(m.Input)-1]
		}
		m.Feedback = ""
	default:
		if len([]rune(key)) == 1 && strings.ContainsAny(key, "0123456789:+-smh.") {
			m.Input += key
			m.Feedback = ""
		}
	}
	return m
}

var (
	relativeRE = regexp.MustCompile(`^([+-])(\d+(?:\.\d+)?)([smh])$`)
	absoluteRE = regexp.MustCompile(`^\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?$`)
)

// padHour zero-pads single-digit hours so lexical clock comparison
// works against the runner's zero-padded timestamps ("1:05" -> "01:05").
func padHour(clock string) string {
	if idx := strings.IndexByte(clock, ':'); idx == 1 {
		return "0" + clock
	}
	return clock
}

// Commit resolves enter: the selected entry when no input was typed,
// otherwise the parsed query. Invalid queries return feedback and keep
// the modal open.
func (m Model) Commit() (Model, Action) {
	input := strings.TrimSpace(m.Input)
	if input == "" {
		if len(m.Entries) == 0 {
			m.Feedback = "no timestamped landmarks in this log"
			return m, Action{Kind: ActionInvalid, Message: m.Feedback}
		}
		entry := m.Entries[m.Selected]
		return m, Action{Kind: ActionJump, Line: entry.Line}
	}
	if match := relativeRE.FindStringSubmatch(input); match != nil {
		value, _ := strconv.ParseFloat(match[2], 64)
		unit := map[string]float64{"s": 1, "m": 60, "h": 3600}[match[3]]
		delta := value * unit
		if match[1] == "-" {
			delta = -delta
		}
		return m, Action{Kind: ActionRelative, DeltaSeconds: delta}
	}
	if start, end, ok := strings.Cut(input, "-"); ok && absoluteRE.MatchString(start) && absoluteRE.MatchString(end) {
		first, lastLine, found := m.resolveRange(padHour(start), padHour(end))
		if !found {
			m.Feedback = fmt.Sprintf("no lines between %s and %s", start, end)
			return m, Action{Kind: ActionInvalid, Message: m.Feedback}
		}
		return m, Action{Kind: ActionRange, Line: first, EndLine: lastLine}
	}
	if absoluteRE.MatchString(input) {
		if line, ok := resolveClock(m.timeline, padHour(input)); ok {
			return m, Action{Kind: ActionJump, Line: line}
		}
		m.Feedback = fmt.Sprintf("no line at/after %s", input)
		return m, Action{Kind: ActionInvalid, Message: m.Feedback}
	}
	m.Feedback = "use HH:MM[:SS], +30s/-2m, or A-B"
	return m, Action{Kind: ActionInvalid, Message: m.Feedback}
}

// resolveClock mirrors the log screen's day-span semantics: the query
// resolves inside the first day whose span contains it.
func resolveClock(timeline []logs.Stamp, query string) (int, bool) {
	if len(timeline) == 0 {
		return 0, false
	}
	maxDay := timeline[len(timeline)-1].Day
	for day := 0; day <= maxDay; day++ {
		first := ""
		for _, stamp := range timeline {
			if stamp.Day == day {
				first = stamp.Clock
				break
			}
		}
		if day == 0 && clockBefore(query, first) {
			continue
		}
		for _, stamp := range timeline {
			if stamp.Day == day && stamp.Clock >= query {
				return stamp.LineNumber, true
			}
		}
	}
	if maxDay == 0 && clockBefore(query, timeline[0].Clock) {
		return timeline[0].LineNumber, true
	}
	return 0, false
}

// resolveRange works in day-aware effective seconds so multi-day logs
// cannot leak later days into the window, while ranges that genuinely
// cross midnight (end clock below start clock) still span the wrap.
func (m Model) resolveRange(start, end string) (int, int, bool) {
	startLine, ok := resolveClock(m.timeline, start)
	if !ok {
		return 0, 0, false
	}
	var startStamp logs.Stamp
	for _, stamp := range m.timeline {
		if stamp.LineNumber == startLine {
			startStamp = stamp
			break
		}
	}
	endClock, ok := logs.ClockSeconds(end)
	if !ok {
		return 0, 0, false
	}
	// Minute-precision ends include the whole minute.
	if len(strings.Split(end, ":")) == 2 {
		endClock += 59.999
	}
	endSeconds := float64(startStamp.Day)*86400 + endClock
	if endSeconds < startStamp.Seconds {
		endSeconds += 86400 // range crosses midnight
	}
	last := 0
	for _, stamp := range m.timeline {
		if stamp.LineNumber >= startLine && stamp.Seconds <= endSeconds {
			last = stamp.LineNumber
		}
	}
	if last < startLine {
		return 0, 0, false
	}
	return startLine, last, true
}

// clockBefore reports query < clock with prefix semantics: a query
// that is a prefix of the clock ("10:00" vs "10:00:00.000") is not
// before it.
func clockBefore(query, clock string) bool {
	if len(clock) > len(query) {
		clock = clock[:len(query)]
	}
	return query < clock
}

func shortClock(clock string) string {
	if idx := strings.IndexByte(clock, '.'); idx > 0 {
		return clock[:idx]
	}
	return clock
}

func humanSeconds(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	return fmt.Sprintf("%dm%02ds", int(seconds)/60, int(seconds)%60)
}

func View(m Model, width int) string {
	lines := []string{"t→" + m.Input + "▌", ""}
	for i, entry := range m.Entries {
		cursor := "  "
		if m.Input == "" && i == m.Selected {
			cursor = "▌ "
		}
		marker := "▸"
		if entry.Kind == EntryGap {
			marker = "◆"
		}
		if entry.Kind == EntryFailure {
			marker = "✗"
		}
		lines = append(lines, fit(fmt.Sprintf("%s%s %s  %s", cursor, marker, entry.Clock, entry.Label), width))
	}
	if m.Feedback != "" {
		lines = append(lines, "", fit("! "+m.Feedback, width))
	}
	lines = append(lines, "", fit("j/k pick · HH:MM[:SS] · +30s/-2m relative · A-B range · ⏎ go · ⎋ cancel", width))
	return strings.Join(lines, "\n")
}

func fit(value string, width int) string {
	runes := []rune(value)
	if width <= 0 || len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
