package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

type QueueResult struct {
	XMLName              xml.Name       `json:"-" xml:"queue_result"`
	Repo                 string         `json:"repo" xml:"repo,attr"`
	Target               QueueTarget    `json:"target" xml:"target"`
	FetchedAt            time.Time      `json:"fetched_at" xml:"fetched_at,attr"`
	Status               string         `json:"status" xml:"status,attr"`
	Decision             QueueDecision  `json:"decision" xml:"decision"`
	Summary              string         `json:"summary" xml:"summary"`
	ConfiguredGroupCount int            `json:"configured_group_count" xml:"configured_group_count,attr"`
	Groups               []QueueGroup   `json:"groups" xml:"groups>group"`
	Blockers             []QueueBlocker `json:"blockers" xml:"blockers>blocker"`
	Error                *QueueError    `json:"error,omitempty" xml:"error,omitempty"`
}

type QueueTarget struct {
	Kind      string `json:"kind" xml:"kind,attr"`
	RunID     int64  `json:"run_id,omitempty" xml:"run_id,attr,omitempty"`
	JobID     int64  `json:"job_id,omitempty" xml:"job_id,attr,omitempty"`
	GroupName string `json:"group_name,omitempty" xml:"group_name,attr,omitempty"`
}

type QueueDecision struct {
	Action       string   `json:"action" xml:"action,attr"`
	Confidence   string   `json:"confidence" xml:"confidence,attr"`
	Reason       string   `json:"reason" xml:"reason"`
	NextCommands []string `json:"next_commands,omitempty" xml:"next_commands>command,omitempty"`
}

type QueueGroup struct {
	GroupName            string        `json:"group_name" xml:"name,attr"`
	GroupURL             string        `json:"group_url,omitempty" xml:"group_url,attr,omitempty"`
	LastAcquiredAt       *time.Time    `json:"last_acquired_at,omitempty" xml:"last_acquired_at,attr,omitempty"`
	Scope                string        `json:"scope" xml:"scope,attr"`
	State                string        `json:"state" xml:"state,attr"`
	ConfiguredGroupCount *int          `json:"configured_group_count,omitempty" xml:"configured_group_count,attr,omitempty"`
	ActiveMemberCount    *int          `json:"active_member_count,omitempty" xml:"active_member_count,attr,omitempty"`
	TargetPosition       *int          `json:"target_position,omitempty" xml:"target_position,attr,omitempty"`
	BlockedByCount       int           `json:"blocked_by_count" xml:"blocked_by_count,attr"`
	Members              []QueueMember `json:"members" xml:"members>member"`
}

type QueueMember struct {
	RunID        int64  `json:"run_id" xml:"run_id,attr"`
	RunName      string `json:"run_name,omitempty" xml:"run_name,attr,omitempty"`
	RunHTMLURL   string `json:"run_html_url,omitempty" xml:"run_html_url,attr,omitempty"`
	JobID        *int64 `json:"job_id,omitempty" xml:"job_id,attr,omitempty"`
	JobName      string `json:"job_name,omitempty" xml:"job_name,attr,omitempty"`
	JobHTMLURL   string `json:"job_html_url,omitempty" xml:"job_html_url,attr,omitempty"`
	Status       string `json:"status" xml:"status,attr"`
	Position     *int   `json:"position,omitempty" xml:"position,attr,omitempty"`
	Target       bool   `json:"target,omitempty" xml:"target,attr,omitempty"`
	BlocksTarget bool   `json:"blocks_target,omitempty" xml:"blocks_target,attr,omitempty"`
}

type QueueBlocker struct {
	GroupName string `json:"group_name" xml:"group_name,attr"`
	RunID     int64  `json:"run_id" xml:"run_id,attr"`
	JobID     *int64 `json:"job_id,omitempty" xml:"job_id,attr,omitempty"`
	Status    string `json:"status" xml:"status,attr"`
}

type QueueError struct {
	Kind    string `json:"kind" xml:"kind,attr"`
	Field   string `json:"field,omitempty" xml:"field,attr,omitempty"`
	Message string `json:"message" xml:"message,attr"`
}

func QueueExitCode(result QueueResult) int {
	if result.Error != nil || result.Status == "error" {
		return ExitError
	}
	if result.Status == "blocked" {
		return ExitPending
	}
	return ExitOK
}

func WriteQueue(w io.Writer, format Format, result QueueResult) error {
	if result.FetchedAt.IsZero() {
		result.FetchedAt = time.Now().UTC()
	}
	if result.Groups == nil {
		result.Groups = []QueueGroup{}
	}
	if result.Blockers == nil {
		result.Blockers = []QueueBlocker{}
	}
	for i := range result.Groups {
		if result.Groups[i].Members == nil {
			result.Groups[i].Members = []QueueMember{}
		}
	}
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		return writeQueueMarkdown(w, result)
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

func writeQueueMarkdown(w io.Writer, result QueueResult) error {
	if _, err := fmt.Fprintf(w, "# gh-hound queue\n\nRepo: `%s`\nStatus: `%s`\n\n%s\n", result.Repo, result.Status, result.Summary); err != nil {
		return err
	}
	if result.Error != nil {
		_, err := fmt.Fprintf(w, "\nError: `%s` — %s\n", result.Error.Kind, result.Error.Message)
		return err
	}
	if len(result.Groups) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\n| Group | Scope | State | Members | Blockers |\n| --- | --- | --- | ---: | ---: |"); err != nil {
		return err
	}
	for _, group := range result.Groups {
		active := "unknown"
		if group.ActiveMemberCount != nil {
			active = fmt.Sprintf("%d", *group.ActiveMemberCount)
		}
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %s | %d |\n", group.GroupName, group.Scope, group.State, active, group.BlockedByCount); err != nil {
			return err
		}
	}
	if len(result.Blockers) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nBlockers:"); err != nil {
		return err
	}
	for _, blocker := range result.Blockers {
		if _, err := fmt.Fprintf(w, "- `%s` run %d %s\n", blocker.GroupName, blocker.RunID, blocker.Status); err != nil {
			return err
		}
	}
	return nil
}
