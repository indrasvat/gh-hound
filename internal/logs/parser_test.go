package logs

import (
	"slices"
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

func TestParseWorkflowCommandsAnnotationsMasksAndStopCommands(t *testing.T) {
	raw := strings.Join([]string{
		"::group::Install dependencies",
		"2026-06-09T00:02:27.3782201Z go test ./...",
		"::notice file=app.js,line=1,col=5,endColumn=7,title=Heads%2CUp::Missing semicolon",
		"::warning file=internal/foo.go,line=12,title=lint::shadowed variable",
		"::error file=internal/foo.go,line=18,endLine=19,col=2,endColumn=8,title=Tests::boom",
		"::add-mask::super secret",
		"token super secret and super",
		"::stop-commands::pause-token",
		"::error::this is literal while commands are stopped",
		"::pause-token::",
		"::error::parsed again",
		"::endgroup::",
	}, "\n")

	doc := Parse(raw)
	if len(doc.Folds) != 1 || doc.Folds[0].Title != "Install dependencies" || doc.Folds[0].StartLine != 1 {
		t.Fatalf("folds = %#v", doc.Folds)
	}
	if len(doc.Annotations) != 4 {
		t.Fatalf("annotations = %#v, want notice/warning/error/error", doc.Annotations)
	}
	notice := doc.Annotations[0]
	if notice.Level != "notice" || notice.Path != "app.js" || notice.StartLine != 1 || notice.StartColumn != 5 || notice.EndColumn != 7 || notice.Title != "Heads,Up" || notice.Message != "Missing semicolon" {
		t.Fatalf("notice annotation = %#v", notice)
	}
	warning := doc.Annotations[1]
	if warning.Level != "warning" || warning.Path != "internal/foo.go" || warning.StartLine != 12 || warning.Title != "lint" {
		t.Fatalf("warning annotation = %#v", warning)
	}
	firstError := doc.Annotations[2]
	if firstError.Level != "error" || firstError.Path != "internal/foo.go" || firstError.StartLine != 18 || firstError.EndLine != 19 || firstError.StartColumn != 2 || firstError.EndColumn != 8 || firstError.Message != "boom" {
		t.Fatalf("error annotation = %#v", firstError)
	}
	if disabled := findCommand(doc.Commands, "this is literal while commands are stopped"); disabled != nil {
		t.Fatalf("stopped command was parsed: %#v", disabled)
	}
	if got := doc.Lines[5].Text; got != "::add-mask::***" {
		t.Fatalf("add-mask line = %q", got)
	}
	if got := doc.Lines[6].Text; got != "token *** and ***" {
		t.Fatalf("masked line = %q", got)
	}
	if !slicesContains(doc.Masks, "super secret") || !slicesContains(doc.Masks, "super") || !slicesContains(doc.Masks, "secret") {
		t.Fatalf("masks = %#v", doc.Masks)
	}
	if got := doc.Annotations[len(doc.Annotations)-1].Message; got != "parsed again" {
		t.Fatalf("resume annotation = %q", got)
	}
}

func TestParseLegacyWorkflowCommandsAndEscapes(t *testing.T) {
	raw := strings.Join([]string{
		"##[group]Run legacy commands",
		"##[warning file=legacy.go,line=3,title=Old%20Runner]careful%25now",
		"##[error file=legacy.go,line=5]legacy failure",
		"##[endgroup]",
	}, "\n")

	doc := Parse(raw)
	if len(doc.Folds) != 1 || doc.Folds[0].Title != "Run legacy commands" || doc.Folds[0].EndLine != 3 {
		t.Fatalf("legacy folds = %#v", doc.Folds)
	}
	if len(doc.Annotations) != 2 {
		t.Fatalf("legacy annotations = %#v", doc.Annotations)
	}
	if got := doc.Annotations[0]; got.Level != "warning" || got.Path != "legacy.go" || got.StartLine != 3 || got.Title != "Old%20Runner" || got.Message != "careful%now" {
		t.Fatalf("legacy warning = %#v", got)
	}
	if got := doc.Annotations[1]; got.Level != "error" || got.Path != "legacy.go" || got.StartLine != 5 || got.Message != "legacy failure" {
		t.Fatalf("legacy error = %#v", got)
	}
}

func TestParseTimestampPrefixedWorkflowCommands(t *testing.T) {
	raw := strings.Join([]string{
		"2026-06-09T00:02:27.3782201Z ::group::Run checks",
		"2026-06-09T00:02:28.3782201Z ::error file=main.go,line=9::broken",
		"2026-06-09T00:02:29.3782201Z ::endgroup::",
	}, "\n")

	doc := Parse(raw)
	if len(doc.Folds) != 1 || doc.Folds[0].Title != "Run checks" || doc.Folds[0].StartLine != 1 || doc.Folds[0].EndLine != 2 {
		t.Fatalf("timestamped fold = %#v", doc.Folds)
	}
	if len(doc.Annotations) != 1 || doc.Annotations[0].Path != "main.go" || doc.Annotations[0].Message != "broken" {
		t.Fatalf("timestamped annotations = %#v", doc.Annotations)
	}
}

func TestParseGenericWorkflowCommandMatrix(t *testing.T) {
	raw := strings.Join([]string{
		"::debug::trace",
		"::echo::on",
		"::save-state name=cache%3Akey::warm%25hit",
		"::set-output name=result::ok",
		"::add-path::/tmp/bin",
		"::set-env name=FEATURE::true",
		"##[debug]legacy trace",
	}, "\n")

	doc := Parse(raw)
	if len(doc.Commands) != 7 {
		t.Fatalf("commands = %#v", doc.Commands)
	}
	if got := doc.Commands[2]; got.Name != "save-state" || got.Properties["name"] != "cache:key" || got.Message != "warm%hit" {
		t.Fatalf("save-state command = %#v", got)
	}
	if got := doc.Commands[3]; got.Name != "set-output" || got.Properties["name"] != "result" || got.Message != "ok" {
		t.Fatalf("set-output command = %#v", got)
	}
	if got := doc.Commands[6]; got.Name != "debug" || !got.Legacy || got.Message != "legacy trace" {
		t.Fatalf("legacy debug command = %#v", got)
	}
	if len(doc.Annotations) != 0 {
		t.Fatalf("generic commands should not create annotations: %#v", doc.Annotations)
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

func TestParseLargeSpecHeavyLogWithinBudget(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("::add-mask::very secret token\n")
	for i := range 10_000 {
		switch {
		case i%200 == 0:
			builder.WriteString("::group::shard\n")
		case i%200 == 100:
			builder.WriteString("::endgroup::\n")
		case i%97 == 0:
			builder.WriteString("::warning file=internal/foo.go,line=12,title=lint::shadowed very secret token\n")
		case i%89 == 0:
			builder.WriteString("::error file=internal/foo.go,line=18::boom very secret token\n")
		default:
			builder.WriteString("2026-06-09T00:02:27.3782201Z ok github.com/openclaw/openclaw/internal/api 0.214s very secret token\n")
		}
	}
	start := time.Now()
	doc := Parse(builder.String())
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("spec-heavy 10k parse took %s, want <= 2s under race detector", elapsed)
	}
	if len(doc.Lines) == 0 || len(doc.Folds) == 0 || len(doc.Annotations) == 0 {
		t.Fatalf("spec-heavy parse missed structures: lines=%d folds=%d annotations=%d", len(doc.Lines), len(doc.Folds), len(doc.Annotations))
	}
	if strings.Contains(doc.Lines[len(doc.Lines)-1].Text, "very secret token") {
		t.Fatalf("large log did not redact mask in tail: %q", doc.Lines[len(doc.Lines)-1].Text)
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

func findCommand(commands []Command, message string) *Command {
	for i := range commands {
		if commands[i].Message == message {
			return &commands[i]
		}
	}
	return nil
}

func slicesContains(values []string, want string) bool {
	return slices.Contains(values, want)
}

func lineHasClass(line Line, class TokenClass) bool {
	for _, token := range line.Tokens {
		if token.Class == class {
			return true
		}
	}
	return false
}
