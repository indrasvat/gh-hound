package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/github"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// viewerGitHub adds the optional /user capability to the recording
// test double.
type viewerGitHub struct {
	*cliGitHub
	login string
	err   error
	calls int
}

func (g *viewerGitHub) AuthenticatedLogin(context.Context) (string, error) {
	g.calls++
	return g.login, g.err
}

func actorTestRun() model.Run {
	return model.Run{
		ID: 1001, RunNumber: 11, Name: "CI", Event: "push", HeadBranch: "main", HeadSHA: "abc123",
		Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess,
		Actor: "alice", TriggeringActor: "bob", RunAttempt: 2,
	}
}

func runActorCLI(t *testing.T, github usecase.GitHub, env map[string]string, args ...string) (int, string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    mapEnv(env),
		IsTTY:  true,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs(args)
	code, err := executeCommand(cmd)
	return code, out.String(), err
}

func TestRunsActorFlagFiltersServerSideAndEmitsActors(t *testing.T) {
	github := &cliGitHub{runs: []model.Run{actorTestRun()}}
	code, out, err := runActorCLI(t, github, nil, "runs", "--actor", "@alice", "--status", "success", "--no-tui", "--json")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out)
	}
	if got := github.filters[0]; got.Actor != "alice" || got.Status != "success" || got.Branch != "main" {
		t.Fatalf("filter = %+v", got)
	}
	run := decodeJSON(t, []byte(out))["runs"].([]any)[0].(map[string]any)
	if run["actor"] != "alice" || run["triggering_actor"] != "bob" {
		t.Fatalf("run actors = %v / %v", run["actor"], run["triggering_actor"])
	}
}

func TestRunsActorMeResolvesThroughAuthenticatedUser(t *testing.T) {
	github := &viewerGitHub{cliGitHub: &cliGitHub{runs: []model.Run{actorTestRun()}}, login: "alice"}
	code, out, err := runActorCLI(t, github, nil, "runs", "--actor", "@me", "--all", "--no-tui", "--json")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out)
	}
	if github.calls != 1 || github.filters[0].Actor != "alice" || github.filters[0].Branch != "" {
		t.Fatalf("calls=%d filter=%+v", github.calls, github.filters[0])
	}
}

func TestRunsActorMeFailureNeverBroadens(t *testing.T) {
	github := &viewerGitHub{
		cliGitHub: &cliGitHub{runs: []model.Run{actorTestRun()}},
		err:       usecase.APIError{Kind: usecase.APIErrorAuth, Status: 401, Message: "Requires authentication"},
	}
	code, out, err := runActorCLI(t, github, nil, "runs", "--actor", "@me", "--no-tui", "--json")
	if code != 2 || err == nil {
		t.Fatalf("code=%d err=%v out=%s", code, err, out)
	}
	if !strings.Contains(err.Error(), "can't resolve @me: not signed in to GitHub") || !strings.Contains(err.Error(), "pass --actor <login>") {
		t.Fatalf("error = %q", err)
	}
	if github.filterCount() != 0 || out != "" {
		t.Fatalf("an unresolved @me must not list anyone's runs: %d calls, out=%q", github.filterCount(), out)
	}
}

func TestRunsActorEnvAndFlagPrecedence(t *testing.T) {
	github := &cliGitHub{runs: []model.Run{actorTestRun()}}
	if code, out, err := runActorCLI(t, github, map[string]string{"HOUND_ACTOR": "alice"}, "runs", "--no-tui", "--json"); err != nil || code != 0 {
		t.Fatalf("env actor code=%d err=%v out=%s", code, err, out)
	}
	if github.filters[0].Actor != "alice" {
		t.Fatalf("HOUND_ACTOR not applied: %+v", github.filters[0])
	}
	github = &cliGitHub{runs: []model.Run{actorTestRun()}}
	if code, out, err := runActorCLI(t, github, map[string]string{"HOUND_ACTOR": "alice"}, "runs", "--actor", "bob", "--no-tui", "--json"); err != nil || code != 0 {
		t.Fatalf("flag actor code=%d err=%v out=%s", code, err, out)
	}
	if github.filters[0].Actor != "bob" {
		t.Fatalf("--actor must beat HOUND_ACTOR: %+v", github.filters[0])
	}
}

