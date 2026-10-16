package fold

import "strings"

// Structural pins GitHub Actions command lines (##[...] and ::...).
type Structural struct{}

func (Structural) Name() string { return "structural" }

func (Structural) Protect(line string) bool {
	body := stripRunnerStamp(line)
	return strings.HasPrefix(body, "##[") || strings.HasPrefix(body, "::")
}

func stripRunnerStamp(line string) string {
	if len(line) > 28 && line[4] == '-' && line[10] == 'T' {
		if sp := strings.IndexByte(line, ' '); sp > 0 && sp < 40 && line[sp-1] == 'Z' {
			return line[sp+1:]
		}
	}
	return line
}
