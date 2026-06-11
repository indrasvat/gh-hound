package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultBranchParsesRepoPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/openclaw/openclaw" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"default_branch":"trunk"}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client())
	branch, err := client.DefaultBranch(context.Background(), "openclaw/openclaw")
	if err != nil || branch != "trunk" {
		t.Fatalf("DefaultBranch = %q, %v", branch, err)
	}
}

func TestRefExistsChecksBranchesThenTags(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/repos/x/y/branches/release%2Fv1":
			_, _ = w.Write([]byte(`{"name":"release/v1"}`))
		case "/repos/x/y/branches/v0.4.1":
			http.Error(w, "not found", http.StatusNotFound)
		case "/repos/x/y/git/ref/tags/v0.4.1":
			_, _ = w.Write([]byte(`{"ref":"refs/tags/v0.4.1"}`))
		case "/repos/x/y/branches/ghost", "/repos/x/y/git/ref/tags/ghost":
			http.Error(w, "not found", http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client())
	ctx := context.Background()

	// Slash branch: must be path-escaped, found on the branch probe.
	exists, err := client.RefExists(ctx, "x/y", "release/v1")
	if err != nil || !exists {
		t.Fatalf("slash branch = %v, %v", exists, err)
	}
	// Tag: branch probe 404s, tag probe hits.
	exists, err = client.RefExists(ctx, "x/y", "v0.4.1")
	if err != nil || !exists {
		t.Fatalf("tag = %v, %v", exists, err)
	}
	// Neither: false without error.
	exists, err = client.RefExists(ctx, "x/y", "ghost")
	if err != nil || exists {
		t.Fatalf("ghost = %v, %v", exists, err)
	}
	if len(paths) != 5 {
		t.Fatalf("probe paths = %v", paths)
	}
}
