package fold

import (
	"fmt"
	"slices"
	"strings"
)

// Summary describes what a fold hides: how many lines, the time span, the
// values that varied and how many lines look like errors.
type Summary struct {
	Run       Run
	Hidden    int
	FirstLine int // 1-based, first hidden line
	LastLine  int // 1-based, last hidden line
	Shapes    int // set folds: distinct recurring shapes hidden
	Severe    int // hidden lines that look like errors (flagged, not pinned)
	TimeFrom  string
	TimeTo    string
	Varying   []SlotVariation
}

// SlotVariation is a masked position whose value varied; Few lists the
// values when there are only a handful.
type SlotVariation struct {
	Distinct    int
	Few         []string
	First, Last string
}

const (
	fewLimit      = 4
	distinctLimit = 1000
	summaryWindow = 64 // set folds: witness lines scanned before the fold
)

// Summarize reads the fold and its visible witnesses, so a value that differs
// only between hidden and visible copies is still reported.
func Summarize(seg Segment, lines []string, n Normalizer) Summary {
	r := seg.Run
	s := Summary{Run: r, Hidden: seg.End - seg.Start, FirstLine: seg.Start + 1, LastLine: seg.End}
	slotter, _ := n.(Slotter)

	type pos struct {
		group string // cycle position, or shape for set runs
		idx   int
	}
	type acc struct {
		values      map[string]bool
		first, last string
		samples     int
	}
	accs := map[pos]*acc{}
	var order []pos
	shapes := map[string]bool{}
	// A set run can fold many times, so each fold scans only a bounded
	// window of witnesses; cycle runs fold once and are scanned whole.
	from, to := r.Start, r.End()
	if r.Set {
		from, to = max(r.Start, seg.Start-summaryWindow), min(r.End(), seg.End+1)
	}
	for i := from; i < to; i++ {
		shape := n.Normalize(lines[i])
		shapes[shape] = true
		if i >= seg.Start && i < seg.End && containsSeverity(lines[i]) {
			s.Severe++
		}
		if slotter == nil {
			continue
		}
		for idx, slot := range slotter.Slots(lines[i]) {
			if isClock(slot.Value) {
				if s.TimeFrom == "" {
					s.TimeFrom = slot.Value
				}
				s.TimeTo = slot.Value
				continue
			}
			g := shape
			if !r.Set {
				g = fmt.Sprint((i - r.Start) % r.Period)
			}
			k := pos{g, idx}
			a := accs[k]
			if a == nil {
				a = &acc{values: map[string]bool{}, first: slot.Value}
				accs[k] = a
				order = append(order, k)
			}
			if len(a.values) < distinctLimit {
				a.values[slot.Value] = true
			}
			a.last = slot.Value
			a.samples++
		}
	}
	s.Shapes = len(shapes)
	if !r.Set {
		s.Shapes = r.Period
	}
	for _, k := range order {
		a := accs[k]
		if len(a.values) < 2 {
			continue
		}
		v := SlotVariation{Distinct: len(a.values), First: a.first, Last: a.last}
		// list values only when they recur; all-distinct values are a range
		if len(a.values) <= fewLimit && a.samples > len(a.values) {
			for val := range a.values {
				v.Few = append(v.Few, val)
			}
			slices.Sort(v.Few)
		}
		s.Varying = append(s.Varying, v)
	}
	slices.SortStableFunc(s.Varying, func(a, b SlotVariation) int { return a.Distinct - b.Distinct })
	return s
}

