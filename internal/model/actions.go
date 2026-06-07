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

type Workflow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
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
