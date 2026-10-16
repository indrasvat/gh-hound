package runs

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone        IntentKind = ""
	IntentOpenDetail  IntentKind = "open_detail"
	IntentOpenLogs    IntentKind = "open_logs"
	IntentWatch       IntentKind = "watch"
	IntentDispatch    IntentKind = "dispatch"
	IntentBrowser     IntentKind = "browser"
	IntentCopy        IntentKind = "copy"
	IntentRerun       IntentKind = "rerun"
	IntentRerunFailed IntentKind = "rerun_failed"
	IntentCancel      IntentKind = "cancel"
	IntentForceCancel IntentKind = "force_cancel"
	IntentFilter      IntentKind = "filter"
	IntentLoadMore    IntentKind = "load_more"
	IntentApprovals   IntentKind = "approvals"
)

type Intent struct {
	Kind   IntentKind
	RunID  int64
	Filter string
}

type Summary struct {
	Failing int
	Running int
	Passed  int
	// Gated counts runs waiting on a deployment review. They are also
	// part of Running (the run is live) — Gated only adds the gate
	// affordance to the summary line.
	Gated int
}

type Model struct {
	Context  usecase.LaunchContext
	Selected int
	// Filter is the applied query. While InputMode is on, edits go to
	// Draft and Filter keeps showing results until Enter applies.
	Filter     string
	Draft      string
	InputMode  bool
	InputError string
	// Viewer is the authenticated login standing in for actor:@me; empty
	// until the identity lookup lands.
	Viewer string
	// DataKeys records the server filter each scope's rows were fetched
	// under. Rows whose key differs from the applied query's are stale:
	// they are only shown through the full query, never as-is.
	DataKeys map[usecase.LaunchScope]string
	// QueryError explains why the applied query has no rows (identity or
	// load failure); the list never falls back to unfiltered rows.
	QueryError string
	Intent     Intent
	Theme      theme.Theme

	// FlakyRuns marks rows whose workflow has a known flaker, from
	// verdicts already computed this session (config flake_badges).
	// The badge never spends API calls of its own.
	FlakyRuns map[int64]bool

	// Loading state is transient render input set by the app from its
	// pending load each frame — never persisted on the model, so a
	// cancelled load can not strand a dimmed list.
	Loading     bool
	LoadingLine string
}

func NewModel(ctx usecase.LaunchContext) Model {
	if ctx.Page == 0 {
		ctx.Page = 1
	}
	if !ctx.HasMore && ctx.PerPage > 0 && len(activeRuns(ctx)) >= ctx.PerPage {
		ctx.HasMore = true
	}
	m := Model{Context: ctx, Filter: ctx.Query, Viewer: ctx.Viewer, QueryError: ctx.QueryError}
	// Only scopes the launch actually listed count as fetched under its
	// filter; any other scope loads when first shown.
	key := m.query().ServerKey()
	m.DataKeys = map[usecase.LaunchScope]string{}
	if ctx.QueryError == "" {
		m.DataKeys[ctx.Scope] = key
	}
	for _, scope := range []usecase.LaunchScope{usecase.LaunchScopeBranch, usecase.LaunchScopeRepo} {
		if len(scopeRuns(ctx, scope)) > 0 {
			m.DataKeys[scope] = key
		}
	}
	return m
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	if m.InputMode {
		return m.updateInput(msg)
	}
	total := len(m.filteredRuns())
	switch msg.Key {
	case "j", "down":
		if m.Selected < total-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "ctrl+d":
		m.Selected = min(m.Selected+10, max(total-1, 0))
	case "ctrl+u":
		m.Selected = max(m.Selected-10, 0)
	case "g":
		m.Selected = 0
	case "G":
		if total > 0 {
			m.Selected = total - 1
		}
		// A filtered page can narrow to nothing while GitHub still has
		// more: G keeps hunting.
		if m.Context.HasMore && (total > 0 || m.Filter != "") {
			m.Intent = Intent{Kind: IntentLoadMore}
		}
	case "s":
		m = m.toggleScope()
	case "f":
		m = m.Apply(WithToken(m.Filter, "status", nextStatusCycle(m.query().Status)))
	case "esc":
		if m.Filter != "" {
			m = m.Apply("")
		}
	case "/":
		m.InputMode = true
		m.Draft = m.Filter
		m.InputError = ""
	case "enter":
		m.Intent = m.intentFor(IntentOpenDetail)
	case "l":
		m.Intent = m.intentFor(IntentOpenLogs)
	case "w":
		m.Intent = m.intentFor(IntentWatch)
	case "D":
		m.Intent = Intent{Kind: IntentDispatch}
	case "o":
		m.Intent = m.intentFor(IntentBrowser)
	case "y":
		m.Intent = m.intentFor(IntentCopy)
	case "r":
		m.Intent = m.intentFor(IntentRerun)
	case "R":
		m.Intent = m.intentFor(IntentRerunFailed)
	case "x":
		m.Intent = m.intentFor(IntentCancel)
	case "X":
		m.Intent = m.intentFor(IntentForceCancel)
	case "A":
		m.Intent = m.intentFor(IntentApprovals)
	}
	return m
}

