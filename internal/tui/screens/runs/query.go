package runs

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// MeActor is the actor value that stands for the authenticated user.
const MeActor = usecase.MeActor

// Query is a parsed runs filter. Qualifiers go to GitHub's list-runs
// API, so they reach beyond the loaded page; Text tokens search the
// rows already loaded.
type Query struct {
	Actor  string
	Status string
	Branch string
	Event  string
	SHA    string
	Text   []string
}

// ParseQuery reads a filter line. An error names the offending token;
// callers must keep the previously applied query when it fails. Only
// known keys are qualifiers: other key:value text ("feat: bump") is
// searched like any word. A lone status or event word (failing, push)
// stays the server filter it always was; among other words it is text.
func ParseQuery(raw string) (Query, error) {
	var q Query
	seen := map[string]bool{}
	var bare []string
	for token := range strings.FieldsSeq(raw) {
		key, value, tagged := strings.Cut(token, ":")
		name := canonicalKey(strings.ToLower(key))
		if !tagged || name == "" {
			bare = append(bare, strings.ToLower(token))
			continue
		}
		if value == "" {
			return Query{}, fmt.Errorf("%s: needs a value", name)
		}
		if seen[name] {
			return Query{}, fmt.Errorf("only one %s: per filter", name)
		}
		seen[name] = true
		switch name {
		case "actor":
			actor, err := usecase.NormalizeActor(value)
			if err != nil {
				return Query{}, err
			}
			q.Actor = actor
		case "status":
			status, ok := NormalizeStatus(value)
			if !ok {
				return Query{}, fmt.Errorf("unknown status %q", value)
			}
			q.Status = status
		case "branch":
			q.Branch = value
		case "event":
			q.Event = strings.ToLower(value)
		case "sha":
			q.SHA = strings.ToLower(value)
		}
	}
	if len(bare) == 1 {
		if status, ok := NormalizeStatus(bare[0]); ok && q.Status == "" {
			q.Status = status
			return q, nil
		}
		if IsKnownEvent(bare[0]) && q.Event == "" {
			q.Event = bare[0]
			return q, nil
		}
	}
	q.Text = bare
	return q, nil
}

func canonicalKey(key string) string {
	switch key {
	case "actor":
		return "actor"
	case "status", "conclusion":
		return "status"
	case "branch":
		return "branch"
	case "event":
		return "event"
	case "sha", "head", "head_sha":
		return "sha"
	default:
		return ""
	}
}

// NormalizeStatus maps a status word (including the f-cycle aliases)
// to the value GitHub's list-runs status parameter accepts.
func NormalizeStatus(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "failure", "failed", "failing", "red":
		return string(model.ConclusionFailure), true
	case "success", "passed", "passing", "green":
		return string(model.ConclusionSuccess), true
	case "cancelled", "canceled":
		return string(model.ConclusionCancelled), true
	case "running", "live":
		return string(model.StatusInProgress), true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if status, err := model.ParseStatus(value); err == nil {
		return string(status), true
	}
	if conclusion, err := model.ParseConclusion(value); err == nil && conclusion != model.ConclusionNone {
		return string(conclusion), true
	}
	return "", false
}

// IsKnownEvent reports the trigger events a bare word filters by.
func IsKnownEvent(value string) bool {
	switch value {
	case "push", "pull_request", "workflow_dispatch", "schedule", "workflow_run", "repository_dispatch", "merge_group":
		return true
	default:
		return false
	}
}

// Server reports whether any part of the query is answered by GitHub.
func (q Query) Server() bool {
	return q.Actor != "" || q.Status != "" || q.Branch != "" || q.Event != "" || q.SHA != ""
}

// ServerKey identifies the server-side request: two queries with the
// same key fetch the same rows, whatever their text tokens.
func (q Query) ServerKey() string {
	if !q.Server() {
		return ""
	}
	return strings.Join([]string{
		"actor:" + strings.ToLower(q.Actor),
		"status:" + q.Status,
		"branch:" + q.Branch,
		"event:" + q.Event,
		"sha:" + q.SHA,
	}, " ")
}

// Matches applies the whole query to one row. viewer stands in for
// @me; an unresolved @me matches nothing rather than everything.
func (q Query) Matches(run model.Run, viewer string) bool {
	if q.Actor != "" {
		actor := q.Actor
		if actor == MeActor {
			actor = viewer
		}
		if actor == "" || !strings.EqualFold(run.Actor, actor) {
			return false
		}
	}
	if q.Status != "" && string(run.Status) != q.Status && string(run.Conclusion) != q.Status {
		return false
	}
	if q.Branch != "" && !strings.EqualFold(run.HeadBranch, q.Branch) {
		return false
	}
	if q.Event != "" && !strings.EqualFold(run.Event, q.Event) {
		return false
	}
	if q.SHA != "" && !strings.HasPrefix(strings.ToLower(run.HeadSHA), q.SHA) {
		return false
	}
	return q.MatchesText(run)
}

// MatchesText applies only the text tokens: every token must match.
func (q Query) MatchesText(run model.Run) bool {
	for _, token := range q.Text {
		if !runMatchesQuery(run, token) {
			return false
		}
	}
	return true
}

// WithToken returns raw with every token for key removed and, when
// value is non-empty, key:value appended — the f cycle and the palette
// change one constraint and keep the rest. A status key also removes
// bare status words.
func WithToken(raw, key, value string) string {
	tokens := strings.Fields(raw)
	bare := 0
	for _, token := range tokens {
		if k, _, tagged := strings.Cut(token, ":"); !tagged || canonicalKey(strings.ToLower(k)) == "" {
			bare++
		}
	}
	kept := []string{}
	for _, token := range tokens {
		tokenKey, _, tagged := strings.Cut(token, ":")
		name := canonicalKey(strings.ToLower(tokenKey))
		if tagged && name == key {
			continue
		}
		// A lone bare status word is the status filter (see ParseQuery).
		if (!tagged || name == "") && key == "status" && bare == 1 {
			if _, ok := NormalizeStatus(token); ok {
				continue
			}
		}
		kept = append(kept, token)
	}
	if value != "" {
		kept = append(kept, key+":"+value)
	}
	return strings.Join(kept, " ")
}

// NormalizeQuery collapses whitespace so the applied query reads the
// way it was typed, minus stray spaces.
func NormalizeQuery(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}
