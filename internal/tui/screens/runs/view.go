package runs

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/numfmt"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/components/sparkline"
	"github.com/indrasvat/gh-hound/internal/tui/components/textinput"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func View(m Model, width int, now time.Time) string {
	return ViewSize(m, width, 0, now)
}

func ViewSize(m Model, width, height int, now time.Time) string {
	if width <= 0 {
		width = 80
	}
	filterActive := m.InputMode || strings.TrimSpace(m.Filter) != ""
	if m.AllGreen() && !filterActive {
		// The all-green celebration is a home state; filter results
		// always use the standard table so columns stay comparable.
		return renderAllGreen(m, width, height, now)
	}
	return renderRuns(m, width, height, now)
}

func renderRuns(m Model, width, height int, now time.Time) string {
	st := stylesFor(m.Theme.OrBramble())
	cols := columnsFor(width)
	runs := m.filteredRuns()
	selected := clampSelection(m.Selected, len(runs))
	filter := filterLines(m, st, width, len(runs))
	empty := emptyLines(m, st, width, len(runs))
	fixedRows := 2 + len(filter) + len(empty)
	notice := noticeLines(st, visibleNotice(m), width)
	fixedRows += len(notice)
	if m.Loading && m.LoadingLine != "" {
		// The loading line needs its own row or exact-fit heights
		// would push it past the frame.
		fixedRows++
	}
	rowCapacity := runRowCapacity(height, fixedRows, len(runs))
	start, end := viewport(selected, len(runs), rowCapacity)
	lines := []string{runsHeader(st, cols, width)}
	lines = append(lines, filter...)
	lines = append(lines, empty...)
	for i, run := range runs[start:end] {
		index := start + i
		line := row(st, cols, run, index == selected, m.FlakyRuns[run.ID], width, now)
		if m.Loading {
			// The previous list stays visible but dimmed while a
			// reload is in flight.
			line = dimLine(st, ansi.Strip(line), width)
		}
		lines = append(lines, line)
	}
	lines = append(lines, notice...)
	summary := m.Summary()
	// Paging belongs to the rows a query fetched; stale rows awaiting a
	// reload advertise none.
	hasMore := m.Context.HasMore && m.DataCurrent()
	lines = append(lines, fitANSI(joinRightANSI(summaryLine(st, summary, hasMore), pageLine(start, end, len(runs), hasMore), width), width))
	if m.Loading && m.LoadingLine != "" {
		lines = append(lines, fitANSI(m.LoadingLine, width))
	}
	return strings.Join(lines, "\n")
}

func renderAllGreen(m Model, width, height int, now time.Time) string {
	st := stylesFor(m.Theme.OrBramble())
	summary := m.Summary()
	runs := m.filteredRuns()
	selected := clampSelection(m.Selected, len(runs))
	latestTitle := ""
	latestMeta := ""
	if len(runs) > 0 {
		run := runs[0]
		latestTitle = plainRunTitle(run)
		latestMeta = fmt.Sprintf("%s · success", duration(run))
	}
	branch := scopeTitle(m.Context.Scope, m.Context.Branch)
	fixedRows := 5
	notice := noticeLines(st, visibleNotice(m), width)
	fixedRows += len(notice)
	if m.Loading && m.LoadingLine != "" {
		fixedRows++
	}
	rowCapacity := runRowCapacity(height, fixedRows, len(runs))
	start, end := viewport(selected, len(runs), rowCapacity)
	actorWidth := columnsFor(width).actor
	lines := []string{
		allGreenBandLine(st, "", width),
		allGreenBandLine(st, joinRightANSI(successLead(st, nonFailingHeadline(runs, branch)), latestTitle, width), width),
		allGreenBandLine(st, joinRightANSI("     "+fmt.Sprintf("%d recent runs · %d failing · last finished %s ago", len(runs), summary.Failing, age(runs[0], now)), latestMeta, width), width),
		allGreenBandLine(st, "", width),
		allGreenHeader(st, width, actorWidth),
	}
	lines = append(lines, notice...)
	for i, run := range runs[start:end] {
		line := allGreenRow(st, run, start+i == selected, width, actorWidth, now)
		if m.Loading {
			line = dimLine(st, ansi.Strip(line), width)
		}
		lines = append(lines, line)
	}
	if len(runs) > 0 {
		moreHint := ""
		if m.Context.HasMore {
			moreHint = " · G load more"
		}
		lines = append(lines, dimLine(st, pageLine(start, end, len(runs), m.Context.HasMore)+moreHint, width))
	}
	if m.Loading && m.LoadingLine != "" {
		lines = append(lines, fitANSI(m.LoadingLine, width))
	}
	return strings.Join(lines, "\n")
}

