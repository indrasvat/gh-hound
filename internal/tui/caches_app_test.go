package tui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/caches"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func cachesTestData() caches.Data {
	now := time.Now().UTC()
	return caches.Data{
		Usage: model.CacheUsage{ActiveSizeInBytes: 3 << 30, ActiveCount: 3},
		Caches: []model.Cache{
			{ID: 1, Key: "setup-go-Linux-x64", Ref: "refs/heads/main", SizeInBytes: 2 << 30, LastAccessedAt: now.Add(-2 * time.Hour)},
			{ID: 2, Key: "go-mod-Linux", Ref: "refs/heads/main", SizeInBytes: 512 << 20, LastAccessedAt: now.Add(-30 * time.Hour)},
			{ID: 3, Key: "go-mod-Linux", Ref: "refs/pull/7/merge", SizeInBytes: 512 << 20, LastAccessedAt: now.Add(-time.Minute)},
		},
	}
}

func openCachesViaPalette(t *testing.T, app App) App {
	t.Helper()
	app, _ = app.Update(KeyMsg{Key: ":"})
	for _, key := range []string{"c", "a", "c", "h", "e", "s"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.Update(KeyMsg{Key: "enter"})
	return app
}

func TestPaletteCachesEntryOpensScreenAsync(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	app := asyncTestApp()
	app.cachesResolver = func(context.Context) (caches.Data, error) {
		calls.Add(1)
		<-release
		return cachesTestData(), nil
	}

	started := time.Now()
	app = openCachesViaPalette(t, app)
	if elapsed := time.Since(started); elapsed > 80*time.Millisecond {
		t.Fatalf("palette caches keystroke blocked for %v", elapsed)
	}
	if app.Route() != RouteCaches {
		t.Fatalf("route = %s, want caches", app.Route())
	}
	if app.load == nil || app.load.kind != loadKindCaches {
		t.Fatal("caches open must go through the shared startLoad path")
	}
	close(release)
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("caches load did not settle")
	}
	if calls.Load() != 1 {
		t.Fatalf("resolver calls = %d, want 1", calls.Load())
	}
	view := app.ViewSize(100, 40)
	if !strings.Contains(view, "kennel: 3/10 GB") {
		t.Fatalf("caches screen missing usage gauge:\n%s", view)
	}
}

func TestCachesResolverNeverRunsOnDefaultPaths(t *testing.T) {
	var calls atomic.Int32
	app := asyncTestApp()
	app.cachesResolver = func(context.Context) (caches.Data, error) {
		calls.Add(1)
		return cachesTestData(), nil
	}
	for _, key := range []string{"j", "k", "enter", "esc", "?", "esc", "f", "esc"} {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	app, _ = app.SettleLoads(time.Second)
	app, _ = app.Refresh()
	if calls.Load() != 0 {
		t.Fatalf("default paths must make zero cache calls, got %d", calls.Load())
	}
}

func TestCachesResolverErrorSetsRouteError(t *testing.T) {
	app := asyncTestApp()
	app.cachesResolver = func(context.Context) (caches.Data, error) {
		return caches.Data{}, errors.New("kennel door stuck")
	}
	app = openCachesViaPalette(t, app)
	app, _ = app.SettleLoads(time.Second)
	view := app.ViewSize(100, 40)
	if !strings.Contains(view, "kennel unavailable: kennel door stuck") {
		t.Fatalf("resolver error must surface on the route:\n%s", view)
	}
}

func cachesLoadedApp(t *testing.T, deleter func(context.Context, CacheDeleteRequest) (int, error)) App {
	t.Helper()
	app := asyncTestApp()
	app.cachesResolver = func(context.Context) (caches.Data, error) {
		return cachesTestData(), nil
	}
	app.cacheDeleter = deleter
	app = openCachesViaPalette(t, app)
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("caches load did not settle")
	}
	return app
}

