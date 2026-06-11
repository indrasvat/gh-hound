package confirm

import (
	"strings"
	"testing"
)

func TestConfirmDefaultsNoAndRequiresExplicitYes(t *testing.T) {
	m := New("force cancel run #571")
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentCancel {
		t.Fatalf("enter default intent = %#v", m.Intent)
	}
	m = New("force cancel run #571").Update(KeyMsg{Key: "y"})
	if m.Intent.Kind != IntentConfirm {
		t.Fatalf("y intent = %#v", m.Intent)
	}
}

func TestViewRendersMultiLineMessages(t *testing.T) {
	m := New("Download artifact \"coverage\" (1.2 MB)?\n→ /tmp/hound-dl/coverage/")
	view := View(m, 80)
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Fatalf("view lines = %d, want 5 (two message lines + blank + two footers)\n%s", len(lines), view)
	}
	if lines[0] != "Download artifact \"coverage\" (1.2 MB)?" || lines[1] != "→ /tmp/hound-dl/coverage/" {
		t.Fatalf("message lines mangled:\n%s", view)
	}
}
