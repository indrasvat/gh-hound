package keys

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Keymap applies config [keybindings] overrides (action → key). The zero
// value is the default keymap.
type Keymap struct {
	moved map[string]string
}

// remappable lists the actions an override may move, with their default
// key. Every other key is fixed: in-pane hints and notices name it, or it
// is not a single printable key.
var remappable = map[string]string{
	"help":         "?",
	"quit":         "q",
	"theme":        "T",
	"rerun":        "r",
	"rerun_job":    "r",
	"rerun_failed": "R",
	"cancel":       "x",
	"force_cancel": "X",
	"logs":         "l",
	"full_log":     "l",
	"watch":        "w",
	"status_cycle": "f",
	"follow":       "f",
	"follow_worst": "f",
	"debug":        "d",
	"browser":      "o",
	"copy_excerpt": "y",
	"compare":      "o",
	"time_jump":    "t",
	"search":       "/",
	"compact":      "c",
	"wrap":         "w",
	"dig_up":       "d",
	"dig_up_key":   "D",
	"copy_json":    "y",
}

// globalBindings act on every screen outside typing, and in help.
var globalBindings = []Binding{
	{Key: "?", Action: "help"},
	{Key: "q", Action: "quit"},
	{Key: "T", Action: "theme"},
}

// fixedKeys are the other printable keys each screen acts on, listed or
// not; an override cannot take them. fixedEverywhere is the palette's ":"
// and the toast layer's "g", which shadows the screen while a toast shows.
var fixedKeys = map[Screen]string{
	ScreenWelcome:    "",
	ScreenRunsList:   "jkgGs/DoyA",
	ScreenDetail:     "AadjknloyYJK", // A: the gate pane names it
	ScreenFailure:    "jk",
	ScreenWatch:      "",
	ScreenWatchBoard: "jk",
	ScreenLog:        "jkgGnNzZ][",
	ScreenDiff:       "jk",
	ScreenFlakes:     "jk",
	ScreenCaches:     "jkgGs/",
	ScreenQueue:      "jkgG",
	ScreenWorkflows:  "jkgGe",
	ScreenHelp:       "",
	ScreenApprovals:  "jkcyn",
}

const fixedEverywhere = ":g"

// handlerScreen is the screen whose handler acts on a footer screen's
// keys: all-green is the runs list, and every watch state is the watch.
func handlerScreen(screen Screen) Screen {
	switch screen {
	case ScreenAllGreen:
		return ScreenRunsList
	case ScreenWatchDone, ScreenWatchPending:
		return ScreenWatch
	}
	return screen
}

// NewKeymap validates overrides: each action must be remappable, each
// key one printable character, and no screen may end up with one key
// for two actions (fixed keys and the global ones included).
func NewKeymap(overrides map[string]string) (Keymap, error) {
	k := Keymap{moved: map[string]string{}}
	for _, action := range sortedKeys(overrides) {
		key := overrides[action]
		def, ok := remappable[action]
		if !ok {
			return Keymap{}, fmt.Errorf("keybinding %q cannot be remapped; remappable: %s", action, strings.Join(sortedKeys(remappable), ", "))
		}
		if r, _ := utf8.DecodeRuneInString(key); utf8.RuneCountInString(key) != 1 || !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return Keymap{}, fmt.Errorf("keybinding %s = %q: use one printable character", action, key)
		}
		if key != def {
			k.moved[action] = key
		}
	}
	for _, screen := range sortedKeys(fixedKeys) {
		owner := map[string]string{}
		claim := func(key, who string) error {
			if prev, taken := owner[key]; taken && prev != who {
				return fmt.Errorf("keybinding conflict on the %s screen: %q is both %s and %s", strings.ReplaceAll(string(screen), "_", " "), key, prev, who)
			}
			owner[key] = who
			return nil
		}
		for _, r := range fixedEverywhere + fixedKeys[screen] {
			if err := claim(string(r), "a fixed key"); err != nil {
				return Keymap{}, err
			}
		}
		for _, b := range k.bound(screen) {
			if err := claim(k.keyOf(b), b.Action); err != nil {
				return Keymap{}, err
			}
		}
	}
	return k, nil
}

// Key translates a key pressed on screen into the default key its
// handler compares. A moved action's default key no longer acts there
// ("" is unhandled everywhere). Callers skip it while an input types.
func (k Keymap) Key(screen Screen, pressed string) string {
	if len(k.moved) == 0 {
		return pressed
	}
	bound := k.bound(handlerScreen(screen))
	for _, b := range bound {
		if k.keyOf(b) == pressed {
			return b.Key
		}
	}
	for _, b := range bound {
		if b.Key == pressed {
			return ""
		}
	}
	return pressed
}

// KeyFor is the key that triggers action (its default when not moved).
func (k Keymap) KeyFor(action string) string {
	if key, ok := k.moved[action]; ok {
		return key
	}
	return remappable[action]
}

// bound is the remappable bindings a screen's handler acts on, then the
// global ones.
func (k Keymap) bound(screen Screen) []Binding {
	var out []Binding
	seen := map[string]bool{}
	for _, b := range footerByScreen[screen] {
		if def, ok := remappable[b.Action]; ok && b.Key == def && !seen[b.Action] {
			seen[b.Action] = true
			out = append(out, b)
		}
	}
	for _, b := range globalBindings {
		if !seen[b.Action] {
			out = append(out, b)
		}
	}
	return out
}

func (k Keymap) keyOf(b Binding) string {
	if key, ok := k.moved[b.Action]; ok && b.Key == remappable[b.Action] {
		return key
	}
	return b.Key
}

// apply shows moved keys in a screen's bindings: a moved key drops the
// default's glyph (↻ for r) for the character itself.
func (k Keymap) apply(bindings []Binding) []Binding {
	out := append([]Binding(nil), bindings...)
	for i, b := range out {
		if key := k.keyOf(b); key != b.Key {
			out[i].Key = key
			out[i].Display = ""
		}
	}
	return out
}

func sortedKeys[V any, K ~string](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
