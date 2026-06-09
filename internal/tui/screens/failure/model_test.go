package failure

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
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
