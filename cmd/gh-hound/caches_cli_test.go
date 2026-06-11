package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// cliGitHub grows the GitHubCaches capability with call counters so
// the call-count gates can prove default paths stay cache-silent.
func (g *cliGitHub) ListCaches(context.Context, string, usecase.CacheFilter) ([]model.Cache, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.listCaches++
	return g.cacheList, g.cachesErr
}

func (g *cliGitHub) CacheUsage(context.Context, string) (model.CacheUsage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cacheUsageCalls++
	return g.cacheUsage, g.cachesErr
}

func (g *cliGitHub) DeleteCacheByID(_ context.Context, _ string, id int64) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deleteCacheByID++
	g.deletedCacheID = id
	if g.cachesErr != nil {
		return 0, g.cachesErr
	}
	return 1, nil
}

func (g *cliGitHub) DeleteCachesByKey(_ context.Context, _ string, key, ref string) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deleteCacheByKey++
	g.deletedCacheKey, g.deletedCacheRef = key, ref
	if g.cachesErr != nil {
		return 0, g.cachesErr
	}
	return g.cacheKeyMatches, nil
}

func cliCache(id int64, key string, size int64) model.Cache {
	return model.Cache{
		ID:             id,
		Key:            key,
		Ref:            "refs/heads/main",
		SizeInBytes:    size,
		LastAccessedAt: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
		CreatedAt:      time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
	}
}

func cachesRuntime(github *cliGitHub, out *bytes.Buffer) commandRuntime {
	return commandRuntime{
		Stdout: out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  false,
		GitHub: github,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}
}

