package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
)

// WorkflowsResult is the pipe envelope for the workflows verb. The
// list form carries workflows only; toggle attempts add accepted plus
// either toggled (the one mutation that was made) or the typed error
// refusal. State is an open string: unknown future API values pass
// through verbatim.
type WorkflowsResult struct {
	XMLName   xml.Name        `json:"-" xml:"workflows_result"`
	Repo      string          `json:"repo" xml:"repo,attr"`
	Workflows []WorkflowInfo  `json:"workflows" xml:"workflows>workflow"`
	Accepted  *bool           `json:"accepted,omitempty" xml:"accepted,attr,omitempty"`
	Toggled   *WorkflowToggle `json:"toggled,omitempty" xml:"toggled,omitempty"`
	Error     *MutationError  `json:"error,omitempty" xml:"error,omitempty"`
}

// WorkflowInfo mirrors the agent contract for one workflow: identity
// plus the state field that answers "why did my cron stop running".
type WorkflowInfo struct {
	ID    int64  `json:"id" xml:"id,attr"`
	Name  string `json:"name" xml:"name,attr"`
	Path  string `json:"path" xml:"path,attr"`
	State string `json:"state" xml:"state,attr"`
}

// WorkflowToggle reports what a toggle attempt did: the selector as
// given, the action, and the state the workflow lands in (`active`
// after enable, `disabled_manually` after disable) — derived, not
// re-fetched, so the toggle stays exactly one API call.
type WorkflowToggle struct {
	Target string `json:"target" xml:"target,attr"`
	Action string `json:"action" xml:"action,attr"`
	State  string `json:"state" xml:"state,attr"`
}

func WriteWorkflows(w io.Writer, format Format, result WorkflowsResult) error {
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		if _, err := fmt.Fprintf(w, "# gh-hound workflows\n\nRepo: `%s`\n\n| ID | Name | Path | State |\n| --- | --- | --- | --- |\n", result.Repo); err != nil {
			return err
		}
		for _, workflow := range result.Workflows {
			if _, err := fmt.Fprintf(w, "| %d | %s | %s | %s |\n", workflow.ID, workflow.Name, workflow.Path, workflow.State); err != nil {
				return err
			}
		}
		if result.Toggled != nil {
			if _, err := fmt.Fprintf(w, "\nToggled `%s`: %s → `%s`\n", result.Toggled.Target, result.Toggled.Action, result.Toggled.State); err != nil {
				return err
			}
		}
		if result.Error != nil {
			if _, err := fmt.Fprintf(w, "\nError: `%s` — %s\n", result.Error.Kind, result.Error.Message); err != nil {
				return err
			}
		}
		return nil
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
