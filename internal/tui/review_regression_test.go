package tui

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/overlay/palette"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// Review finding: launch --all --actor alice (repo scope, BranchRuns never fetched),
// then s: branch scope must load, not claim "current" and show nothing.
func TestScopeNeverFetchedUnderLaunchFilterLoadsOnToggle(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	cfg := config.Default()
	cfg.Welcome = false
	alice := []model.Run{actorRun(302, "alice", "trunk", model.ConclusionSuccess)}
	app := NewApp(Options{Config: cfg, RunsResolver: server.list, Launch: usecase.LaunchContext{
		Repo: "cli/cli", Branch: "trunk", Scope: usecase.LaunchScopeRepo, State: usecase.LaunchStateRuns,
		PerPage: 30, Page: 1, Runs: alice, RepoRuns: alice, Query: "actor:alice",
	}})
	before := server.count()
	app = typeKeys(t, app, "s")
	t.Logf("scope=%s load=%v calls=%d->%d rows=%d", app.runs.Context.Scope, app.load != nil, before, server.count(), len(app.runs.FilteredRuns()))
	app = settleApp(t, app)
	t.Logf("after settle calls=%d rows=%d\n%s", server.count(), len(app.runs.FilteredRuns()), view(app))
	if app.runs.Context.Scope == usecase.LaunchScopeBranch && server.count() == before {
		t.Errorf("switched to never-fetched branch scope without loading")
	}
}

// Review finding: palette "runs --all" from branch scope with no repo cache, unfiltered.
func TestPaletteAllBranchesLoadsMissingRepoCache(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := newActorApp(server, nil)
	// sniffRepoRuns failed at launch: no repo rows and no repo fetch.
	app.runs.Context.RepoRuns = nil
	keys := maps.Clone(app.runs.DataKeys)
	delete(keys, usecase.LaunchScopeRepo)
	app.runs.DataKeys = keys
	before := server.count()
	app = app.openPalette()
	app = app.handlePaletteIntent(palette.Intent{Route: "runs --all"})
	app = settleApp(t, app)
	t.Logf("scope=%s calls=%d->%d rows=%d", app.runs.Context.Scope, before, server.count(), len(app.runs.FilteredRuns()))
	if server.count() == before {
		t.Errorf("runs --all with empty repo cache started no fetch; list shows %d rows", len(app.runs.FilteredRuns()))
	}
}

// Review finding: poll re-anchors the cursor with an index into the unfiltered rows.
func TestPollKeepsCursorOnSameRunUnderTextFilter(t *testing.T) {
	rows := []model.Run{
		{ID: 1, RunNumber: 1, Name: "CI", DisplayTitle: "beta", HeadBranch: "trunk", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Actor: "x"},
		{ID: 2, RunNumber: 2, Name: "CI", DisplayTitle: "alpha one", HeadBranch: "trunk", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Actor: "x"},
		{ID: 3, RunNumber: 3, Name: "CI", DisplayTitle: "alpha two", HeadBranch: "trunk", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Actor: "x"},
		{ID: 4, RunNumber: 4, Name: "CI", DisplayTitle: "alpha three", HeadBranch: "trunk", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Actor: "x"},
	}
	server := &actorRuns{rows: rows}
	app := newActorApp(server, nil)
	app = settleApp(t, typeQuery(t, app, "alpha"))
	app = typeKeys(t, app, "j")
	sel, _ := app.runs.SelectedRun()
	app, _ = pollCycle(t, app)
	after, _ := app.runs.SelectedRun()
	t.Logf("selected before poll=%d after poll=%d", sel.ID, after.ID)
	if sel.ID != after.ID {
		t.Errorf("poll moved the cursor from #%d to #%d", sel.ID, after.ID)
	}
}

// Review finding: palette My runs while a log load is in flight.
func TestPaletteJumpCancelsOtherLoadSoRunsReloadStarts(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := newActorApp(server, &viewerStub{login: "indrasvat"})
	release := make(chan struct{})
	app.logResolver = func(ctx context.Context, run model.Run, job model.Job, p func(int64, int64)) (logscreen.Model, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return logscreen.Model{}, nil
	}
	app = typeKeys(t, app, "l")
	if app.load == nil {
		t.Fatal("no log load")
	}
	before := server.count()
	app = typeKeys(t, app, ":")
	app = app.handlePaletteIntent(palette.Intent{Route: "runs:mine"})
	t.Logf("route=%s filter=%q loadKind=%v calls=%d->%d rows=%d", app.Route(), app.runs.Filter, app.load.kind, before, server.count(), len(app.runs.FilteredRuns()))
	if app.load.kind != loadKindRuns {
		t.Errorf("runs:mine applied but its fetch was refused (log load still owns the slot)")
	}
	close(release)
}

