package keys

import "testing"

func TestValidateLayerRejectsUnintendedCollisions(t *testing.T) {
	layer := Layer{
		Name: "runs",
		Bindings: []Binding{
			{Key: "r", Action: "rerun_failed", Help: "rerun failed"},
			{Key: "r", Action: "refresh", Help: "refresh"},
		},
	}

	if err := ValidateLayer(layer); err == nil {
		t.Fatal("ValidateLayer accepted duplicate key binding")
	}
}

func TestInputModeCapturesPrintableKeys(t *testing.T) {
	global := Layer{
		Name: "global",
		Bindings: []Binding{
			{Key: "q", Action: "quit", Help: "quit"},
			{Key: "?", Action: "help", Help: "help"},
		},
	}

	got := Resolve(ResolveInput{
		Key:       "q",
		InputMode: true,
		Global:    global,
	})
	if got.Action != ActionInsertText {
		t.Fatalf("input-mode q action = %q, want insert_text", got.Action)
	}

	got = Resolve(ResolveInput{Key: "q", Global: global})
	if got.Action != "quit" {
		t.Fatalf("normal q action = %q, want quit", got.Action)
	}
}

func TestShortHelpComesFromBindings(t *testing.T) {
	layer := Layer{
		Name: "runs",
		Bindings: []Binding{
			{Key: "⏎", Action: "open", Help: "open", ShowInFooter: true},
			{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
			{Key: "x", Action: "cancel", Help: "cancel"},
		},
	}

	got := ShortHelp(layer)
	want := []string{"⏎ open", "? help"}
	if len(got) != len(want) {
		t.Fatalf("ShortHelp length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ShortHelp[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
