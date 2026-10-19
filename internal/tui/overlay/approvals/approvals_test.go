package approvals

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestCommentBackspaceRemovesAWholeRune(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, []model.PendingDeployment{{EnvironmentID: 1, EnvironmentName: "production", CurrentUserCanApprove: true}})
	for _, key := range []string{"c", "c", "é", "backspace"} {
		m = m.Update(KeyMsg{Key: key})
	}
	if m.Comment != "c" {
		t.Fatalf("comment = %q, want %q", m.Comment, "c")
	}
}

func TestLongCommentKeepsItsEndInView(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, []model.PendingDeployment{{EnvironmentID: 1, EnvironmentName: "production", CurrentUserCanApprove: true}})
	for _, key := range append([]string{"c"}, strings.Split(strings.Repeat("x", 90)+"END", "")...) {
		m = m.Update(KeyMsg{Key: key})
	}
	if view := View(m, 70); !strings.Contains(view, "END") {
		t.Fatalf("typed end scrolled off:\n%s", view)
	}
}