func TestCachesJSONListsUsageAndCaches(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{
		cacheList:  []model.Cache{cliCache(9001, "setup-go-Linux", 302526514)},
		cacheUsage: model.CacheUsage{ActiveSizeInBytes: 2961168053, ActiveCount: 11},
	}
	cmd := newRootCommandWithRuntime(cachesRuntime(github, &out), testBuildInfo())
	cmd.SetArgs([]string{"caches", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("caches exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		Repo  string `json:"repo"`
		Usage struct {
			ActiveSizeInBytes int64 `json:"active_size_in_bytes"`
			ActiveCount       int   `json:"active_count"`
			CapBytes          int64 `json:"cap_bytes"`
		} `json:"usage"`
		Caches []struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"caches"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded.Repo != "indrasvat/gh-hound" || decoded.Usage.ActiveCount != 11 {
		t.Fatalf("envelope wrong:\n%s", out.String())
	}
	if decoded.Usage.CapBytes != usecase.CacheCapFallbackBytes {
		t.Fatalf("cap_bytes = %d, want the documented fallback", decoded.Usage.CapBytes)
	}
	if len(decoded.Caches) != 1 || decoded.Caches[0].ID != 9001 {
		t.Fatalf("caches listing wrong:\n%s", out.String())
	}
	if github.listCaches != 1 || github.cacheUsageCalls != 1 {
		t.Fatalf("list path must make exactly one list + one usage call, got %d/%d", github.listCaches, github.cacheUsageCalls)
	}
}

func TestCachesDeleteByIDWritesAcceptedEnvelope(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{}
	cmd := newRootCommandWithRuntime(cachesRuntime(github, &out), testBuildInfo())
	cmd.SetArgs([]string{"caches", "--delete-id", "4902531779", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("delete-id exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		Deleted struct {
			Action       string `json:"action"`
			ID           int64  `json:"id"`
			Accepted     bool   `json:"accepted"`
			DeletedCount int    `json:"deleted_count"`
		} `json:"deleted"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded.Deleted.Action != "delete_id" || !decoded.Deleted.Accepted || decoded.Deleted.DeletedCount != 1 || decoded.Deleted.ID != 4902531779 {
		t.Fatalf("delete envelope wrong:\n%s", out.String())
	}
	if github.deletedCacheID != 4902531779 || github.deleteCacheByID != 1 {
		t.Fatalf("adapter delete-by-id not reached: %#v", github)
	}
	if github.listCaches != 0 || github.cacheUsageCalls != 0 {
		t.Fatal("delete path must not also list the kennel")
	}
}

func TestCachesDeleteByKeyReportsMatchCountAndForwardsRef(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{cacheKeyMatches: 3}
	cmd := newRootCommandWithRuntime(cachesRuntime(github, &out), testBuildInfo())
	cmd.SetArgs([]string{"caches", "--delete-key", "go-mod", "--ref", "refs/heads/main", "--no-tui", "--json"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("delete-key exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		Deleted struct {
			Action       string `json:"action"`
			Key          string `json:"key"`
			Ref          string `json:"ref"`
			DeletedCount int    `json:"deleted_count"`
		} `json:"deleted"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded.Deleted.Action != "delete_key" || decoded.Deleted.DeletedCount != 3 || decoded.Deleted.Ref != "refs/heads/main" {
		t.Fatalf("delete-key envelope wrong:\n%s", out.String())
	}
	if github.deletedCacheKey != "go-mod" || github.deletedCacheRef != "refs/heads/main" {
		t.Fatalf("key/ref not forwarded: %#v", github)
	}
}

func TestCachesDeleteFlagsAreMutuallyExclusive(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{}
	cmd := newRootCommandWithRuntime(cachesRuntime(github, &out), testBuildInfo())
	cmd.SetArgs([]string{"caches", "--delete-id", "1", "--delete-key", "go-mod", "--no-tui", "--json"})

	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("ambiguous delete must exit 2, got %d\n%s", code, out.String())
	}
	assertCachesErrorKind(t, out.Bytes(), "validation")
	if github.deleteCacheByID != 0 || github.deleteCacheByKey != 0 {
		t.Fatal("validation refusal must not reach the adapter")
	}
}

func TestCachesRefRequiresDeleteKey(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(cachesRuntime(&cliGitHub{}, &out), testBuildInfo())
	cmd.SetArgs([]string{"caches", "--ref", "refs/heads/main", "--no-tui", "--json"})

	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("--ref without --delete-key must exit 2, got %d\n%s", code, out.String())
	}
	assertCachesErrorKind(t, out.Bytes(), "validation")
}

func TestCachesDeleteNotFoundExitsTwoWithTypedError(t *testing.T) {
	var out bytes.Buffer
	github := &cliGitHub{cachesErr: usecase.ActionError{Kind: usecase.ActionErrorNotFound, Message: "no caches matched key \"ghost\""}}
	cmd := newRootCommandWithRuntime(cachesRuntime(github, &out), testBuildInfo())
	cmd.SetArgs([]string{"caches", "--delete-key", "ghost", "--no-tui", "--json"})

	code, _ := executeCommand(cmd)
	if code != 2 {
		t.Fatalf("no-match delete must exit 2, got %d\n%s", code, out.String())
	}
	assertCachesErrorKind(t, out.Bytes(), "not_found")
	var decoded struct {
		Deleted struct {
			Accepted     bool `json:"accepted"`
			DeletedCount int  `json:"deleted_count"`
		} `json:"deleted"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if decoded.Deleted.Accepted || decoded.Deleted.DeletedCount != 0 {
		t.Fatalf("refusal envelope must report accepted=false, count 0:\n%s", out.String())
	}
}

func TestDefaultPathsMakeZeroCacheCalls(t *testing.T) {
	github := &cliGitHub{
		runs:      []model.Run{cliRun(908, "CI", model.StatusCompleted, model.ConclusionSuccess)},
		cacheList: []model.Cache{cliCache(9001, "setup-go-Linux", 1024)},
	}
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(cachesRuntime(github, &out), testBuildInfo())
	cmd.SetArgs([]string{"runs", "--no-tui", "--json"})
	if code, err := executeCommand(cmd); err != nil || code != 0 {
		t.Fatalf("runs exit = %d, err = %v", code, err)
	}
	if github.listCaches != 0 || github.cacheUsageCalls != 0 || github.deleteCacheByID != 0 || github.deleteCacheByKey != 0 {
		t.Fatalf("default runs path must make zero cache calls: %#v", github)
	}
}

func TestCachesFakeScenarioIsDeterministic(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: io.Discard,
		Env:    emptyEnv,
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"caches", "--no-tui", "--json", "--fake-scenario", "green"})

	code, err := executeCommand(cmd)
	if err != nil || code != 0 {
		t.Fatalf("fake caches exit = %d, err = %v\n%s", code, err, out.String())
	}
	var decoded struct {
		Usage struct {
			ActiveSizeInBytes int64 `json:"active_size_in_bytes"`
			CapBytes          int64 `json:"cap_bytes"`
		} `json:"usage"`
		Caches []any `json:"caches"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, out.String())
	}
	if len(decoded.Caches) != 5 {
		t.Fatalf("fake kennel = %d caches, want 5\n%s", len(decoded.Caches), out.String())
	}
	if float64(decoded.Usage.ActiveSizeInBytes)/float64(decoded.Usage.CapBytes) <= 0.9 {
		t.Fatalf("fake kennel must sit past the eviction warning:\n%s", out.String())
	}
}

func assertCachesErrorKind(t *testing.T, raw []byte, kind string) {
	t.Helper()
	var decoded struct {
		Error *struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(raw))
	}
	if decoded.Error == nil || decoded.Error.Kind != kind {
		t.Fatalf("error.kind = %#v, want %q\n%s", decoded.Error, kind, string(raw))
	}
}
