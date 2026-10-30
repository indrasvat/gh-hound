package queue

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
)

func TestQueueViewRendersRequiredBlockedSignals(t *testing.T) {
	view := ViewSize(NewModel(testReport(2)), 120, 40)
	plain := ansi.Strip(view)
	for _, want := range []string{"GitHub queue", "blocked", "1 item(s) ahead", "deploy-prod", "30433642", "30433644", "target"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view missing %q:\n%s", want, plain)
		}
	}
	for _, want := range []string{"◌", "⠹"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing glyph %q:\n%s", want, view)
		}
	}
}

func TestQueueViewFitsEightyColumns(t *testing.T) {
	view := ViewSize(NewModel(testReport(2)), 80, 24)
	for i, line := range strings.Split(ansi.Strip(view), "\n") {
		if width := ansi.StringWidth(line); width > 80 {
			t.Fatalf("line %d width = %d, want <= 80:\n%s", i+1, width, line)
		}
	}
}

func TestQueueViewKeepsSelectionVisibleOnOverflow(t *testing.T) {
	m := NewModel(testReport(120))
	m.Selected = 119
	view := ansi.Strip(ViewSize(m, 120, 24))
	if !strings.Contains(view, "rows ") {
		t.Fatalf("overflow view missing page line:\n%s", view)
	}
	if !strings.Contains(view, "30433644") {
		t.Fatalf("selected target must stay visible:\n%s", view)
	}
}

func TestQueueViewKeepsDetailBlockVisibleOnWideOverflow(t *testing.T) {
	m := NewModel(testReport(120))
	m.Selected = 119
	view := ansi.Strip(ViewSize(m, 120, 34))
	if !strings.Contains(view, "selected") {
		t.Fatalf("wide overflow view dropped detail block:\n%s", view)
	}
	if strings.Contains(view, "\n…") {
		t.Fatalf("wide overflow view should reserve detail height, got clipped body:\n%s", view)
	}
}

func TestQueueViewSuppressesDetailBlockWhenWideHeightIsTight(t *testing.T) {
	m := NewModel(testReport(120))
	m.Selected = 119
	view := ansi.Strip(ViewSize(m, 120, 8))
	if strings.Contains(view, "selected") {
		t.Fatalf("tight wide view should suppress detail block:\n%s", view)
	}
	if lines := strings.Split(view, "\n"); len(lines) > 8 {
		t.Fatalf("tight wide view rendered %d lines, want <= 8:\n%s", len(lines), view)
	}
}

func TestQueueViewHeaderColumnsAlignWithRows(t *testing.T) {
	header := ansi.Strip(memberHeader(styles{}, 120))
	for label, want := range map[string]int{
		"Run":     5,
		"Pos":     15,
		"Name":    20,
		"Group":   50,
		"Verdict": 72,
	} {
		if got := strings.Index(header, label); got != want {
			t.Fatalf("%s column = %d, want %d:\n%s", label, got, want, header)
		}
	}
}

func TestQueueViewShowsEachMemberGroup(t *testing.T) {
	report := testReport(2)
	jobID := int64(798245260)
	report.Groups = append(report.Groups, model.ConcurrencyGroup{
		GroupName: "ci-build",
		Members: []model.ConcurrencyMember{{
			RunID:   30433645,
			JobID:   &jobID,
			JobName: "build",
			Status:  model.StatusPending,
		}},
	})
	view := ansi.Strip(ViewSize(NewModel(report), 120, 40))
	for _, want := range []string{"deploy-prod", "ci-build", "2 group(s)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

// TestQueueViewRecoloursFromTheme pins #56: every colour on the queue
// screen comes from the model's theme, so Bone leaves no Bramble behind.
func TestQueueViewRecoloursFromTheme(t *testing.T) {
	m := NewModel(testReport(3))
	m.Theme = theme.ForMode(theme.ModeBone)
	bone := ViewSize(m, 120, 40)
	bramt, bonet := theme.ForMode(theme.ModeBramble), m.Theme
	for _, token := range []string{bonet.OK, bonet.Run, bonet.Info, bonet.Dim, bonet.Subtle, bonet.FG, bonet.FGSoft} {
		if !strings.Contains(bone, theme.Fg(token)) {
			t.Errorf("Bone view missing token %s", token)
		}
	}
	for _, token := range []string{bramt.OK, bramt.Run, bramt.Info, bramt.Dim, bramt.Subtle, bramt.FG, bramt.FGSoft} {
		if strings.Contains(bone, theme.Fg(token)) {
			t.Errorf("Bone view still carries Bramble token %s", token)
		}
	}
}
