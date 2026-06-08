package watch

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

func TestModelFollowCancelDebugAndDetach(t *testing.T) {
	m := NewModel(State{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Run:    model.Run{ID: 570, Name: "CI", RunNumber: 570, Status: model.StatusInProgress},
		Lines:  logs.Parse("041 17:43:02Z go test ./...\n042 ok internal/api 0.214s\n").Lines,
	})
	if !m.Follow {
		t.Fatalf("follow should default on")
	}
	m = m.Update(KeyMsg{Key: "f"})
	if m.Follow {
		t.Fatalf("follow should toggle off")
	}
	m = m.Update(KeyMsg{Key: "d"})
	if !m.Debug {
		t.Fatalf("debug should toggle on")
	}
	m = m.Update(KeyMsg{Key: "x"})
	if m.Intent.Kind != IntentCancel {
		t.Fatalf("cancel intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.Intent.Kind != IntentDetach {
		t.Fatalf("detach intent = %#v", m.Intent)
	}
}

func TestViewMatchesStreamingMock(t *testing.T) {
	m := NewModel(State{
		Repo:    "indrasvat/gh-hound",
		Branch:  "main",
		Elapsed: "0m48s",
		Run:     model.Run{ID: 570, Name: "CI", RunNumber: 570, Status: model.StatusInProgress},
		Lines: logs.Parse(strings.Join([]string{
			"17:43:02.781Z go test ./... -race -count=1",
			"ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
			"ok    github.com/indrasvat/gh-hound/internal/render 0.331s",
			"=== RUN   TestLexIdent",
			"=== RUN   TestLexIdent/basic",
			"--- PASS: TestLexIdent/basic (0.00s)",
		}, "\n")).Lines,
	})
	view := View(m, 80)
	plain := ansi.Strip(view)
	for _, want := range []string{
		"watch · CI #570 · main",
		"streaming ⠹ 0m48s follow ●",
		"▾ Run go test ./...",
		"041 17:43:02.781Z go test ./... -race -count=1",
		"046 --- PASS: TestLexIdent/basic (0.00s)█",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("watch view missing %q\n%s", want, view)
		}
	}
	for _, banned := range []string{"041 041", "incoming ▾ active step tail"} {
		if strings.Contains(plain, banned) {
			t.Fatalf("watch view should not contain placeholder/duplicate text %q\n%s", banned, view)
		}
	}
	assertWidth(t, view, 80)
}

func TestViewMatchesMockStreamingColors(t *testing.T) {
	m := NewModel(State{
		Repo:    "indrasvat/gh-hound",
		Branch:  "main",
		Elapsed: "0m48s",
		Run:     model.Run{ID: 570, Name: "CI", RunNumber: 570, Status: model.StatusInProgress},
		Lines: logs.Parse(strings.Join([]string{
			"17:43:02.781Z go test ./... -race -count=1",
			"ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
			"--- PASS: TestLexIdent/basic (0.00s)",
		}, "\n")).Lines,
	})
	view := View(m, 80)
	for _, want := range []string{
		"\x1b[38;2;107;112;96m17:43:02.781Z\x1b[0m",
		"\x1b[38;2;110;156;181mgo test ./... -race -count=1\x1b[0m",
		"\x1b[38;2;79;211;122mok\x1b[0m",
		"\x1b[38;2;224;163;62m--- PASS: TestLexIdent/basic (0.00s)\x1b[0m\x1b[38;2;79;211;122m█\x1b[0m",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("watch view missing styled token %q\n%s", want, view)
		}
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
