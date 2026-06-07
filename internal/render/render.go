package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	FormatJSON     Format = "json"
	FormatMarkdown Format = "md"
	FormatXML      Format = "xml"

	ExitOK           = 0
	ExitActionNeeded = 1
	ExitError        = 2
	ExitPending      = 3
)

type Format string

type Result struct {
	XMLName xml.Name `json:"-" xml:"result"`
	Repo    string   `json:"repo" xml:"repo,attr"`
	Branch  string   `json:"branch" xml:"branch,attr"`
	Runs    []Run    `json:"runs" xml:"runs>run"`
}

type Run struct {
	ID         int64     `json:"id" xml:"id,attr"`
	Workflow   string    `json:"workflow" xml:"workflow,attr"`
	RunNumber  int       `json:"run_number" xml:"run_number,attr"`
	Event      string    `json:"event" xml:"event,attr"`
	HeadBranch string    `json:"head_branch" xml:"head_branch,attr"`
	HeadSHA    string    `json:"head_sha" xml:"head_sha,attr"`
	Status     string    `json:"status" xml:"status,attr"`
	Conclusion string    `json:"conclusion" xml:"conclusion,attr"`
	CreatedAt  time.Time `json:"created_at" xml:"created_at,attr"`
	HTMLURL    string    `json:"html_url" xml:"html_url,attr"`
	Failed     []Failure `json:"failed" xml:"failed>failure"`
}

type Failure struct {
	Job         string       `json:"job" xml:"job,attr"`
	Step        string       `json:"step" xml:"step,attr"`
	ExitCode    int          `json:"exit_code" xml:"exit_code,attr"`
	Annotations []Annotation `json:"annotations" xml:"annotations>annotation"`
	LogExcerpt  string       `json:"log_excerpt" xml:"log_excerpt"`
}

type Annotation struct {
	Path    string `json:"path" xml:"path,attr"`
	Line    int    `json:"line" xml:"line,attr"`
	Level   string `json:"level" xml:"level,attr"`
	Message string `json:"message" xml:"message,attr"`
}

func Write(w io.Writer, format Format, result Result) error {
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		return writeMarkdown(w, result)
	case FormatXML:
		_, err := fmt.Fprintln(w, xml.Header[:len(xml.Header)-1])
		if err != nil {
			return err
		}
		encoder := xml.NewEncoder(w)
		encoder.Indent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return err
		}
		_, err = fmt.Fprintln(w)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func ExitCode(result Result, err error) int {
	if err != nil {
		return ExitError
	}
	for _, run := range result.Runs {
		if run.Status == "queued" || run.Status == "in_progress" || run.Status == "waiting" || run.Status == "pending" || run.Status == "requested" {
			return ExitPending
		}
		if run.Conclusion == "failure" || run.Conclusion == "action_required" || run.Conclusion == "timed_out" {
			return ExitActionNeeded
		}
	}
	return ExitOK
}

func writeMarkdown(w io.Writer, result Result) error {
	if _, err := fmt.Fprintf(w, "# gh-hound\n\nRepo: `%s`\nBranch: `%s`\n\n", result.Repo, result.Branch); err != nil {
		return err
	}
	for _, run := range result.Runs {
		title := strings.TrimSpace(fmt.Sprintf("%s #%d", run.Workflow, run.RunNumber))
		if _, err := fmt.Fprintf(w, "## %s\n\n- Status: `%s`\n- Conclusion: `%s`\n- URL: %s\n", title, run.Status, run.Conclusion, run.HTMLURL); err != nil {
			return err
		}
		for _, failure := range run.Failed {
			if _, err := fmt.Fprintf(w, "\n### %s · %s\n\nExit code: `%d`\n\n```text\n%s\n```\n", failure.Job, failure.Step, failure.ExitCode, failure.LogExcerpt); err != nil {
				return err
			}
		}
	}
	return nil
}