// runColumns is the one width spec the header and every row share, so
// labels can never drift from their values. Zero width hides a column.
type runColumns struct {
	status, label, actor, event, duration int
}

const (
	numberWidth = 8
	ageWidth    = 5
	// minLabelWidth is what Workflow / detail keeps before Duration and
	// then Event give up their space; Actor and Age never do.
	minLabelWidth = 32
)

func columnsFor(width int) runColumns {
	c := runColumns{status: 2, actor: 12, event: 16, duration: 8}
	if width >= 110 {
		c.actor = 16
	}
	if width >= 160 {
		c.actor = 24
		c.event = 20
	}
	c.label = width - c.fixedWidth()
	if c.label < minLabelWidth {
		c.duration = 0
		c.label = width - c.fixedWidth()
	}
	if c.label < minLabelWidth {
		c.event = 0
		c.label = width - c.fixedWidth()
	}
	c.label = max(c.label, 8)
	if width >= 160 {
		// Wide terminals split the flexible width 2:1:1 between
		// Workflow / detail, Actor and Event (the design reference's
		// grid), so Actor stays near its run and nothing pads blank.
		flex := c.label + c.actor + c.event
		c.actor, c.event = flex/4, flex/4
		c.label = flex - c.actor - c.event
	}
	return c
}

// fixedWidth is every cell but the label: cursor, number, status,
// actor, the optional event and duration, age, and one-space gutters.
func (c runColumns) fixedWidth() int {
	width := 1 + numberWidth + 1 + c.status + 1 + 1 + c.actor + 1 + ageWidth
	if c.event > 0 {
		width += c.event + 1
	}
	if c.duration > 0 {
		width += c.duration + 1
	}
	return width
}

// cells lays one row out; header and data rows both come through here.
func (c runColumns) cells(prefix, number, status, label, actor, event, duration, age string) string {
	line := prefix + padANSI(number, numberWidth) + " " + padANSI(status, c.status) + " " + padANSI(label, c.label) + " " + padANSI(actor, c.actor)
	if c.event > 0 {
		line += " " + padANSI(event, c.event)
	}
	if c.duration > 0 {
		line += " " + padANSI(duration, c.duration)
	}
	return line + " " + age
}

func runsHeader(st styles, c runColumns, width int) string {
	return dimLine(st, c.cells(" ", "#", "St", truncate("Workflow / detail", c.label), "Actor", "Event", truncate("Duration", c.duration), "Age"), width)
}

func row(st styles, c runColumns, run model.Run, selected, flaky bool, width int, now time.Time) string {
	prefix := " "
	if selected {
		prefix = colorize(st.ok, icons.Cursor)
	}
	number := colorize(st.muted, fmt.Sprintf("#%d", run.RunNumber))
	status := colorize(statusColor(st, run), glyph(run))
	label := runLabel(st, run, c.label)
	if flaky {
		// The flake badge lives inside the label column so every
		// column keeps its width math and the badge survives 80 cols;
		// the glyph is plain ASCII by design.
		label = colorize(st.run, icons.Flake) + " " + runLabel(st, run, c.label-2)
	}
	event := colorize(eventColor(st, run), truncate(run.Event, c.event))
	duration := colorize(durationColor(st, run), sparkline.Render(sparkValues(run), 5))
	runAge := colorize(st.subtle, age(run, now))
	return fitANSI(c.cells(prefix, number, status, label, actorCell(st, run.Actor, c.actor), event, duration, runAge), width)
}

// actorCell renders the original run actor: @login, bots verbatim, and
// an explicit "unavailable" rather than a guess when GitHub sent none.
func actorCell(st styles, actor string, width int) string {
	if strings.TrimSpace(actor) == "" {
		return colorize(st.dim, truncate("unavailable", width))
	}
	return colorize(st.muted, truncate("@"+actor, width))
}

