package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// Dispatch response handling. Live-verified 2026-06-10 on API
// v2026-03-10: the dispatches endpoint returns 200 with
// {workflow_run_id, run_url, html_url}. Older hosts (GHES) still
// answer 204 No Content — both shapes are pinned here.
func TestDispatchWorkflowConsumesThe200Body(t *testing.T) {
	payload, err := os.ReadFile("testdata/dispatch_200_live.json")
	if err != nil {
		t.Fatalf("read pinned dispatch body: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/workflows/release.yml/dispatches" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	result, err := client.DispatchWorkflow(context.Background(), "indrasvat/gh-hound", "release.yml", usecase.DispatchRequest{Ref: "main"})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if result.WorkflowRunID != 27318354797 {
		t.Fatalf("workflow_run_id = %d, want 27318354797", result.WorkflowRunID)
	}
	if result.RunURL != "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/27318354797" {
		t.Fatalf("run_url = %q", result.RunURL)
	}
	if result.HTMLURL != "https://github.com/indrasvat/gh-hound/actions/runs/27318354797" {
		t.Fatalf("html_url = %q", result.HTMLURL)
	}
}

func TestDispatchWorkflow204FallbackLeavesRunIdentityEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	result, err := client.DispatchWorkflow(context.Background(), "indrasvat/gh-hound", "release.yml", usecase.DispatchRequest{Ref: "main"})
	if err != nil {
		t.Fatalf("204 dispatch returned error: %v", err)
	}
	if result.WorkflowRunID != 0 || result.RunURL != "" || result.HTMLURL != "" {
		t.Fatalf("204 dispatch must leave run identity empty for discovery, got %#v", result)
	}
}

func TestListRunsByHeadSHAMapsThePinnedLivePayload(t *testing.T) {
	payload, err := os.ReadFile("testdata/runs_by_head_sha_live.json")
	if err != nil {
		t.Fatalf("read pinned runs payload: %v", err)
	}
	var seenQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	runs, err := client.ListRuns(context.Background(), usecase.RunFilter{
		Repo:    "indrasvat/gh-hound",
		HeadSHA: "42e1eb6d02d76a7bf10fd6bd049f9ef25b901aba",
		PerPage: 50,
	})
	if err != nil {
		t.Fatalf("list runs returned error: %v", err)
	}
	if seenQuery != "head_sha=42e1eb6d02d76a7bf10fd6bd049f9ef25b901aba&per_page=50" {
		t.Fatalf("query = %q", seenQuery)
	}
	if len(runs) != 3 {
		t.Fatalf("runs = %d, want the 3-run pack", len(runs))
	}
	// The same push sha carries two events: push (CI, Release) and the
	// chained workflow_run (Deploy Pages). Grouping must see both.
	events := map[string]int{}
	for _, run := range runs {
		events[run.Event]++
	}
	if events["push"] != 2 || events["workflow_run"] != 1 {
		t.Fatalf("event mix = %#v, want push:2 workflow_run:1", events)
	}
}

func TestListRunsSendsCreatedAfterQualifier(t *testing.T) {
	var seenCreated string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCreated = r.URL.Query().Get("created")
		_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	since := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	if _, err := client.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound", CreatedAfter: since}); err != nil {
		t.Fatalf("list runs returned error: %v", err)
	}
	if seenCreated != ">=2026-06-11T10:00:00Z" {
		t.Fatalf("created qualifier = %q, want >=2026-06-11T10:00:00Z", seenCreated)
	}
}

func TestListWorkflowRunsSendsDiscoveryFilters(t *testing.T) {
	var seenPath, seenQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	since := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	_, err := client.ListWorkflowRuns(context.Background(), "indrasvat/gh-hound", "release.yml", usecase.RunFilter{
		Branch:       "main",
		Event:        "workflow_dispatch",
		CreatedAfter: since,
		PerPage:      10,
	})
	if err != nil {
		t.Fatalf("list workflow runs returned error: %v", err)
	}
	if seenPath != "/repos/indrasvat/gh-hound/actions/workflows/release.yml/runs" {
		t.Fatalf("path = %q", seenPath)
	}
	want := "branch=main&created=%3E%3D2026-06-11T10%3A00%3A00Z&event=workflow_dispatch&per_page=10"
	if seenQuery != want {
		t.Fatalf("query = %q, want %q", seenQuery, want)
	}
}
