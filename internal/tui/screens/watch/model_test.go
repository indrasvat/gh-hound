package watch

import (
	"strings"
	"testing"

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
			"041 17:43:02.781Z go test ./... -race -count=1",
			"042 ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
			"043 ok    github.com/indrasvat/gh-hound/internal/render 0.331s",
			"044 === RUN   TestLexIdent",
			"045 === RUN   TestLexIdent/basic",
			"046 --- PASS: TestLexIdent/basic (0.00s)",
		}, "\n")).Lines,
	})
	view := View(m, 80)
	for _, want := range []string{
		"watch · CI #570 · main",
		"streaming ⠹ 0m48s follow ●",
		"041 041 17:43:02.781Z go test ./... -race -count=1",
		"046 046 --- PASS: TestLexIdent/basic (0.00s)",
		"incoming ▾ active step tail █",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("watch view missing %q\n%s", want, view)
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
