package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/tui/screens/runs"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// actorRuns is a scripted list-runs endpoint: it filters like GitHub
// (actor, status, branch) and records every request.
type actorRuns struct {
	mu    sync.Mutex
	rows  []model.Run
	calls []usecase.RunFilter
	// gates holds an actor's responses until the test releases them.
	gates map[string]chan struct{}
	err   error
}

func (s *actorRuns) list(ctx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	s.mu.Lock()
	s.calls = append(s.calls, filter)
	gate, err := s.gates[filter.Actor], s.err
	s.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	out := []model.Run{}
	for _, run := range s.rows {
		if filter.Actor != "" && !strings.EqualFold(run.Actor, filter.Actor) {
			continue
		}
		if filter.Status != "" && string(run.Conclusion) != filter.Status && string(run.Status) != filter.Status {
			continue
		}
		if filter.Branch != "" && run.HeadBranch != filter.Branch {
			continue
		}
		out = append(out, run)
	}
	return out, nil
}

func (s *actorRuns) last() usecase.RunFilter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[len(s.calls)-1]
}

func (s *actorRuns) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func actorRun(id int64, actor, branch string, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID: id, RunNumber: int(id), Name: "CI", DisplayTitle: "change " + actor, Event: "push",
		HeadBranch: branch, Status: model.StatusCompleted, Conclusion: conclusion, Actor: actor,
	}
}

func actorRows() []model.Run {
	return []model.Run{
		actorRun(301, "indrasvat", "trunk", model.ConclusionFailure),
		actorRun(302, "alice", "trunk", model.ConclusionSuccess),
		actorRun(303, "bob", "trunk", model.ConclusionFailure),
		actorRun(304, "indrasvat", "feat/x", model.ConclusionFailure),
	}
}

type viewerStub struct {
	calls atomic.Int32
	login string
	err   error
}

func (v *viewerStub) resolve(context.Context) (string, error) {
	v.calls.Add(1)
	return v.login, v.err
}

func newActorApp(server *actorRuns, viewer *viewerStub) App {
	cfg := config.Default()
	cfg.Welcome = false
	rows := server.rows
	trunk := []model.Run{}
	for _, run := range rows {
		if run.HeadBranch == "trunk" {
			trunk = append(trunk, run)
		}
	}
	options := Options{
		Config: cfg,
		Launch: usecase.LaunchContext{
			Repo: "cli/cli", Branch: "trunk", Actor: "Git Author Name", Scope: usecase.LaunchScopeBranch,
			State: usecase.LaunchStateRuns, PerPage: 30, Page: 1,
			Runs: trunk, BranchRuns: trunk, RepoRuns: rows,
		},
		RunsResolver: server.list,
	}
	if viewer != nil {
		options.ViewerResolver = viewer.resolve
	}
	return NewApp(options)
}

func typeKeys(t *testing.T, app App, keys ...string) App {
	t.Helper()
	for _, key := range keys {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	return app
}

func typeQuery(t *testing.T, app App, query string) App {
	t.Helper()
	app = typeKeys(t, app, "/", "ctrl+u")
	for _, r := range query {
		key := string(r)
		if key == " " {
			key = "space"
		}
		app = typeKeys(t, app, key)
	}
	return typeKeys(t, app, "enter")
}

func view(app App) string {
	return ansi.Strip(app.ViewSize(120, 30))
}

func TestActorMeResolvesOnceAndFiltersServerSide(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	viewer := &viewerStub{login: "indrasvat"}
	app := settleApp(t, typeQuery(t, newActorApp(server, viewer), "actor:@me"))

	if got := server.last(); got.Actor != "indrasvat" || got.Branch != "trunk" {
		t.Fatalf("filter = %+v", got)
	}
	screen := view(app)
	if !strings.Contains(screen, "/actor:@me → @indrasvat · 1 run · Esc clear") || strings.Contains(screen, "@alice") || strings.Contains(screen, "@bob") {
		t.Fatalf("@me list:\n%s", screen)
	}
	for range 5 {
		app, _ = pollCycle(t, app)
	}
	if got := server.last(); got.Actor != "indrasvat" {
		t.Fatalf("poll dropped the actor: %+v", got)
	}
	if calls := viewer.calls.Load(); calls != 1 {
		t.Fatalf("identity lookups = %d, want 1 per session", calls)
	}
}

func TestHeaderShowsOnlyAuthenticatedIdentity(t *testing.T) {
	app := newActorApp(&actorRuns{rows: actorRows()}, nil)
	if strings.Contains(view(app), "Git Author Name") || strings.Contains(strings.SplitN(view(app), "\n", 3)[1], "@") {
		t.Fatalf("header must not present the git author as a login:\n%s", view(app))
	}
	viewer := &viewerStub{login: "indrasvat"}
	app = newActorApp(&actorRuns{rows: actorRows()}, viewer)
	if strings.Contains(strings.SplitN(view(app), "\n", 3)[1], "@indrasvat") {
		t.Fatal("identity must not appear before the lookup lands")
	}
	if viewer.calls.Load() != 0 {
		t.Fatal("identity lookup must not run before the first paint")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(strings.SplitN(view(app), "\n", 3)[1], "⎇ branch trunk · @indrasvat") {
		if time.Now().After(deadline) {
			t.Fatalf("header never showed the identity:\n%s", view(app))
		}
		app, _ = app.Refresh()
		time.Sleep(time.Millisecond)
	}
}

func TestStaleActorResponseNeverReplacesNewerQuery(t *testing.T) {
	gate := make(chan struct{})
	server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"alice": gate}}
	app := newActorApp(server, nil)
	app = typeQuery(t, app, "actor:alice")
	app = typeQuery(t, app, "actor:bob")
	close(gate)
	app = settleApp(t, app)
	screen := view(app)
	if !strings.Contains(screen, "/actor:bob") || !strings.Contains(screen, "@bob") || strings.Contains(screen, "@alice") {
		t.Fatalf("an older query's rows landed:\n%s", screen)
	}
}

