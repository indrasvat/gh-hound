package model

import (
	"encoding/json"
	"fmt"
	"time"
)

type Status string

const (
	StatusQueued     Status = "queued"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusRequested  Status = "requested"
	StatusWaiting    Status = "waiting"
	StatusPending    Status = "pending"
)

type Conclusion string

const (
	ConclusionNone           Conclusion = ""
	ConclusionSuccess        Conclusion = "success"
	ConclusionFailure        Conclusion = "failure"
	ConclusionCancelled      Conclusion = "cancelled"
	ConclusionSkipped        Conclusion = "skipped"
	ConclusionNeutral        Conclusion = "neutral"
	ConclusionTimedOut       Conclusion = "timed_out"
	ConclusionActionRequired Conclusion = "action_required"
	ConclusionStale          Conclusion = "stale"
)

type Run struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	DisplayTitle    string     `json:"display_title"`
	Status          Status     `json:"status"`
	Conclusion      Conclusion `json:"conclusion"`
	Event           string     `json:"event"`
	HeadBranch      string     `json:"head_branch"`
	HeadSHA         string     `json:"head_sha"`
	Path            string     `json:"path"`
	RunNumber       int        `json:"run_number"`
	RunAttempt      int        `json:"run_attempt"`
	WorkflowID      int64      `json:"workflow_id"`
	Actor           string     `json:"actor"`
	TriggeringActor string     `json:"triggering_actor"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	RunStartedAt    time.Time  `json:"run_started_at"`
	HTMLURL         string     `json:"html_url"`
	JobsURL         string     `json:"jobs_url"`
	LogsURL         string     `json:"logs_url"`
	PullRequests    []int      `json:"pull_requests"`
}

type Job struct {
	ID              int64      `json:"id"`
	RunID           int64      `json:"run_id"`
	Status          Status     `json:"status"`
	Conclusion      Conclusion `json:"conclusion"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     time.Time  `json:"completed_at"`
	Name            string     `json:"name"`
	Steps           []Step     `json:"steps"`
	Labels          []string   `json:"labels"`
	RunnerName      string     `json:"runner_name"`
	RunnerGroupName string     `json:"runner_group_name"`
	WorkflowName    string     `json:"workflow_name"`
	HeadBranch      string     `json:"head_branch"`
	HTMLURL         string     `json:"html_url"`
	CheckRunURL     string     `json:"check_run_url"`
}

type Step struct {
	Name        string     `json:"name"`
	Status      Status     `json:"status"`
	Conclusion  Conclusion `json:"conclusion"`
	Number      int        `json:"number"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at"`
}

type Annotation struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Title     string `json:"title"`
}

type Artifact struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	SizeInBytes int64     `json:"size_in_bytes"`
	Expired     bool      `json:"expired"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Digest      string    `json:"digest"`
	RunID       int64     `json:"run_id"`
	HeadBranch  string    `json:"head_branch"`
	HeadSHA     string    `json:"head_sha"`
}

// PendingDeployment is one environment gate holding a `waiting` run:
// who may open it, how long the wait timer is, and whether the current
// user can review it.
type PendingDeployment struct {
	EnvironmentID         int64                `json:"environment_id"`
	EnvironmentName       string               `json:"environment_name"`
	WaitTimer             int                  `json:"wait_timer"`
	CurrentUserCanApprove bool                 `json:"current_user_can_approve"`
	Reviewers             []DeploymentReviewer `json:"reviewers"`
}

// DeploymentReviewer names a required reviewer: a User login or a Team
// slug, with Type distinguishing the two.
type DeploymentReviewer struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// Cache is one GitHub Actions cache entry (the kennel's contents).
// Shape pinned against the live API 2026-06-10: the wire payload also
// carries a version hash, deliberately dropped — it names the cache
// internals, not anything a user can act on.
type Cache struct {
	ID             int64     `json:"id"`
	Key            string    `json:"key"`
	Ref            string    `json:"ref"`
	SizeInBytes    int64     `json:"size_in_bytes"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// CacheUsage is the repo-level Actions cache footprint. The API does
// not expose the eviction cap on github.com (the usage-policy endpoint
// is GHES-only — verified live 2026-06-10, 404), so callers compare
// against the documented 10 GB fallback.
type CacheUsage struct {
	ActiveSizeInBytes int64 `json:"active_size_in_bytes"`
	ActiveCount       int   `json:"active_count"`
}
// Workflow states GitHub documents on list-workflows. State stays an
// open string everywhere: unknown future values render verbatim with
// a neutral badge and are never rejected.
const (
	WorkflowStateActive             = "active"
	WorkflowStateDisabledManually   = "disabled_manually"
	WorkflowStateDisabledInactivity = "disabled_inactivity"
	WorkflowStateDisabledFork       = "disabled_fork"
	WorkflowStateDeleted            = "deleted"
)

type Workflow struct {
	ID      int64           `json:"id"`
	Name    string          `json:"name"`
	Path    string          `json:"path"`
	State   string          `json:"state"`
	HTMLURL string          `json:"html_url"`
	Inputs  []WorkflowInput `json:"inputs,omitempty"`
}

// Toggleable reports whether enable/disable is a valid move for this
// workflow: active flips off, manually- or inactivity-disabled flip
// back on. Fork-disabled and deleted workflows cannot be toggled, and
// unknown states are left alone rather than guessed at.
func (w Workflow) Toggleable() bool {
	switch w.State {
	case WorkflowStateActive, WorkflowStateDisabledManually, WorkflowStateDisabledInactivity:
		return true
	default:
		return false
	}
}

type WorkflowInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required"`
	Type        string   `json:"type"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
}

func ParseStatus(raw string) (Status, error) {
	status := Status(raw)
	if !status.Valid() {
		return "", fmt.Errorf("invalid GitHub Actions status %q", raw)
	}
	return status, nil
}

func (s Status) Valid() bool {
	switch s {
	case StatusQueued, StatusInProgress, StatusCompleted, StatusRequested, StatusWaiting, StatusPending:
		return true
	default:
		return false
	}
}

func ParseConclusion(raw string) (Conclusion, error) {
	if raw == "null" {
		return ConclusionNone, nil
	}
	conclusion := Conclusion(raw)
	if !conclusion.Valid() {
		return "", fmt.Errorf("invalid GitHub Actions conclusion %q", raw)
	}
	return conclusion, nil
}

func (c Conclusion) Valid() bool {
	switch c {
	case ConclusionNone, ConclusionSuccess, ConclusionFailure, ConclusionCancelled, ConclusionSkipped, ConclusionNeutral, ConclusionTimedOut, ConclusionActionRequired, ConclusionStale:
		return true
	default:
		return false
	}
}

func (c Conclusion) MarshalJSON() ([]byte, error) {
	if c == ConclusionNone {
		return []byte("null"), nil
	}
	if !c.Valid() {
		return nil, fmt.Errorf("invalid GitHub Actions conclusion %q", c)
	}
	return json.Marshal(string(c))
}

func (c *Conclusion) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*c = ConclusionNone
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	conclusion, err := ParseConclusion(raw)
	if err != nil {
		return err
	}
	*c = conclusion
	return nil
}
