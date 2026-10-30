package log

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/theme"
)

var bramble = stylesFor(theme.ForMode(theme.ModeBramble))

// Bone recolours every log surface, tints and fold bands included, and
// leaves no Bramble colour behind; switching Theme back recolours again.
func TestBoneRecoloursTheLog(t *testing.T) {
	bone := theme.ForMode(theme.ModeBone)
	m := search(NewModel(document(), 1, 12), "trail").Update(KeyMsg{Key: "g"})
	m.Theme = bone
	views := []string{View(m, 100), View(m.Update(KeyMsg{Key: "n"}), 100)}
	folded := compactModel(t, pollLog(), 20)
	folded.Theme = bone
	views = append(views, View(folded, 100))
	view := strings.Join(views, "\n")
	for _, want := range []string{
		theme.Fg(bone.FG), theme.Fg(bone.Dim), theme.Fg(bone.Info), theme.Fg(bone.OK), theme.Fg(bone.Fail),
		theme.Fg(bone.Line2), theme.Fg(bone.Run), theme.Bg(bone.RunTint), theme.Bg(bone.RunTintStrong), theme.Bg(bone.Surface), theme.Bg(bone.InfoTint),
	} {
		if !strings.Contains(view, want) {
			t.Errorf("bone log is missing %q", want)
		}
	}
	for _, c := range []string{"\x1b[38;2;", "\x1b[48;2;"} {
		for _, seq := range strings.Split(view, c)[1:] {
			if sgr := c + seq[:strings.IndexByte(seq, 'm')+1]; isBrambleOnly(sgr) {
				t.Fatalf("bone log still carries Bramble colour %q", sgr)
			}
		}
	}
	m.Theme = theme.ForMode(theme.ModeBramble)
	if again := View(m, 100); again == views[0] || !strings.Contains(again, bramble.runTintStrongBG) {
		t.Fatalf("switching Theme back to Bramble did not recolour the log")
	}
}

// isBrambleOnly reports an SGR that is a Bramble token and no Bone one.
func isBrambleOnly(sgr string) bool {
	has := func(th theme.Theme) bool {
		for _, c := range []string{th.BG, th.Surface, th.Surface2, th.Line, th.Line2, th.Dim, th.Subtle, th.Muted, th.FGSoft, th.FG,
			th.OK, th.Fail, th.Run, th.Info, th.Warn, th.RunTint, th.RunTintStrong, th.FailTint, th.InfoTint} {
			if sgr == theme.Fg(c) || sgr == theme.Bg(c) {
				return true
			}
		}
		return false
	}
	return has(theme.ForMode(theme.ModeBramble)) && !has(theme.ForMode(theme.ModeBone))
}

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
	visible := ansi.Strip(view)
	for _, want := range []string{
		"log · /trail · match 1/2",
		"001 17:42:53Z go test ./... -race",
		"002 ▸ Run go test ./... 7 lines",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("log view missing %q\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "010") {
		t.Fatalf("view rendered outside visible viewport:\n%s", visible)
	}
	assertANSIWidth(t, view, 80)
}

func TestViewHighlightsLogTokensAndSearchHits(t *testing.T) {
	m := NewModel(document(), 1, 10)
	m = m.Update(KeyMsg{Key: "/"})
	for _, key := range []string{"t", "r", "a", "i", "l", "enter"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m.Offset = 1

	view := View(m, 100)
	for _, want := range []string{
		bramble.dim + "17:42:53Z" + sgrReset,
		bramble.info + "go test ./... -race" + sgrReset,
		bramble.ok + "ok" + sgrReset,
		bramble.path + "lexer_test.go:88" + sgrReset,
		bramble.ok + "\"foo\"" + sgrReset,
		bramble.fail + "\"foo_\"" + sgrReset,
		bramble.fail + "--- FAIL: TestLexIdent/trailing_underscore (0.00s)" + sgrReset,
		bramble.runTintBG,
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("highlighted log view missing %q\n%s", want, view)
		}
	}
	assertANSIWidth(t, view, 100)
}

func TestRenderLargeLogUsesVisibleWindow(t *testing.T) {
	var builder strings.Builder
	for range 10_000 {
		builder.WriteString("17:42:53Z ok internal/api 0.214s\n")
	}
	m := NewModel(logs.Parse(builder.String()), 9_990, 8)
	start := time.Now()
	view := View(m, 100)
	visible := ansi.Strip(view)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("large render took %s", elapsed)
	}
	if strings.Contains(visible, "0001") || !strings.Contains(visible, "9990") {
		t.Fatalf("viewport did not stay near offset:\n%s", visible)
	}
}

func TestRenderLargeHighlightedLogUsesVisibleWindow(t *testing.T) {
	var builder strings.Builder
	for i := range 10_000 {
		if i%7 == 0 {
			builder.WriteString("2026-06-09T00:02:27.3782201Z ##[error]Process completed with exit code 1\n")
			continue
		}
		builder.WriteString("2026-06-09T00:02:27.3782201Z ok github.com/openclaw/openclaw/internal/api 0.214s\n")
	}
	m := NewModel(logs.Parse(builder.String()), 9_990, 12)
	start := time.Now()
	view := View(m, 120)
	visible := ansi.Strip(view)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("large highlighted render took %s", elapsed)
	}
	if strings.Contains(visible, "0001") || !strings.Contains(visible, "9990") {
		t.Fatalf("viewport did not stay near offset:\n%s", visible)
	}
	if !strings.Contains(view, bramble.dim) || !strings.Contains(view, bramble.ok) {
		t.Fatalf("large highlighted render missed semantic colors:\n%s", view)
	}
	assertANSIWidth(t, view, 120)
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

func assertANSIWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line too wide (%d): %q\n%s", got, ansi.Strip(line), view)
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
