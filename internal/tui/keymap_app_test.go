package tui

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func keymapTestApp(t *testing.T, overrides map[string]string) App {
	t.Helper()
	km, err := keys.NewKeymap(overrides)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Welcome = false
	return NewApp(Options{Config: cfg, Keymap: km, Launch: usecase.LaunchContext{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		State:  usecase.LaunchStateRuns,
		Runs: []model.Run{{
			ID: 9001, Name: "CI", RunNumber: 77, HeadBranch: "main", HeadSHA: "deadbee",
			Status: model.StatusCompleted, Conclusion: model.ConclusionFailure,
		}},
	}})
}

func TestMovedKeysActAndTheirOldKeysDoNot(t *testing.T) {
	app := keymapTestApp(t, map[string]string{"help": "h", "theme": "V", "quit": "Q"})

	if got := pressKeys(app, "?"); got.TopOverlay() != OverlayNone {
		t.Fatalf("? still opens help after help = \"h\"")
	}
	help := pressKeys(app, "h")
	if help.TopOverlay() != OverlayHelp {
		t.Fatalf("h did not open help")
	}
	if view := help.ViewSize(120, 40); !strings.Contains(view, "h close") {
		t.Fatalf("help footer does not show h close:\n%s", view)
	}
	if closed := pressKeys(help, "h"); closed.TopOverlay() != OverlayNone {
		t.Fatalf("h in help did not close it")
	}

	if got := pressKeys(app, "T"); got.Theme().Mode != theme.ModeBramble {
		t.Fatalf("T still toggles the theme after theme = \"V\"")
	}
	if got := pressKeys(app, "V"); got.Theme().Mode != theme.ModeBone {
		t.Fatalf("V did not toggle the theme")
	}

	if got := pressKeys(app, "q"); got.quit {
		t.Fatalf("q still quits after quit = \"Q\"")
	}
	if got := pressKeys(app, "Q"); !got.quit {
		t.Fatalf("Q did not quit")
	}
}

// A filter types what is pressed: overrides never reach an input.
func TestMovedKeysAreTypedIntoInputs(t *testing.T) {
	app := keymapTestApp(t, map[string]string{"help": "h", "cancel": "c"})
	app = pressKeys(app, "/", "h", "c", "x", "?")
	if !app.runs.InputMode || app.runs.Draft != "hcx?" {
		t.Fatalf("filter draft = %q (input %v), want the keys as typed", app.runs.Draft, app.runs.InputMode)
	}
	if app.TopOverlay() != OverlayNone {
		t.Fatalf("a key typed into the filter opened %v", app.TopOverlay())
	}
}

func TestRunsFooterAndHelpShowMovedKeys(t *testing.T) {
	app := keymapTestApp(t, map[string]string{"cancel": "c", "status_cycle": "v"})
	if view := app.ViewSize(200, 40); !strings.Contains(view, "c cancel") || strings.Contains(view, "✗ cancel") {
		t.Fatalf("runs footer does not show c cancel:\n%s", view)
	}
	help := pressKeys(app, "?").ViewSize(120, 40)
	if !strings.Contains(help, "c cancel") || !strings.Contains(help, "v · s keep the rest") {
		t.Fatalf("help does not show the moved keys:\n%s", help)
	}
}
