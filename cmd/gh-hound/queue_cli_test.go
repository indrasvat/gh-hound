package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func runQueueCommand(t *testing.T, runtime commandRuntime, args ...string) (int, string, error) {
	t.Helper()
	var out bytes.Buffer
	if runtime.Stdout == nil {
		runtime.Stdout = &out
	}
	if runtime.Stderr == nil {
		runtime.Stderr = io.Discard
	}
	if runtime.Env == nil {
		runtime.Env = emptyEnv
	}
	cmd := newRootCommandWithRuntime(runtime, testBuildInfo())
	cmd.SetArgs(append([]string{"queue"}, args...))
	code, err := executeCommand(cmd)
	return code, out.String(), err
}

func TestQueueBlockedScenarioExitsPending(t *testing.T) {
	code, out, _ := runQueueCommand(t, commandRuntime{IsTTY: true},
		"--run", "30433644", "--no-tui", "--json", "--fake-scenario", "queue")
	if code != 3 {
		t.Fatalf("exit = %d, want 3\n%s", code, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != "blocked" {
		t.Fatalf("status = %v, want blocked\n%s", decoded["status"], out)
	}
	decision := decoded["decision"].(map[string]any)
	if decision["action"] != "wait" {
		t.Fatalf("decision.action = %v, want wait\n%s", decision["action"], out)
	}
}

func TestQueueGroupsScenarioListsSummariesWithoutAllFlag(t *testing.T) {
	code, out, err := runQueueCommand(t, commandRuntime{IsTTY: true},
		"--groups", "--no-tui", "--json", "--fake-scenario", "queue")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want 0\n%s", code, err, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != "listed" {
		t.Fatalf("status = %v, want listed\n%s", decoded["status"], out)
	}
	groups := decoded["groups"].([]any)
	if len(groups) == 0 {
		t.Fatalf("groups empty\n%s", out)
	}
	group := groups[0].(map[string]any)
	if _, ok := group["active_member_count"]; ok {
		t.Fatalf("summary-only --groups must not emit false active_member_count\n%s", out)
	}
	if _, ok := group["last_acquired_at"]; !ok {
		t.Fatalf("summary-only --groups should preserve last_acquired_at\n%s", out)
	}
}

func TestQueueGroupsAPIErrorReturnsTypedEnvelope(t *testing.T) {
	github := &cliGitHub{
		concurrencyErr: usecase.APIError{Kind: usecase.APIErrorRateLimit, Status: 403, Message: "rate limit exceeded"},
	}
	code, out, _ := runQueueCommand(t, commandRuntime{
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat"}},
		IsTTY:  true,
	}, "--groups", "--no-tui", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != "error" {
		t.Fatalf("status = %v, want error\n%s", decoded["status"], out)
	}
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != string(usecase.APIErrorRateLimit) {
		t.Fatalf("error.kind = %v, want rate_limit\n%s", errObj["kind"], out)
	}
}

func TestQueueGroupsEndpointNotFoundReturnsUnsupported(t *testing.T) {
	github := &cliGitHub{
		concurrencyErr: usecase.APIError{Kind: usecase.APIErrorNotFound, Status: 404, Message: "Not Found"},
	}
	code, out, err := runQueueCommand(t, commandRuntime{
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat"}},
		IsTTY:  true,
	}, "--groups", "--no-tui", "--json")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want unsupported exit 0\n%s", code, err, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != string(model.ConcurrencyStatusUnsupported) {
		t.Fatalf("status = %v, want unsupported\n%s", decoded["status"], out)
	}
	decision := decoded["decision"].(map[string]any)
	if decision["action"] != string(model.ConcurrencyActionUnsupported) {
		t.Fatalf("decision.action = %v, want unsupported\n%s", decision["action"], out)
	}
}

func TestQueueRunEndpointNotFoundAfterRunLookupReturnsUnsupported(t *testing.T) {
	github := &cliGitHub{
		runByID: map[int64]model.Run{
			30433644: {ID: 30433644, Status: model.StatusPending},
		},
		concurrencyErr: usecase.APIError{Kind: usecase.APIErrorNotFound, Status: 404, Message: "Not Found"},
	}
	code, out, err := runQueueCommand(t, commandRuntime{
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat"}},
		IsTTY:  true,
	}, "--run", "30433644", "--no-tui", "--json")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want unsupported exit 0\n%s", code, err, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != string(model.ConcurrencyStatusUnsupported) {
		t.Fatalf("status = %v, want unsupported\n%s", decoded["status"], out)
	}
}

