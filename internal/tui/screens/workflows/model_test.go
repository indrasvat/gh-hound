package workflows

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
)

func fixtureWorkflows() []model.Workflow {
	return []model.Workflow{
		{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml", State: model.WorkflowStateActive, HTMLURL: "https://github.com/indrasvat/gh-hound/actions/workflows/ci.yml"},
		{ID: 124, Name: "Nightly Sweep", Path: ".github/workflows/nightly.yml", State: model.WorkflowStateDisabledInactivity},
		{ID: 125, Name: "Stale Patrol", Path: ".github/workflows/stale.yml", State: model.WorkflowStateDisabledManually},
		{ID: 126, Name: "Fork Gate", Path: ".github/workflows/fork-gate.yml", State: model.WorkflowStateDisabledFork},
		{ID: 127, Name: "Old Patrol", Path: ".github/workflows/old-patrol.yml", State: model.WorkflowStateDeleted},
		{ID: 128, Name: "Future Hound", Path: ".github/workflows/future.yml", State: "disabled_by_future_rule"},
	}
}

func TestUpdateMovesSelectionAndEmitsToggleIntentOnlyWhenValid(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", fixtureWorkflows())
	m = m.Update(KeyMsg{Key: "j"})
	if m.Selected != 1 {
		t.Fatalf("selected = %d, want 1", m.Selected)
	}
	m = m.Update(KeyMsg{Key: "e"})
	if m.Intent.Kind != IntentToggle || m.Intent.Workflow.ID != 124 {
		t.Fatalf("intent = %#v, want toggle on Nightly Sweep", m.Intent)
	}

	// disabled_fork: the badge gets a why-line instead of the toggle.
	m = NewModel("indrasvat/gh-hound", fixtureWorkflows())
	for range 3 {
		m = m.Update(KeyMsg{Key: "j"})
	}
	m = m.Update(KeyMsg{Key: "e"})
	if m.Intent.Kind == IntentToggle {
		t.Fatal("fork-disabled workflow must not offer the toggle")
	}

	// deleted: same refusal.
	m = m.Update(KeyMsg{Key: "j"})
	m = m.Update(KeyMsg{Key: "e"})
	if m.Intent.Kind == IntentToggle {
		t.Fatal("deleted workflow must not offer the toggle")
	}

	// unknown future state: rendered, never toggled.
	m = m.Update(KeyMsg{Key: "j"})
	m = m.Update(KeyMsg{Key: "e"})
	if m.Intent.Kind == IntentToggle {
		t.Fatal("unknown-state workflow must not offer the toggle")
	}

	m = m.Update(KeyMsg{Key: "esc"})
	if m.Intent.Kind != IntentBack {
		t.Fatalf("esc intent = %#v", m.Intent)
	}
}

func TestWithToggledFlipsStateLocallyWithoutRefetch(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", fixtureWorkflows())
	m = m.WithToggled(".github/workflows/nightly.yml", true)
	if m.Workflows[1].State != model.WorkflowStateActive {
		t.Fatalf("enable flip state = %q", m.Workflows[1].State)
	}
	m = m.WithToggled("123", false)
	if m.Workflows[0].State != model.WorkflowStateDisabledManually {
		t.Fatalf("disable flip state = %q", m.Workflows[0].State)
	}
}

func TestViewRendersBadgesWhyLinesAndAlignedHeader(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", fixtureWorkflows())
	view := ansi.Strip(View(m, 100))
	for _, want := range []string{"◌ asleep", "⊘ muzzled", "⊘ fork-disabled", "✗ deleted", "✔ active"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing badge %q:\n%s", want, view)
		}
	}
	// Unknown states render verbatim with the neutral badge.
	if !strings.Contains(view, "disabled_by_future_rule") {
		t.Fatalf("unknown state must render verbatim:\n%s", view)
	}
	// Header columns derive from the same width math as the rows: the
	// State header must start at the same column as the badges.
	lines := strings.Split(view, "\n")
	headerIdx := strings.Index(lines[0], "State")
	if headerIdx <= 0 {
		t.Fatalf("header missing State column:\n%s", view)
	}
	// Measure on a non-selected row: everything before the badge is
	// ASCII there, so byte offsets equal columns.
	rowIdx := strings.Index(lines[2], "◌ asleep")
	if rowIdx != headerIdx {
		t.Fatalf("State header at col %d but badge at col %d:\n%s", headerIdx, rowIdx, view)
	}

	// Toggleable selection advertises the leash.
	if !strings.Contains(view, "e muzzle") {
		t.Fatalf("active selection must advertise e muzzle:\n%s", view)
	}
	m = m.Update(KeyMsg{Key: "j"})
	view = ansi.Strip(View(m, 100))
	if !strings.Contains(view, "e wake") {
		t.Fatalf("asleep selection must advertise e wake:\n%s", view)
	}
	if !strings.Contains(view, "fell asleep after 60 quiet days") {
		t.Fatalf("asleep selection must explain the 60-day nap:\n%s", view)
	}

	// Non-toggleable selection gets the why-line instead of the toggle.
	for range 2 {
		m = m.Update(KeyMsg{Key: "j"})
	}
	view = ansi.Strip(View(m, 100))
	if strings.Contains(view, "e wake") || strings.Contains(view, "e muzzle") {
		t.Fatalf("fork-disabled selection must not advertise the toggle:\n%s", view)
	}
	if !strings.Contains(view, "the fork holds this leash") {
		t.Fatalf("fork-disabled why-line missing:\n%s", view)
	}
	m = m.Update(KeyMsg{Key: "j"})
	view = ansi.Strip(View(m, 100))
	if !strings.Contains(view, "nothing left to wake") {
		t.Fatalf("deleted why-line missing:\n%s", view)
	}
}

func TestSummaryCountsUseSingularAndPlural(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", fixtureWorkflows())
	view := ansi.Strip(View(m, 120))
	if !strings.Contains(view, "6 workflows") {
		t.Fatalf("summary missing plural count:\n%s", view)
	}
	if !strings.Contains(view, "1 asleep") || !strings.Contains(view, "1 muzzled") {
		t.Fatalf("summary missing state counts:\n%s", view)
	}

	single := NewModel("indrasvat/gh-hound", fixtureWorkflows()[:1])
	view = ansi.Strip(View(single, 120))
	if !strings.Contains(view, "1 workflow ") && !strings.HasSuffix(strings.TrimRight(view, "\n "), "1 workflow") && !strings.Contains(view, "1 workflow ·") {
		t.Fatalf("summary must use the singular for one workflow:\n%s", view)
	}
	if strings.Contains(view, "1 workflows") {
		t.Fatalf("singular count must not read '1 workflows':\n%s", view)
	}
}

func TestViewFitsNarrowWidths(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", fixtureWorkflows())
	for _, width := range []int{78, 118, 198} {
		view := View(m, width)
		for line := range strings.SplitSeq(view, "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d: line overflows (%d): %q", width, got, ansi.Strip(line))
			}
		}
	}
}
