package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func actorModel() Model {
	runs := []model.Run{
		run(701, "CI", "push", model.StatusCompleted, model.ConclusionFailure),
		run(700, "Release", "workflow_dispatch", model.StatusInProgress, model.ConclusionNone),
		run(699, "CodeQL", "schedule", model.StatusCompleted, model.ConclusionSuccess),
		run(698, "Docs", "pull_request", model.StatusCompleted, model.ConclusionSuccess),
	}
	runs[0].Actor = "alice"
	runs[1].Actor = "a-very-long-contributor-login-name"
	runs[2].Actor = "dependabot[bot]"
	return NewModel(usecase.LaunchContext{Repo: "cli/cli", Branch: "trunk", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, Runs: runs, BranchRuns: runs})
}

func TestRunsColumnsAlignHeaderWithRowsAtEveryWidth(t *testing.T) {
	now := time.Date(2026, 6, 7, 17, 45, 0, 0, time.UTC)
	for _, width := range []int{46, 78, 92, 118, 198} {
		lines := strings.Split(ansi.Strip(ViewSize(actorModel(), width, 20, now)), "\n")
		header := lines[0]
		for _, label := range []string{"Workflow", "Actor", "Age"} {
			found := strings.Contains(header, label)
			if !found {
				t.Fatalf("width %d: header lacks %q: %q", width, label, header)
			}
		}
		actorCol := len([]rune(header[:strings.Index(header, "Actor")]))
		ageCol := len([]rune(header[:strings.Index(header, "Age")]))
		for _, line := range lines[1:5] {
			runes := []rune(line)
			if len(runes) > width {
				t.Fatalf("width %d: row overflows: %q", width, line)
			}
			if cell := strings.TrimSpace(string(runes[actorCol:min(actorCol+11, len(runes))])); !strings.HasPrefix(cell, "@") && !strings.HasPrefix(cell, "unavailable") {
				t.Fatalf("width %d: actor not under its header in %q (cell %q)", width, line, cell)
			}
			if age := strings.TrimSpace(string(runes[ageCol:])); age == "" {
				t.Fatalf("width %d: age missing under header in %q", width, line)
			}
		}
	}
}

func TestNarrowWidthsYieldDurationThenEventNeverActorOrAge(t *testing.T) {
	tests := []struct {
		width                   int
		wantEvent, wantDuration bool
	}{
		{198, true, true},
		{118, true, true},
		{100, true, true},
		{92, true, true},
		{88, true, false},
		{78, false, false},
	}
	for _, tt := range tests {
		c := columnsFor(tt.width)
		if (c.event > 0) != tt.wantEvent || (c.duration > 0) != tt.wantDuration {
			t.Fatalf("width %d: event=%d duration=%d", tt.width, c.event, c.duration)
		}
		if c.actor < 12 || c.label < minLabelWidth || c.fixedWidth()+c.label != tt.width {
			t.Fatalf("width %d: actor=%d label=%d fixed=%d", tt.width, c.actor, c.label, c.fixedWidth())
		}
	}
	if columnsFor(198).actor <= columnsFor(118).actor {
		t.Fatal("wide terminals give Actor more room")
	}
}

