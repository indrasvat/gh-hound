package banner

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/indrasvat/gh-hound/internal/theme"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

const Mark = `██╗  ██╗ ██████╗ ██╗   ██╗███╗   ██╗██████╗
██║  ██║██╔═══██╗██║   ██║████╗  ██║██╔══██╗
███████║██║   ██║██║   ██║██╔██╗ ██║██║  ██║
██╔══██║██║   ██║██║   ██║██║╚██╗██║██║  ██║
██║  ██║╚██████╔╝╚██████╔╝██║ ╚████║██████╔╝
╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝╚═════╝`

func RenderVersion(info BuildInfo) string {
	var out strings.Builder
	out.WriteString(RenderMark())
	out.WriteString("\n\n")
	out.WriteString(info.Version)
	out.WriteString(" · commit ")
	out.WriteString(info.Commit)
	out.WriteString(" · built ")
	out.WriteString(info.Date)
	out.WriteString("\nHunt down your GitHub Actions CI\n")
	return out.String()
}

// RenderMark is the banner in Bramble, as --version prints it.
func RenderMark() string {
	return RenderMarkTheme(theme.ForMode(theme.ModeBramble))
}

func RenderMarkTheme(th theme.Theme) string {
	lines := strings.Split(Mark, "\n")
	for i, line := range lines {
		lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Mark[i%len(th.Mark)])).Render(line)
	}
	return strings.Join(lines, "\n")
}
