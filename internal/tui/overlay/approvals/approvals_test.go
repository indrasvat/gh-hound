package approvals

import (
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