func TestTextOnlyQueryDropsInFlightServerLoad(t *testing.T) {
	gate := make(chan struct{})
	server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"alice": gate}}
	app := newActorApp(server, nil)
	app = typeQuery(t, app, "actor:alice")
	app = typeQuery(t, app, "change")
	close(gate)
	app = settleApp(t, app)
	screen := view(app)
	if !strings.Contains(screen, "/change · 3 of 3 loaded") || !strings.Contains(screen, "@bob") {
		t.Fatalf("text search must cover the unfiltered rows, not alice's:\n%s", screen)
	}
}

func TestStatusCycleAndScopeKeepActor(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := settleApp(t, typeQuery(t, newActorApp(server, &viewerStub{login: "indrasvat"}), "actor:@me"))

	app = settleApp(t, typeKeys(t, app, "f"))
	if got := server.last(); got.Actor != "indrasvat" || got.Status != "failure" {
		t.Fatalf("f dropped the actor: %+v", got)
	}
	app = settleApp(t, typeKeys(t, app, "s"))
	if got := server.last(); got.Actor != "indrasvat" || got.Branch != "" || got.Status != "failure" {
		t.Fatalf("s dropped a constraint: %+v", got)
	}
	screen := view(app)
	if !strings.Contains(screen, "/actor:@me status:failure → @indrasvat · 2 runs") || !strings.Contains(screen, "#304") {
		t.Fatalf("repo scope under the filter:\n%s", screen)
	}
	// Back to branch scope under the same filter: its cache is current,
	// so the switch is instant.
	calls := server.count()
	app = typeKeys(t, app, "s")
	if server.count() != calls || app.load != nil || !strings.Contains(view(app), "#301") {
		t.Fatalf("current branch cache must switch without a fetch:\n%s", view(app))
	}
	// Change the filter in repo scope: the branch cache is now stale and
	// must reload rather than show rows from another filter.
	app = settleApp(t, typeKeys(t, app, "s", "f"))
	app = typeKeys(t, app, "s")
	if strings.Contains(view(app), "#301") || strings.Contains(view(app), "@bob") {
		t.Fatalf("stale branch cache leaked rows from another filter:\n%s", view(app))
	}
	app = settleApp(t, app)
	if got := server.last(); got.Actor != "indrasvat" || got.Branch != "trunk" || got.Status != "in_progress" {
		t.Fatalf("branch reload = %+v", got)
	}
}

func TestScopeResultParksWhenUserMovesOn(t *testing.T) {
	gate := make(chan struct{})
	server := &actorRuns{rows: actorRows(), gates: map[string]chan struct{}{"bob": gate}}
	app := newActorApp(server, nil)
	app = typeQuery(t, app, "actor:bob") // branch-scope load held
	app = typeKeys(t, app, "s")          // repo-scope load supersedes it
	close(gate)
	app = settleApp(t, app)
	if app.runs.Context.Scope != usecase.LaunchScopeRepo || !strings.Contains(view(app), "@bob") || strings.Contains(view(app), "@alice") {
		t.Fatalf("scope switch under the filter:\n%s", view(app))
	}
}