// Review finding: esc with a toast visible cancels the reload but leaves the new filter applied with no fetch.
func TestEscCancellingFilterReloadRestoresLoadedQuery(t *testing.T) {
	gate := make(chan struct{})
	server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"bob": gate}}
	app := newActorApp(server, nil)
	app.pushToast("x", usecase.Resilience{Severity: usecase.SeverityWarn, Title: "t", Message: "m"})
	app = typeQuery(t, app, "actor:bob")
	if app.load == nil {
		t.Fatal("no reload")
	}
	app = typeKeys(t, app, "esc")
	t.Logf("filter=%q load=%v current=%v rows=%d", app.runs.Filter, app.load != nil, app.runs.DataCurrent(), len(app.runs.FilteredRuns()))
	if app.runs.Filter != "" && app.load == nil && !app.runs.DataCurrent() {
		t.Errorf("esc ate by toast: filter %q stays applied over stale rows with no fetch:\n%s", app.runs.Filter, view(app))
	}
	close(gate)
}

// Review finding: a branch-scope reload that fails after the user toggled to a
// current repo cache blanks the repo list with the branch's error.
func TestLeftScopeErrorNeverBlanksCurrentScope(t *testing.T) {
	gate := make(chan struct{})
	server := &actorRuns{rows: actorRows()}
	app := newActorApp(server, nil)
	// Repo scope: load actor:indrasvat so repo's cache is current for it.
	app = settleApp(t, typeKeys(t, app, "s"))
	app = settleApp(t, typeQuery(t, app, "actor:indrasvat"))
	// Back to branch: stale -> reload, gated and set to fail.
	server.mu.Lock()
	server.gates = map[string]chan struct{}{"indrasvat": gate}
	server.err = errors.New("boom")
	server.mu.Unlock()
	app = typeKeys(t, app, "s")
	if app.load == nil || app.runs.Context.Scope != usecase.LaunchScopeBranch {
		t.Fatalf("expected branch reload, scope=%s load=%v", app.runs.Context.Scope, app.load != nil)
	}
	app = typeKeys(t, app, "s") // back to repo: current cache, no new load
	t.Logf("scope=%s current=%v rows(before land)=%d", app.runs.Context.Scope, app.runs.DataCurrent(), len(app.runs.FilteredRuns()))
	close(gate)
	app = settleApp(t, app)
	t.Logf("after land: scope=%s queryErr=%q rows=%d", app.runs.Context.Scope, app.runs.QueryError, len(app.runs.FilteredRuns()))
	if app.runs.QueryError != "" {
		t.Errorf("branch reload error blanked the current repo list:\n%s", view(app))
	}
}

// Review finding: a poll that heals a failed reload replaces rows but keeps the old query's page cursor.
func TestHealingPollRestartsPaging(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := newActorApp(server, nil)
	app.runs.Context.Page = 3 // as if G had been pressed twice under the old query
	server.mu.Lock()
	server.err = errors.New("blip")
	server.mu.Unlock()
	app = settleApp(t, typeQuery(t, app, "actor:bob"))
	t.Logf("after failed reload: queryErr=%q page=%d", app.runs.QueryError, app.runs.Context.Page)
	server.mu.Lock()
	server.err = nil
	server.mu.Unlock()
	app, _ = pollCycle(t, app)
	t.Logf("after healing poll: queryErr=%q current=%v page=%d rows=%d", app.runs.QueryError, app.runs.DataCurrent(), app.runs.Context.Page, len(app.runs.FilteredRuns()))
	if app.runs.Context.Page != 1 {
		t.Errorf("healed rows are page 1 of actor:bob but Page=%d; next G fetches page %d", app.runs.Context.Page, app.runs.Context.Page+1)
	}
}

