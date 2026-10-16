package runs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

// MeActor is the actor value that stands for the authenticated user.
const MeActor = "@me"

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

var (
	qualifierKey = regexp.MustCompile(`^[a-z_]+$`)
	githubLogin  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$`)
)

// ParseQuery reads a filter line. An error names the offending token;
// callers must keep the previously applied query when it fails.
func ParseQuery(raw string) (Query, error) {
	var q Query
	seen := map[string]bool{}
	for token := range strings.FieldsSeq(raw) {
		key, value, tagged := strings.Cut(token, ":")
		key = strings.ToLower(key)
		if !tagged || !qualifierKey.MatchString(key) {
			lower := strings.ToLower(token)
			if status, ok := NormalizeStatus(lower); ok && q.Status == "" {
				q.Status = status
				continue
			}
			if IsKnownEvent(lower) && q.Event == "" {
				q.Event = lower
				continue
			}
			q.Text = append(q.Text, lower)
			continue
		}
		name := canonicalKey(key)
		if name == "" {
			return Query{}, fmt.Errorf("unknown filter %q — try actor, status, branch, event or sha", key+":")
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
			actor, err := parseActor(value)
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

func parseActor(value string) (string, error) {
	if strings.EqualFold(value, MeActor) {
		return MeActor, nil
	}
	login := strings.TrimPrefix(value, "@")
	if !githubLogin.MatchString(login) {
		return "", fmt.Errorf("%q isn't a GitHub login", value)
	}
	return login, nil
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
	kept := []string{}
	for token := range strings.FieldsSeq(raw) {
		tokenKey, _, tagged := strings.Cut(token, ":")
		if tagged && canonicalKey(strings.ToLower(tokenKey)) == key {
			continue
		}
		if !tagged && key == "status" {
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
