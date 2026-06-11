package tui

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/detail"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func artifactsTestApp(t *testing.T, downloader func(model.Artifact, string, bool, func(usecase.DownloadProgress)) (usecase.DownloadResult, error)) App {
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
		DetailResolver: func(_ context.Context, run model.Run) (detail.Model, error) {
			return detail.NewModel(run, []model.Job{{ID: 7001, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}}).WithRepo("indrasvat/gh-hound"), nil
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
	// Jobs settle first: the artifacts block renders off the steps
	// pane, which shows its loading hint until jobs land.
	app = settleApp(t, app)
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
	app := artifactsTestApp(t, func(artifact model.Artifact, destDir string, force bool, _ func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
		calls.Add(1)
		if artifact.ID != 901 || destDir != "." {
			t.Errorf("downloader got artifact %d dir %q", artifact.ID, destDir)
		}
		if force {
			t.Error("first download attempt must not force")
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
	app := artifactsTestApp(t, func(model.Artifact, string, bool, func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
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

func TestSecondDownloadRefusedWhileOneIsActive(t *testing.T) {
	block := make(chan struct{})
	var calls atomic.Int32
	app := artifactsTestApp(t, func(model.Artifact, string, bool, func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
		calls.Add(1)
		<-block
		return usecase.DownloadResult{Path: "./coverage", FileCount: 1}, nil
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app, _ = app.Update(KeyMsg{Key: "y"})

	// Second download attempt while the first is still in flight.
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.TopOverlay() == OverlayConfirm {
		app, _ = app.Update(KeyMsg{Key: "y"})
	}
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "download in progress") {
		t.Fatalf("second download must be refused with a busy toast\n%s", view)
	}
	close(block)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var changed bool
		app, changed = app.Refresh()
		if changed && calls.Load() == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("downloader calls = %d, want exactly 1", calls.Load())
	}
}

func TestToastTTLActuallyExpires(t *testing.T) {
	app := artifactsTestApp(t, nil)
	app.pushToast("ttl-check", usecase.Resilience{Severity: usecase.SeverityInfo, Title: "ttl", Message: "check"})
	// Backdate the tick clock instead of sleeping 8s.
	app.lastToastTick = time.Now().Add(-10 * time.Second)
	app, changed := app.Refresh()
	if !changed || len(app.toasts.Toasts) != 0 {
		t.Fatalf("toast must expire after its TTL: changed=%v remaining=%d", changed, len(app.toasts.Toasts))
	}
}

// artifactsTestAppFull wires opener/copier doubles alongside the
// downloader so the post-download o/y actions are observable.
func artifactsTestAppFull(t *testing.T, downloader func(model.Artifact, string, bool, func(usecase.DownloadProgress)) (usecase.DownloadResult, error), opener, copier func(string) error) App {
	t.Helper()
	app := artifactsTestApp(t, downloader)
	app.openURL = opener
	app.copyText = copier
	return app
}

func refreshUntil(t *testing.T, app App, want func(App) bool) App {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app, _ = app.Refresh()
		if want(app) {
			return app
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition never reached\n%s", ansi.Strip(app.ViewSize(120, 40)))
	return app
}

func TestDownloadConfirmShowsAbsoluteDestination(t *testing.T) {
	app := artifactsTestApp(t, nil)
	app.artifactDir = "/tmp/hound-dl"
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "d"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatalf("d should open the download confirm: top=%s", app.TopOverlay())
	}
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "→ /tmp/hound-dl/coverage/") {
		t.Fatalf("confirm must show the absolute destination\n%s", view)
	}
}

func TestDownloadLifecycleProgressPathAndActions(t *testing.T) {
	transfer := make(chan struct{})
	finish := make(chan struct{})
	var opened, copied atomic.Value
	app := artifactsTestAppFull(t,
		func(artifact model.Artifact, destDir string, force bool, onProgress func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
			onProgress(usecase.DownloadProgress{Phase: usecase.DownloadPhaseTransfer, Bytes: 4096})
			close(transfer)
			<-finish
			return usecase.DownloadResult{Path: "/tmp/hound-dl/coverage", FileCount: 3}, nil
		},
		func(path string) error { opened.Store(path); return nil },
		func(text string) error { copied.Store(text); return nil },
	)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "d"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	<-transfer

	// The row must carry the live byte counter while transferring.
	app = refreshUntil(t, app, func(a App) bool {
		return strings.Contains(ansi.Strip(a.ViewSize(120, 40)), "↓ 4.0 KB")
	})
	if got := app.PollInterval(); got != loadFrameInterval {
		t.Fatalf("a live download must tick at frame rate, got %v", got)
	}
	close(finish)

	// Done: header chip, row verdict, selected-row path subline.
	app = refreshUntil(t, app, func(a App) bool {
		view := ansi.Strip(a.ViewSize(120, 40))
		return strings.Contains(view, "3 files") && strings.Contains(view, "↳ /tmp/hound-dl/coverage")
	})
	view := ansi.Strip(app.ViewSize(120, 40))
	for _, want := range []string{"Artifacts (2 · 1 ✔)", "o open", "y copy path", "o open folder", "d re-download"} {
		if !strings.Contains(view, want) {
			t.Fatalf("post-download view missing %q\n%s", want, view)
		}
	}

	// o opens the extracted folder, not the browser.
	app, _ = app.Update(KeyMsg{Key: "o"})
	if got, _ := opened.Load().(string); got != "/tmp/hound-dl/coverage" {
		t.Fatalf("o opened %q, want the extraction path", got)
	}
	if !strings.Contains(ansi.Strip(app.ViewSize(120, 40)), "Opened folder") {
		t.Fatal("opening the folder must toast 'Opened folder'")
	}

	// y copies the path, not the run URL.
	app, _ = app.Update(KeyMsg{Key: "y"})
	if !strings.Contains(ansi.Strip(app.ViewSize(120, 40)), "Copied") {
		t.Fatal("copying the path must toast 'Copied'")
	}
	if got, _ := copied.Load().(string); got != "/tmp/hound-dl/coverage" {
		t.Fatalf("y copied %q, want the extraction path", got)
	}
}

func TestOpenAndCopyKeepBrowserMeaningsWithoutDownload(t *testing.T) {
	var opened, copied atomic.Value
	app := artifactsTestAppFull(t, nil,
		func(path string) error { opened.Store(path); return nil },
		func(text string) error { copied.Store(text); return nil },
	)
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "o"})
	if got, _ := opened.Load().(string); !strings.HasPrefix(got, "https://") {
		t.Fatalf("o without a download must open the browser URL, got %q", got)
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	if !strings.Contains(ansi.Strip(app.ViewSize(120, 40)), "Copied") {
		t.Fatal("copy-URL must toast 'Copied'")
	}
	if got, _ := copied.Load().(string); !strings.HasPrefix(got, "https://") {
		t.Fatalf("y without a download must copy the run URL, got %q", got)
	}
}

func TestDestinationExistsOffersOverwriteAndRetriesWithForce(t *testing.T) {
	var mu sync.Mutex
	var forces []bool
	app := artifactsTestApp(t, func(artifact model.Artifact, destDir string, force bool, _ func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
		mu.Lock()
		forces = append(forces, force)
		mu.Unlock()
		if !force {
			return usecase.DownloadResult{}, usecase.DestinationExistsError{Path: "./coverage"}
		}
		return usecase.DownloadResult{Path: "/tmp/hound-dl/coverage", FileCount: 2}, nil
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "d"})
	app, _ = app.Update(KeyMsg{Key: "y"})

	// The exists outcome must surface as an overwrite question.
	app = refreshUntil(t, app, func(a App) bool { return a.TopOverlay() == OverlayConfirm })
	view := ansi.Strip(app.ViewSize(120, 40))
	if !strings.Contains(view, "Destination exists") || !strings.Contains(view, "Overwrite and re-download") {
		t.Fatalf("overwrite confirm wording missing\n%s", view)
	}
	// And the row must not be stranded mid-"downloading".
	if state := app.DetailModel().Download(901).State; state != detail.DownloadStateNone {
		t.Fatalf("row state after exists = %q, want pre-attempt state", state)
	}

	app, _ = app.Update(KeyMsg{Key: "y"})
	app = refreshUntil(t, app, func(a App) bool {
		return a.DetailModel().Download(901).State == detail.DownloadStateDone
	})
	if !strings.Contains(ansi.Strip(app.ViewSize(120, 40)), "Artifacts (2 · 1 ✔)") {
		t.Fatal("forced re-download must land the downloaded chip")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(forces) != 2 || forces[0] || !forces[1] {
		t.Fatalf("downloader force sequence = %v, want [false true]", forces)
	}
}

func TestOverwriteCancelLeavesRowUntouched(t *testing.T) {
	app := artifactsTestApp(t, func(model.Artifact, string, bool, func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
		return usecase.DownloadResult{}, usecase.DestinationExistsError{Path: "./coverage"}
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "d"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	app = refreshUntil(t, app, func(a App) bool { return a.TopOverlay() == OverlayConfirm })
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.TopOverlay() == OverlayConfirm {
		t.Fatal("esc must close the overwrite confirm")
	}
	if state := app.DetailModel().Download(901).State; state != detail.DownloadStateNone {
		t.Fatalf("cancelled overwrite stranded the row in %q", state)
	}
}

func TestDownloadVerdictSurvivesRunNavigation(t *testing.T) {
	app := artifactsTestApp(t, func(model.Artifact, string, bool, func(usecase.DownloadProgress)) (usecase.DownloadResult, error) {
		return usecase.DownloadResult{Path: "/tmp/hound-dl/coverage", FileCount: 1}, nil
	})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = waitForArtifacts(t, app)
	app, _ = app.Update(KeyMsg{Key: "a"})
	app, _ = app.Update(KeyMsg{Key: "d"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	app = refreshUntil(t, app, func(a App) bool {
		return a.DetailModel().Download(901).State == detail.DownloadStateDone
	})
	// Leave detail and come back: the verdict must survive the
	// skeleton + resolver round trip.
	app, _ = app.Update(KeyMsg{Key: "esc"})
	app, _ = app.Update(KeyMsg{Key: "enter"})
	app = refreshUntil(t, app, func(a App) bool {
		return len(a.DetailModel().Artifacts) > 0 && a.load == nil
	})
	if state := app.DetailModel().Download(901).State; state != detail.DownloadStateDone {
		t.Fatalf("download verdict lost across navigation: %q", state)
	}
}
