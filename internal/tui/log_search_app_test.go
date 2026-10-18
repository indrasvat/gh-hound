package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
)

func press(app App, keys string) App {
	for k := range strings.FieldsSeq(keys) {
		app, _ = app.Update(KeyMsg{Key: k})
	}
	return app
}

func footerLine(app App, width, height int) string {
	lines := strings.Split(ansi.Strip(app.ViewSize(width, height)), "\n")
	return lines[len(lines)-2]
}

func TestLogSearchInputHasItsOwnFooter(t *testing.T) {
	app := press(openLog(t, logApp(t, config.Default(), noisyRaw())), "/ j k")
	footer := footerLine(app, 80, 24)
	if !strings.Contains(footer, "type to search · ⏎ find · ⎋ cancel") || strings.Contains(footer, "scroll") {
		t.Fatalf("typing footer: %q", footer)
	}
	app = press(app, "esc")
	if footer := footerLine(app, 80, 24); !strings.Contains(footer, "j/k scroll") {
		t.Fatalf("esc restores the log footer: %q", footer)
	}
}

func TestLogEscClearsTheSearchBeforeLeaving(t *testing.T) {
	app := openLog(t, logApp(t, config.Default(), noisyRaw()))
	from := app.routes[len(app.routes)-2]
	app = press(app, "/ 1 0 : 2 0 enter")
	if app.log.Search.Total == 0 {
		t.Fatal("setup: no matches")
	}
	app, handled := app.Update(KeyMsg{Key: "esc"})
	if !handled || app.Route() != RouteLog || app.log.Search.Query != "" {
		t.Fatalf("first esc clears the search: route=%s search=%q", app.Route(), app.log.Search.Query)
	}
	if head := firstLine(ansi.Strip(app.ViewSize(120, 30))); strings.Contains(head, "match") {
		t.Fatalf("the title still counts matches: %q", head)
	}
	if app = press(app, "esc"); app.Route() != from {
		t.Fatalf("second esc goes back to %s, got %s", from, app.Route())
	}
}

func TestTimeJumpEchoClearsWhenTheViewMovesAway(t *testing.T) {
	app := press(openLog(t, logApp(t, config.Default(), noisyRaw())), "Z t 1 0 : 2 0 enter")
	if !strings.Contains(ansi.Strip(app.ViewSize(120, 30)), "log · t→10:20") {
		t.Fatalf("the jump echoes:\n%s", ansi.Strip(app.ViewSize(120, 30)))
	}
	if app = press(app, "k"); !strings.Contains(ansi.Strip(app.ViewSize(120, 30)), "t→10:20") {
		t.Fatal("k keeps the jumped line on screen and the echo")
	}
	if app = press(app, "G"); strings.Contains(ansi.Strip(app.ViewSize(120, 30)), "t→") {
		t.Fatalf("G leaves the jumped line, the echo must go:\n%s", ansi.Strip(app.ViewSize(120, 30)))
	}
}

func TestTimeJumpHintFitsAt80Columns(t *testing.T) {
	app := press(openLog(t, logApp(t, config.Default(), noisyRaw())), "t")
	view := ansi.Strip(app.ViewSize(80, 24))
	if !strings.Contains(view, "A-B range · ⏎ go") || !strings.Contains(view, "│⎋ cancel") || strings.Contains(view, "⏎ g…") {
		t.Fatalf("the hint must wrap inside the box at 80 columns:\n%s", view)
	}
}
