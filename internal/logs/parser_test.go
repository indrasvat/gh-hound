package logs

import (
	"strings"
	"testing"
	"time"
)

func TestParseExtractsFailureWindowFoldsAndTokens(t *testing.T) {
	raw := strings.Join([]string{
		"17:42:53.114Z go test ./... -race",
		"##[group] Run go test ./...",
		"ok    internal/api 0.214s",
		"##[group] test output",
		"=== RUN   TestLexIdent/trailing_underscore",
		"    lexer_test.go:88: got \"foo\" want \"foo_\"",
		"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"FAIL  github.com/indrasvat/gh-ghent/internal/parser  0.412s",
		"##[error]Process completed with exit code 1",
		"##[endgroup]",
		"##[endgroup]",
	}, "\n")

	doc := Parse(raw)
	if len(doc.Lines) != 9 {
		t.Fatalf("lines = %d, want 9", len(doc.Lines))
	}
	if len(doc.Folds) != 2 {
		t.Fatalf("folds = %#v, want nested 2 folds", doc.Folds)
	}
	if doc.Folds[0].Title != "Run go test ./..." || doc.Folds[0].StartLine != 2 || doc.Folds[0].EndLine != 9 {
		t.Fatalf("outer fold = %#v", doc.Folds[0])
	}
	if !doc.Failure.Found || doc.Failure.AnchorLine != 6 {
		t.Fatalf("failure = %#v", doc.Failure)
	}
	if !containsLine(doc.Failure.Lines, "lexer_test.go:88") {
		t.Fatalf("failure lines miss assertion: %#v", doc.Failure.Lines)
	}
	if !lineHasClass(doc.Lines[0], ClassTimestamp) || !lineHasClass(doc.Lines[0], ClassCommand) {
		t.Fatalf("timestamp/command tokens missing: %#v", doc.Lines[0].Tokens)
	}
	if !lineHasClass(doc.Lines[5], ClassPath) || !lineHasClass(doc.Lines[5], ClassString) || !lineHasClass(doc.Lines[5], ClassWant) {
		t.Fatalf("assertion tokens missing: %#v", doc.Lines[5].Tokens)
	}
	if !lineHasClass(doc.Lines[8], ClassFail) || !lineHasClass(doc.Lines[8], ClassNumber) {
		t.Fatalf("error tokens missing: %#v", doc.Lines[8].Tokens)
	}
}

func TestParseNoFailureStillReturnsSearchableLines(t *testing.T) {
	doc := Parse("ok    internal/api 0.214s\nPASS\n")
	if doc.Failure.Found {
		t.Fatalf("unexpected failure: %#v", doc.Failure)
	}
	if len(doc.Lines) != 2 || !lineHasClass(doc.Lines[0], ClassOK) {
		t.Fatalf("parsed lines = %#v", doc.Lines)
	}
}

func TestParseLargeLogWithinBudget(t *testing.T) {
	var builder strings.Builder
	for range 10_000 {
		builder.WriteString("ok    internal/api 0.214s\n")
	}
	start := time.Now()
	doc := Parse(builder.String())
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("10k parse took %s, want <= 2s under race detector", elapsed)
	}
	if len(doc.Lines) != 10_000 {
		t.Fatalf("lines = %d, want 10000", len(doc.Lines))
	}
}

func BenchmarkParse10kLines(b *testing.B) {
	var builder strings.Builder
	for range 10_000 {
		builder.WriteString("17:42:53Z ok    internal/api 0.214s\n")
	}
	raw := builder.String()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Parse(raw)
	}
}

func containsLine(lines []Line, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line.Text, needle) {
			return true
		}
	}
	return false
}

func lineHasClass(line Line, class TokenClass) bool {
	for _, token := range line.Tokens {
		if token.Class == class {
			return true
		}
	}
	return false
}
