package fold

import "strings"

// Shape strips terminal escapes and replaces volatile values with '#':
// numbers, durations, clocks, timestamps, hashes, pointers and uuids, and the
// random-looking parts of identifiers (w-a3f9c1 -> w-#). Named ordinals
// (shard-7, TestFoo2, abc100) and short outcome values (exit code 137) stay
// literal, so distinct names and outcomes never merge.
type Shape struct{}

func (Shape) Name() string { return "shape" }

func (Shape) Normalize(line string) string {
	var b strings.Builder
	b.Grow(len(line))
	scanShape(line, func(a, z int, volatile bool) {
		if volatile {
			b.WriteByte('#')
		} else {
			b.WriteString(line[a:z])
		}
	})
	return strings.TrimRight(b.String(), " \t\r")
}

func (Shape) KeyAndSpans(dst []Span, line string) (string, []Span) {
	var b strings.Builder
	b.Grow(len(line))
	scanShape(line, func(a, z int, volatile bool) {
		if volatile {
			b.WriteByte('#')
			dst = append(dst, Span{int32(a), int32(z)})
		} else {
			b.WriteString(line[a:z])
		}
	})
	return strings.TrimRight(b.String(), " \t\r"), dst
}

func (Shape) Slots(line string) []Slot {
	var dst []Slot
	scanShape(line, func(a, z int, volatile bool) {
		if volatile {
			dst = append(dst, Slot{Class: "#", Value: line[a:z]})
		}
	})
	return dst
}

func scanShape(line string, emit func(start, end int, volatile bool)) {
	lit := 0 // start of pending literal text
	flush := func(end int) {
		if end > lit {
			emit(lit, end, false)
		}
	}
	semantic := 0 // words left during which values stay literal
	for i := 0; i < len(line); {
		c := line[i]
		if c == 0x1b {
			flush(i)
			i += escapeLen(line, i)
			lit = i
			continue
		}
		if !isWordByte(c) {
			i++
			continue
		}
		j := i
		for j < len(line) && isWordByte(line[j]) {
			j++
		}
		word := line[i:j]
		keep := false
		if !punctOnly(word) {
			keep = semantic > 0 && shortNumber(word)
			switch {
			case isSemanticKey(word):
				semantic = 2 // "exit code 137", "HTTP/1.1 503"
			case keep:
				semantic-- // a second number may follow (HTTP/1.1 503)
			default:
				semantic = 0 // "status update 5": the value is not next
			}
		}
		switch {
		case keep:
		case volatileWord(word):
			flush(i)
			emit(i, j, true)
			lit = j
		case hasJoiner(word):
			start := i
			for k := i; k <= j; k++ {
				if k < j && line[k] != '-' && line[k] != '_' {
					continue
				}
				if part := line[start:k]; randomPart(part) {
					flush(start)
					emit(start, k, true)
					lit = k
				}
				start = k + 1
			}
		}
		i = j
	}
	flush(len(line))
}

func hasJoiner(w string) bool {
	for i := 0; i < len(w); i++ {
		if w[i] == '-' || w[i] == '_' {
			return true
		}
	}
	return false
}

// escapeLen is the length of the escape at line[i] (ESC): CSI, OSC ending in
// BEL or ST, or a two-byte escape; truncated ones run to end of line.
func escapeLen(line string, i int) int {
	if i+1 >= len(line) {
		return 1
	}
	switch line[i+1] {
	case '[':
		j := i + 2
		for j < len(line) && (line[j] < '@' || line[j] > '~') {
			j++
		}
		return min(j+1, len(line)) - i
	case ']':
		for j := i + 2; j < len(line); j++ {
			if line[j] == 0x07 {
				return j + 1 - i
			}
			if line[j] == 0x1b && j+1 < len(line) && line[j+1] == '\\' {
				return j + 2 - i
			}
		}
		return len(line) - i
	default:
		return 2
	}
}

func isWordByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c == '.' || c == ':' || c == '-' || c == '_'
}

var units = map[string]bool{
	"ns": true, "us": true, "ms": true, "s": true, "m": true, "h": true,
	"b": true, "kb": true, "mb": true, "gb": true, "kib": true, "mib": true, "gib": true,
	"t": true, "z": true, // ISO 'T'/'Z'
}

// semanticKeys precede outcome values that are part of a line's shape.
var semanticKeys = map[string]bool{
	"exit": true, "code": true, "status": true, "rc": true, "state": true, "phase": true,
	"result": true, "reason": true, "http": true, "signal": true, "errno": true, "conclusion": true,
}

func shortNumber(w string) bool {
	if w == "" || len(w) > 8 {
		return false
	}
	digits := 0
	for i := 0; i < len(w); i++ {
		switch {
		case isDigit(w[i]):
			digits++
		case w[i] == '.':
		default:
			return false
		}
	}
	return digits > 0 && digits <= 6
}

func punctOnly(w string) bool {
	for i := 0; i < len(w); i++ {
		if c := w[i]; c != '.' && c != ':' && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func isSemanticKey(word string) bool {
	end := len(word)
	for end > 0 && (word[end-1] == ':' || word[end-1] == '.') {
		end--
	}
	start := end
	for start > 0 && word[start-1] != '_' && word[start-1] != '-' {
		start--
	}
	if n := end - start; n < 2 || n > 10 {
		return false
	}
	var buf [10]byte
	for i := start; i < end; i++ {
		buf[i-start] = word[i] | 0x20
	}
	return semanticKeys[string(buf[:end-start])] // no allocation for map lookups
}

func volatileWord(word string) bool {
	if len(word) > 2 && word[0] == '0' && (word[1] == 'x' || word[1] == 'X') && isHex(word[2:]) {
		return true // pointer / hex literal
	}
	digits, alnum := 0, 0
	for i := 0; i < len(word); i++ {
		c := word[i] | 0x20
		switch {
		case word[i] >= '0' && word[i] <= '9':
			digits++
			alnum++
		case c >= 'a' && c <= 'z':
			alnum++
		}
	}
	if digits == 0 {
		return false
	}
	if namedOrdinal(word) {
		return false // abc100, b200, build42: a name with a number
	}
	sawNonHex := false
	for i := 0; i < len(word); {
		c := word[i] | 0x20
		if c < 'a' || c > 'z' {
			i++
			continue
		}
		j, nonHex := i, false
		for ; j < len(word); j++ {
			cj := word[j] | 0x20
			if cj < 'a' || cj > 'z' {
				break
			}
			nonHex = nonHex || cj > 'f'
		}
		if nonHex {
			sawNonHex = true
			if !isUnit(word[i:j]) {
				return false // TestFoo2, v1beta1, sha256sum: identifiers
			}
		}
		i = j
	}
	if !sawNonHex {
		// short words like "bad1" are identifiers; 7+ chars are hash-like
		return digits*2 >= alnum || alnum >= 7
	}
	return true
}

func isUnit(run string) bool {
	if len(run) > 3 {
		return false
	}
	var buf [3]byte
	for i := 0; i < len(run); i++ {
		buf[i] = run[i] | 0x20
	}
	return units[string(buf[:len(run)])]
}

func namedOrdinal(word string) bool {
	i := 0
	for i < len(word) && (word[i]|0x20 >= 'a' && word[i]|0x20 <= 'z') {
		i++
	}
	if i == 0 || i == len(word) {
		return false
	}
	for _, c := range []byte(word[i:]) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func randomPart(part string) bool {
	return len(part) >= 6 && strings.ContainsAny(part, "0123456789") && isHex(part)
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if (s[i] < '0' || s[i] > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ShapeSpans returns the byte ranges Shape masks in line.
func ShapeSpans(line string) []Span {
	_, spans := Shape{}.KeyAndSpans(nil, line)
	return spans
}
