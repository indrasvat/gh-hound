package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
)

func footerLine(app App, width, height int) string {
	lines := strings.Split(ansi.Strip(app.ViewSize(width, height)), "\n")
	return lines[len(lines)-2]
}

func TestLogSearchInputHasItsOwnFooter(t *testing.T) {
	app := typeKeys(t, openLog(t, logApp(t, config.Default(), noisyRaw())), strings.Fields("/ j k")...)
	footer := footerLine(app, 80, 24)
	if !strings.Contains(footer, "type to search · ⏎ find · ⎋ cancel") || strings.Contains(footer, "scroll") {
		t.Fatalf("typing footer: %q", footer)
	}
	app = typeKeys(t, app, strings.Fields("esc")...)
	if footer := footerLine(app, 80, 24); !strings.Contains(footer, "j/k scroll") {
		t.Fatalf("esc restores the log footer: %q", footer)
	}
}

func TestLogEscClearsTheSearchBeforeLeaving(t *testing.T) {
	app := openLog(t, logApp(t, config.Default(), noisyRaw()))
	from := app.routes[len(app.routes)-2]
	app = typeKeys(t, app, strings.Fields("/ 1 0 : 2 0 enter")...)
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
	if app = typeKeys(t, app, strings.Fields("esc")...); app.Route() != from {
		t.Fatalf("second esc goes back to %s, got %s", from, app.Route())
	}
}

func TestTimeJumpEchoClearsWhenTheViewMovesAway(t *testing.T) {
	app := typeKeys(t, openLog(t, logApp(t, config.Default(), noisyRaw())), strings.Fields("Z t 1 0 : 2 0 enter")...)
	if !strings.Contains(ansi.Strip(app.ViewSize(120, 30)), "log · t→10:20") {
		t.Fatalf("the jump echoes:\n%s", ansi.Strip(app.ViewSize(120, 30)))
	}
	if app = typeKeys(t, app, strings.Fields("k")...); !strings.Contains(ansi.Strip(app.ViewSize(120, 30)), "t→10:20") {
		t.Fatal("k keeps the jumped line on screen and the echo")
	}
	if app = typeKeys(t, app, strings.Fields("G")...); strings.Contains(ansi.Strip(app.ViewSize(120, 30)), "t→") {
		t.Fatalf("G leaves the jumped line, the echo must go:\n%s", ansi.Strip(app.ViewSize(120, 30)))
	}
}

func TestTimeJumpHintFitsAt80Columns(t *testing.T) {
	app := typeKeys(t, openLog(t, logApp(t, config.Default(), noisyRaw())), strings.Fields("t")...)
	view := ansi.Strip(app.ViewSize(80, 24))
	if !strings.Contains(view, "A-B range · ⏎ go") || !strings.Contains(view, "│⎋ cancel") || strings.Contains(view, "⏎ g…") {
		t.Fatalf("the hint must wrap inside the box at 80 columns:\n%s", view)
	}
}

func TestFailureHeaderCountsTheRowsOnScreen(t *testing.T) {
	app := typeKeys(t, logApp(t, config.Default(), noisyRaw()), strings.Fields("enter")...)
	app = settleApp(t, app)
	if app.Route() != RouteFailure {
		t.Fatalf("setup: route %s", app.Route())
	}
	view := ansi.Strip(app.ViewSize(80, 10))
	if !strings.Contains(view, "error window · 1 of 483 lines") || strings.Contains(view, "│…"+strings.Repeat(" ", 77)+"│") {
		t.Fatalf("a 4-row body draws 1 excerpt line and says so:\n%s", view)
	}
}

func TestTimeJumpModalFitsALongPickerAt80x24(t *testing.T) {
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "17:%02d:00.000Z ##[group]step %02d %s\n17:%02d:30.000Z work\n##[endgroup]\n", i, i, strings.Repeat("x", 80), i)
	}
	app := typeKeys(t, openLog(t, logApp(t, config.Default(), b.String())), "t")
	lines := strings.Split(ansi.Strip(app.ViewSize(80, 24)), "\n")
	body := strings.Join(lines[3:len(lines)-3], "\n")
	if !strings.Contains(body, "⎋ cancel") || !strings.Contains(body, "╰") || strings.Contains(body, "│…") {
		t.Fatalf("the modal keeps its hint and bottom border:\n%s", strings.Join(lines, "\n"))
	}
	for _, l := range lines {
		if parts := strings.Split(l, "│"); len(parts) == 5 && strings.Contains(parts[2], "step 00") && utf8.RuneCountInString(strings.TrimRight(parts[2], " ")) != 66 {
			t.Fatalf("a long entry fills the box's 66 columns: %q", parts[2])
		}
	}
}

func TestFailureFixturesCountTheRowsOnScreen(t *testing.T) {
	for h := 10; h <= 24; h++ {
		for _, view := range []string{RenderFixtureSize("failure", 80, h), RenderFixtureSize("failure-flaky", 80, h), RenderInteractionFixtureSize("failure-actions", 80, h)} {
			plain := ansi.Strip(view)
			drawn := 0
			for l := range strings.SplitSeq(plain, "\n") {
				if rest, ok := strings.CutPrefix(l, "│"); ok && len(rest) > 4 && rest[3] == ' ' && strings.Trim(rest[:3], "0123456789") == "" {
					drawn++
				}
			}
			if want := fmt.Sprintf("error window · %d of", drawn); strings.Contains(plain, "error window") && !strings.Contains(plain, want) {
				t.Fatalf("height %d: header must count the %d drawn rows:\n%s", h, drawn, plain)
			}
		}
	}
}
