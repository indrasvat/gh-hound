package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// TestListCachesDecodesLivePinnedPayload replays the exact payload
// captured from GET /repos/indrasvat/gh-hound/actions/caches
// (2026-06-10, API v2026-03-10) so the DTO mapping is pinned to the
// real wire shape, version field and fractional timestamps included.
func TestListCachesDecodesLivePinnedPayload(t *testing.T) {
	payload, err := os.ReadFile("testdata/caches_list_live.json")
	if err != nil {
		t.Fatalf("read pinned payload: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/caches" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	caches, err := client.ListCaches(context.Background(), "indrasvat/gh-hound", usecase.CacheFilter{})
	if err != nil {
		t.Fatalf("ListCaches error: %v", err)
	}
	if len(caches) != 3 {
		t.Fatalf("caches = %d, want 3", len(caches))
	}
	first := caches[0]
	if first.ID != 4902531779 || first.Ref != "refs/heads/main" || first.SizeInBytes != 302526514 {
		t.Fatalf("live mapping wrong: %#v", first)
	}
	if !strings.HasPrefix(first.Key, "setup-go-Linux-x64-ubuntu24-go-1.26.4-") {
		t.Fatalf("key mapping wrong: %q", first.Key)
	}
	if first.LastAccessedAt.IsZero() || first.CreatedAt.IsZero() {
		t.Fatalf("timestamps must decode: %#v", first)
	}
}

func TestListCachesPaginatesAndForwardsFilterParams(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/actions/caches") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("per_page") != "100" {
			t.Fatalf("per_page = %q, want 100", query.Get("per_page"))
		}
		if query.Get("sort") != "size_in_bytes" || query.Get("direction") != "desc" {
			t.Fatalf("sort params not forwarded: %v", query)
		}
		if query.Get("key") != "go-mod" || query.Get("ref") != "refs/heads/main" {
			t.Fatalf("key/ref params not forwarded: %v", query)
		}
		pages++
		var rows strings.Builder
		count := 100
		start := 0
		if query.Get("page") == "2" {
			count = 1
			start = 100
		}
		for i := range count {
			if i > 0 {
				rows.WriteString(",")
			}
			_, _ = fmt.Fprintf(&rows, `{"id":%d,"ref":"refs/heads/main","key":"go-mod-%d","last_accessed_at":"2026-06-10T01:00:55Z","created_at":"2026-06-10T01:00:35Z","size_in_bytes":%d}`, start+i+1, start+i+1, (start+i+1)*1024)
		}
		_, _ = fmt.Fprintf(w, `{"total_count":101,"actions_caches":[%s]}`, rows.String())
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	caches, err := client.ListCaches(context.Background(), "indrasvat/gh-hound", usecase.CacheFilter{
		Key:       "go-mod",
		Ref:       "refs/heads/main",
		Sort:      "size_in_bytes",
		Direction: "desc",
	})
	if err != nil {
		t.Fatalf("ListCaches error: %v", err)
	}
	if pages != 2 || len(caches) != 101 {
		t.Fatalf("must paginate: pages=%d caches=%d", pages, len(caches))
	}
}

func TestCacheUsageDecodesLivePinnedPayload(t *testing.T) {
	payload, err := os.ReadFile("testdata/cache_usage_live.json")
	if err != nil {
		t.Fatalf("read pinned payload: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/cache/usage" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	usage, err := client.CacheUsage(context.Background(), "indrasvat/gh-hound")
	if err != nil {
		t.Fatalf("CacheUsage error: %v", err)
	}
	if usage.ActiveSizeInBytes != 2961168053 || usage.ActiveCount != 11 {
		t.Fatalf("usage mapping wrong: %#v", usage)
	}
}

// TestCacheStorageLimitReadsConfiguredCapAndFallsBackOn404 pins the
// ghent P2: a configured 5 GB limit must drive the gauge, and hosts
// without the endpoint report unknown instead of failing the kennel.
func TestCacheStorageLimitReadsConfiguredCapAndFallsBackOn404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/cache/storage-limit" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"max_cache_size_gb": 5}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client())
	limit, err := client.CacheStorageLimit(context.Background(), "indrasvat/gh-hound")
	if err != nil || limit != int64(5)<<30 {
		t.Fatalf("limit = %d, %v; want 5 GiB, nil", limit, err)
	}

	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer missing.Close()
	client = NewClient(missing.URL, missing.Client())
	limit, err = client.CacheStorageLimit(context.Background(), "indrasvat/gh-hound")
	if err != nil || limit != 0 {
		t.Fatalf("404 must report unknown (0, nil), got %d, %v", limit, err)
	}
}

func TestDeleteCacheByIDUsesDeleteVerb(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	count, err := client.DeleteCacheByID(context.Background(), "indrasvat/gh-hound", 4902531779)
	if err != nil {
		t.Fatalf("DeleteCacheByID error: %v", err)
	}
	if count != 1 {
		t.Fatalf("deleted count = %d, want 1", count)
	}
	if method != http.MethodDelete || path != "/repos/indrasvat/gh-hound/actions/caches/4902531779" {
		t.Fatalf("wrong request: %s %s", method, path)
	}
}

func TestDeleteCachesByKeyReportsMatchCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.HasSuffix(r.URL.Path, "/actions/caches") {
			t.Fatalf("wrong request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("key") != "setup-go" || r.URL.Query().Get("ref") != "refs/heads/main" {
			t.Fatalf("key/ref not forwarded: %v", r.URL.Query())
		}
		_, _ = fmt.Fprint(w, `{"total_count":2,"actions_caches":[{"id":1,"key":"setup-go-a"},{"id":2,"key":"setup-go-b"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	count, err := client.DeleteCachesByKey(context.Background(), "indrasvat/gh-hound", "setup-go", "refs/heads/main")
	if err != nil {
		t.Fatalf("DeleteCachesByKey error: %v", err)
	}
	if count != 2 {
		t.Fatalf("deleted count = %d, want 2", count)
	}
}

func TestDeleteCachesMapNotFoundToTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	for name, call := range map[string]func() (int, error){
		"by_key": func() (int, error) {
			return client.DeleteCachesByKey(context.Background(), "x/y", "ghost", "")
		},
		"by_id": func() (int, error) {
			return client.DeleteCacheByID(context.Background(), "x/y", 99)
		},
	} {
		_, err := call()
		var actionErr usecase.ActionError
		if !errors.As(err, &actionErr) || actionErr.Kind != usecase.ActionErrorNotFound {
			t.Fatalf("%s: error = %v, want typed not_found", name, err)
		}
	}
}

func TestDeleteCachesByKeyZeroMatchesIsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"total_count":0,"actions_caches":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.DeleteCachesByKey(context.Background(), "x/y", "ghost", "")
	var actionErr usecase.ActionError
	if !errors.As(err, &actionErr) || actionErr.Kind != usecase.ActionErrorNotFound {
		t.Fatalf("zero matches must be typed not_found, got %v", err)
	}
}

func TestDeleteCachesMapPermissionAndConflict(t *testing.T) {
	status := http.StatusForbidden
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, `{"message":"nope"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.DeleteCacheByID(context.Background(), "x/y", 7)
	var actionErr usecase.ActionError
	if !errors.As(err, &actionErr) || actionErr.Kind != usecase.ActionErrorPermission {
		t.Fatalf("403 must map to permission, got %v", err)
	}

	status = http.StatusConflict
	_, err = client.DeleteCachesByKey(context.Background(), "x/y", "k", "")
	if !errors.As(err, &actionErr) || actionErr.Kind != usecase.ActionErrorConflict {
		t.Fatalf("409 must map to conflict, got %v", err)
	}
}
