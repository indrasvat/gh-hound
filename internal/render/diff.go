package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
)

// DiffResult is the pipe envelope for the diff verb: the regression
// verdict agents branch on. status is the source of truth — exit 1
// means a boundary was located (action needed), exit 0 covers both
// green and inconclusive (no action derivable), exit 2 carries the
// typed error. Exit 3 is never used here: nothing about a finished
// scan is pending.
type DiffResult struct {
	XMLName        xml.Name   `json:"-" xml:"diff_result"`
	Repo           string     `json:"repo" xml:"repo,attr"`
	Workflow       string     `json:"workflow" xml:"workflow,attr"`
	Branch         string     `json:"branch" xml:"branch,attr"`
	Status         string     `json:"status" xml:"status,attr"`
	LastGood       *Run       `json:"last_good,omitempty" xml:"last_good>run,omitempty"`
	FirstBad       *Run       `json:"first_bad,omitempty" xml:"first_bad>run,omitempty"`
	SuspectCommits []Commit   `json:"suspect_commits" xml:"suspect_commits>commit"`
	TotalSuspects  int        `json:"total_suspects" xml:"total_suspects,attr"`
	CompareURL     string     `json:"compare_url,omitempty" xml:"compare_url,attr,omitempty"`
	RunsScanned    int        `json:"runs_scanned" xml:"runs_scanned,attr"`
	Verdict        string     `json:"verdict" xml:"verdict"`
	Error          *DiffError `json:"error,omitempty" xml:"error,omitempty"`
}

// Commit is one suspect in the located range. message is the subject
// line only.
type Commit struct {
	SHA     string `json:"sha" xml:"sha,attr"`
	Author  string `json:"author" xml:"author,attr"`
	Message string `json:"message" xml:"message,attr"`
}

// DiffError is the typed refusal for exit 2: kind mirrors the APIError
// taxonomy (auth, permission, not_found, rate_limit, network,
// validation, unknown).
type DiffError struct {
	Kind    string `json:"kind" xml:"kind,attr"`
	Message string `json:"message" xml:"message,attr"`
}

// DiffExitCode maps the verdict to the global exit contract: 1 when a
// boundary was located (a regression exists — action needed), 2 on
// error, 0 otherwise (green or inconclusive).
func DiffExitCode(result DiffResult) int {
	if result.Error != nil || result.Status == "error" {
		return ExitError
	}
	if result.Status == "located" {
		return ExitActionNeeded
	}
	return ExitOK
}

func WriteDiff(w io.Writer, format Format, result DiffResult) error {
	if result.SuspectCommits == nil {
		result.SuspectCommits = []Commit{}
	}
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		return writeDiffMarkdown(w, result)
	case FormatXML:
		if _, err := fmt.Fprintln(w, xml.Header[:len(xml.Header)-1]); err != nil {
			return err
		}
		encoder := xml.NewEncoder(w)
		encoder.Indent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeDiffMarkdown(w io.Writer, result DiffResult) error {
	if _, err := fmt.Fprintf(w, "# gh-hound diff\n\nRepo: `%s`\nWorkflow: `%s`\nBranch: `%s`\nStatus: `%s`\n\n%s\n", result.Repo, result.Workflow, result.Branch, result.Status, result.Verdict); err != nil {
		return err
	}
	if result.Error != nil {
		_, err := fmt.Fprintf(w, "\nError: `%s` — %s\n", result.Error.Kind, result.Error.Message)
		return err
	}
	if result.Status != "located" {
		return nil
	}
	if result.LastGood != nil && result.FirstBad != nil {
		if _, err := fmt.Fprintf(w, "\nLast good: [#%d](%s) · First bad: [#%d](%s)\n", result.LastGood.RunNumber, result.LastGood.HTMLURL, result.FirstBad.RunNumber, result.FirstBad.HTMLURL); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\nSuspects (%d of %d):\n\n| SHA | Author | Subject |\n| --- | --- | --- |\n", len(result.SuspectCommits), result.TotalSuspects); err != nil {
		return err
	}
	for _, commit := range result.SuspectCommits {
		sha := commit.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		if _, err := fmt.Fprintf(w, "| %s | %s | %s |\n", sha, commit.Author, commit.Message); err != nil {
			return err
		}
	}
	if result.CompareURL != "" {
		if _, err := fmt.Fprintf(w, "\nCompare: %s\n", result.CompareURL); err != nil {
			return err
		}
	}
	return nil
}
