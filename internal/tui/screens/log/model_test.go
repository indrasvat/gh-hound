package log

import (
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func TestModelSearchFoldWrapAndOffset(t *testing.T) {
	m := NewModel(document(), 5, 6)
	if m.Offset != 5 || m.Height != 6 {
		t.Fatalf("initial model = %#v", m)
	}

	m = m.Update(KeyMsg{Key: "/"})
	m = m.Update(KeyMsg{Key: "t"})
	m = m.Update(KeyMsg{Key: "r"})
	m = m.Update(KeyMsg{Key: "a"})
	m = m.Update(KeyMsg{Key: "i"})
	m = m.Update(KeyMsg{Key: "l"})
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Search.Query != "trail" || m.Search.Total != 2 || m.Search.Current != 1 {
		t.Fatalf("search = %#v", m.Search)
	}
	m = m.Update(KeyMsg{Key: "n"})
	if m.Search.Current != 2 || m.Offset != 6 {
		t.Fatalf("next match search=%#v offset=%d", m.Search, m.Offset)
	}
	m = m.Update(KeyMsg{Key: "N"})
	if m.Search.Current != 1 || m.Offset != 5 {
		t.Fatalf("previous match search=%#v offset=%d", m.Search, m.Offset)
	}

	m = m.Update(KeyMsg{Key: "z"})
	if !m.Collapsed(2) {
		t.Fatalf("fold at line 2 should be collapsed")
	}
	m = m.Update(KeyMsg{Key: "Z"})
	if m.Collapsed(2) {
		t.Fatalf("fold at line 2 should be expanded")
	}
	m = m.Update(KeyMsg{Key: "w"})
	if !m.Wrap {
		t.Fatalf("wrap should toggle on")
	}
}

func TestViewRendersViewportOnlyGutterFoldsSearchAndFooter(t *testing.T) {
	m := NewModel(document(), 1, 7)
	m = m.Update(KeyMsg{Key: "/"})
	m = m.Update(KeyMsg{Key: "t"})
	m = m.Update(KeyMsg{Key: "r"})
	m = m.Update(KeyMsg{Key: "a"})
	m = m.Update(KeyMsg{Key: "i"})
	m = m.Update(KeyMsg{Key: "l"})
	m = m.Update(KeyMsg{Key: "enter"})
	m.Offset = 1
	m = m.Update(KeyMsg{Key: "z"})

	view := View(m, 80)
	for _, want := range []string{
		"log · /trail · match 1/2",
		"001 17:42:53Z go test ./... -race",
		"002 ▸ Run go test ./... 7 lines",
		"j/k scroll · g/G ends · / search · n/N match · z/Z fold · w wrap · esc back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("log view missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "010") {
		t.Fatalf("view rendered outside visible viewport:\n%s", view)
	}
	assertWidth(t, view, 80)
}

func TestRenderLargeLogUsesVisibleWindow(t *testing.T) {
	var builder strings.Builder
	for range 10_000 {
		builder.WriteString("17:42:53Z ok internal/api 0.214s\n")
	}
	m := NewModel(logs.Parse(builder.String()), 9_990, 8)
	start := time.Now()
	view := View(m, 100)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("large render took %s", elapsed)
	}
	if strings.Contains(view, "0001") || !strings.Contains(view, "9990") {
		t.Fatalf("viewport did not stay near offset:\n%s", view)
	}
}

func BenchmarkView10kLineViewport(b *testing.B) {
	var builder strings.Builder
	for range 10_000 {
		builder.WriteString("17:42:53Z ok internal/api 0.214s\n")
	}
	m := NewModel(logs.Parse(builder.String()), 9_990, 8)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = View(m, 100)
	}
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if len([]rune(line)) > width {
			t.Fatalf("line too wide (%d): %q\n%s", len([]rune(line)), line, view)
		}
	}
}

func document() logs.Document {
	return logs.Parse(strings.Join([]string{
		"17:42:53Z go test ./... -race",
		"##[group] Run go test ./...",
		"ok    internal/api 0.214s",
		"##[group] test output",
		"=== RUN   TestLexIdent/trailing_underscore",
		"    lexer_test.go:88: got \"foo\" want \"foo_\"",
		"--- FAIL: TestLexIdent/trailing_underscore (0.00s)",
		"FAIL  github.com/indrasvat/gh-hound/internal/parser  0.412s",
		"##[error]Process completed with exit code 1",
		"##[endgroup]",
		"##[endgroup]",
	}, "\n"))
}
