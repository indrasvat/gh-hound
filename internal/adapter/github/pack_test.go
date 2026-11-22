package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
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

// A run GitHub could not start (an invalid workflow file, a blocked
// action) concludes startup_failure. One such run on a page must not
// fail the decode of every run beside it. The payload's shape is copied
// from a live response; ids and names are local.
func TestListRunsDecodesAPageHoldingAStartupFailure(t *testing.T) {
	payload, err := os.ReadFile("testdata/runs_startup_failure.json")
	if err != nil {
		t.Fatalf("read startup_failure payload: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	runs, err := client.ListRuns(context.Background(), usecase.RunFilter{Repo: "indrasvat/gh-hound", PerPage: 30})
	if err != nil {
		t.Fatalf("list runs returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want both runs on the page", len(runs))
	}
	if runs[0].Conclusion != model.ConclusionStartupFailure {
		t.Fatalf("first run conclusion = %q, want startup_failure", runs[0].Conclusion)
	}
	if runs[1].Conclusion != model.ConclusionSuccess {
		t.Fatalf("second run conclusion = %q, want success", runs[1].Conclusion)
	}
}

// A fork PR's runs carry the fork as head_repository: that is what
// marks a held run as approvable. The shapes are copied from live
// runs on a public repo; ids and names are local.
func TestListRunsMarksForkRuns(t *testing.T) {
	payload, err := os.ReadFile("testdata/runs_never_ran.json")
	if err != nil {
		t.Fatalf("read never-ran payload: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	runs, err := NewClient(server.URL, server.Client()).ListRuns(context.Background(), usecase.RunFilter{Repo: "acme/forge", PerPage: 30})
	if err != nil {
		t.Fatalf("list runs returned error: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(runs))
	}
	for i, want := range []bool{true, true, false} {
		if runs[i].FromFork != want || (want && runs[i].HeadRepository == "") {
			t.Fatalf("run %d FromFork = %v, want %v", runs[i].ID, runs[i].FromFork, want)
		}
	}
	if runs[0].Conclusion != model.ConclusionActionRequired {
		t.Fatalf("held run conclusion = %q, want action_required as GitHub reports it", runs[0].Conclusion)
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

// A runner's diagnostic can sit past the first page of a job's
// annotations: every page is read.
func TestListAnnotationsReadsEveryPage(t *testing.T) {
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page")+"/"+r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		n := 100
		if r.URL.Query().Get("page") == "2" {
			n = 1
		}
		items := make([]map[string]any, n)
		for i := range items {
			items[i] = map[string]any{"path": "lint.go", "annotation_level": "warning", "message": "unused"}
		}
		if n == 1 {
			items[0] = map[string]any{"path": ".github", "annotation_level": "failure", "message": "The runner has received a shutdown signal."}
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer server.Close()

	annotations, err := NewClient(server.URL, server.Client()).ListAnnotations(context.Background(), "acme/forge", model.Job{ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 101 || annotations[100].Message != "The runner has received a shutdown signal." {
		t.Fatalf("annotations = %d (pages %v), want 101 with the shutdown last", len(annotations), pages)
	}
	if strings.Join(pages, ",") != "1/100,2/100" {
		t.Fatalf("pages = %v, want 1/100,2/100", pages)
	}
}

// Approving a held fork run needs write access: CanPush reads it from
// the repository's permissions, and a payload without them is an error.
func TestCanPushReadsTheViewersPermission(t *testing.T) {
	for _, tc := range []struct {
		body    string
		want    bool
		wantErr bool
	}{
		{`{"permissions":{"admin":false,"push":true,"pull":true}}`, true, false},
		{`{"permissions":{"push":false,"pull":true}}`, false, false},
		{`{"full_name":"acme/forge"}`, false, true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/acme/forge" {
				t.Errorf("path = %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(tc.body))
		}))
		got, err := NewClient(server.URL, server.Client()).CanPush(context.Background(), "acme/forge")
		server.Close()
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Fatalf("CanPush(%s) = %v, %v", tc.body, got, err)
		}
	}
}

// Exactly the cap is fine (an empty 11th page confirms it); past the
// cap the listing errors rather than hiding what it could not read.
func TestListAnnotationsCapsAtTenFullPages(t *testing.T) {
	for _, tc := range []struct {
		eleventh int
		wantErr  bool
	}{{0, false}, {1, true}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := 100
			if r.URL.Query().Get("page") == "11" {
				n = tc.eleventh
			}
			items := make([]map[string]any, n)
			for i := range items {
				items[i] = map[string]any{"annotation_level": "warning", "message": "x"}
			}
			_ = json.NewEncoder(w).Encode(items)
		}))
		got, err := NewClient(server.URL, server.Client()).ListAnnotations(context.Background(), "acme/forge", model.Job{ID: 7})
		server.Close()
		if (err != nil) != tc.wantErr || (!tc.wantErr && len(got) != 1000) {
			t.Fatalf("eleventh page %d: %d annotations, err=%v", tc.eleventh, len(got), err)
		}
	}
}
