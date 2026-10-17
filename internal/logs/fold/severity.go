package fold

// severityWords are counted, not pinned: repeated errors are the noise users
// want folded, and the fold summary warns about them.
var severityWords = []string{"error", "errors", "fail", "failed", "failure", "fatal", "panic", "exception", "traceback"}

func containsSeverity(line string) bool {
	for i := 0; i < len(line); i++ {
		c := line[i] | 0x20
		if c != 'e' && c != 'f' && c != 'p' && c != 't' {
			continue
		}
		if i > 0 && isAlnum(line[i-1]) {
			continue
		}
		for _, w := range severityWords {
			end := i + len(w)
			if end <= len(line) && asciiEqualFold(line[i:end], w) && (end == len(line) || !isAlnum(line[end])) {
				return true
			}
		}
	}
	return false
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func asciiEqualFold(s, lower string) bool {
	for i := 0; i < len(s); i++ {
		if s[i]|0x20 != lower[i] {
			return false
		}
	}
	return true
}
