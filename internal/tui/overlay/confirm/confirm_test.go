package confirm

import "testing"

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
