package render

import (
	"encoding/json"
	"io"
	"time"
)

// The watch --group pipe surface speaks NDJSON: one JSON object per
// line, state transitions per run until the pack settles, then one
// terminal summary object that closes the stream. Events are
// run-level ONLY — job/step fields belong to single-run watch; the
// group poll budget never fetches jobs.

const (
	// GroupEventTypeRun tags a per-run state transition line.
	GroupEventTypeRun = "run"
	// GroupEventTypeSummary tags the terminal summary line.
	GroupEventTypeSummary = "summary"
)

// GroupEvent is one run's state transition on the stream.
type GroupEvent struct {
	Type       string    `json:"type"`
	TS         time.Time `json:"ts"`
	RunID      int64     `json:"run_id"`
	Workflow   string    `json:"workflow"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
}

// GroupSummary closes the stream: the scent, the member count, and
// the hound's tallies. It lands when the pack settles OR when a
// bounded --timeout expires first — `timed_out` tells the two apart
// (a timed-out summary still carries live `running` members).
type GroupSummary struct {
	Type     string    `json:"type"`
	TS       time.Time `json:"ts"`
	Repo     string    `json:"repo"`
	HeadSHA  string    `json:"head_sha"`
	Event    string    `json:"event"`
	Runs     int       `json:"runs"`
	Running  int       `json:"running"`
	Home     int       `json:"home"`
	Lost     int       `json:"lost"`
	TimedOut bool      `json:"timed_out"`
}

func WriteGroupEvent(w io.Writer, event GroupEvent) error {
	event.Type = GroupEventTypeRun
	return writeNDJSONLine(w, event)
}

func WriteGroupSummary(w io.Writer, summary GroupSummary) error {
	summary.Type = GroupEventTypeSummary
	return writeNDJSONLine(w, summary)
}

// writeNDJSONLine emits exactly one compact JSON line (Encode appends
// the newline) — never indented, never split.
func writeNDJSONLine(w io.Writer, value any) error {
	return json.NewEncoder(w).Encode(value)
}
