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
	ID         int64      `json:"id" xml:"id,attr"`
	Workflow   string     `json:"workflow" xml:"workflow,attr"`
	RunNumber  int        `json:"run_number" xml:"run_number,attr"`
	Event      string     `json:"event" xml:"event,attr"`
	HeadBranch string     `json:"head_branch" xml:"head_branch,attr"`
	HeadSHA    string     `json:"head_sha" xml:"head_sha,attr"`
	Status     string     `json:"status" xml:"status,attr"`
	Attempt    int        `json:"attempt,omitempty" xml:"attempt,attr,omitempty"`
	Conclusion string     `json:"conclusion" xml:"conclusion,attr"`
	CreatedAt  time.Time  `json:"created_at" xml:"created_at,attr"`
	HTMLURL    string     `json:"html_url" xml:"html_url,attr"`
	Failed     []Failure  `json:"failed" xml:"failed>failure"`
	Artifacts  []Artifact `json:"artifacts,omitempty" xml:"artifacts>artifact,omitempty"`
	// PendingEnvironments names the deployment gates holding a waiting
	// run. Present only with `runs --approvals` (zero extra API calls
	// on the default path, Task 200 precedent).
	PendingEnvironments []string `json:"pending_environments,omitempty" xml:"pending_environments>environment,omitempty"`
}

