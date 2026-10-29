package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/screens/dispatch"
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

func waitingKeymapApp(t *testing.T, overrides map[string]string) App {
	t.Helper()
	km, err := keys.NewKeymap(overrides)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := waitingTestApp(t)
	app.keymap = km
	app = settleApp(t, pressKeys(app, "A"))
	if app.TopOverlay() != OverlayApprovals {
		t.Fatalf("overlay = %q, want approvals", app.TopOverlay())
	}
	return app
}

// Approvals is an overlay with keys of its own: moved help and quit act
// there too, and their old keys do not.
func TestApprovalsHonoursMovedQuitAndHelp(t *testing.T) {
	app := waitingKeymapApp(t, map[string]string{"help": "h", "quit": "Q"})
	if got := pressKeys(app, "q"); got.quit {
		t.Error("q quit from approvals after quit = \"Q\"")
	}
	if got := pressKeys(app, "?"); got.TopOverlay() == OverlayHelp {
		t.Error("? opened help from approvals after help = \"h\"")
	}
	if got := pressKeys(app, "h"); got.TopOverlay() != OverlayHelp {
		t.Error("h did not open help from approvals")
	}
	if got := pressKeys(app, "Q"); !got.quit {
		t.Error("Q did not quit from approvals")
	}
	// Swapped, the help key must never quit.
	if got := pressKeys(waitingKeymapApp(t, map[string]string{"help": "q", "quit": "?"}), "q"); got.quit || got.TopOverlay() != OverlayHelp {
		t.Errorf("help = \"q\" in approvals: quit=%v overlay=%q, want help", got.quit, got.TopOverlay())
	}
}

func TestApprovalsKeysCannotBeTaken(t *testing.T) {
	for _, key := range []string{"c", "y", "n", "j", "k"} {
		if _, err := keys.NewKeymap(map[string]string{"help": key}); err == nil || !strings.Contains(err.Error(), "approvals") {
			t.Errorf("help = %q: err = %v, want an approvals conflict", key, err)
		}
	}
}

// The detail gate pane names A, so no override may take it there.
func TestKeymapRejectsAKeyTheDetailGatePaneNames(t *testing.T) {
	if _, err := keys.NewKeymap(map[string]string{"rerun_job": "A"}); err == nil || !strings.Contains(err.Error(), "detail") {
		t.Fatalf("rerun_job = \"A\": err = %v, want a detail conflict", err)
	}
}

// The palette types what is pressed: once help moves, ? is just text.
func TestPaletteTypesTheOldHelpKeyAfterHelpMoves(t *testing.T) {
	app := pressKeys(keymapTestApp(t, map[string]string{"help": "h"}), ":", "?", "h")
	if app.TopOverlay() != OverlayPalette || app.palette.Query != "?h" {
		t.Fatalf("palette after : ? h: overlay %q, query %q; want the palette with ?h", app.TopOverlay(), app.palette.Query)
	}
	if got := pressKeys(keymapTestApp(t, nil), ":", "?"); got.TopOverlay() != OverlayHelp {
		t.Fatalf("default ? in the palette opened %q, want help", got.TopOverlay())
	}
}

// A fetch that finished but has not been applied when the next key
// arrives: the key belongs to the screen the fetch opens (here a typing
// form), so it must be read there, not translated for the runs list.
func TestKeyIsReadWhereADrainedFetchLandsIt(t *testing.T) {
	app := keymapTestApp(t, map[string]string{"help": "h"})
	app.dispatchWorkflowsResolver = func(context.Context) ([]dispatch.Workflow, error) {
		return []dispatch.Workflow{sampleDispatchModel().Workflow}, nil
	}
	app = pressKeys(app, "D")
	deadline := time.Now().Add(2 * time.Second)
	for app.load != nil {
		if done, _, _, _ := app.load.snapshot(); done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("workflows fetch did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	app = pressKeys(app, "h")
	if app.Route() != RouteDispatch || app.dispatch.Fields[0].Value != "h" {
		t.Fatalf("route %v, version field %q: want dispatch with h typed", app.Route(), app.dispatch.Fields[0].Value)
	}
}

// Help opened while a fetch is out stays on top when the fetch lands a
// typing screen under it: help's keys follow the keymap, not the input.
func TestOverlayKeysFollowTheKeymapOverATypingRoute(t *testing.T) {
	app := keymapTestApp(t, map[string]string{"help": "h", "quit": "Q"})
	release := make(chan struct{})
	app.dispatchWorkflowsResolver = func(context.Context) ([]dispatch.Workflow, error) {
		<-release
		return []dispatch.Workflow{sampleDispatchModel().Workflow}, nil
	}
	app = pressKeys(app, "D", "h")
	close(release)
	app = settleApp(t, app)
	if app.Route() != RouteDispatch || app.TopOverlay() != OverlayHelp {
		t.Fatalf("route %v overlay %q, want help over dispatch", app.Route(), app.TopOverlay())
	}
	if got := pressKeys(app, "q"); got.quit {
		t.Fatal("old q quit from help over the dispatch form")
	}
	if got := pressKeys(app, "Q"); !got.quit {
		t.Fatal("Q did not quit from help over the dispatch form")
	}
}
