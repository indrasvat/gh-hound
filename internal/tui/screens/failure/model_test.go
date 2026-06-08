package failure

import (
	"strings"
	"testing"
	"time"

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
	for _, want := range []string{
		"… › build › ✗ go test ./... · step 6 · exit 1",
		"Annotations",
		"✗ _internal/parser/lexer.go:142_ — identifier mismatch",
		"error window · 5 of 8 lines",
		"005 │ HIT      internal/parser/lexer.go:142: got \"foo\" want \"foo_\"",
		"l opens full log at this offset · y copies excerpt",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("failure view missing %q\n%s", want, view)
		}
	}
	assertWidth(t, view, 80)
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if len([]rune(line)) > width {
			t.Fatalf("line too wide (%d): %q\n%s", len([]rune(line)), line, view)
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
			Message:   "identifier mismatch",
			Title:     "go test",
		}},
	}
}