// Artifact mirrors the pipe contract's artifact metadata. Download
// URLs are deliberately absent: the API's archive URLs redirect to
// short-lived signed links that must never be emitted.
type Artifact struct {
	ID          int64     `json:"id" xml:"id,attr"`
	Name        string    `json:"name" xml:"name,attr"`
	SizeInBytes int64     `json:"size_in_bytes" xml:"size_in_bytes,attr"`
	Expired     bool      `json:"expired" xml:"expired,attr"`
	CreatedAt   time.Time `json:"created_at" xml:"created_at,attr"`
	ExpiresAt   time.Time `json:"expires_at" xml:"expires_at,attr"`
	Digest      string    `json:"digest,omitempty" xml:"digest,attr,omitempty"`
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

// ArtifactsResult is the pipe envelope for the artifacts command.
type ArtifactsResult struct {
	XMLName    xml.Name   `json:"-" xml:"artifacts_result"`
	Repo       string     `json:"repo" xml:"repo,attr"`
	RunID      int64      `json:"run_id" xml:"run_id,attr"`
	Artifacts  []Artifact `json:"artifacts" xml:"artifacts>artifact"`
	Downloaded *Download  `json:"downloaded,omitempty" xml:"downloaded,omitempty"`
}

type Download struct {
	Name      string `json:"name" xml:"name,attr"`
	Path      string `json:"path" xml:"path,attr"`
	FileCount int    `json:"file_count" xml:"file_count,attr"`
}

// ApprovalsResult is the pipe envelope for the approvals verb. The
// list form carries pending only; review attempts add accepted (true
// with reviewed, false with the typed error refusal).
type ApprovalsResult struct {
	XMLName  xml.Name            `json:"-" xml:"approvals_result"`
	Repo     string              `json:"repo" xml:"repo,attr"`
	RunID    int64               `json:"run_id" xml:"run_id,attr"`
	Pending  []PendingDeployment `json:"pending" xml:"pending>environment"`
	Accepted *bool               `json:"accepted,omitempty" xml:"accepted,attr,omitempty"`
	Reviewed *DeploymentReview   `json:"reviewed,omitempty" xml:"reviewed,omitempty"`
	Error    *MutationError      `json:"error,omitempty" xml:"error,omitempty"`
}

// PendingDeployment mirrors the agent contract for one gate: the
// environment, its wait timer, whether the caller can approve, and the
// required reviewers. No URLs are emitted.
type PendingDeployment struct {
	EnvironmentID         int64                `json:"environment_id" xml:"environment_id,attr"`
	Environment           string               `json:"environment" xml:"environment,attr"`
	WaitTimer             int                  `json:"wait_timer" xml:"wait_timer,attr"`
	CurrentUserCanApprove bool                 `json:"current_user_can_approve" xml:"current_user_can_approve,attr"`
	Reviewers             []DeploymentReviewer `json:"reviewers" xml:"reviewers>reviewer"`
}

type DeploymentReviewer struct {
	Type string `json:"type" xml:"type,attr"`
	Name string `json:"name" xml:"name,attr"`
}

// DeploymentReview reports what a review attempt posted: the state,
// the environments it covered, and the comment that accompanied it.
type DeploymentReview struct {
	State        string   `json:"state" xml:"state,attr"`
	Environments []string `json:"environments" xml:"environments>environment"`
	Comment      string   `json:"comment" xml:"comment,attr"`
}

func WriteApprovals(w io.Writer, format Format, result ApprovalsResult) error {
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		if _, err := fmt.Fprintf(w, "# gh-hound approvals\n\nRepo: `%s`\nRun: `%d`\n\n| Environment | Wait | Can approve | Reviewers |\n| --- | --- | --- | --- |\n", result.Repo, result.RunID); err != nil {
			return err
		}
		for _, pending := range result.Pending {
			reviewers := make([]string, 0, len(pending.Reviewers))
			for _, reviewer := range pending.Reviewers {
				reviewers = append(reviewers, reviewer.Name)
			}
			if _, err := fmt.Fprintf(w, "| %s | %ds | %t | %s |\n", pending.Environment, pending.WaitTimer, pending.CurrentUserCanApprove, strings.Join(reviewers, ", ")); err != nil {
				return err
			}
		}
		if result.Reviewed != nil {
			if _, err := fmt.Fprintf(w, "\nReviewed `%s`: %s — %q\n", result.Reviewed.State, strings.Join(result.Reviewed.Environments, ", "), result.Reviewed.Comment); err != nil {
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

// MutationResult is the pipe envelope for the rerun and cancel verbs.
// The action enum covers every mutation path; html_url is always
// reconstructed from repo + run id, never fetched (the mutation
// endpoints return no body and the one-call budget holds).
type MutationResult struct {
	XMLName  xml.Name       `json:"-" xml:"mutation_result"`
	Repo     string         `json:"repo" xml:"repo,attr"`
	RunID    int64          `json:"run_id" xml:"run_id,attr"`
	JobID    int64          `json:"job_id,omitempty" xml:"job_id,attr,omitempty"`
	Action   string         `json:"action" xml:"action,attr"`
	Accepted bool           `json:"accepted" xml:"accepted,attr"`
	HTMLURL  string         `json:"html_url" xml:"html_url,attr"`
	Error    *MutationError `json:"error,omitempty" xml:"error,omitempty"`
}

// MutationError is the typed refusal agents branch on when a mutation
// is not accepted (exit 2): kind mirrors the ActionError taxonomy
// (validation, permission, conflict, rate_limit, network, unknown).
type MutationError struct {
	Kind string `json:"kind" xml:"kind,attr"`
	// Field names the offending input for validation refusals (run,
	// job, ref) so agents can correct programmatically.
	Field   string `json:"field,omitempty" xml:"field,attr,omitempty"`
	Message string `json:"message" xml:"message,attr"`
}

func WriteMutation(w io.Writer, format Format, result MutationResult) error {
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		if _, err := fmt.Fprintf(w, "# gh-hound %s\n\nRepo: `%s`\nRun: `%d`\nAccepted: %t\nURL: %s\n", result.Action, result.Repo, result.RunID, result.Accepted, result.HTMLURL); err != nil {
			return err
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

func WriteArtifacts(w io.Writer, format Format, result ArtifactsResult) error {
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		if _, err := fmt.Fprintf(w, "# gh-hound artifacts\n\nRepo: `%s`\nRun: `%d`\n\n| Name | Size | Expired | Expires |\n| --- | --- | --- | --- |\n", result.Repo, result.RunID); err != nil {
			return err
		}
		for _, artifact := range result.Artifacts {
			if _, err := fmt.Fprintf(w, "| %s | %d | %t | %s |\n", artifact.Name, artifact.SizeInBytes, artifact.Expired, artifact.ExpiresAt.Format(time.RFC3339)); err != nil {
				return err
			}
		}
		if result.Downloaded != nil {
			if _, err := fmt.Fprintf(w, "\nDownloaded `%s` to `%s` (%d files).\n", result.Downloaded.Name, result.Downloaded.Path, result.Downloaded.FileCount); err != nil {
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