// Live QA finding: paging was one value shared by both scopes, so a
// trip to a short branch scope dropped the repo's "+" and dead-ended G.
func TestEachScopeKeepsItsOwnPaging(t *testing.T) {
	repoPage := []model.Run{}
	for i := range 30 {
		repoPage = append(repoPage, actorRun(int64(800-i), "alice", "feat/x", model.ConclusionSuccess))
	}
	var requests []usecase.RunFilter
	app := newActorApp(&actorRuns{rows: actorRows()}, nil)
	app.runsResolver = func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
		requests = append(requests, filter)
		if filter.Branch == "" {
			return repoPage, nil // repo scope: a full page, more to come
		}
		return []model.Run{actorRun(1, "alice", "trunk", model.ConclusionSuccess)}, nil
	}
	app = settleApp(t, typeKeys(t, app, "s"))
	app = settleApp(t, typeQuery(t, app, "actor:alice"))
	if !app.runs.Context.HasMore || !strings.Contains(view(app), "30+ runs") {
		t.Fatalf("repo scope should have more pages:\n%s", view(app))
	}
	app = settleApp(t, typeKeys(t, app, "s"))
	if app.runs.Context.HasMore || strings.Contains(view(app), "1+ run") {
		t.Fatalf("a short branch page has no more:\n%s", view(app))
	}
	app = typeKeys(t, app, "s")
	if !app.runs.Context.HasMore || !strings.Contains(view(app), "30+ runs") {
		t.Fatalf("back in repo scope the paging must be the repo's:\n%s", view(app))
	}
	app = settleApp(t, typeKeys(t, app, "G"))
	if last := requests[len(requests)-1]; last.Page != 2 || last.Actor != "alice" || last.Branch != "" {
		t.Fatalf("G after the round trip must fetch the repo's page 2: %+v", last)
	}
}

// Esc while a filter reload is pending cancels it back to the loaded
// query — and the same Esc still closes whatever sits on top.
func TestEscDuringReloadStillClosesOverlaysAndScreens(t *testing.T) {
	for _, open := range []string{"?", ":"} {
		gate := make(chan struct{})
		server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"bob": gate}}
		app := typeQuery(t, newActorApp(server, nil), "actor:bob")
		app = typeKeys(t, app, open, "esc")
		if len(app.overlays) != 0 || app.runs.Filter != "" || strings.Contains(view(app), "/actor:bob") {
			t.Fatalf("%s then esc: overlays=%v filter=%q", open, app.overlays, app.runs.Filter)
		}
		close(gate)
	}
	for _, route := range []string{"artifacts", "caches"} {
		gate := make(chan struct{})
		server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"bob": gate}}
		app := typeQuery(t, newActorApp(server, nil), "actor:bob")
		app = app.openPalette()
		app = app.handlePaletteIntent(palette.Intent{Route: route})
		left := app.Route()
		app = typeKeys(t, app, "esc")
		if left != RouteRuns && app.Route() != RouteRuns {
			t.Fatalf("%s: esc left the user on %s", route, app.Route())
		}
		close(gate)
	}
}

func TestEscRestoresAnErroredViewWithItsReason(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"alice": gate}}
	app := settleApp(t, typeQuery(t, newActorApp(server, &viewerStub{err: errors.New("boom")}), "actor:@me"))
	app = typeKeys(t, typeQuery(t, app, "actor:alice"), "esc")
	if app.runs.Filter != "actor:@me" || !strings.Contains(view(app), "can't resolve @me") {
		t.Fatalf("esc must return to @me with its reason:\n%s", view(app))
	}

	server = &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"alice": gate}}
	app = newActorApp(server, nil)
	server.mu.Lock()
	server.err = errors.New("502")
	server.mu.Unlock()
	app = settleApp(t, typeQuery(t, app, "actor:bob"))
	server.mu.Lock()
	server.err = nil
	server.mu.Unlock()
	app = typeKeys(t, typeQuery(t, app, "actor:alice"), "esc")
	if screen := view(app); !strings.Contains(screen, "couldn't load runs: 502") || strings.Contains(screen, "#303") {
		t.Fatalf("esc must return to bob's failure, not rows picked from another page:\n%s", screen)
	}
}

func TestLaunchFilteredScopeKeepsPagingOnToggle(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	server := &actorRuns{rows: actorRows()}
	var branch, repo []model.Run
	for i := range 3 {
		branch = append(branch, actorRun(int64(400+i), "alice", "trunk", model.ConclusionSuccess))
	}
	for i := range 8 {
		repo = append(repo, actorRun(int64(500+i), "alice", "x", model.ConclusionSuccess))
	}
	app := NewApp(Options{Config: cfg, RunsResolver: server.list, Launch: usecase.LaunchContext{
		Repo: "cli/cli", Branch: "trunk", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns,
		PerPage: 30, Page: 1, HasMore: true, Runs: branch, BranchRuns: branch, RepoRuns: repo, Query: "actor:alice",
	}})
	app = typeKeys(t, app, "s")
	before := server.count()
	app = typeKeys(t, app, "G")
	if !app.runs.Context.HasMore || server.count() == before && app.load == nil {
		t.Fatalf("narrowed launch rows must keep paging: hasMore=%v load=%v", app.runs.Context.HasMore, app.load != nil)
	}
}
