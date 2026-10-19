package failure

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/icons"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestModelActionsRouteWithSameLogOffset(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", 571, report())
	if m.Offset != 4 {
		t.Fatalf("offset = %d, want anchor line 4", m.Offset)
	}

	for _, tt := range []struct {
		key  string
		kind IntentKind
	}{
		{"l", IntentFullLog},
		{"y", IntentCopyExcerpt},
		{"o", IntentBrowser},
		{"r", IntentRerunJob},
		{"R", IntentRerunFailed},
	} {
		got := m.Update(KeyMsg{Key: tt.key})
		if got.Intent.Kind != tt.kind || got.Intent.Offset != 4 || got.Intent.JobID != 100 {
			t.Fatalf("%s intent = %#v", tt.key, got.Intent)
		}
	}
}

func TestViewRendersAnnotationsExcerptAndFooter(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", 571, report())
	view := View(m, 80)
	plain := ansi.Strip(view)
	for _, want := range []string{
		"… › build › ✗ go test ./... · step 6 · exit 1",
		"Annotations",
		"✗ internal/parser/lexer.go:142",
		"identifier lexer drops trailing underscore",
		"✗ internal/parser/lexer_test.go:88",
		"FAIL TestLexIdent/trailing_underscore",
		"error window · 5 of 8 lines",
		"005 internal/parser/lexer.go:142: got \"foo\" want \"foo_\"",
		"⤓ expand full log (l)",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("failure view missing %q\n%s", want, view)
		}
	}
	assertWidth(t, view, 80)
}

func TestViewMatchesMockErrorWindowChromeAndColors(t *testing.T) {
	view := View(NewModel("indrasvat/gh-hound", 571, report()), 80)
	plain := ansi.Strip(view)
	for _, banned := range []string{"╭", "╮", "╰", "╯", "HIT ", "FAIL ##[error]"} {
		if strings.Contains(plain, banned) {
			t.Fatalf("failure view should use pane/log styling, not marker %q\n%s", banned, view)
		}
	}
	for _, want := range []string{
		"\x1b[38;2;226;86;75m✗\x1b[0m",
		"\x1b[38;2;110;156;181m\x1b[4minternal/parser/lexer.go:142\x1b[0m",
		"\x1b[48;2;43;33;24m",
		"\x1b[38;2;79;211;122m\"foo\"\x1b[0m",
		"\x1b[38;2;226;86;75m\"foo_\"\x1b[0m",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("failure view missing styled token %q\n%s", want, view)
		}
	}
}

func TestViewDoesNotInventMissingStepOrExitCode(t *testing.T) {
	report := usecase.FailureReport{
		Job: model.Job{
			ID:         7001,
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
		},
		Log: logs.Parse("build failed without a parsed exit code"),
	}
	view := ansi.Strip(View(NewModel("openclaw/openclaw", 99, report), 100))
	for _, banned := range []string{"unknown", "step 0", "exit 1"} {
		if strings.Contains(view, banned) {
			t.Fatalf("failure view invented fallback %q\n%s", banned, view)
		}
	}
	if !strings.Contains(view, "job 7001") || !strings.Contains(view, "failed step unavailable") {
		t.Fatalf("failure view did not surface real/missing state clearly:\n%s", view)
	}
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line too wide (%d): %q\n%s", got, line, view)
		}
	}
}

func report() usecase.FailureReport {
	start := time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC)
	raw := strings.Join([]string{
		"17:42:53.114Z go test ./... -race",
		"ok internal/api 0.214s",
		"##[group] test output",
		"=== RUN   TestLexIdent/trailing_underscore",
		"    internal/parser/lexer.go:142: got \"foo\" want \"foo_\"",
		"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"FAIL  github.com/indrasvat/gh-hound/internal/parser  0.412s",
		"##[error]Process completed with exit code 1",
		"##[endgroup]",
	}, "\n")
	return usecase.FailureReport{
		Job: model.Job{
			ID:         100,
			RunID:      571,
			Name:       "build",
			Status:     model.StatusCompleted,
			Conclusion: model.ConclusionFailure,
			StartedAt:  start,
			Steps: []model.Step{{
				Number:     6,
				Name:       "go test ./...",
				Status:     model.StatusCompleted,
				Conclusion: model.ConclusionFailure,
			}},
			HTMLURL: "https://github.com/indrasvat/gh-hound/actions/runs/571/job/100",
		},
		Log: logs.Parse(raw),
		Annotations: []model.Annotation{{
			Path:      "internal/parser/lexer.go",
			StartLine: 142,
			EndLine:   142,
			Level:     "failure",
			Message:   "identifier lexer drops trailing underscore",
			Title:     "go test",
		}, {
			Path:      "internal/parser/lexer_test.go",
			StartLine: 88,
			EndLine:   88,
			Level:     "failure",
			Message:   "FAIL TestLexIdent/trailing_underscore",
			Title:     "go test",
		}},
	}
}

