package dispatch

import (
	"strings"
	"testing"
)

func TestDispatchFormInputsValidationAndSubmit(t *testing.T) {
	m := NewModel(Workflow{
		Name: "Release",
		ID:   "release.yml",
		Ref:  "main",
		Inputs: []Input{
			{Name: "version", Required: true, Type: InputText},
			{Name: "prerelease", Type: InputBool, Options: []string{"false", "true"}},
			{Name: "channel", Type: InputSelect, Options: []string{"stable", "beta", "nightly"}},
		},
	})
	m = m.Update(KeyMsg{Key: "T"})
	if m.Intent.Kind != IntentNone || m.Fields[0].Value != "T" {
		t.Fatalf("input mode should capture printable T: %#v", m)
	}
	for _, key := range []string{"v", "0", ".", "1", "2", ".", "0"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m = m.Update(KeyMsg{Key: "tab"})
	m = m.Update(KeyMsg{Key: "right"})
	m = m.Update(KeyMsg{Key: "tab"})
	m = m.Update(KeyMsg{Key: "right"})
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentSubmit || m.Intent.Request.Inputs["version"] != "Tv0.12.0" || m.Intent.Request.Inputs["channel"] != "beta" {
		t.Fatalf("submit intent = %#v", m.Intent)
	}
	view := View(m, 80)
	for _, want := range []string{"dispatch · Release", "ref main ▾", "version", "POST …/workflows/release.yml/dispatches", "⏎ run · ⇥ next · ⎋ cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dispatch view missing %q\n%s", want, view)
		}
	}
}
