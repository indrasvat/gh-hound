// Package numfmt formats counts the way every screen shows them.
package numfmt

import "strconv"

// Group separates thousands with commas: 1587 → "1,587".
func Group(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}