func TestErrorWindowCountsTheLinesItDraws(t *testing.T) {
	r := report()
	lines := make([]string, 0, 1587)
	for i := range 1583 {
		lines = append(lines, fmt.Sprintf("ok internal/pkg%d 0.1s", i))
	}
	lines = append(lines, "--- FAIL: TestLexIdent (0.00s)", "FAIL", "github.com/indrasvat/gh-hound/internal/parser", "##[error]Process completed with exit code 1")
	r.Log = logs.Parse(strings.Join(lines, "\n"))
	m := NewModel("indrasvat/gh-hound", 571, r)
	plain := ansi.Strip(View(m, 120))
	drawn := 0
	for l := range strings.SplitSeq(plain, "\n") {
		if len(l) > 4 && l[0] == '1' && l[4] == ' ' {
			drawn++
		}
	}
	if drawn == 0 || drawn >= 12 || !strings.Contains(plain, fmt.Sprintf("error window · %d of 1,587 lines", drawn)) {
		t.Fatalf("header must count the %d drawn lines of 1,587:\n%s", drawn, plain)
	}
}

func TestErrorWindowCountsOnlyTheRowsThatFit(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", 571, report())
	view := ViewSize(m, 80, 8)
	lines := strings.Split(ansi.Strip(view), "\n")
	if len(lines) > 8 || !strings.Contains(view, "error window · 3 of 8 lines") || !strings.HasPrefix(lines[len(lines)-1], "006 ") {
		t.Fatalf("8 rows hold the header, 2 annotations and 3 excerpt lines:\n%s", ansi.Strip(view))
	}
	if full := ansi.Strip(ViewSize(m, 80, 0)); !strings.Contains(full, "error window · 5 of 8 lines") {
		t.Fatalf("no height draws the whole excerpt:\n%s", full)
	}
	flaky := m.WithFlake(usecase.JobFlake{Job: "build", FlakedRuns: 2, Verdict: usecase.FlakeStatus("flaky")}, 10)
	for h := 8; h <= 14; h++ {
		lines := strings.Split(ansi.Strip(ViewSize(flaky, 80, h)), "\n")
		drawn := 0
		for _, l := range lines {
			if len(l) > 3 && l[3] == ' ' && strings.Trim(l[:3], "0123456789") == "" {
				drawn++
			}
		}
		if len(lines) > h || !strings.Contains(strings.Join(lines, "\n"), fmt.Sprintf("error window · %d of 8 lines", drawn)) {
			t.Fatalf("height %d: %d lines, header must count %d drawn rows:\n%s", h, len(lines), drawn, strings.Join(lines, "\n"))
		}
	}
	if view := ansi.Strip(ViewSize(flaky, 80, 8)); len(strings.Split(view, "\n")) > 8 || strings.Contains(view, "seen this one before") {
		t.Fatalf("a cut excerpt leaves no room for the flake panel:\n%s", view)
	}
}

func TestExcerptTabsAndEscapesKeepTheRowWidth(t *testing.T) {
	r := report()
	r.Log = logs.Parse("ok\n--- FAIL: TestX\nZ\thave ()\x1b[2K\nZ\twant (bool)\n##[error]exit code 1")
	view := View(NewModel("indrasvat/gh-hound", 571, r), 80)
	for l := range strings.SplitSeq(view, "\n") {
		if strings.ContainsAny(l, "\t") || strings.Contains(l, "\x1b[2K") || ansi.StringWidth(l) > 80 {
			t.Fatalf("a row moves the cursor or overflows: %q", l)
		}
	}
}

func flakyModel() Model {
	evidence := make([]usecase.FlakeEvidence, 0, 4)
	for i := range 4 {
		evidence = append(evidence, usecase.FlakeEvidence{
			Run: model.Run{ID: int64(560 + i), RunNumber: 560 + i}, Kind: "attempt_flip", Detail: "failed, then passed",
		})
	}
	return NewModel("indrasvat/gh-hound", 571, report()).WithFlake(usecase.JobFlake{
		Job: "build", FlakedRuns: 2, Verdict: usecase.FlakeStatus("flaky"), Evidence: evidence,
	}, 10)
}

