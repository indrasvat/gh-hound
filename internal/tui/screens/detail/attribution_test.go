package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
)

func TestAttributionNamesActorAttemptAndRerunner(t *testing.T) {
	tests := []struct {
		run  model.Run
		want string
	}{
		{model.Run{Actor: "alice", TriggeringActor: "alice", RunAttempt: 1}, "Started by @alice"},
		{model.Run{Actor: "alice", TriggeringActor: "bob", RunAttempt: 2}, "Started by @alice · Attempt 2 · Re-run by @bob"},
		{model.Run{Actor: "alice", TriggeringActor: "ALICE", RunAttempt: 3}, "Started by @alice · Attempt 3"},
		{model.Run{Actor: "alice", TriggeringActor: "bob", RunAttempt: 1}, "Started by @alice · Triggered by @bob"},
		{model.Run{Actor: "dependabot[bot]"}, "Started by @dependabot[bot]"},
		{model.Run{}, "actor unavailable"},
	}
	for _, tt := range tests {
		if got := Attribution(tt.run); got != tt.want {
			t.Fatalf("Attribution(%+v) = %q, want %q", tt.run, got, tt.want)
		}
	}
}

func TestAttributionWrapsInsteadOfTruncatingLogins(t *testing.T) {
	run := model.Run{ID: 1, Name: "CI", RunNumber: 571, HeadBranch: "fix/parser", HeadSHA: "a1b2c3d4", Actor: "alice", TriggeringActor: "a-very-long-contributor-login-name", RunAttempt: 2}
	m := NewModel(run, nil).WithRepo("indrasvat/gh-hound")
	for _, width := range []int{78, 118} {
		view := ansi.Strip(ViewSize(m, width, 20))
		if !strings.Contains(view, "Re-run by @a-very-long-contributor-login-name") {
			t.Fatalf("width %d lost the login:\n%s", width, view)
		}
		for line := range strings.SplitSeq(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
	}
	wide := strings.Split(ansi.Strip(ViewSize(m, 160, 20)), "\n")[0]
	if !strings.HasPrefix(wide, "indrasvat/gh-hound › CI #571") || !strings.HasSuffix(wide, "Re-run by @a-very-long-contributor-login-name") {
		t.Fatalf("wide attribution should share the breadcrumb row: %q", wide)
	}
}

func TestAttributionWrapsByPartAtNarrowWidths(t *testing.T) {
	run := model.Run{ID: 1, Name: "CI", RunNumber: 7, Actor: strings.Repeat("a", 30), TriggeringActor: strings.Repeat("b", 30), RunAttempt: 3}
	view := ansi.Strip(ViewSize(NewModel(run, nil).WithRepo("o/r"), 78, 20))
	if !strings.Contains(view, "Re-run by @"+strings.Repeat("b", 30)) || !strings.Contains(view, "Started by @"+strings.Repeat("a", 30)) {
		t.Fatalf("both logins must survive at 78 columns:\n%s", view)
	}
}
