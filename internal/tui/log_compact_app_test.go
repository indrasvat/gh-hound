package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	failurescreen "github.com/indrasvat/gh-hound/internal/tui/screens/failure"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func noisyRaw() string {
	var b strings.Builder
	b.WriteString("##[group]Run e2e\nstarting cluster\n##[endgroup]\n")
	for i := range 240 {
		fmt.Fprintf(&b, "%02d:%02d:%02dZ checking if cluster is running\n", 10, i/6, i%6*10)
		fmt.Fprintf(&b, "%02d:%02d:%02dZ state :: &{ 0x%012x }\n", 10, i/6, i%6*10+1, i*7919)
	}
	b.WriteString("##[error]timed out waiting for cluster\n")
	return b.String()
}

func logApp(t *testing.T, cfg config.Config, raw string) App {
	t.Helper()
	cfg.Welcome = false
	run := model.Run{ID: 9300, Name: "e2e", RunNumber: 93}
	job := model.Job{ID: 9301, RunID: 9300, Name: "lifecycle", Status: model.StatusCompleted, Conclusion: model.ConclusionFailure}
	report := usecase.FailureReport{Job: job, Log: logs.Parse(raw)}
	app := NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{Repo: "acme/svc", Branch: "main", State: usecase.LaunchStateRuns, Runs: []model.Run{run}, BranchRuns: []model.Run{run}},
		DetailResolver: func(context.Context, model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{job}).WithRepo("acme/svc"), nil
		},
		LogResolver: func(ctx context.Context, _ model.Run, _ model.Job, _ func(read, total int64)) (logscreen.Model, error) {
			return logscreen.Fold(ctx, logscreen.NewModel(logs.Parse(raw), 1, 6)), nil
		},
		FailureResolver: func(ctx context.Context, _ model.Run, _ model.Job) (failurescreen.Model, logscreen.Model, error) {
			full := logscreen.Fold(ctx, logscreen.NewModel(report.Log, 1, 6))
			return failurescreen.NewModel("acme/svc", run.ID, report), full, nil
		},
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	return settleApp(t, app)
}

func openLog(t *testing.T, app App) App {
	t.Helper()
	app, handled := app.Update(KeyMsg{Key: "l"})
	if !handled || app.Route() != RouteLog {
		t.Fatalf("log open handled=%v route=%s", handled, app.Route())
	}
	return settleApp(t, app)
}

func TestLogOpensCompactByDefault(t *testing.T) {
	app := openLog(t, logApp(t, config.Default(), noisyRaw()))
	view := ansi.Strip(app.ViewSize(120, 30))
	for _, want := range []string{"log · compact", "lines → ", "⋯ ×", "timed out waiting for cluster", "c compact"} {
		if !strings.Contains(view, want) {
			t.Fatalf("compact view missing %q:\n%s", want, view)
		}
	}
	app, _ = app.Update(KeyMsg{Key: "c"})
	view = ansi.Strip(app.ViewSize(120, 30))
	if strings.Contains(view, "log · compact") || strings.Contains(view, "⋯ ×") || !strings.Contains(view, "483 lines") {
		t.Fatalf("c should show the full log:\n%s", view)
	}
}

func TestLogCompactConfigOffOpensFullLog(t *testing.T) {
	cfg := config.Default()
	cfg.LogCompact = false
	app := openLog(t, logApp(t, cfg, noisyRaw()))
	if view := ansi.Strip(app.ViewSize(120, 30)); strings.Contains(view, "log · compact") {
		t.Fatalf("log_compact=false must open the full log:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "c"})
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "log · compact") {
		t.Fatalf("c must still fold:\n%s", view)
	}
}

func TestCompactWithNothingToFoldShowsAToast(t *testing.T) {
	app := openLog(t, logApp(t, config.Default(), "one\ntwo\nthree\n"))
	app, handled := app.Update(KeyMsg{Key: "c"})
	if !handled {
		t.Fatal("c not handled")
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "nothing to fold") {
		t.Fatalf("missing toast:\n%s", view)
	}
}

func TestFailureFullLogReusesThePlan(t *testing.T) {
	app := logApp(t, config.Default(), noisyRaw())
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = settleApp(t, app)
	if app.Route() != RouteFailure {
		t.Fatalf("enter on detail: route=%s, want failure", app.Route())
	}
	app, _ = app.Update(KeyMsg{Key: "l"})
	if app.Route() != RouteLog || !app.log.HasPlan() || !app.log.Folded() {
		t.Fatalf("full log from failure: route=%s plan=%v folded=%v", app.Route(), app.log.HasPlan(), app.log.Folded())
	}
	if view := ansi.Strip(app.ViewSize(120, 30)); !strings.Contains(view, "timed out waiting for cluster") {
		t.Fatalf("full log should land on the failure:\n%s", view)
	}
}

func TestLogFooterFitsAt80Columns(t *testing.T) {
	app := openLog(t, logApp(t, config.Default(), noisyRaw()))
	lines := strings.Split(ansi.Strip(app.ViewSize(80, 24)), "\n")
	footer := lines[len(lines)-2]
	if !strings.Contains(footer, "⎋ back") || !strings.Contains(footer, "c compact") || strings.Contains(footer, "…") {
		t.Fatalf("80-col footer: %q", footer)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 80 {
			t.Fatalf("line width %d != 80: %q", w, l)
		}
	}
}