func TestCacheDeleteIsConfirmGatedAndToasts(t *testing.T) {
	var got CacheDeleteRequest
	app := cachesLoadedApp(t, func(_ context.Context, request CacheDeleteRequest) (int, error) {
		got = request
		return 1, nil
	})
	app, _ = app.Update(KeyMsg{Key: "d"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatal("d must confirm before digging")
	}
	view := app.ViewSize(100, 40)
	if !strings.Contains(view, "dig up 1 cache") {
		t.Fatalf("confirm must lead with the match count:\n%s", view)
	}

	// Default-deny: enter cancels.
	app, _ = app.Update(KeyMsg{Key: "enter"})
	if app.TopOverlay() == OverlayConfirm {
		t.Fatal("enter must cancel the confirm")
	}
	if got.ID != 0 {
		t.Fatal("cancelled confirm must not delete")
	}

	app, _ = app.Update(KeyMsg{Key: "d"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("delete did not settle")
	}
	if got.ID == 0 {
		t.Fatal("confirmed delete must reach the deleter with the cache ID")
	}
	view = app.ViewSize(100, 40)
	if !strings.Contains(view, "dug that one up.") {
		t.Fatalf("delete toast missing:\n%s", view)
	}
	if app.caches.Usage.ActiveCount != 2 {
		t.Fatalf("usage must fold the delete, got %#v", app.caches.Usage)
	}
}

func TestCacheDeleteKeyConfirmShowsMatchCount(t *testing.T) {
	var got CacheDeleteRequest
	app := cachesLoadedApp(t, func(_ context.Context, request CacheDeleteRequest) (int, error) {
		got = request
		return 2, nil
	})
	// Move to a go-mod-Linux row (size sort: setup-go first, then the
	// two go-mod halves).
	app, _ = app.Update(KeyMsg{Key: "j"})
	app, _ = app.Update(KeyMsg{Key: "D"})
	if app.TopOverlay() != OverlayConfirm {
		t.Fatal("D must confirm before digging")
	}
	view := app.ViewSize(120, 40)
	if !strings.Contains(view, "dig up 2 caches keyed") {
		t.Fatalf("key confirm must show the match count first:\n%s", view)
	}
	app, _ = app.Update(KeyMsg{Key: "y"})
	app, ok := app.SettleLoads(time.Second)
	if !ok {
		t.Fatal("delete did not settle")
	}
	if got.Key != "go-mod-Linux" {
		t.Fatalf("deleter request = %#v, want key go-mod-Linux", got)
	}
	view = app.ViewSize(120, 40)
	if !strings.Contains(view, "dug up 2 caches.") {
		t.Fatalf("multi-delete toast missing:\n%s", view)
	}
	if app.caches.Usage.ActiveCount != 1 {
		t.Fatalf("usage must fold the key delete, got %#v", app.caches.Usage)
	}
}

func TestCacheDeleteErrorKeepsKennelAndToasts(t *testing.T) {
	app := cachesLoadedApp(t, func(context.Context, CacheDeleteRequest) (int, error) {
		return 0, usecase.ActionError{Kind: usecase.ActionErrorPermission, Message: "permission denied"}
	})
	app, _ = app.Update(KeyMsg{Key: "d"})
	app, _ = app.Update(KeyMsg{Key: "y"})
	app, _ = app.SettleLoads(time.Second)
	if app.caches.Usage.ActiveCount != 3 {
		t.Fatalf("failed delete must not fold usage, got %#v", app.caches.Usage)
	}
	view := app.ViewSize(120, 40)
	if !strings.Contains(view, "permission denied") {
		t.Fatalf("delete failure toast missing:\n%s", view)
	}
}

func TestCachesEscNavigatesBack(t *testing.T) {
	app := cachesLoadedApp(t, nil)
	app, _ = app.Update(KeyMsg{Key: "esc"})
	if app.Route() == RouteCaches {
		t.Fatal("esc must leave the caches screen")
	}
}

func TestCachesFixturesRender(t *testing.T) {
	for fixture, needle := range map[string]string{
		"caches":          "kennel: 3.1/10 GB",
		"caches-pressure": "kennel's almost full — GitHub starts evicting at 10 GB.",
		"caches-empty":    "the kennel's empty — nothing cached on this repo.",
	} {
		view := RenderFixtureSize(fixture, 120, 40)
		if !strings.Contains(view, needle) {
			t.Fatalf("fixture %s missing %q:\n%s", fixture, needle, view)
		}
	}
}
