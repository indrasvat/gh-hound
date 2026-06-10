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

func RenderMark() string {
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#4FD37A")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#66BE8A")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#6E9CB5")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#97AFA9")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#CFCDBB")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#EAE8D9")),
	}
	lines := strings.Split(Mark, "\n")
	for i, line := range lines {
		lines[i] = styles[i%len(styles)].Render(line)
	}
	return strings.Join(lines, "\n")
}
