package welcome

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/indrasvat/gh-hound/internal/tui/banner"
)

func View(model Model) string {
	var out strings.Builder
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#4FD37A")).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#CFCDBB"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#66BE8A")).Bold(true)

	out.WriteString(banner.Mark)
	out.WriteString("\n\n")
	out.WriteString(title.Render("Hunt down your GitHub Actions CI"))
	out.WriteString(muted.Render(" — without the click-through"))
	out.WriteString("\n\n")
	out.WriteString(accent.Render("WATCH"))
	out.WriteString(muted.Render(" failing and live runs"))
	out.WriteString("   ")
	out.WriteString(accent.Render("DIAGNOSE"))
	out.WriteString(muted.Render(" annotated failure logs"))
	out.WriteString("   ")
	out.WriteString(accent.Render("RERUN"))
	out.WriteString(muted.Render(" failed jobs"))
	out.WriteString("\n\n")
	out.WriteString("Enter to continue")
	out.WriteString("\n")
	out.WriteString(model.Build.Version)
	out.WriteString(" · github.com/indrasvat/gh-hound")
	return out.String()
}
