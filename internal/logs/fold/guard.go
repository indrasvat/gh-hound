package fold

import (
	"strings"

	"github.com/indrasvat/gh-hound/internal/logs"
)

// Structural pins GitHub Actions command lines (##[...] and ::...).
type Structural struct{}

func (Structural) Name() string { return "structural" }

func (Structural) Protect(line string) bool {
	body := logs.CommandText(line)
	return strings.HasPrefix(body, "##[") || strings.HasPrefix(body, "::")
}
