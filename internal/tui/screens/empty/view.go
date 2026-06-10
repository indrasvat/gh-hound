package empty

import (
	"fmt"
	"strings"
)

type Kind string

const (
	KindNoWorkflows  Kind = "no_workflows"
	KindNoRepository Kind = "no_repository"
	KindNoRuns       Kind = "no_runs"
	KindError        Kind = "error"
)

type Model struct {
	Kind    Kind
	Repo    string
	Branch  string
	Message string
}

func (m Model) Title() string {
	switch m.Kind {
	case KindNoWorkflows:
		return "No workflows found"
	case KindNoRepository:
		return "Repository needed"
	case KindNoRuns:
		return "No runs found"
	case KindError:
		return "Runs unavailable"
	default:
		return "Nothing to show"
	}
}

func View(model Model, width int) string {
	if width <= 0 {
		width = 80
	}
	lines := []string{
		"hound · empty",
		"",
		model.Title(),
	}
	for _, line := range body(model) {
		lines = append(lines, wrap(line, width)...)
	}
	return strings.Join(lines, "\n")
}

func body(model Model) []string {
	switch model.Kind {
	case KindNoWorkflows:
		return []string{
			fmt.Sprintf("Actions is not configured for %s.", model.Repo),
			"Add a workflow under .github/workflows or enable GitHub Actions for this repository.",
		}
	case KindNoRepository:
		return []string{
			first(model.Message, "Not in a git repo / no resolvable remote."),
			"Run gh hound -R owner/repo to choose a repository explicitly.",
		}
	case KindNoRuns:
		return []string{
			fmt.Sprintf("No workflow runs were found for %s on %s.", model.Repo, model.Branch),
			"Push the branch or use --all to show runs across every branch.",
		}
	case KindError:
		return []string{
			first(model.Message, "GitHub Actions runs could not be loaded."),
			"Check repository access, GitHub auth, rate limits, or try again later.",
		}
	default:
		return []string{"No data is available for this screen yet."}
	}
}

func wrap(value string, width int) []string {
	if len([]rune(value)) <= width {
		return []string{value}
	}
	words := strings.Fields(value)
	lines := []string{}
	var current strings.Builder
	for _, word := range words {
		nextLen := len([]rune(current.String())) + len([]rune(word))
		if current.Len() > 0 {
			nextLen++
		}
		if nextLen > width && current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(word)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
