package banner

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
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
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#4FD37A")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#66BE8A")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#6E9CB5")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#97AFA9")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#CFCDBB")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#EAE8D9")),
	}
	for i, line := range strings.Split(Mark, "\n") {
		out.WriteString(styles[i%len(styles)].Render(line))
		out.WriteByte('\n')
	}
	out.WriteByte('\n')
	out.WriteString(info.Version)
	out.WriteString(" · commit ")
	out.WriteString(info.Commit)
	out.WriteString(" · built ")
	out.WriteString(info.Date)
	out.WriteString("\nHunt down your GitHub Actions CI\n")
	return out.String()
}
