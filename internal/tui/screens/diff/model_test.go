package diff

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func locatedVerdict() usecase.RegressionVerdict {
	return usecase.RegressionVerdict{
		Repo:     "indrasvat/gh-hound",
		Workflow: "CI",
		Branch:   "main",
		Status:   usecase.RegressionLocated,
		LastGood: model.Run{ID: 572, RunNumber: 572, RunAttempt: 2, HeadSHA: "c2b3a49", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
		FirstBad: model.Run{ID: 573, RunNumber: 573, RunAttempt: 1, HeadSHA: "d3c4b5a", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure},
		SuspectCommits: []model.Commit{
			{SHA: "d3c4b5a9f0e1d2c3b4a5968778695a4b3c2d1e0f", Author: "indrasvat", Message: "feat: sharpen the lexer"},
			{SHA: "cc99aa1b2c3d4e5f60718293a4b5c6d7e8f90a1b", Author: "dependabot[bot]", Message: "chore(deps): bump charmbracelet/x/ansi with an unreasonably long subject line that must be truncated"},
		},
		TotalSuspects: 2,
		CompareURL:    "https://github.com/indrasvat/gh-hound/compare/c2b3a49...d3c4b5a",
		RunsScanned:   4,
		Verdict:       "scent picked up: #572 was clean, #573 wasn't.",
	}
}

func TestDiffSelectionMovesWithinSuspects(t *testing.T) {
	m := NewModel(locatedVerdict())
	if m.Selected != 0 {
		t.Fatalf("initial selection = %d", m.Selected)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if m.Selected != 1 {
		t.Fatalf("selection after j = %d, want 1", m.Selected)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if m.Selected != 1 {
		t.Fatalf("selection must clamp at the last suspect, got %d", m.Selected)
	}
	m = m.Update(KeyMsg{Key: "k"})
	if m.Selected != 0 {
		t.Fatalf("selection after k = %d, want 0", m.Selected)
	}
}

func TestDiffIntents(t *testing.T) {
	m := NewModel(locatedVerdict())
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentOpenFirstBad {
		t.Fatalf("enter intent = %q, want open_first_bad", m.Intent.Kind)
	}
	m = m.Update(KeyMsg{Key: "o"})
	if m.Intent.Kind != IntentBrowser {
		t.Fatalf("o intent = %q, want browser", m.Intent.Kind)
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.Intent.Kind != IntentBack {
		t.Fatalf("esc intent = %q, want back", m.Intent.Kind)
	}
	// j must clear the previous intent.
	m = m.Update(KeyMsg{Key: "j"})
	if m.Intent.Kind != IntentNone {
		t.Fatalf("j left a stale intent %q", m.Intent.Kind)
	}
}

func TestDiffEnterIsInertWithoutBoundary(t *testing.T) {
	verdict := locatedVerdict()
	verdict.Status = usecase.RegressionInconclusive
	verdict.FirstBad = model.Run{}
	verdict.SuspectCommits = nil
	m := NewModel(verdict)
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentNone {
		t.Fatalf("enter on inconclusive verdict = %q, want none", m.Intent.Kind)
	}
}

func TestDiffViewRendersBoundaryAndAlignedSuspects(t *testing.T) {
	view := View(NewModel(locatedVerdict()), 100)
	plain := stripANSI(view)
	if !strings.Contains(plain, "scent picked up: #572 was clean, #573 wasn't.") {
		t.Fatalf("view missing verdict line:\n%s", plain)
	}
	if !strings.Contains(plain, "✔ #572") || !strings.Contains(plain, "✗ #573") {
		t.Fatalf("view missing boundary summary:\n%s", plain)
	}
	if !strings.Contains(plain, "attempt 2") {
		t.Fatalf("view must surface the rerun-flipped attempt:\n%s", plain)
	}
	if !strings.Contains(plain, "suspects · 2 of 2") {
		t.Fatalf("view missing suspect count header:\n%s", plain)
	}
	if !strings.Contains(plain, "d3c4b5a") || !strings.Contains(plain, "indrasvat") {
		t.Fatalf("view missing suspect row:\n%s", plain)
	}
	// sha and author columns align across rows (rune columns: the
	// selection cursor is multi-byte but single-cell).
	runeCol := func(line, needle string) int {
		before, _, ok := strings.Cut(line, needle)
		if !ok {
			return -1
		}
		return len([]rune(before))
	}
	var shaCols []int
	for line := range strings.SplitSeq(plain, "\n") {
		if col := runeCol(line, "d3c4b5a"); col >= 0 && strings.Contains(line, "feat:") {
			shaCols = append(shaCols, col)
		}
		if col := runeCol(line, "cc99aa1"); col >= 0 {
			shaCols = append(shaCols, col)
		}
	}
	if len(shaCols) != 2 || shaCols[0] != shaCols[1] {
		t.Fatalf("sha columns misaligned: %v\n%s", shaCols, plain)
	}
	// Long subjects truncate with an ellipsis instead of wrapping.
	for line := range strings.SplitSeq(plain, "\n") {
		if len([]rune(line)) > 100 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("long subject did not truncate:\n%s", plain)
	}
}

func TestDiffViewInconclusiveAndGreenStates(t *testing.T) {
	verdict := locatedVerdict()
	verdict.Status = usecase.RegressionInconclusive
	verdict.LastGood = model.Run{}
	verdict.FirstBad = model.Run{}
	verdict.SuspectCommits = nil
	verdict.RunsScanned = 1000
	verdict.Verdict = "trail went cold after 1,000 runs."
	plain := stripANSI(View(NewModel(verdict), 80))
	if !strings.Contains(plain, "trail went cold after 1,000 runs.") {
		t.Fatalf("inconclusive view missing verdict:\n%s", plain)
	}
	if !strings.Contains(plain, "diff_max_pages") {
		t.Fatalf("inconclusive view should hint at the page cap:\n%s", plain)
	}

	verdict.Status = usecase.RegressionGreen
	verdict.Verdict = "nothing to chase: #102 came back clean."
	plain = stripANSI(View(NewModel(verdict), 80))
	if !strings.Contains(plain, "nothing to chase") {
		t.Fatalf("green view missing verdict:\n%s", plain)
	}
}

func TestDiffViewSizeBoundsSuspectRows(t *testing.T) {
	verdict := locatedVerdict()
	commits := make([]model.Commit, 40)
	for i := range commits {
		commits[i] = model.Commit{SHA: strings.Repeat("a", 40), Author: "x", Message: "m"}
	}
	verdict.SuspectCommits = commits
	verdict.TotalSuspects = 40
	view := ViewSize(NewModel(verdict), 80, 12)
	if lines := strings.Count(view, "\n") + 1; lines > 12 {
		t.Fatalf("view height = %d lines, want <= 12", lines)
	}
}

func stripANSI(value string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range value {
		switch {
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		case r == 0x1b:
			inEscape = true
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