func (m Model) updateInput(msg KeyMsg) Model {
	switch msg.Key {
	case "esc":
		// Cancel restores the applied query untouched.
		m.InputMode = false
		m.Draft = ""
		m.InputError = ""
	case "enter":
		if _, err := ParseQuery(m.Draft); err != nil {
			m.InputError = err.Error()
			return m
		}
		m.InputMode = false
		m = m.Apply(NormalizeQuery(m.Draft))
		m.Draft = ""
	case "backspace":
		if runes := []rune(m.Draft); len(runes) > 0 {
			m.Draft = string(runes[:len(runes)-1])
		}
		m.InputError = ""
	case "ctrl+u":
		m.Draft = ""
		m.InputError = ""
	case "space":
		m.Draft += " "
		m.InputError = ""
	default:
		if len([]rune(msg.Key)) == 1 {
			m.Draft += msg.Key
			m.InputError = ""
		}
	}
	return m
}

// Apply makes query the applied filter and asks the app to load it.
// Callers pass only queries that parse.
func (m Model) Apply(query string) Model {
	m.Filter = query
	m.QueryError = ""
	m.Selected = 0
	m.Intent = Intent{Kind: IntentFilter, Filter: query}
	return m
}

func (m Model) query() Query {
	q, _ := ParseQuery(m.Filter)
	return q
}

// ServerKey is the server-side half of the applied query.
func (m Model) ServerKey() string {
	return m.query().ServerKey()
}

// DataCurrent reports whether the active scope's rows were fetched
// under the applied query's server filter.
func (m Model) DataCurrent() bool {
	return m.scopeCurrent(m.Context.Scope)
}

// scopeCurrent reports whether a scope's cached rows answer the applied
// query's server half. A scope with no recorded fetch is current only
// for an unfiltered query and only if it holds rows; an empty or
// unrecorded cache must load rather than claim "no runs".
func (m Model) scopeCurrent(scope usecase.LaunchScope) bool {
	key, fetched := m.DataKeys[scope]
	if !fetched {
		return m.ServerKey() == "" && len(scopeRuns(m.Context, scope)) > 0
	}
	return key == m.ServerKey()
}

// Restore puts back an applied query and scope whose rows are already
// loaded — what Esc returns to when it cancels the reload for a newer
// one.
func (m Model) Restore(filter string, scope usecase.LaunchScope) Model {
	m.Filter = filter
	m.Context.Scope = scope
	m.Context.Runs = scopeRuns(m.Context, scope)
	m.QueryError = ""
	m.Selected = clampSelection(m.Selected, len(m.filteredRuns()))
	return m
}

func (m Model) Summary() Summary {
	var summary Summary
	for _, run := range m.filteredRuns() {
		if run.Status == model.StatusWaiting {
			summary.Gated++
		}
		if isRunning(run) {
			summary.Running++
			continue
		}
		if isFailing(run) {
			summary.Failing++
			continue
		}
		if isPassing(run) {
			summary.Passed++
		}
	}
	return summary
}

// nextStatusCycle advances the f-key status token: none -> failure ->
// in_progress -> success -> none. Any other status restarts the cycle.
func nextStatusCycle(current string) string {
	switch current {
	case string(model.ConclusionFailure):
		return string(model.StatusInProgress)
	case string(model.StatusInProgress):
		return string(model.ConclusionSuccess)
	case string(model.ConclusionSuccess):
		return ""
	default:
		return string(model.ConclusionFailure)
	}
}

// FilteredRuns exposes the rows the list will render after filtering.

func (m Model) FilteredRuns() []model.Run {
	return m.filteredRuns()
}

func (m Model) AllGreen() bool {
	summary := m.Summary()
	runs := m.filteredRuns()
	if len(runs) == 0 || summary.Failing > 0 {
		return false
	}
	for _, run := range runs {
		if run.Status != model.StatusCompleted {
			return false
		}
	}
	return true
}

func (m Model) selectedRun() (model.Run, bool) {
	runs := m.filteredRuns()
	if len(runs) == 0 {
		return model.Run{}, false
	}
	selected := max(m.Selected, 0)
	if selected >= len(runs) {
		selected = len(runs) - 1
	}
	return runs[selected], true
}

func (m Model) SelectedRun() (model.Run, bool) {
	return m.selectedRun()
}

func (m Model) intentFor(kind IntentKind) Intent {
	run, ok := m.selectedRun()
	if !ok {
		return Intent{}
	}
	return Intent{Kind: kind, RunID: run.ID}
}

