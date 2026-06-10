package tui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func artifactsTestApp(t *testing.T, downloader func(model.Artifact, string) (usecase.DownloadResult, error)) App {
	t.Helper()
	cfg := config.Default()
	cfg.Welcome = false
	run := model.Run{ID: 9001, RunNumber: 44, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, HeadBranch: "main", Name: "CI"}
	return NewApp(Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo:  "indrasvat/gh-hound",
			State: usecase.LaunchStateRuns,
			Runs:  []model.Run{run},
		},
		DetailResolver: func(run model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{{ID: 7001, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}}), nil
		},
		ArtifactsResolver: func(run model.Run) ([]model.Artifact, error) {
			return []model.Artifact{
				{ID: 901, Name: "coverage", SizeInBytes: 1262848},
				{ID: 902, Name: "old-report", SizeInBytes: 52480, Expired: true},
			}, nil
		},
		ArtifactDownloader: downloader,
	})
}

func waitForArtifacts(t *testing.T, app App) App {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var changed bool
		app, changed = app.Refresh()
		if changed && len(app.DetailModel().Artifacts) > 0 {
			return app
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("artifacts never applied to detail model")
	return app
}

func TestDetailArtifactsLoadAsyncAndRender(t *testing.T) {
	app := artifactsTestApp(t, nil)
	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled || app.Route() != RouteDetail {
		t.Fatalf("enter should open detail: handled=%v route=%s", handled, app.Route())
	}
	// First paint must not block on artifacts.
	if len(app.DetailModel().Artifacts) != 0 {
		t.Fatal("artifacts should load asynchronously, not at detail-open time")
	}
	app = waitForArtifacts(t, app)
	view := ansi.Strip(app.ViewSize(120, 40))
	for _, want := range []string{"Artifacts (2)", "coverage", "1.2 MB", "old-report", "[expired]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail view missing %q\n%s", want, view)
		}
	}
}

func TestArtifactDownloadConfirmsThenToasts(t *testing.T) {
	var calls atomic.Int32
	app := artifactsTestApp(t, func(artifact model.Artifact, destDir string) (usecase.DownloadResult, error) {
		calls.Add(1)
		if artifact.ID != 901 || destDir != "." {
			t.Errorf("downloader got artifact %d dir %q", artifact.ID, destDir)
		}
		return usecase.DownloadResult{Path: "./coverage", FileCount: 3}, nil
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)

	app, handled := app.Update(KeyMsg{Key: "a"})
	if !handled || app.DetailModel().Focus != detail.FocusArtifacts {
		t.Fatalf("a should focus artifacts: %s", app.DetailModel().Focus)
	}
	app, handled = app.Update(KeyMsg{Key: "enter"})
	if !handled || app.TopOverlay() != OverlayConfirm {
		t.Fatalf("enter on artifact should open confirm: top=%s", app.TopOverlay())
	}
	if calls.Load() != 0 {
		t.Fatal("download must not start before confirmation")
	}
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "coverage") {
		t.Fatalf("confirm should name the artifact\n%s", view)
	}

	app, _ = app.Update(KeyMsg{Key: "y"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var changed bool
		app, changed = app.Refresh()
		if changed {
			view := ansi.Strip(app.ViewSize(120, 40))
			if strings.Contains(view, "coverage") && strings.Contains(view, "3 files") {
				if calls.Load() != 1 {
					t.Fatalf("downloader calls = %d, want 1", calls.Load())
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("download success toast never appeared\n%s", ansi.Strip(app.ViewSize(120, 40)))
}

func TestExpiredArtifactDownloadRefusedUpFront(t *testing.T) {
	app := artifactsTestApp(t, func(model.Artifact, string) (usecase.DownloadResult, error) {
		t.Error("downloader must not be called for an expired artifact")
		return usecase.DownloadResult{}, nil
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, handled := app.Update(KeyMsg{Key: "enter"})
	if !handled {
		t.Fatal("enter on expired artifact should be handled")
	}
	if app.TopOverlay() == OverlayConfirm {
		t.Fatal("expired artifact must not open a download confirm")
	}
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(strings.ToLower(view), "expired") {
		t.Fatalf("expired refusal should surface a toast\n%s", view)
	}
}