// isClock reports a wall-clock H:MM:SS that is not part of an IPv6 address.
func isClock(v string) bool {
	for i := 0; i+7 <= len(v); i++ {
		if i > 0 && isDigit(v[i-1]) {
			continue
		}
		j := i
		if isDigit(v[j]) && j+1 < len(v) && isDigit(v[j+1]) {
			j += 2
		} else if isDigit(v[j]) {
			j++
		} else {
			continue
		}
		if j+6 <= len(v) && v[j] == ':' && isDigit(v[j+1]) && isDigit(v[j+2]) && v[j+3] == ':' && isDigit(v[j+4]) && isDigit(v[j+5]) &&
			(j+6 == len(v) || !isDigit(v[j+6]) && v[j+6] != ':') &&
			(i == 0 || v[i-1] != ':' && !isHexLetter(v[i-1])) && !strings.Contains(v, "::") {
			return true // H:MM:SS not glued to more colon groups (IPv6)
		}
	}
	return false
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHexLetter(c byte) bool { c |= 0x20; return c >= 'a' && c <= 'f' }

// Span is the fold's first and last wall-clock time (HH:MM:SS), if any.
func (s Summary) Span() (string, string) {
	if s.TimeFrom == "" {
		return "", ""
	}
	return shortClock(s.TimeFrom), shortClock(s.TimeTo)
}

// Marker is the one-line stand-in for a fold in plain text output.
func (s Summary) Marker() string {
	return fmt.Sprintf("⋯ %s · %d lines hidden (L%d–L%d)", s.Title(), s.Hidden, s.FirstLine, s.LastLine)
}

// Title describes the fold without its size: shape, time span, varying
// values, then an error warning.
func (s Summary) Title() string {
	var b strings.Builder
	if s.Run.Set {
		fmt.Fprintf(&b, "%d recurring line%s", s.Shapes, plural(s.Shapes))
	} else {
		fmt.Fprintf(&b, "×%s", groupDigits(s.Run.Count))
		if s.Run.Period > 1 {
			fmt.Fprintf(&b, " %d-line block", s.Run.Period)
		}
	}
	if from, to := shortClock(s.TimeFrom), shortClock(s.TimeTo); s.TimeFrom != "" && from != to {
		fmt.Fprintf(&b, " · %s → %s", from, to)
	}
	seen := map[string]bool{} // a value repeated in one line (tag -> tag) is listed once
	shown, ids := 0, 0
	for _, v := range s.Varying {
		sig := v.First + "\x00" + v.Last + "\x00" + strings.Join(v.Few, "|")
		if seen[sig] {
			continue
		}
		seen[sig] = true
		if len(v.Few) == 0 && !decimal(v.First) {
			ids++ // pointers, hashes, uuids: a range says nothing
			continue
		}
		if shown == 3 {
			continue
		}
		shown++
		switch {
		case len(v.Few) > 0:
			fmt.Fprintf(&b, " · %s", strings.Join(v.Few, "|"))
		case v.Distinct >= distinctLimit:
			fmt.Fprintf(&b, " · %s → %s (%d+ values)", short(v.First), short(v.Last), v.Distinct)
		default:
			fmt.Fprintf(&b, " · %s → %s (%d values)", short(v.First), short(v.Last), v.Distinct)
		}
	}
	if ids > 0 {
		fmt.Fprintf(&b, " · %d changing id%s", ids, plural(ids))
	}
	if s.Severe > 0 {
		fmt.Fprintf(&b, " · %s %d error-like inside", SevereGlyph, s.Severe)
	}
	return b.String()
}

// SevereGlyph prefixes the error warning in a Title.
const SevereGlyph = "⚠"

func shortClock(v string) string {
	if t := strings.IndexByte(v, 'T'); t >= 0 && t+9 <= len(v) {
		return v[t+1 : t+9]
	}
	if sp := strings.LastIndexByte(v, ' '); sp >= 0 {
		v = v[sp+1:]
	}
	if len(v) > 8 && v[2] == ':' {
		return v[:8]
	}
	return v
}

func decimal(v string) bool {
	digits := 0
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case isDigit(c):
			digits++
		case c == '.' || c == '%':
		case c|0x20 >= 'a' && c|0x20 <= 'z' && digits > 0:
		default:
			return false
		}
	}
	return digits > 0 && !strings.HasPrefix(v, "0x")
}

func short(v string) string {
	if len(v) > 12 {
		return v[:8] + "…"
	}
	return v
}

func groupDigits(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