func TestPaginationAndDetailBackKeepFilter(t *testing.T) {
	rows := []model.Run{}
	for i := range 45 {
		rows = append(rows, actorRun(int64(500-i), "alice", "trunk", model.ConclusionSuccess))
	}
	server := &actorRuns{rows: rows}
	app := newActorApp(server, nil)
	app.runsResolver = func(ctx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
		all, err := server.list(ctx, filter)
		if filter.Page > 1 {
			return all[min(30, len(all)):], err
		}
		return all[:min(30, len(all))], err
	}
	app = settleApp(t, typeQuery(t, app, "actor:alice"))
	app = settleApp(t, typeKeys(t, app, "G"))
	if got := server.last(); got.Actor != "alice" || got.Page != 2 {
		t.Fatalf("next page dropped the actor: %+v", got)
	}
	calls := server.count()
	app = typeKeys(t, app, "enter")
	app = settleApp(t, typeKeys(t, app, "esc"))
	if app.Route() != RouteRuns || app.runs.Filter != "actor:alice" || server.count() != calls {
		t.Fatalf("back from detail: route=%s filter=%q calls %d→%d", app.Route(), app.runs.Filter, calls, server.count())
	}
}

func TestActorMeFailureExplainsAndNeverBroadens(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	viewer := &viewerStub{err: usecase.APIError{Kind: usecase.APIErrorAuth, Status: 401}}
	app := settleApp(t, typeQuery(t, newActorApp(server, viewer), "actor:@me"))
	screen := view(app)
	if !strings.Contains(screen, "can't resolve @me: not signed in to GitHub") || !strings.Contains(screen, "actor:<login> still works") {
		t.Fatalf("@me failure not explained:\n%s", screen)
	}
	if strings.Contains(screen, "#30") {
		t.Fatalf("unfiltered rows shown under an unresolved @me:\n%s", screen)
	}
	for range 3 {
		app, _ = pollCycle(t, app)
	}
	if viewer.calls.Load() != 1 || server.count() != 0 {
		t.Fatalf("polls retried the lookup (%d) or listed runs (%d)", viewer.calls.Load(), server.count())
	}
	viewer.err, viewer.login = nil, "indrasvat"
	app = settleApp(t, typeQuery(t, app, "actor:@me"))
	if viewer.calls.Load() != 2 || !strings.Contains(view(app), "→ @indrasvat") {
		t.Fatalf("re-applying must retry the lookup:\n%s", view(app))
	}
	if !strings.Contains(view(app), "/actor:<login>") && strings.Contains(view(app), "can't resolve") {
		t.Fatal("stale error survived a successful retry")
	}
}

func TestFilterLoadErrorHidesRowsAndPollHeals(t *testing.T) {
	server := &actorRuns{rows: actorRows(), err: usecase.APIError{Kind: usecase.APIErrorNetwork, Message: "connection reset"}}
	app := settleApp(t, typeQuery(t, newActorApp(server, nil), "actor:alice"))
	screen := view(app)
	if !strings.Contains(screen, "couldn't load runs") || strings.Contains(screen, "@bob") {
		t.Fatalf("load error must replace rows, not show everyone's:\n%s", screen)
	}
	server.mu.Lock()
	server.err = nil
	server.mu.Unlock()
	app, _ = pollCycle(t, app)
	if screen := view(app); strings.Contains(screen, "couldn't load") || !strings.Contains(screen, "@alice") || strings.Contains(screen, "@bob") {
		t.Fatalf("poll did not heal the filtered list:\n%s", screen)
	}
}

func TestEditingKeepsAppliedQueryAndTypedKeysNeverAct(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := settleApp(t, typeQuery(t, newActorApp(server, nil), "actor:alice"))
	calls := server.count()
	app = typeKeys(t, app, "/")
	if app.runs.Draft != "actor:alice" {
		t.Fatalf("input must open pre-filled, got %q", app.runs.Draft)
	}
	app = typeKeys(t, app, "ctrl+u", "q", "j", "k", "T", ":", "?", "s", "f", "r", "x", "G", "space", "w")
	if app.ShouldQuit() || app.TopOverlay() != OverlayNone || app.Route() != RouteRuns || app.Theme().Mode != "bramble" || server.count() != calls {
		t.Fatalf("typed keys acted: quit=%v overlay=%s route=%s theme=%s calls=%d", app.ShouldQuit(), app.TopOverlay(), app.Route(), app.Theme().Mode, server.count())
	}
	if app.runs.Draft != "qjkT:?sfrxG w" {
		t.Fatalf("draft = %q", app.runs.Draft)
	}
	app = typeKeys(t, app, "esc")
	if app.runs.InputMode || app.runs.Filter != "actor:alice" || server.count() != calls {
		t.Fatalf("esc while editing must restore the applied query: %q", app.runs.Filter)
	}
	app = settleApp(t, typeKeys(t, app, "esc"))
	if app.runs.Filter != "" || strings.Contains(view(app), "/actor:alice") || !strings.Contains(view(app), "@bob") {
		t.Fatalf("esc outside editing must clear:\n%s", view(app))
	}
}

