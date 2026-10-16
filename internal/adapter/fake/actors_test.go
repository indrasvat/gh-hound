package fake

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestActorsScenarioFiltersByActorAcrossPages(t *testing.T) {
	adapter := New(ScenarioActors)
	ctx := context.Background()
	all, err := adapter.ListRuns(ctx, usecase.RunFilter{Repo: "indrasvat/gh-hound"})
	if err != nil || len(all) != 40 {
		t.Fatalf("all runs = %d, %v", len(all), err)
	}
	first, _ := adapter.ListRuns(ctx, usecase.RunFilter{PerPage: 30, Page: 1})
	second, _ := adapter.ListRuns(ctx, usecase.RunFilter{PerPage: 30, Page: 2})
	if len(first) != 30 || len(second) != 10 || first[0].ID == second[0].ID {
		t.Fatalf("pages = %d + %d", len(first), len(second))
	}
	mine, _ := adapter.ListRuns(ctx, usecase.RunFilter{Actor: "@INDRASVAT"})
	if len(mine) == 0 {
		t.Fatal("actor filter matched nothing")
	}
	for _, run := range mine {
		if !strings.EqualFold(run.Actor, "indrasvat") {
			t.Fatalf("actor filter leaked %q", run.Actor)
		}
	}
	failing, _ := adapter.ListRuns(ctx, usecase.RunFilter{Actor: "alice", Status: "failure", Branch: "main", Event: "push"})
	for _, run := range failing {
		if run.Actor != "alice" || run.Conclusion != "failure" || run.HeadBranch != "main" || run.Event != "push" {
			t.Fatalf("combined filter leaked %+v", run)
		}
	}
	if len(failing) == 0 {
		t.Fatal("combined filter matched nothing")
	}
	none, _ := adapter.ListRuns(ctx, usecase.RunFilter{Actor: "nobody-here"})
	if len(none) != 0 {
		t.Fatalf("unknown actor = %d runs, want 0", len(none))
	}
}

func TestActorsScenarioCoversIdentityEdgeCases(t *testing.T) {
	runs, _ := New(ScenarioActors).ListRuns(context.Background(), usecase.RunFilter{})
	var missing, bot, long, rerun bool
	for _, run := range runs {
		missing = missing || run.Actor == ""
		bot = bot || strings.HasSuffix(run.Actor, "[bot]")
		long = long || len(run.Actor) > 30
		rerun = rerun || (run.RunAttempt > 1 && run.TriggeringActor != run.Actor)
	}
	if !missing || !bot || !long || !rerun {
		t.Fatalf("missing=%v bot=%v long=%v rerun=%v", missing, bot, long, rerun)
	}
}

func TestFakeIdentity(t *testing.T) {
	ctx := context.Background()
	if login, err := New(ScenarioActors).AuthenticatedLogin(ctx); err != nil || login != "indrasvat" {
		t.Fatalf("login = %q, %v", login, err)
	}
	_, err := New(ScenarioActorsAnon).AuthenticatedLogin(ctx)
	var apiErr usecase.APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != usecase.APIErrorAuth {
		t.Fatalf("anon identity error = %v", err)
	}
	if _, err := usecase.ResolveActor(ctx, New(ScenarioActorsAnon), "@me"); err == nil {
		t.Fatal("@me must not resolve without an identity")
	}
}
