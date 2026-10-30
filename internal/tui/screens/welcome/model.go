package welcome

import (
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
)

type Model struct {
	Build banner.BuildInfo
	Theme theme.Theme
}