func TestRunsActorNeverRedirectsExplicitRun(t *testing.T) {
	run := actorTestRun()
	github := &cliGitHub{runByID: map[int64]model.Run{run.ID: run}}
	code, _, err := runActorCLI(t, github, nil, "runs", "--run", "1001", "--actor", "bob", "--no-tui", "--json")
	if code != 2 || err == nil || !strings.Contains(err.Error(), "--actor cannot be combined with --run") {
		t.Fatalf("explicit --actor with --run: code=%d err=%v", code, err)
	}
	code, out, err := runActorCLI(t, github, map[string]string{"HOUND_ACTOR": "bob"}, "runs", "--run", "1001", "--no-tui", "--json")
	if err != nil || code != 0 {
		t.Fatalf("HOUND_ACTOR must not block --run: code=%d err=%v", code, err)
	}
	if got := decodeJSON(t, []byte(out))["runs"].([]any); len(got) != 1 {
		t.Fatalf("--run must show that run whoever started it: %s", out)
	}
}

func TestRunsActorRejectsInvalidLogin(t *testing.T) {
	github := &cliGitHub{}
	code, _, err := runActorCLI(t, github, nil, "runs", "--actor", "not a login", "--no-tui", "--json")
	if code != 2 || err == nil || !strings.Contains(err.Error(), "isn't a GitHub login") {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if github.filterCount() != 0 {
		t.Fatal("invalid input must not reach GitHub")
	}
}

func TestRunsActorEmptyBranchDiagnosticRespectsActor(t *testing.T) {
	// Branch has runs, just none by this actor: a filter miss, not a
	// target mismatch.
	busy := &cliGitHub{runBatches: [][]model.Run{{}, {actorTestRun()}}}
	_, out, _ := runActorCLI(t, busy, nil, "runs", "--actor", "carol", "--no-tui", "--json")
	if _, ok := decodeJSON(t, []byte(out))["diagnostic"]; ok {
		t.Fatalf("filter miss must not claim a target mismatch:\n%s", out)
	}
	if busy.filterCount() != 2 || busy.filters[1].Actor != "" {
		t.Fatalf("expected an unfiltered branch probe, got %#v", busy.filters)
	}

	empty := &cliGitHub{runBatches: [][]model.Run{{}, {}}}
	_, out, _ = runActorCLI(t, empty, nil, "runs", "--actor", "carol", "--no-tui", "--json")
	diagnostic := decodeJSON(t, []byte(out))["diagnostic"].(map[string]any)
	commands := diagnostic["next_commands"].([]any)
	if !strings.Contains(commands[0].(string), "--all --actor carol") || !strings.HasPrefix(diagnostic["message"].(string), "No runs by @carol") {
		t.Fatalf("diagnostic must keep the actor: %v", diagnostic)
	}
}

func TestRunsActorFakeScenario(t *testing.T) {
	_, out, _ := runActorCLI(t, nil, nil, "runs", "--actor", "@me", "--no-tui", "--json", "--fake-scenario", "failure")
	run := decodeJSON(t, []byte(out))["runs"].([]any)[0].(map[string]any)
	if run["actor"] != "indrasvat" {
		t.Fatalf("fake run actor = %v", run["actor"])
	}
	_, out, _ = runActorCLI(t, nil, nil, "runs", "--actor", "someone-else", "--no-tui", "--json", "--fake-scenario", "failure")
	decoded := decodeJSON(t, []byte(out))
	if runs := decoded["runs"].([]any); len(runs) != 0 {
		t.Fatalf("other actor must match nothing: %s", out)
	}
	if _, ok := decoded["diagnostic"]; ok {
		t.Fatalf("an actor miss is not a target mismatch: %s", out)
	}
}

func TestRunsActorDropsLooseGitHubMatches(t *testing.T) {
	bot := actorTestRun()
	bot.ID, bot.Actor, bot.TriggeringActor = 1002, "cli-triage[bot]", "cli-triage[bot]"
	github := &cliGitHub{runs: []model.Run{actorTestRun(), bot}}
	_, out, err := runActorCLI(t, github, nil, "runs", "--actor", "alice", "--all", "--no-tui", "--json")
	if err != nil {
		t.Fatal(err)
	}
	runs := decodeJSON(t, []byte(out))["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["actor"] != "alice" {
		t.Fatalf("--actor must list only that actor's runs: %s", out)
	}
}

func TestHoundActorNeverTouchesRunOrWatch(t *testing.T) {
	run := actorTestRun()
	github := &cliGitHub{runByID: map[int64]model.Run{run.ID: run}}
	code, out, err := runActorCLI(t, github, map[string]string{"HOUND_ACTOR": "not a login"}, "runs", "--run", "1001", "--no-tui", "--json")
	if err != nil || code != 0 || len(decodeJSON(t, []byte(out))["runs"].([]any)) != 1 {
		t.Fatalf("HOUND_ACTOR must be ignored for --run: code=%d err=%v out=%s", code, err, out)
	}
	_, out, _ = runActorCLI(t, nil, map[string]string{"HOUND_ACTOR": "bob"}, "runs", "--run", "5", "--no-tui", "--json", "--fake-scenario", "failure")
	if len(decodeJSON(t, []byte(out))["runs"].([]any)) != 1 {
		t.Fatalf("fake --run must ignore HOUND_ACTOR: %s", out)
	}
	code, out, _ = runActorCLI(t, nil, map[string]string{"HOUND_ACTOR": "bob"}, "watch", "--json", "--fake-scenario", "pending")
	if code != 3 || len(decodeJSON(t, []byte(out))["runs"].([]any)) != 1 {
		t.Fatalf("watch must not narrow by HOUND_ACTOR: code=%d out=%s", code, out)
	}
}

func TestRunsActorPagesPastLooseMatches(t *testing.T) {
	loose := []model.Run{}
	for i := range 30 {
		run := actorTestRun()
		run.ID, run.Actor = int64(2000+i), "cli-triage[bot]"
		loose = append(loose, run)
	}
	github := &cliGitHub{runBatches: [][]model.Run{loose, {actorTestRun()}}}
	_, out, err := runActorCLI(t, github, nil, "runs", "--actor", "alice", "--all", "--no-tui", "--json")
	if err != nil || len(decodeJSON(t, []byte(out))["runs"].([]any)) != 1 || github.filters[1].Page != 2 {
		t.Fatalf("an actor listing must read past a page of loose matches: err=%v filters=%+v out=%s", err, github.filters, out)
	}
}

type headerRecorder struct {
	auth []string
}

func (r *headerRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.auth = append(r.auth, req.URL.Path+" "+req.Header.Get("Authorization"))
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"login":"alice"}`)), Request: req}, nil
}

// @me must authenticate exactly like run requests: GH_TOKEN, then
// GITHUB_TOKEN, then the gh keyring — never git config.
func TestActorMeUsesTheRunRequestTokenPrecedence(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"GH_TOKEN": "gh-env", "GITHUB_TOKEN": "github-env"}, "Bearer gh-env"},
		{map[string]string{"GITHUB_TOKEN": "github-env"}, "Bearer github-env"},
		{map[string]string{}, "Bearer keyring"},
	}
	for _, tt := range tests {
		client := authenticatedHTTPClient(mapEnv(tt.env), func() string { return "keyring" })
		recorder := &headerRecorder{}
		transport := client.Transport.(authTransport)
		transport.base = recorder
		client.Transport = transport
		resolve := viewerResolver(github.NewClient("https://api.github.com", client))
		login, err := resolve(context.Background())
		if err != nil || login != "alice" || len(recorder.auth) != 1 || recorder.auth[0] != "/user "+tt.want {
			t.Fatalf("env %v: login=%q err=%v requests=%q, want /user %s", tt.env, login, err, recorder.auth, tt.want)
		}
	}
}
