package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
)

// FlakesResult is the pipe envelope for the flakes verb: the scored
// flake verdict agents branch on. status is the source of truth —
// exit 1 means flaky or suspect (rerun vs investigate is distinguished
// in JSON, both demand attention), exit 0 covers clean AND
// insufficient_data (no action derivable), exit 2 carries the typed
// error. Exit 3 is never used here: a finished scan is never pending.
type FlakesResult struct {
	XMLName          xml.Name    `json:"-" xml:"flakes_result"`
	Repo             string      `json:"repo" xml:"repo,attr"`
	Workflow         string      `json:"workflow" xml:"workflow,attr"`
	Branch           string      `json:"branch" xml:"branch,attr"`
	Status           string      `json:"status" xml:"status,attr"`
	SampleSize       int         `json:"sample_size" xml:"sample_size,attr"`
	Window           int         `json:"window" xml:"window,attr"`
	RunsScanned      int         `json:"runs_scanned" xml:"runs_scanned,attr"`
	SignalsEvaluated []string    `json:"signals_evaluated" xml:"signals_evaluated>signal"`
	Jobs             []FlakeJob  `json:"jobs" xml:"jobs>job"`
	Verdict          string      `json:"verdict" xml:"verdict"`
	Error            *FlakeError `json:"error,omitempty" xml:"error,omitempty"`
}

// FlakeJob scores one job name over the window. The signal counts use
// sober wire names; the hound voice lives in verdict prose only.
type FlakeJob struct {
	Job        string          `json:"job" xml:"name,attr"`
	FlakeScore float64         `json:"flake_score" xml:"flake_score,attr"`
	Verdict    string          `json:"verdict" xml:"verdict,attr"`
	Flips      int             `json:"attempt_flips" xml:"attempt_flips,attr"`
	Flaps      int             `json:"cross_run_flaps" xml:"cross_run_flaps,attr"`
	Masks      int             `json:"retry_masks" xml:"retry_masks,attr"`
	FlakedRuns int             `json:"flaked_runs" xml:"flaked_runs,attr"`
	Evidence   []FlakeEvidence `json:"evidence" xml:"evidence>item"`
}

// FlakeEvidence is one observed wobble with its drill-down handle.
type FlakeEvidence struct {
	RunID     int64  `json:"run_id" xml:"run_id,attr"`
	RunNumber int    `json:"run_number" xml:"run_number,attr"`
	Attempt   int    `json:"attempt" xml:"attempt,attr"`
	Kind      string `json:"kind" xml:"kind,attr"`
	Detail    string `json:"detail" xml:",chardata"`
}

// FlakeError is the typed refusal for exit 2: kind mirrors the
// APIError taxonomy (auth, permission, not_found, rate_limit, network,
// validation, unknown).
type FlakeError struct {
	Kind    string `json:"kind" xml:"kind,attr"`
	Message string `json:"message" xml:"message,attr"`
}

// FlakesExitCode maps the verdict to the global exit contract: 1 when
// any job verdict is flaky or suspect (action needed — rerun or
// investigate), 2 on error, 0 otherwise (clean or insufficient_data).
// The status field, not the sample size, drives the code: an
// underfilled-but-flaky window still exits 1.
func FlakesExitCode(result FlakesResult) int {
	if result.Error != nil || result.Status == "error" {
		return ExitError
	}
	if result.Status == "flaky" || result.Status == "suspect" {
		return ExitActionNeeded
	}
	return ExitOK
}

func WriteFlakes(w io.Writer, format Format, result FlakesResult) error {
	if result.Jobs == nil {
		result.Jobs = []FlakeJob{}
	}
	if result.SignalsEvaluated == nil {
		result.SignalsEvaluated = []string{}
	}
	for i := range result.Jobs {
		if result.Jobs[i].Evidence == nil {
			result.Jobs[i].Evidence = []FlakeEvidence{}
		}
	}
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		return writeFlakesMarkdown(w, result)
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

func writeFlakesMarkdown(w io.Writer, result FlakesResult) error {
	if _, err := fmt.Fprintf(w, "# gh-hound flakes\n\nRepo: `%s`\nWorkflow: `%s`\nBranch: `%s`\nStatus: `%s`\nSample: %d of %d-run window\n\n%s\n", result.Repo, result.Workflow, result.Branch, result.Status, result.SampleSize, result.Window, result.Verdict); err != nil {
		return err
	}
	if result.Error != nil {
		_, err := fmt.Fprintf(w, "\nError: `%s` — %s\n", result.Error.Kind, result.Error.Message)
		return err
	}
	if len(result.Jobs) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n| Job | Score | Verdict | Flips | Flaps | Masks |\n| --- | --- | --- | --- | --- | --- |\n"); err != nil {
		return err
	}
	for _, job := range result.Jobs {
		if _, err := fmt.Fprintf(w, "| %s | %.2f | %s | %d | %d | %d |\n", job.Job, job.FlakeScore, job.Verdict, job.Flips, job.Flaps, job.Masks); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\nEvidence:\n\n"); err != nil {
		return err
	}
	for _, job := range result.Jobs {
		for _, item := range job.Evidence {
			if _, err := fmt.Fprintf(w, "- `%s` %s — %s\n", job.Job, item.Kind, item.Detail); err != nil {
				return err
			}
		}
	}
	return nil
}