// drawnRows reads the view: the excerpt line numbers on screen, the
// evidence rows on screen, and whether the selection marker is one of
// them.
func drawnRows(m Model) (excerpt []int, evidence int, selectedDrawn bool) {
	for line := range strings.SplitSeq(ansi.Strip(ViewSize(m, 80, m.Height)), "\n") {
		var n int
		if _, err := fmt.Sscanf(line, "%03d ", &n); err == nil && len(line) > 3 && line[3] == ' ' {
			excerpt = append(excerpt, n)
		}
		if strings.Contains(line, " attempt_flip ") {
			evidence++
			selectedDrawn = selectedDrawn || strings.HasPrefix(strings.TrimSpace(line), icons.Cursor)
		}
	}
	return excerpt, evidence, selectedDrawn
}

// A cut view only offers what it draws, at every terminal height from 8
// to 24 (body 2 to 18): j/k reaches every excerpt line, tab skips a
// panel with no evidence drawn, and the selection stays on a drawn row.
func TestCutFailureViewOnlyOffersWhatItDraws(t *testing.T) {
	failing := len(report().Log.Failure.Lines)
	for body := 2; body <= 18; body++ {
		m := flakyModel()
		m.Height = body
		seen := map[int]bool{}
		for _, key := range append(slices.Repeat([]string{"j"}, 12), slices.Repeat([]string{"k"}, 12)...) {
			m = m.Update(KeyMsg{Key: key})
			excerpt, _, _ := drawnRows(m)
			for _, n := range excerpt {
				seen[n] = true
			}
		}
		if len(seen) > 0 && len(seen) != failing {
			t.Fatalf("body %d: j/k reached %d of %d excerpt lines", body, len(seen), failing)
		}
		_, evidence, _ := drawnRows(m)
		m = m.Update(KeyMsg{Key: "tab"})
		if m.PanelFocus != (evidence > 0) {
			t.Fatalf("body %d: tab focus = %v with %d evidence rows drawn", body, m.PanelFocus, evidence)
		}
		for range 6 {
			m = m.Update(KeyMsg{Key: "j"})
			if _, _, selected := drawnRows(m); m.PanelFocus && !selected {
				t.Fatalf("body %d: the selection left the drawn rows (selected %d)", body, m.PanelSelected)
			}
		}
	}
}

// The failure can open with its window past the last full window (the
// anchor sits near the end): j must not move it up.
func TestJNeverScrollsTheExcerptUp(t *testing.T) {
	r := report()
	r.Log = logs.Parse("one\ntwo\n--- FAIL: TestX\nfour\n##[error]Process completed with exit code 1")
	m := NewModel("indrasvat/gh-hound", 571, r)
	for _, height := range []int{0, 7, 30} {
		m.Height = height
		before, _, _ := drawnRows(m)
		after, _, _ := drawnRows(m.Update(KeyMsg{Key: "j"}))
		if after[0] < before[0] {
			t.Fatalf("height %d: j moved the window up from %v to %v", height, before, after)
		}
	}
}

func TestGrowingTheViewRefillsAScrolledExcerpt(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", 571, report())
	m.Height = 4 // one excerpt row
	for range 10 {
		m = m.Update(KeyMsg{Key: "j"})
	}
	m.Height = 30
	if excerpt, _, _ := drawnRows(m); len(excerpt) != len(m.Report.Log.Failure.Lines) && len(excerpt) != 5 {
		t.Fatalf("grown view draws %v, want a full window", excerpt)
	}
}

func TestJKDoNothingWithNoExcerptRowDrawn(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", 571, report())
	m.Height = 4 // header, two annotations, error window
	if got := m.Update(KeyMsg{Key: "j"}); got.Offset != m.Offset {
		t.Fatalf("j moved an undrawn excerpt from %d to %d", m.Offset, got.Offset)
	}
}

func TestMultiLineAnnotationTakesOneRow(t *testing.T) {
	r := report()
	r.Annotations[0].Message = "first line\nsecond line\r\nthird"
	m := NewModel("indrasvat/gh-hound", 571, r)
	for height := 6; height <= 12; height++ {
		if lines := strings.Split(ViewSize(m, 80, height), "\n"); len(lines) > height {
			t.Fatalf("height %d drew %d lines", height, len(lines))
		}
	}
}

func TestAFailureNearTheEndOpensTheFullLogWhereItOpened(t *testing.T) {
	r := report()
	r.Log = logs.Parse("one\ntwo\n--- FAIL: TestX\nfour\n##[error]Process completed with exit code 1")
	m := NewModel("indrasvat/gh-hound", 571, r)
	opened := m.Offset
	m.Height = 30
	if got := m.Update(KeyMsg{Key: "l"}).Intent.Offset; got != opened {
		t.Fatalf("l opens the full log at %d, want %d where the view opened", got, opened)
	}
}