func TestInvalidQueryIsRejectedInline(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := newActorApp(server, nil)
	app = typeQuery(t, app, "actor:alice actor:bob")
	if !app.runs.InputMode || app.runs.Filter != "" || server.count() != 0 {
		t.Fatalf("invalid query applied: mode=%v filter=%q calls=%d", app.runs.InputMode, app.runs.Filter, server.count())
	}
	if !strings.Contains(view(app), "only one actor: per filter") {
		t.Fatalf("error not shown:\n%s", view(app))
	}
}

func TestPaletteMyRunsAndAllActorsChangeOnlyActor(t *testing.T) {
	server := &actorRuns{rows: actorRows()}
	app := settleApp(t, typeQuery(t, newActorApp(server, &viewerStub{login: "indrasvat"}), "status:failure"))
	choose := func(app App, name string) App {
		app = typeKeys(t, app, ":")
		for _, r := range name {
			key := string(r)
			if key == " " {
				key = "space"
			}
			app = typeKeys(t, app, key)
		}
		return settleApp(t, typeKeys(t, app, "enter"))
	}
	app = choose(app, "My runs")
	if app.runs.Filter != "status:failure actor:@me" || server.last().Actor != "indrasvat" || server.last().Status != "failure" {
		t.Fatalf("My runs: filter=%q request=%+v", app.runs.Filter, server.last())
	}
	app = choose(app, "All actors")
	if app.runs.Filter != "status:failure" || server.last().Actor != "" || server.last().Status != "failure" {
		t.Fatalf("All actors: filter=%q request=%+v", app.runs.Filter, server.last())
	}
}

func TestActorLaunchQueryIsAppliedFromStart(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	mine := []model.Run{actorRun(301, "indrasvat", "trunk", model.ConclusionFailure)}
	app := NewApp(Options{Config: cfg, Launch: usecase.LaunchContext{
		Repo: "cli/cli", Branch: "trunk", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns,
		Query: "actor:@me", Viewer: "indrasvat", Runs: mine, BranchRuns: mine, RepoRuns: mine, PerPage: 30,
	}})
	if !app.runs.DataCurrent() || !strings.Contains(view(app), "/actor:@me → @indrasvat · 1 run") {
		t.Fatalf("launch filter not applied:\n%s", view(app))
	}
}

func TestRunsQueryErrorKeepsViewerCause(t *testing.T) {
	err := runsQueryError(usecase.ViewerError{Err: errors.New("boom")})
	if err != "can't resolve @me: boom" {
		t.Fatalf("viewer cause = %q", err)
	}
	if !strings.HasPrefix(runsQueryError(errors.New("socket closed")), "couldn't load runs: ") {
		t.Fatal("load cause prefix missing")
	}
	_ = runs.MeActor
}

func TestRunsHelpWithFilterSyntaxFitsSmallestFrame(t *testing.T) {
	for _, screen := range []string{"help"} {
		frame := ansi.Strip(RenderFixtureSize(screen, 80, 24))
		if !strings.Contains(frame, "Filter /") || !strings.Contains(frame, "Legend") || strings.Count(frame, "╰") != 2 || strings.Contains(frame, "│…") {
			t.Fatalf("help overlay clipped at 80x24:\n%s", frame)
		}
	}
}

