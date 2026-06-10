package toast

import (
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func Glyph(severity usecase.Severity) string {
	switch severity {
	case usecase.SeverityOK:
		return icons.Success
	case usecase.SeverityWarn:
		return icons.ActionRequired
	case usecase.SeverityError:
		return icons.Failure
	default:
		return icons.Neutral
	}
}
