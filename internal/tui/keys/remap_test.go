package keys

import (
	"strings"
	"testing"
)

func mustKeymap(t *testing.T, overrides map[string]string) Keymap {
	t.Helper()
	k, err := NewKeymap(overrides)
	if err != nil {
		t.Fatalf("NewKeymap(%v): %v", overrides, err)
	}
	return k
}

// The default tables must not already bind one key twice on a screen:
// a drifted table would make every override check meaningless.
func TestDefaultKeymapIsConflictFree(t *testing.T) {
	mustKeymap(t, nil)
	for action, def := range remappable {
		if (Keymap{}).KeyFor(action) != def {
			t.Fatalf("KeyFor(%s) = %q, want default %q", action, (Keymap{}).KeyFor(action), def)
		}
	}
}

func TestKeymapMovesAKeyOnEveryScreenThatActsOnIt(t *testing.T) {
	k := mustKeymap(t, map[string]string{"cancel": "c", "help": "h"})
	tests := []struct {
		screen  Screen
		pressed string
		want    string
	}{
		{ScreenRunsList, "c", "x"},
		{ScreenRunsList, "x", ""}, // the old key no longer cancels
		{ScreenAllGreen, "c", "x"},
		{ScreenDetail, "c", "x"},
		{ScreenWatch, "c", "x"},
		{ScreenWatchDone, "x", ""},
		{ScreenWatchBoard, "c", "x"},
		{ScreenLog, "c", "c"}, // compact: the log does not cancel
		{ScreenLog, "h", "?"},
		{ScreenLog, "?", ""},
		{ScreenHelp, "h", "?"},
		{ScreenRunsList, "j", "j"},
		{ScreenRunsList, "enter", "enter"},
	}
	for _, tt := range tests {
		if got := k.Key(tt.screen, tt.pressed); got != tt.want {
			t.Errorf("Key(%s, %q) = %q, want %q", tt.screen, tt.pressed, got, tt.want)
		}
	}
}

func TestKeymapShowsMovedKeysWhereverKeysAreListed(t *testing.T) {
	k := mustKeymap(t, map[string]string{"cancel": "c", "help": "h", "status_cycle": "v"})
	runs := k.FooterWidth(ScreenRunsList, 200)
	if !strings.Contains(runs, "c cancel") || strings.Contains(runs, "✗ cancel") || !strings.Contains(runs, "· h") {
		t.Fatalf("runs footer does not show the moved keys: %q", runs)
	}
	if help := k.Footer(ScreenHelp); !strings.Contains(help, "h close") {
		t.Fatalf("help footer = %q, want h close", help)
	}
	if toast := k.ToastFooter(false); !strings.Contains(toast, "h help") {
		t.Fatalf("toast footer = %q, want h help", toast)
	}
	if syntax := strings.Join(k.RunsFilterSyntax(), "\n"); !strings.Contains(syntax, "v · s keep the rest") {
		t.Fatalf("filter syntax still names the old status key:\n%s", syntax)
	}
	if def := FooterForScreen(ScreenRunsList); !strings.Contains(def, "✗ cancel") {
		t.Fatalf("default footer changed: %q", def)
	}
}

// One key may serve actions that never share a screen.
func TestKeymapAllowsOneKeyOnDifferentScreens(t *testing.T) {
	k := mustKeymap(t, map[string]string{"rerun": "e", "rerun_job": "e", "follow": "e"})
	if k.Key(ScreenRunsList, "e") != "r" || k.Key(ScreenDetail, "e") != "r" || k.Key(ScreenWatch, "e") != "f" {
		t.Fatal("e did not reach each screen's own action")
	}
}

func TestKeymapRejectsBadOverrides(t *testing.T) {
	tests := []struct {
		overrides map[string]string
		want      string
	}{
		{map[string]string{"cancel": "j"}, `conflict on the detail screen: "j" is both a fixed key and cancel`},
		{map[string]string{"cancel": "s"}, `conflict on the runs list screen: "s" is both a fixed key and cancel`},
		{map[string]string{"cancel": "r"}, `"r" is both rerun_job and cancel`},
		{map[string]string{"debug": "x"}, `conflict on the watch screen`},
		{map[string]string{"help": "g"}, `"g" is both a fixed key and help`},
		{map[string]string{"theme": ":"}, `a fixed key and theme`},
		{map[string]string{"quit": "Y"}, `conflict on the detail screen`},
		{map[string]string{"rerun": "e", "logs": "e"}, `"e" is both rerun and logs`},
		{map[string]string{"rerun_run": "e"}, `keybinding "rerun_run" cannot be remapped; remappable: browser,`},
		{map[string]string{"filter": "e"}, `"filter" cannot be remapped`},
		{map[string]string{"cancel": "ab"}, `cancel = "ab": use one printable character`},
		{map[string]string{"cancel": " "}, `use one printable character`},
		{map[string]string{"cancel": "\t"}, `use one printable character`},
		{map[string]string{"cancel": "enter"}, `use one printable character`},
		{map[string]string{"cancel": "\u200f"}, `use one printable character`},
	}
	for _, tt := range tests {
		_, err := NewKeymap(tt.overrides)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("NewKeymap(%v) = %v, want error containing %q", tt.overrides, err, tt.want)
		}
	}
}

// An override naming the default key is no override at all.
func TestKeymapIgnoresAnOverrideToTheDefault(t *testing.T) {
	k := mustKeymap(t, map[string]string{"cancel": "x"})
	if k.Key(ScreenRunsList, "x") != "x" {
		t.Fatal("cancel = \"x\" disabled x")
	}
}

// FuzzKeymap: for any accepted overrides, on every screen each action is
// reached by exactly its shown key, a moved default is dead unless it
// now belongs to another action, and untouched keys pass through.
func FuzzKeymap(f *testing.F) {
	actions := sortedKeys(remappable)
	f.Add(uint8(0), "c", uint8(1), "h", uint8(2), "v")
	f.Add(uint8(5), "e", uint8(6), "e", uint8(7), "x")
	f.Add(uint8(3), "r", uint8(4), "R", uint8(9), "?")
	f.Fuzz(func(t *testing.T, a1 uint8, k1 string, a2 uint8, k2 string, a3 uint8, k3 string) {
		overrides := map[string]string{
			actions[int(a1)%len(actions)]: k1,
			actions[int(a2)%len(actions)]: k2,
			actions[int(a3)%len(actions)]: k3,
		}
		k, err := NewKeymap(overrides)
		if err != nil {
			return
		}
		for screen := range fixedKeys {
			bound := k.bound(screen)
			shown := map[string]string{}
			for _, b := range bound {
				key := k.keyOf(b)
				if got := k.Key(screen, key); got != b.Key {
					t.Fatalf("%s: %s's key %q translates to %q, want %q", screen, b.Action, key, got, b.Key)
				}
				if prev, dup := shown[key]; dup {
					t.Fatalf("%s: %q shown for %s and %s", screen, key, prev, b.Action)
				}
				shown[key] = b.Action
			}
			for _, b := range bound {
				if key := k.keyOf(b); key != b.Key {
					if _, taken := shown[b.Key]; !taken && k.Key(screen, b.Key) != "" {
						t.Fatalf("%s: moved %s still acts on %q", screen, b.Action, b.Key)
					}
				}
			}
			for _, r := range fixedEverywhere + fixedKeys[screen] {
				if got := k.Key(screen, string(r)); got != string(r) {
					t.Fatalf("%s: fixed key %q translated to %q", screen, r, got)
				}
			}
		}
	})
}
