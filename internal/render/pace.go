package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
)

// PaceResult is the pipe envelope for the pace verb: one run judged
// against its workflow's recent green runs. status is the source of
// truth: exit 1 for slow or overrun, 3 while running within the
// baseline, 2 on error, 0 otherwise (on_pace, insufficient_data,
// not_comparable). Durations are whole seconds.
type PaceResult struct {
	XMLName  xml.Name `json:"-" xml:"pace_result"`
	Repo     string   `json:"repo" xml:"repo,attr"`
	Workflow string   `json:"workflow" xml:"workflow,attr"`
	Branch   string   `json:"branch" xml:"branch,attr"`
	// BaselineBranch is null when the branch had too few green runs
	// and the baseline widened to the same event on any branch.
	BaselineBranch *string  `json:"baseline_branch" xml:"baseline_branch,attr,omitempty"`
	BaselineEvent  string   `json:"baseline_event" xml:"baseline_event,attr"`
	Status         string   `json:"status" xml:"status,attr"`
	Window         int      `json:"window" xml:"window,attr"`
	SampleSize     int      `json:"sample_size" xml:"sample_size,attr"`
	RunsScanned    int      `json:"runs_scanned" xml:"runs_scanned,attr"`
	Statistic      string   `json:"statistic" xml:"statistic,attr"`
	Run            *PaceRun `json:"run" xml:"run,omitempty"`
	// Jobs and Slow judge the run's jobs and steps; absent with
	// --no-steps, Slow is [] when nothing ran slow.
	Jobs    *[]PaceJob  `json:"jobs,omitempty" xml:"jobs>job,omitempty"`
	Slow    *[]PaceSlow `json:"slow,omitempty" xml:"slow>item,omitempty"`
	Verdict string      `json:"verdict" xml:"verdict"`
	Error   *PaceError  `json:"error,omitempty" xml:"error,omitempty"`
}

// PaceRun is the judged run. The baseline figures are null when there
// is no baseline to read (insufficient_data, not_comparable).
type PaceRun struct {
	ID             int64    `json:"id" xml:"id,attr"`
	Number         int      `json:"number" xml:"number,attr"`
	Attempt        int      `json:"attempt" xml:"attempt,attr"`
	Event          string   `json:"event" xml:"event,attr"`
	Status         string   `json:"status" xml:"status,attr"`
	Conclusion     string   `json:"conclusion" xml:"conclusion,attr"`
	ElapsedSeconds int64    `json:"elapsed_seconds" xml:"elapsed_seconds,attr"`
	QueuedSeconds  int64    `json:"queued_seconds" xml:"queued_seconds,attr"`
	ETASeconds     *int64   `json:"eta_seconds" xml:"eta_seconds,attr,omitempty"`
	P50Seconds     *int64   `json:"p50_seconds" xml:"p50_seconds,attr,omitempty"`
	P90Seconds     *int64   `json:"p90_seconds" xml:"p90_seconds,attr,omitempty"`
	Ratio          *float64 `json:"ratio" xml:"ratio,attr,omitempty"`
	Slow           bool     `json:"slow" xml:"slow,attr"`
	Reason         string   `json:"reason,omitempty" xml:"reason,attr,omitempty"`
}

// PaceJob is one job of the judged run; the baseline figures are null
// below 5 samples or for work Pace does not judge (failed, skipped,
// queued).
type PaceJob struct {
	Name           string     `json:"name" xml:"name,attr"`
	Status         string     `json:"status" xml:"status,attr"`
	Conclusion     string     `json:"conclusion" xml:"conclusion,attr"`
	ElapsedSeconds *int64     `json:"elapsed_seconds" xml:"elapsed_seconds,attr,omitempty"`
	P50Seconds     *int64     `json:"p50_seconds" xml:"p50_seconds,attr,omitempty"`
	P90Seconds     *int64     `json:"p90_seconds" xml:"p90_seconds,attr,omitempty"`
	Ratio          *float64   `json:"ratio" xml:"ratio,attr,omitempty"`
	SampleSize     int        `json:"sample_size" xml:"sample_size,attr"`
	Slow           bool       `json:"slow" xml:"slow,attr"`
	Steps          []PaceStep `json:"steps" xml:"step"`
}