func (m Model) filteredRuns() []model.Run {
	if m.QueryError != "" {
		return nil
	}
	q := m.query()
	current := m.DataCurrent()
	if current && len(q.Text) == 0 {
		return m.activeRuns()
	}
	runs := m.activeRuns()
	filtered := make([]model.Run, 0, len(runs))
	for _, run := range runs {
		// Current rows already satisfy the server qualifiers; stale rows
		// (a reload in flight) must satisfy the whole query.
		if (current && q.MatchesText(run)) || (!current && q.Matches(run, m.Viewer)) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func runMatchesQuery(run model.Run, query string) bool {
	for _, alias := range queryAliases(query) {
		if strings.Contains(strings.ToLower(run.Name), alias) ||
			strings.Contains(strings.ToLower(run.DisplayTitle), alias) ||
			strings.Contains(strings.ToLower(run.Event), alias) ||
			strings.Contains(strings.ToLower(run.HeadBranch), alias) ||
			strings.Contains(strings.ToLower(string(run.Status)), alias) ||
			strings.Contains(strings.ToLower(string(run.Conclusion)), alias) ||
			strings.Contains(strings.ToLower(run.Actor), alias) ||
			strings.Contains(fmt.Sprintf("%d", run.RunNumber), alias) {
			return true
		}
	}
	return false
}

func queryAliases(query string) []string {
	switch query {
	case "failed", "failing", "red":
		return []string{query, "failure"}
	case "passed", "passing", "green":
		return []string{query, "success"}
	case "canceled":
		return []string{query, "cancelled"}
	case "running", "live":
		return []string{query, "in_progress"}
	default:
		return []string{query}
	}
}

func (m Model) activeRuns() []model.Run {
	return activeRuns(m.Context)
}

func activeRuns(ctx usecase.LaunchContext) []model.Run {
	switch ctx.Scope {
	case usecase.LaunchScopeRepo:
		if len(ctx.RepoRuns) > 0 {
			return ctx.RepoRuns
		}
	case usecase.LaunchScopeBranch:
		if len(ctx.BranchRuns) > 0 {
			return ctx.BranchRuns
		}
	}
	return ctx.Runs
}

// toggleScope flips branch/repo scope and keeps the applied query.
func (m Model) toggleScope() Model {
	target, ok := m.scopeTarget()
	if !ok {
		return m
	}
	if m.ServerKey() == "" && len(scopeRuns(m.Context, target)) == 0 {
		// Nothing cached to show for an unfiltered list: stay put.
		return m
	}
	return m.WithScope(target)
}

// WithScope switches to scope keeping the applied query. A cache that
// is missing or was fetched under another filter switches immediately
// (its rows shown only through the query) and asks the app to load it.
func (m Model) WithScope(scope usecase.LaunchScope) Model {
	cache := scopeRuns(m.Context, scope)
	m.Context.Scope = scope
	m.Context.Runs = cache
	m.Selected = 0
	// An error belongs to the scope that failed; this one loads afresh.
	m.QueryError = ""
	if !m.scopeCurrent(scope) || len(cache) == 0 {
		m.Intent = Intent{Kind: IntentFilter, Filter: m.Filter}
	}
	return m
}

func (m Model) scopeTarget() (usecase.LaunchScope, bool) {
	switch m.Context.Scope {
	case usecase.LaunchScopeBranch:
		return usecase.LaunchScopeRepo, true
	case usecase.LaunchScopeRepo:
		return usecase.LaunchScopeBranch, strings.TrimSpace(m.Context.Branch) != ""
	default:
		return usecase.LaunchScopeRepo, len(m.Context.BranchRuns) > 0 && len(m.Context.RepoRuns) > 0
	}
}

func scopeRuns(ctx usecase.LaunchContext, scope usecase.LaunchScope) []model.Run {
	switch scope {
	case usecase.LaunchScopeBranch:
		return ctx.BranchRuns
	case usecase.LaunchScopeRepo:
		return ctx.RepoRuns
	default:
		return ctx.Runs
	}
}

func isRunning(run model.Run) bool {
	switch run.Status {
	case model.StatusInProgress, model.StatusQueued, model.StatusWaiting, model.StatusPending, model.StatusRequested:
		return true
	default:
		return false
	}
}

func isFailing(run model.Run) bool {
	switch run.Conclusion {
	case model.ConclusionFailure, model.ConclusionActionRequired, model.ConclusionTimedOut:
		return true
	default:
		return false
	}
}

func isPassing(run model.Run) bool {
	if run.Status != model.StatusCompleted {
		return false
	}
	switch run.Conclusion {
	case model.ConclusionSuccess, model.ConclusionSkipped, model.ConclusionNeutral:
		return true
	default:
		return false
	}
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