// GitHub's actor parameter also returns runs tied to the login's pushes
// that someone else started; the list must still show only the actor's.
func TestActorFilterDropsGitHubsLooseMatchesButKeepsPaging(t *testing.T) {
	page := []model.Run{}
	for i := range 30 {
		actor := "cli-triage[bot]"
		if i%3 == 0 {
			actor = "BagToad"
		}
		page = append(page, actorRun(int64(900-i), actor, "trunk", model.ConclusionSuccess))
	}
	var requests []usecase.RunFilter
	app := newActorApp(&actorRuns{rows: actorRows()}, nil)
	app.runsResolver = func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
		requests = append(requests, filter)
		if filter.Page > 1 {
			return []model.Run{actorRun(100, "cli-triage[bot]", "trunk", model.ConclusionSuccess)}, nil
		}
		return page, nil // loose: GitHub ignored most of the actor filter
	}
	app = settleApp(t, typeQuery(t, app, "actor:BagToad"))
	screen := view(app)
	if strings.Contains(screen, "@cli-triage[bot]") || !strings.Contains(screen, "/actor:BagToad · 10+ runs") {
		t.Fatalf("loose matches leaked or paging lost:\n%s", screen)
	}
	app = settleApp(t, typeKeys(t, app, "G"))
	if last := requests[len(requests)-1]; last.Page != 2 || last.Actor != "BagToad" {
		t.Fatalf("G request = %+v", last)
	}
	if strings.Contains(view(app), "@cli-triage[bot]") || app.runs.Context.HasMore {
		t.Fatalf("short last page must end paging without leaking rows:\n%s", view(app))
	}
}

func TestEmptyFilteredPageStillOffersOlderRuns(t *testing.T) {
	page := []model.Run{}
	for i := range 30 {
		page = append(page, actorRun(int64(900-i), "cli-triage[bot]", "trunk", model.ConclusionSuccess))
	}
	var requests []usecase.RunFilter
	app := newActorApp(&actorRuns{rows: actorRows()}, nil)
	app.runsResolver = func(_ context.Context, filter usecase.RunFilter) ([]model.Run, error) {
		requests = append(requests, filter)
		if filter.Page > 1 {
			return []model.Run{actorRun(42, "carol", "trunk", model.ConclusionSuccess)}, nil
		}
		return page, nil
	}
	app = settleApp(t, typeQuery(t, app, "actor:carol"))
	if !strings.Contains(view(app), "G search older runs") {
		t.Fatalf("empty first page must offer older runs:\n%s", view(app))
	}
	app = settleApp(t, typeKeys(t, app, "G"))
	if !strings.Contains(view(app), "@carol") || requests[len(requests)-1].Page != 2 {
		t.Fatalf("G on an empty filtered page must fetch the next one:\n%s", view(app))
	}
}

func TestFilteredGreenListKeepsTheStandardFooter(t *testing.T) {
	green := []model.Run{actorRun(1, "alice", "trunk", model.ConclusionSuccess), actorRun(2, "alice", "trunk", model.ConclusionSuccess)}
	app := newActorApp(&actorRuns{rows: green}, nil)
	if app.footerScreen() != keys.ScreenAllGreen {
		t.Fatalf("unfiltered all-green home = %s", app.footerScreen())
	}
	app = settleApp(t, typeQuery(t, app, "actor:alice"))
	if app.footerScreen() != keys.ScreenRunsList || !strings.Contains(view(app), "f status") {
		t.Fatalf("filtered table needs the runs footer:\n%s", view(app))
	}
}

func TestLaunchWithUnresolvedMeExplainsAndStaysUsable(t *testing.T) {
	cfg := config.Default()
	cfg.Welcome = false
	server := &actorRuns{rows: actorRows()}
	viewer := &viewerStub{err: usecase.APIError{Kind: usecase.APIErrorAuth, Status: 401}}
	app := NewApp(Options{Config: cfg, RunsResolver: server.list, ViewerResolver: viewer.resolve, Launch: usecase.LaunchContext{
		Repo: "cli/cli", Branch: "trunk", Scope: usecase.LaunchScopeBranch, State: usecase.LaunchStateRuns, PerPage: 30,
		Query: "actor:@me", QueryError: usecase.ViewerError{Err: viewer.err}.Error(),
	}})
	screen := view(app)
	if !strings.Contains(screen, "can't resolve @me") || !strings.Contains(screen, "actor:<login> still works") {
		t.Fatalf("launch failure must open on the explained runs list:\n%s", screen)
	}
	for range 3 {
		app, _ = pollCycle(t, app)
	}
	if server.count() != 0 || strings.Contains(view(app), "#30") {
		t.Fatalf("polls broadened an unresolved @me: %d listings\n%s", server.count(), view(app))
	}
	app = settleApp(t, typeQuery(t, app, "actor:alice"))
	if screen := view(app); !strings.Contains(screen, "/actor:alice · 1 run") || strings.Contains(screen, "can't resolve") {
		t.Fatalf("actor:<login> must work after a failed @me launch:\n%s", screen)
	}
}
