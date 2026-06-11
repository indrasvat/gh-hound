package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// Payloads under testdata/ are pinned from live GETs against
// indrasvat/gh-hound (API version 2026-03-10), trimmed to the fields
// the adapter parses:
//
//	GET /repos/indrasvat/gh-hound/actions/workflows/ci.yml/runs?branch=main&per_page=2
//	GET /repos/indrasvat/gh-hound/compare/9d3b3e5...f2b85a7
func TestListWorkflowRunsScopesPathAndMapsPinnedPayload(t *testing.T) {
	payload, err := os.ReadFile("testdata/workflow_runs.json")
	if err != nil {
		t.Fatalf("read pinned payload: %v", err)
	}
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	runs, err := client.ListWorkflowRuns(context.Background(), "indrasvat/gh-hound", "ci.yml", usecase.RunFilter{
		Branch:        "main",
		PerPage:       100,
		Page:          2,
		CreatedBefore: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListWorkflowRuns returned error: %v", err)
	}
	if gotPath != "/repos/indrasvat/gh-hound/actions/workflows/ci.yml/runs" {
		t.Fatalf("path = %q, want workflow-scoped runs path", gotPath)
	}
	if gotQuery != "branch=main&created=%3C%3D2026-06-10T12%3A00%3A00Z&page=2&per_page=100" {
		t.Fatalf("query = %q", gotQuery)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	if runs[0].RunNumber != 102 || runs[0].Conclusion != "success" || runs[0].HeadBranch != "main" {
		t.Fatalf("first run mapped wrong: %+v", runs[0])
	}
	// The pinned page includes a rerun-flipped run: attempt 2, success.
	// The list endpoint reports the LATEST attempt's conclusion — the
	// attempt rule the regression scan depends on.
	if runs[1].RunNumber != 100 || runs[1].RunAttempt != 1 {
		t.Fatalf("second run mapped wrong: %+v", runs[1])
	}
}

func TestListWorkflowRunsRequiresWorkflow(t *testing.T) {
	client := NewClient("http://127.0.0.1:0", nil)
	if _, err := client.ListWorkflowRuns(context.Background(), "o/r", "  ", usecase.RunFilter{}); err == nil {
		t.Fatal("expected error for empty workflow")
	}
}

func TestCompareCommitsParsesPinnedPayload(t *testing.T) {
	payload, err := os.ReadFile("testdata/compare.json")
	if err != nil {
		t.Fatalf("read pinned payload: %v", err)
	}
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	rangeInfo, err := client.CompareCommits(context.Background(), "indrasvat/gh-hound", "9d3b3e5", "f2b85a7")
	if err != nil {
		t.Fatalf("CompareCommits returned error: %v", err)
	}
	if gotPath != "/repos/indrasvat/gh-hound/compare/9d3b3e5...f2b85a7" {
		t.Fatalf("path = %q, want compare path", gotPath)
	}
	if gotQuery != "per_page=100" {
		t.Fatalf("query = %q, want per_page=100", gotQuery)
	}
	if rangeInfo.TotalCommits != 1 {
		t.Fatalf("TotalCommits = %d, want 1", rangeInfo.TotalCommits)
	}
	if rangeInfo.HTMLURL != "https://github.com/indrasvat/gh-hound/compare/9d3b3e5...f2b85a7" {
		t.Fatalf("HTMLURL = %q", rangeInfo.HTMLURL)
	}
	if len(rangeInfo.Commits) != 1 {
		t.Fatalf("commits = %d, want 1", len(rangeInfo.Commits))
	}
	commit := rangeInfo.Commits[0]
	if commit.SHA != "f2b85a73d866512fa76484ee2034d46e28ab9de1" {
		t.Fatalf("sha = %q", commit.SHA)
	}
	if commit.Author != "indrasvat" {
		t.Fatalf("author = %q, want the login", commit.Author)
	}
	// Subject line only: multi-paragraph bodies never leak into the
	// suspect list (clean-excerpts precedent).
	if commit.Message != "fix: terminal resize, filtered-view columns, palette dispatch toast (#16)" {
		t.Fatalf("message = %q, want subject line only", commit.Message)
	}
}

func TestCompareCommitsFallsBackToCommitAuthorName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ahead","total_commits":1,"html_url":"https://github.com/o/r/compare/a...b",` +
			`"commits":[{"sha":"abc123","commit":{"message":"chore: bump","author":{"name":"Web Flow"}},"author":null}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	rangeInfo, err := client.CompareCommits(context.Background(), "o/r", "a", "b")
	if err != nil {
		t.Fatalf("CompareCommits returned error: %v", err)
	}
	if rangeInfo.Commits[0].Author != "Web Flow" {
		t.Fatalf("author = %q, want commit author name fallback", rangeInfo.Commits[0].Author)
	}
}

func TestCompareCommitsRequiresBaseAndHead(t *testing.T) {
	client := NewClient("http://127.0.0.1:0", nil)
	if _, err := client.CompareCommits(context.Background(), "o/r", "", "head"); err == nil {
		t.Fatal("expected error for empty base")
	}
	if _, err := client.CompareCommits(context.Background(), "o/r", "base", ""); err == nil {
		t.Fatal("expected error for empty head")
	}
}

func TestCompareCommitsMapsNotFoundToAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.CompareCommits(context.Background(), "o/r", "gone", "missing")
	var apiErr usecase.APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != usecase.APIErrorNotFound {
		t.Fatalf("err = %v, want APIError not_found", err)
	}
}