// PaceStep is one step of a job, by number and name.
type PaceStep struct {
	Number         int      `json:"number" xml:"number,attr"`
	Name           string   `json:"name" xml:"name,attr"`
	Status         string   `json:"status" xml:"status,attr"`
	Conclusion     string   `json:"conclusion" xml:"conclusion,attr"`
	ElapsedSeconds *int64   `json:"elapsed_seconds" xml:"elapsed_seconds,attr,omitempty"`
	P50Seconds     *int64   `json:"p50_seconds" xml:"p50_seconds,attr,omitempty"`
	P90Seconds     *int64   `json:"p90_seconds" xml:"p90_seconds,attr,omitempty"`
	Ratio          *float64 `json:"ratio" xml:"ratio,attr,omitempty"`
	SampleSize     int      `json:"sample_size" xml:"sample_size,attr"`
	Slow           bool     `json:"slow" xml:"slow,attr"`
}

// PaceSlow lists one slow job or step, the summary agents act on.
type PaceSlow struct {
	Kind           string  `json:"kind" xml:"kind,attr"`
	Job            string  `json:"job" xml:"job,attr"`
	Step           string  `json:"step,omitempty" xml:"step,attr,omitempty"`
	Number         int     `json:"number,omitempty" xml:"number,attr,omitempty"`
	ElapsedSeconds int64   `json:"elapsed_seconds" xml:"elapsed_seconds,attr"`
	P50Seconds     int64   `json:"p50_seconds" xml:"p50_seconds,attr"`
	Ratio          float64 `json:"ratio" xml:"ratio,attr"`
}

// PaceError is the typed refusal for exit 2, in the APIError taxonomy.
type PaceError struct {
	Kind    string `json:"kind" xml:"kind,attr"`
	Message string `json:"message" xml:"message,attr"`
}

// PaceExitCode maps the status to the global exit contract.
func PaceExitCode(result PaceResult) int {
	switch {
	case result.Error != nil || result.Status == "error":
		return ExitError
	case result.Status == "slow" || result.Status == "overrun":
		return ExitActionNeeded
	case result.Status == "running":
		return ExitPending
	default:
		return ExitOK
	}
}

func WritePace(w io.Writer, format Format, result PaceResult) error {
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		return writePaceMarkdown(w, result)
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

func writePaceMarkdown(w io.Writer, result PaceResult) error {
	baseline := "any branch"
	if result.BaselineBranch != nil {
		baseline = "`" + *result.BaselineBranch + "`"
	}
	if _, err := fmt.Fprintf(w, "# gh-hound pace\n\nRepo: `%s`\nWorkflow: `%s`\nBranch: `%s`\nBaseline: %s, `%s` events, %d green runs (window %d)\nStatus: `%s`\n\n%s\n",
		result.Repo, result.Workflow, result.Branch, baseline, result.BaselineEvent, result.SampleSize, result.Window, result.Status, result.Verdict); err != nil {
		return err
	}
	if result.Error != nil {
		_, err := fmt.Fprintf(w, "\nError: `%s` — %s\n", result.Error.Kind, result.Error.Message)
		return err
	}
	if result.Run == nil {
		return nil
	}
	run := result.Run
	if _, err := fmt.Fprintf(w, "\n| Run | Attempt | Status | Elapsed | Queued | ETA | p50 | p90 | Ratio | Slow |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n| #%d | %d | %s | %ds | %ds | %s | %s | %s | %s | %t |\n",
		run.Number, run.Attempt, run.Status, run.ElapsedSeconds, run.QueuedSeconds, secondsCell(run.ETASeconds), secondsCell(run.P50Seconds), secondsCell(run.P90Seconds), ratioCell(run.Ratio), run.Slow); err != nil {
		return err
	}
	if result.Slow == nil {
		return nil
	}
	if len(*result.Slow) == 0 {
		_, err := fmt.Fprintln(w, "\nNo job or step ran slow.")
		return err
	}
	if _, err := fmt.Fprint(w, "\n| Slow | Job | Step | Elapsed | p50 | Ratio |\n| --- | --- | --- | --- | --- | --- |\n"); err != nil {
		return err
	}
	for _, slow := range *result.Slow {
		step := "—"
		if slow.Kind == "step" {
			step = fmt.Sprintf("%d %s", slow.Number, slow.Step)
		}
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %ds | %ds | %.2f× |\n", slow.Kind, slow.Job, step, slow.ElapsedSeconds, slow.P50Seconds, slow.Ratio); err != nil {
			return err
		}
	}
	return nil
}

func secondsCell(v *int64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%ds", *v)
}

func ratioCell(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f×", *v)
}