func glyph(run model.Run) string {
	if run.Status == model.StatusCompleted {
		return icons.ForConclusion(run.Conclusion)
	}
	if run.Status == model.StatusWaiting {
		// The gate badge: this run is holding for a deployment review.
		return icons.Gate
	}
	return icons.ForStatus(run.Status)
}

func age(run model.Run, now time.Time) string {
	base := run.UpdatedAt
	if base.IsZero() {
		base = run.RunStartedAt
	}
	if base.IsZero() || now.Before(base) {
		return "now"
	}
	duration := now.Sub(base)
	if duration < time.Second {
		return "now"
	}
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	if duration < 48*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd", int(duration.Hours()/24))
}

func duration(run model.Run) string {
	if run.RunStartedAt.IsZero() || run.UpdatedAt.IsZero() || run.UpdatedAt.Before(run.RunStartedAt) {
		return "queued"
	}
	d := run.UpdatedAt.Sub(run.RunStartedAt)
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func runLabel(st styles, run model.Run, width int) string {
	name := runName(run)
	detail := runDetail(run)
	if detail == "" {
		return colorize(st.fgSoft, truncate(name, width))
	}
	nameWidth := max(min(ansi.StringWidth(name), width/2), 6)
	if ansi.StringWidth(name)+3+ansi.StringWidth(detail) <= width {
		nameWidth = ansi.StringWidth(name)
	}
	detailWidth := max(width-nameWidth-3, 1)
	return colorize(st.fgSoft, truncate(name, nameWidth)) +
		colorize(st.subtle, " · "+truncate(detail, detailWidth))
}

func runName(run model.Run) string {
	name := first(strings.TrimSpace(run.Name), strings.TrimSpace(run.Path), strings.TrimSpace(run.DisplayTitle))
	if name != "" {
		return name
	}
	switch {
	case run.RunNumber > 0:
		return fmt.Sprintf("#%d", run.RunNumber)
	case run.ID > 0:
		return fmt.Sprintf("run %d", run.ID)
	default:
		return ""
	}
}

func runDetail(run model.Run) string {
	for _, value := range []string{run.DisplayTitle, run.HeadBranch, run.Event} {
		value = strings.TrimSpace(value)
		if value != "" && !strings.EqualFold(value, strings.TrimSpace(run.Name)) {
			return value
		}
	}
	return ""
}

func summaryLine(st styles, summary Summary, hasMore bool) string {
	line := colorize(st.fail, fmt.Sprintf("%d failing", summary.Failing)) +
		" · " +
		colorize(st.run, fmt.Sprintf("%d running", summary.Running)) +
		" · " +
		colorize(st.subtle, fmt.Sprintf("%d passed", summary.Passed))
	if summary.Gated > 0 {
		// The contextual gate affordance: A only acts on waiting runs,
		// so it is advertised only while one is on screen.
		line += " · " + colorize(st.info, fmt.Sprintf("%s %d gated · A review", icons.Gate, summary.Gated))
	}
	if hasMore {
		line += " · " + colorize(st.subtle, "G load more")
	}
	return line
}

// filterLines renders the filter rows under the column header: the
// shaded input plus a hint (or error) row while editing, otherwise a
// one-row summary of the applied query.
func filterLines(m Model, st styles, width, matches int) []string {
	if m.InputMode {
		hint := hintLine(m, st, width)
		if m.InputError != "" {
			return append([]string{inputLine(m.Draft, st, width)}, failureLines(st, m.InputError, width)...)
		}
		return []string{inputLine(m.Draft, st, width), hint}
	}
	if strings.TrimSpace(m.Filter) == "" {
		return nil
	}
	q := m.query()
	suffix := ""
	if q.Actor == MeActor && m.Viewer != "" {
		suffix += colorize(st.ok, " → @"+m.Viewer)
	}
	if count := matchCount(m, q, matches); count != "" {
		suffix += colorize(st.subtle, " · "+count)
	}
	suffix += colorize(st.subtle, " · Esc clear")
	// A long query gives way before its count and the Esc hint do.
	query := truncate("/"+m.Filter, max(width-2-ansi.StringWidth(suffix), 8))
	return []string{fitANSI("  "+colorize(st.info, query)+suffix, width)}
}

// matchCount is honest about what it counted: rows GitHub returned for
// a server query, or loaded rows a text search looked through.
func matchCount(m Model, q Query, matches int) string {
	if m.QueryError != "" || (m.Loading && !m.DataCurrent()) {
		return ""
	}
	if len(q.Text) > 0 {
		return numfmt.Group(matches) + " of " + numfmt.Group(len(m.activeRuns())) + " loaded"
	}
	more := ""
	if m.Context.HasMore {
		more = "+"
	}
	noun := "runs"
	if matches == 1 && more == "" {
		noun = "run"
	}
	return fmt.Sprintf("%d%s %s", matches, more, noun)
}

// inputLine is the shaded edit field: prompt, text, and a block cursor
// that stays visible by scrolling long text from the left.
func inputLine(draft string, st styles, width int) string {
	cursor := st.okBG + " " + st.surfaceBG
	body := draft
	if body == "" {
		body = st.dim + "\x1b[3m" + "Filter runs…" + "\x1b[23m" + st.fg
	} else {
		body = textinput.Tail(body, width-6)
	}
	content := "  " + st.ok + "/" + st.fg + " "
	if draft == "" {
		content += cursor + body
	} else {
		content += body + cursor
	}
	return shadedLine(content, width, st.fg+st.surfaceBG)
}

// shadedLine fills the whole row with style, re-applying it after any
// nested reset so the shading never breaks mid-row or bleeds past it.
func shadedLine(content string, width int, style string) string {
	content = fitANSI(content, width)
	content = padANSI(content, width)
	content = strings.ReplaceAll(content, sgrReset, sgrReset+style)
	return style + content + sgrReset
}

var filterHints = []string{"actor:@me", "actor:login", "status:failure", "branch:%s", "event:push"}

// hintLine shows usable examples, dropping whole items from the right
// so the actor examples survive narrow widths.
func hintLine(m Model, st styles, width int) string {
	branch := first(strings.TrimSpace(m.Context.Branch), "main")
	items := make([]string, 0, len(filterHints))
	for _, hint := range filterHints {
		if strings.Contains(hint, "%s") {
			hint = fmt.Sprintf(hint, branch)
		}
		candidate := "  " + strings.Join(append(items, hint), " · ")
		if ansi.StringWidth(candidate) > width {
			break
		}
		items = append(items, hint)
	}
	return dimLine(st, "  "+strings.Join(items, " · "), width)
}

// emptyLines explains a list with nothing to show: why the applied
// query failed, or which constraints matched nothing.
func emptyLines(m Model, st styles, width, matches int) []string {
	if m.QueryError != "" {
		hint := "  / edit · Esc clear"
		if m.query().Actor == MeActor {
			hint = "  actor:<login> still works · / edit · Esc clear"
		}
		return append(failureLines(st, m.QueryError, width), dimLine(st, hint, width))
	}
	if matches > 0 || m.Loading {
		return nil
	}
	where := scopePhrase(m.Context.Scope, m.Context.Branch)
	q := m.query()
	var message string
	switch {
	case strings.TrimSpace(m.Filter) == "":
		message = fmt.Sprintf("No runs %s.", where)
	case !q.Server():
		message = fmt.Sprintf("No loaded runs match %s (%d loaded).", m.Filter, len(m.activeRuns()))
	default:
		message = fmt.Sprintf("No runs match %s %s.", m.Filter, where)
	}
	lines := []string{}
	for _, line := range wrapWords(message, max(width-4, 8)) {
		lines = append(lines, fitANSI("  "+colorize(st.fgSoft, line), width))
	}
	if strings.TrimSpace(m.Filter) != "" {
		hint := "  / edit · Esc clear"
		if m.Context.HasMore {
			hint = "  G search older runs · / edit · Esc clear"
		}
		lines = append(lines, dimLine(st, hint, width))
	}
	return lines
}

// failureLines word-wraps a reason under a fail glyph so advice at the
// end of a long message is never cut off at narrow widths.
func failureLines(st styles, reason string, width int) []string {
	lines := wrapWords(reason, max(width-4, 8))
	for i, line := range lines {
		prefix := "    "
		if i == 0 {
			prefix = "  " + icons.Failure + " "
		}
		lines[i] = fitANSI(colorize(st.fail, prefix+line), width)
	}
	return lines
}

// noticeLines wraps the launch notice (branch-miss advice, repo
// activity) so its advice is read whole.
func noticeLines(st styles, notice string, width int) []string {
	if notice == "" {
		return nil
	}
	lines := wrapWords(notice, max(width-4, 8))
	for i, line := range lines {
		lines[i] = dimLine(st, "  "+line, width)
	}
	return lines
}

func wrapWords(text string, room int) []string {
	lines := []string{}
	current := ""
	for word := range strings.FieldsSeq(text) {
		if current != "" && ansi.StringWidth(current)+1+ansi.StringWidth(word) > room {
			lines = append(lines, current)
			current = word
			continue
		}
		current = strings.TrimSpace(current + " " + word)
	}
	return append(lines, current)
}

func scopePhrase(scope usecase.LaunchScope, branch string) string {
	if scope == usecase.LaunchScopeRepo || strings.TrimSpace(branch) == "" {
		return "across all branches"
	}
	return "on branch " + strings.TrimSpace(branch)
}

func statusColor(st styles, run model.Run) string {
	if run.Status == model.StatusCompleted {
		switch run.Conclusion {
		case model.ConclusionSuccess:
			return st.ok
		case model.ConclusionFailure:
			return st.fail
		case model.ConclusionCancelled, model.ConclusionSkipped, model.ConclusionNeutral, model.ConclusionNone:
			return st.neutral
		case model.ConclusionActionRequired, model.ConclusionTimedOut:
			return st.warn
		default:
			return st.neutral
		}
	}
	switch run.Status {
	case model.StatusInProgress:
		return st.run
	case model.StatusQueued, model.StatusPending, model.StatusRequested, model.StatusWaiting:
		return st.info
	default:
		return st.neutral
	}
}

func eventColor(st styles, run model.Run) string {
	if run.Status == model.StatusInProgress {
		return st.run
	}
	if run.Status != model.StatusCompleted {
		return st.info
	}
	if run.Conclusion == model.ConclusionSuccess {
		return st.ok
	}
	return st.subtle
}

func durationColor(st styles, run model.Run) string {
	if run.Status == model.StatusInProgress {
		return st.run
	}
	if run.Status != model.StatusCompleted {
		return st.info
	}
	if run.Conclusion == model.ConclusionSuccess {
		return st.ok
	}
	return st.fgSoft
}

func sparkValues(run model.Run) []int {
	switch {
	case run.Status == model.StatusInProgress:
		return []int{40, 55, 80, 30, 20}
	case run.Status != model.StatusCompleted:
		return []int{15, 15}
	case run.Conclusion == model.ConclusionSuccess:
		return []int{30, 35, 50, 65, 50}
	case run.Conclusion == model.ConclusionCancelled:
		return []int{25, 25}
	default:
		return []int{35, 60, 100, 70, 45}
	}
}

func colorize(sgr, value string) string {
	return sgr + value + sgrReset
}

func allGreenBandLine(st styles, value string, width int) string {
	return shadedLine(value, width, st.fgSoft+st.bandBG)
}

func successLead(st styles, title string) string {
	return "  " +
		st.ok + sgrBold + icons.Success + sgrNoBold +
		st.fgSoft + "  " +
		st.fg + sgrBold + title + sgrNoBold +
		st.fgSoft
}

// allGreenStatusWidth keeps the de-emphasised list's status slot; the
// header and rows both lay out through it.
func allGreenStatusWidth(width int) int {
	if width < 70 {
		return 2
	}
	return 7
}

func allGreenHeader(st styles, width, actorWidth int) string {
	status := "Status"
	if allGreenStatusWidth(width) < len(status) {
		status = "St"
	}
	left := "  " + padANSI("#", numberWidth) + " " + padANSI(status, allGreenStatusWidth(width)) + " Workflow / detail"
	// Actor and Age right-align in allGreenRow; the labels must too.
	right := padANSI("Actor", actorWidth) + " " + fmt.Sprintf("%*s", ageWidth, "Age")
	return dimLine(st, joinRightANSI(left, right, width), width)
}

func allGreenRow(st styles, run model.Run, selected bool, width, actorWidth int, now time.Time) string {
	prefix := "  "
	if selected {
		prefix = icons.Cursor + " "
	}
	right := padANSI(actorCell(st, run.Actor, actorWidth), actorWidth) + " " + colorize(st.subtle, fmt.Sprintf("%*s", ageWidth, age(run, now)))
	leftFixed := 2 + numberWidth + 1 + allGreenStatusWidth(width) + 1
	labelWidth := max(width-leftFixed-ansi.StringWidth(right)-2, 8)
	left := prefix + padANSI(colorize(st.dim, fmt.Sprintf("#%d", run.RunNumber)), numberWidth) + " " +
		padANSI(colorize(st.ok, glyph(run)), allGreenStatusWidth(width)) + " " + runLabel(st, run, labelWidth)
	return fitANSI(joinRightANSI(left, right, width), width)
}

func scopeTitle(scope usecase.LaunchScope, branch string) string {
	if scope == usecase.LaunchScopeRepo || strings.TrimSpace(branch) == "" {
		return "repo all branches"
	}
	return "branch " + strings.TrimSpace(branch)
}

func plainRunTitle(run model.Run) string {
	name := first(strings.TrimSpace(run.Name), strings.TrimSpace(run.Path), strings.TrimSpace(run.DisplayTitle))
	switch {
	case name != "" && run.RunNumber > 0:
		return fmt.Sprintf("%s #%d", name, run.RunNumber)
	case name != "":
		return name
	case run.RunNumber > 0:
		return fmt.Sprintf("#%d", run.RunNumber)
	case run.ID > 0:
		return fmt.Sprintf("run %d", run.ID)
	default:
		return ""
	}
}

// visibleNotice shows a notice only in the scope it describes: after s
// leaves repo scope, "showing all branches now" is no longer true.
func visibleNotice(m Model) string {
	scope := m.Context.NoticeScope
	if scope == "" {
		scope = usecase.LaunchScopeBranch
	}
	if m.Context.Scope != scope {
		return ""
	}
	return m.Context.Notice
}

func nonFailingHeadline(runs []model.Run, scope string) string {
	for _, run := range runs {
		if run.Status != model.StatusCompleted || run.Conclusion != model.ConclusionSuccess {
			return "No failing checks on " + scope
		}
	}
	return "All checks passing on " + scope
}

func runRowCapacity(height, fixedRows, total int) int {
	if height <= 0 {
		return total
	}
	capacity := height - fixedRows
	if total > capacity {
		capacity--
	}
	return max(capacity, 1)
}

func viewport(selected, total, capacity int) (int, int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	if capacity >= total {
		return 0, total
	}
	start := max(selected-capacity/2, 0)
	if start+capacity > total {
		start = total - capacity
	}
	return start, start + capacity
}

func clampSelection(selected, total int) int {
	if total <= 0 || selected < 0 {
		return 0
	}
	if selected >= total {
		return total - 1
	}
	return selected
}

func pageLine(start, end, total int, hasMore bool) string {
	if total == 0 {
		return "0/0"
	}
	suffix := ""
	if hasMore {
		suffix = "+"
	}
	return fmt.Sprintf("rows %d-%d/%d%s", start+1, end, total, suffix)
}

func dimLine(st styles, value string, width int) string {
	return colorize(st.subtle, padANSI(fitANSI(value, width), width))
}

func joinRightANSI(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = fitANSI(right, width)
	leftWidth := max(width-ansi.StringWidth(right)-2, 1)
	left = fitANSI(left, leftWidth)
	spaces := max(width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", spaces) + right
}

// styles are the theme's tokens as SGR sequences, resolved once per
// frame so T recolours every runs surface.
type styles struct {
	ok, fail, run, info, warn, neutral string
	dim, subtle, muted, fgSoft, fg     string
	surfaceBG, okBG, bandBG            string
}

func stylesFor(th theme.Theme) styles {
	return styles{
		ok: theme.Fg(th.OK), fail: theme.Fg(th.Fail), run: theme.Fg(th.Run),
		info: theme.Fg(th.Info), warn: theme.Fg(th.Warn), neutral: theme.Fg(th.Neutral),
		dim: theme.Fg(th.Dim), subtle: theme.Fg(th.Subtle), muted: theme.Fg(th.Muted),
		fgSoft: theme.Fg(th.FGSoft), fg: theme.Fg(th.FG),
		surfaceBG: theme.Bg(th.Surface), okBG: theme.Bg(th.OK), bandBG: theme.Bg(th.OKTint),
	}
}

const (
	sgrBold   = "\x1b[1m"
	sgrNoBold = "\x1b[22m"
	sgrReset  = "\x1b[0m"
)

func fitANSI(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}

func padANSI(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