func TestQueueRequiresExactlyOneTargetMode(t *testing.T) {
	code, out, _ := runQueueCommand(t, commandRuntime{IsTTY: true},
		"--run", "30433644", "--groups", "--no-tui", "--json", "--fake-scenario", "queue")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out)
	}
	decoded := decodeJSON(t, []byte(out))
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "validation" {
		t.Fatalf("error.kind = %v, want validation\n%s", errObj["kind"], out)
	}
}

func TestQueueBareValidationUsesSchemaValidUnknownTarget(t *testing.T) {
	code, out, _ := runQueueCommand(t, commandRuntime{IsTTY: true},
		"--no-tui", "--json", "--fake-scenario", "queue")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out)
	}
	decoded := decodeJSON(t, []byte(out))
	errObj := decoded["error"].(map[string]any)
	if errObj["kind"] != "validation" {
		t.Fatalf("error.kind = %v, want validation\n%s", errObj["kind"], out)
	}
	target := decoded["target"].(map[string]any)
	if target["kind"] != "unknown" {
		t.Fatalf("target.kind = %v, want unknown\n%s", target["kind"], out)
	}
	if _, ok := decoded["fetched_at"]; !ok {
		t.Fatalf("error envelope missing fetched_at\n%s", out)
	}
}

func TestQueueGroupInactiveReturnsNormalWatch(t *testing.T) {
	github := &cliGitHub{
		concurrencyErr: usecase.APIError{Kind: usecase.APIErrorNotFound, Status: 404, Message: "Not Found"},
	}
	code, out, err := runQueueCommand(t, commandRuntime{
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat"}},
		IsTTY:  true,
	}, "--group", "deploy-prod", "--no-tui", "--json")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want 0\n%s", code, err, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != "inactive" {
		t.Fatalf("status = %v, want inactive\n%s", decoded["status"], out)
	}
	decision := decoded["decision"].(map[string]any)
	if decision["action"] != "normal_watch" {
		t.Fatalf("decision.action = %v, want normal_watch\n%s", decision["action"], out)
	}
}

func TestQueueGroupSuccessReturnsListed(t *testing.T) {
	github := &cliGitHub{
		concurrencyGroup: model.ConcurrencyGroup{
			GroupName: "deploy-prod",
			Members:   []model.ConcurrencyMember{{RunID: 30433642, Status: model.StatusInProgress}},
		},
	}
	code, out, err := runQueueCommand(t, commandRuntime{
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat"}},
		IsTTY:  true,
	}, "--group", "deploy-prod", "--no-tui", "--json")
	if code != 0 || err != nil {
		t.Fatalf("exit = %d err = %v, want 0\n%s", code, err, out)
	}
	decoded := decodeJSON(t, []byte(out))
	if decoded["status"] != "listed" {
		t.Fatalf("status = %v, want listed\n%s", decoded["status"], out)
	}
	decision := decoded["decision"].(map[string]any)
	if decision["action"] != "list_only" {
		t.Fatalf("decision.action = %v, want list_only\n%s", decision["action"], out)
	}
}

func (g *cliGitHub) ListConcurrencyGroups(context.Context, string, usecase.ConcurrencyListOptions) ([]model.ConcurrencyGroupSummary, error) {
	if g.concurrencyErr != nil {
		return nil, g.concurrencyErr
	}
	return g.concurrencyGroups, nil
}

func (g *cliGitHub) GetConcurrencyGroup(context.Context, string, string, usecase.ConcurrencyAheadFilter) (model.ConcurrencyGroup, error) {
	if g.concurrencyErr != nil {
		return model.ConcurrencyGroup{}, g.concurrencyErr
	}
	return g.concurrencyGroup, nil
}

func (g *cliGitHub) ListRunConcurrencyGroups(context.Context, string, int64, usecase.ConcurrencyListOptions) ([]model.ConcurrencyGroup, error) {
	if g.concurrencyErr != nil {
		return nil, g.concurrencyErr
	}
	return g.runGroups, nil
}
