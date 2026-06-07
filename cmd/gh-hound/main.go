package main

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type buildInfo struct {
	Version string
	Commit  string
	Date    string
}

const banner = `██╗  ██╗ ██████╗ ██╗   ██╗███╗   ██╗██████╗
██║  ██║██╔═══██╗██║   ██║████╗  ██║██╔══██╗
███████║██║   ██║██║   ██║██╔██╗ ██║██║  ██║
██╔══██║██║   ██║██║   ██║██║╚██╗██║██║  ██║
██║  ██║╚██████╔╝╚██████╔╝██║ ╚████║██████╔╝
╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝╚═════╝`

func main() {
	if err := newRootCommand(os.Stdout, os.Stderr, buildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func newRootCommand(stdout, stderr io.Writer, info buildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "gh-hound",
		Short:         "Hunt down your GitHub Actions CI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			showVersion, err := cmd.Flags().GetBool("version")
			if err != nil {
				return err
			}
			if showVersion {
				return printVersion(stdout, info)
			}
			return printPlaceholder(stdout)
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().BoolP("version", "v", false, "print version information")
	cmd.Flags().Bool("no-tui", false, "disable the TUI and write structured output (env HOUND_NO_TUI)")
	cmd.Flags().String("format", "json", "pipe output format: json, md, xml (env HOUND_FORMAT)")
	cmd.Flags().StringP("repo", "R", "", "GitHub repository owner/name (env GH_REPO or HOUND_REPO)")
	cmd.Flags().String("log-level", "info", "log level: off, error, warn, info, debug (env HOUND_LOG_LEVEL)")
	cmd.Flags().Bool("trace-http", false, "trace GitHub API calls to the JSON log (env HOUND_TRACE_HTTP)")

	cmd.AddCommand(newVersionCommand(stdout, info))
	cmd.AddCommand(newRunsCommand(stdout))
	cmd.AddCommand(newWatchCommand(stdout))
	cmd.AddCommand(newDispatchCommand(stdout))
	return cmd
}

func newVersionCommand(stdout io.Writer, info buildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printVersion(stdout, info)
		},
	}
}

func newRunsCommand(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List GitHub Actions runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(stdout, `{"repo":"","branch":"","runs":[]}`)
			return err
		},
	}
	cmd.Flags().String("status", "", "filter runs by status (env HOUND_STATUS)")
	cmd.Flags().Bool("json", false, "write JSON output")
	cmd.Flags().Bool("no-tui", true, "disable the TUI")
	return cmd
}

func newWatchCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch the current or selected run",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(stdout, "watch is not implemented yet")
			return err
		},
	}
}

func newDispatchCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "dispatch",
		Short: "Trigger a workflow_dispatch workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(stdout, "dispatch is not implemented yet")
			return err
		},
	}
}

func printVersion(w io.Writer, info buildInfo) error {
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#4FD37A")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#66BE8A")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#6E9CB5")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#97AFA9")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#CFCDBB")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#EAE8D9")),
	}
	for i, line := range splitLines(banner) {
		if _, err := fmt.Fprintln(w, styles[i%len(styles)].Render(line)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "\n%s · commit %s · built %s\nHunt down your GitHub Actions CI\n",
		info.Version, info.Commit, info.Date)
	return err
}

func printPlaceholder(w io.Writer) error {
	_, err := fmt.Fprintln(w, "gh-hound TUI scaffold is ready; screen implementation starts in Task 080.")
	return err
}

func splitLines(s string) []string {
	lines := make([]string, 0, 6)
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