func TestActorCellsHandleLongBotAndMissingLogins(t *testing.T) {
	m := actorModel()
	m.Context.Runs[3].Actor = ""
	m.Context.BranchRuns = m.Context.Runs
	view := ansi.Strip(ViewSize(m, 118, 20, time.Now()))
	for _, want := range []string{"@alice", "@a-very-long-co…", "@dependabot[bot]", "unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestInputLineIsShadedWithCursorAndNeverBleeds(t *testing.T) {
	st := stylesFor(theme.ForMode(theme.ModeBramble))
	empty := inputLine("", st, 60)
	if !strings.Contains(ansi.Strip(empty), "/  Filter runs…") || !strings.Contains(empty, st.okBG+" ") {
		t.Fatalf("empty input needs placeholder and cursor: %q", empty)
	}
	typed := inputLine("actor:@me", st, 60)
	if !strings.HasPrefix(typed, st.fg+st.surfaceBG) || !strings.HasSuffix(typed, sgrReset) {
		t.Fatalf("shading must open the row and close it with a reset: %q", typed)
	}
	if got := ansi.StringWidth(typed); got != 60 {
		t.Fatalf("shaded row width = %d, want the full 60", got)
	}
	if strings.Count(typed, sgrReset) != 1 {
		t.Fatalf("a nested reset would end the shading mid-row: %q", typed)
	}
	long := inputLine(strings.Repeat("x", 80)+"END", st, 40)
	if plain := ansi.Strip(long); !strings.Contains(plain, "END") || ansi.StringWidth(long) != 40 {
		t.Fatalf("long input must scroll to keep the cursor end visible: %q", plain)
	}
}

func TestHintRowKeepsActorExamplesAtNarrowWidths(t *testing.T) {
	m := actorModel()
	st := stylesFor(theme.ForMode(theme.ModeBramble))
	wide := ansi.Strip(hintLine(m, st, 118))
	if !strings.Contains(wide, "actor:@me · actor:login · status:failure · branch:trunk · event:push") {
		t.Fatalf("wide hint: %q", wide)
	}
	narrow := ansi.Strip(hintLine(m, st, 40))
	if !strings.Contains(narrow, "actor:@me · actor:login") || strings.Contains(narrow, "event:push") || strings.Contains(narrow, "…") {
		t.Fatalf("narrow hint must drop whole items from the right: %q", narrow)
	}
}

func TestEditingKeepsResultsUntilEnter(t *testing.T) {
	m := actorModel()
	m = m.Update(KeyMsg{Key: "/"})
	for _, key := range []string{"z", "z"} {
		m = m.Update(KeyMsg{Key: key})
	}
	if len(m.FilteredRuns()) != 4 || m.Filter != "" {
		t.Fatalf("draft must not filter before Enter: %d rows, filter %q", len(m.FilteredRuns()), m.Filter)
	}
	view := ansi.Strip(ViewSize(m, 118, 20, time.Now()))
	if !strings.Contains(view, "/ zz") || !strings.Contains(view, "actor:@me · actor:login") {
		t.Fatalf("editing view:\n%s", view)
	}
}

func TestSummaryCountsAreHonestAboutWhatTheyCounted(t *testing.T) {
	m := actorModel()
	m = m.Apply("change")
	if got := ansi.Strip(strings.Join(filterLines(m, stylesFor(m.theme()), 118, 0), "")); !strings.Contains(got, "/change · 0 of 4 loaded · Esc clear") {
		t.Fatalf("text search count: %q", got)
	}
	m = m.Apply("actor:alice")
	m.DataKeys = map[usecase.LaunchScope]string{usecase.LaunchScopeBranch: m.ServerKey()}
	m.Context.HasMore = true
	if got := ansi.Strip(strings.Join(filterLines(m, stylesFor(m.theme()), 118, 4), "")); !strings.Contains(got, "/actor:alice · 4+ runs · Esc clear") {
		t.Fatalf("server count: %q", got)
	}
	m.Loading = true
	m.DataKeys = nil
	if got := ansi.Strip(strings.Join(filterLines(m, stylesFor(m.theme()), 118, 1), "")); strings.Contains(got, "runs") {
		t.Fatalf("a loading query must not claim a count for stale rows: %q", got)
	}
}

func TestEmptyAndErrorStatesNameTheirCause(t *testing.T) {
	m := actorModel().Apply("actor:carol status:failure")
	m.DataKeys = map[usecase.LaunchScope]string{usecase.LaunchScopeBranch: m.ServerKey()}
	m.Context.Runs, m.Context.BranchRuns = nil, nil
	view := ansi.Strip(ViewSize(m, 118, 20, time.Now()))
	if !strings.Contains(view, "No runs match actor:carol status:failure on branch trunk.") || !strings.Contains(view, "/ edit · Esc clear") {
		t.Fatalf("empty state:\n%s", view)
	}
	m = actorModel().Apply("actor:@me")
	m.QueryError = "can't resolve @me: not signed in to GitHub (set GH_TOKEN or run gh auth login)"
	view = ansi.Strip(ViewSize(m, 118, 20, time.Now()))
	if !strings.Contains(view, "✗ can't resolve @me") || !strings.Contains(view, "actor:<login> still works") || strings.Contains(view, "#701") {
		t.Fatalf("error state:\n%s", view)
	}
}

func TestThemeRecoloursRunsSurfaces(t *testing.T) {
	bone := theme.ForMode(theme.ModeBone)
	m := actorModel()
	m.Theme = bone
	m = m.Update(KeyMsg{Key: "/"})
	view := ViewSize(m, 118, 20, time.Now())
	for _, token := range []string{bone.Fail, bone.Subtle, bone.Surface, bone.OK} {
		if !strings.Contains(view, sgrHex(token, false)) && !strings.Contains(view, sgrHex(token, true)) {
			t.Fatalf("Bone token %s not used", token)
		}
	}
	bramble := theme.ForMode(theme.ModeBramble)
	if strings.Contains(view, sgrHex(bramble.FGSoft, false)) || strings.Contains(view, sgrHex(bramble.Fail, false)) {
		t.Fatal("Bramble colours leaked into the Bone runs list")
	}
}
