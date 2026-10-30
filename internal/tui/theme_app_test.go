package tui

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/theme"
	diffscreen "github.com/indrasvat/gh-hound/internal/tui/screens/diff"
	flakesscreen "github.com/indrasvat/gh-hound/internal/tui/screens/flakes"
	queuescreen "github.com/indrasvat/gh-hound/internal/tui/screens/queue"
	workflowsscreen "github.com/indrasvat/gh-hound/internal/tui/screens/workflows"
)

// brambleOnly is every Bramble token that Bone draws differently, as the
// SGR a screen would write for it.
func brambleOnly() []string {
	bramble, bone := theme.ForMode(theme.ModeBramble), theme.ForMode(theme.ModeBone)
	pairs := [][2]string{
		{bramble.OK, bone.OK}, {bramble.Fail, bone.Fail}, {bramble.Run, bone.Run}, {bramble.Info, bone.Info},
		{bramble.Warn, bone.Warn}, {bramble.FG, bone.FG}, {bramble.FGSoft, bone.FGSoft}, {bramble.Dim, bone.Dim},
		{bramble.Subtle, bone.Subtle}, {bramble.Muted, bone.Muted}, {bramble.Line, bone.Line}, {bramble.Line2, bone.Line2},
		{bramble.Surface, bone.Surface}, {bramble.Surface2, bone.Surface2}, {bramble.OKTint, bone.OKTint},
		{bramble.RunTint, bone.RunTint}, {bramble.RunTintStrong, bone.RunTintStrong},
		{bramble.FailTint, bone.FailTint}, {bramble.InfoTint, bone.InfoTint},
	}
	var out []string
	for _, p := range pairs {
		if p[0] != p[1] {
			out = append(out, theme.Fg(p[0]), theme.Bg(p[0]))
		}
	}
	return out
}

func themedRouteApp(route Route) App {
	cfg := config.Default()
	cfg.Welcome = route == RouteWelcome
	app := NewApp(Options{Config: cfg, Build: BuildInfo{Version: "test"}})
	app.runs = sampleRunsModel()
	app.detail = sampleDetailModel()
	app.failure = sampleFailureModel()
	app.log = sampleLogModel()
	app.watch = sampleWatchModel()
	app.board = sampleBoardModel()
	app.diff = diffscreen.NewModel(sampleDiffVerdict())
	app.flakes = flakesscreen.NewModel(sampleFlakeReport())
	app.queue = queuescreen.NewModel(sampleQueueReport())
	app.workflows = workflowsscreen.NewModel("indrasvat/gh-hound", sampleWorkflows())
	app.caches = sampleCachesPressureModel()
	if route != RouteRuns && route != RouteWelcome {
		app.PushRoute(route)
	}
	return app
}

// frameBody drops the frame's top three and bottom three rows, so only
// the screen's own drawing is judged, never the chrome around it.
func frameBody(view string) string {
	lines := strings.Split(view, "\n")
	if len(lines) <= 6 {
		return ""
	}
	return strings.Join(lines[3:len(lines)-3], "\n")
}

// T recolours every screen: after it, no screen keeps a Bramble colour.
func TestThemeToggleRecoloursEveryScreen(t *testing.T) {
	routes := []Route{RouteWelcome, RouteRuns, RouteDetail, RouteFailure, RouteLog, RouteWatch, RouteWatchBoard, RouteDiff, RouteFlakes, RouteCaches, RouteQueue, RouteWorkflows}
	bramble := brambleOnly()
	for _, route := range routes {
		t.Run(string(route), func(t *testing.T) {
			app := themedRouteApp(route)
			before := frameBody(app.ViewSize(120, 40))
			drawn := 0
			for _, sgr := range bramble {
				if strings.Contains(before, sgr) {
					drawn++
				}
			}
			if drawn < 3 {
				t.Fatalf("%s drew %d Bramble colours; the fixture did not reach the screen:\n%s", route, drawn, before)
			}
			app, _ = app.Update(KeyMsg{Key: "T"})
			after := frameBody(app.ViewSize(120, 40))
			for _, sgr := range bramble {
				if strings.Contains(after, sgr) {
					t.Errorf("%s keeps Bramble %q after T", route, sgr)
				}
			}
			bone := theme.ForMode(theme.ModeBone)
			drawnBone := 0
			for _, c := range []string{bone.OK, bone.Fail, bone.Run, bone.Info, bone.FG, bone.FGSoft, bone.Dim, bone.Subtle, bone.Muted} {
				if strings.Contains(after, theme.Fg(c)) {
					drawnBone++
				}
			}
			if drawnBone < 2 {
				t.Errorf("%s body shows %d Bone colours after T", route, drawnBone)
			}
		})
	}
}

// Without truecolor the fills come from the 256 palette, and T keeps them.
func TestLimitedColorKeepsPaletteFillsAcrossT(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	app := NewApp(Options{Config: cfg, LimitedColor: true})
	if app.Theme().Surface2 != theme.ForMode(theme.ModeBramble).Limited().Surface2 {
		t.Fatalf("limited app theme = %+v", app.Theme())
	}
	app, _ = app.Update(KeyMsg{Key: "T"})
	if app.Theme().Mode != theme.ModeBone || app.Theme().Surface2 != theme.ForMode(theme.ModeBone).Limited().Surface2 {
		t.Fatalf("T lost the limited fills: %+v", app.Theme())
	}
}

// The palette's selected row takes the surface-2 fill behind a green bar.
func TestPaletteSelectionHasTheSurfaceFill(t *testing.T) {
	app := pressKeys(themedRouteApp(RouteRuns), ":")
	th := app.Theme()
	view := app.ViewSize(120, 40)
	if !strings.Contains(view, theme.Bg(th.Surface2)+theme.Fg(th.OK)+"▌") {
		t.Fatalf("palette selection has no surface-2 fill and green bar:\n%q", view)
	}
}
