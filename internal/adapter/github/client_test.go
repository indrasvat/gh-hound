package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestClientDecodesReadEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-GitHub-Api-Version"); got != APIVersion {
			t.Fatalf("api version header = %q, want %q", got, APIVersion)
		}
		switch r.URL.Path {
		case "/repos/indrasvat/gh-hound/actions/runs":
			writeJSON(w, runsFixture)
		case "/repos/indrasvat/gh-hound/actions/runs/30433642":
			writeJSON(w, runFixture)
		case "/repos/indrasvat/gh-hound/actions/runs/30433642/jobs":
			if r.URL.Query().Get("filter") != "latest" {
				t.Fatalf("jobs filter = %q, want latest", r.URL.Query().Get("filter"))
			}
			writeJSON(w, jobsFixture)
		case "/repos/indrasvat/gh-hound/actions/jobs/399444496":
			writeJSON(w, jobFixture)
		case "/repos/indrasvat/gh-hound/actions/workflows":
			writeJSON(w, workflowsFixture)
		case "/repos/indrasvat/gh-hound/check-runs/399444496/annotations":
			writeJSON(w, annotationsFixture)
		default:
			t.Fatalf("unexpected path %s", r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ctx := context.Background()
	runs, err := client.ListRuns(ctx, usecase.RunFilter{Repo: "indrasvat/gh-hound", Branch: "main", Status: string(model.StatusCompleted), PerPage: 30})
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != 30433642 || runs[0].Conclusion != model.ConclusionFailure {
		t.Fatalf("unexpected runs: %#v", runs)
	}
	run, err := client.GetRun(ctx, "indrasvat/gh-hound", 30433642)
	if err != nil || run.Name != "CI" {
		t.Fatalf("GetRun = %#v, %v", run, err)
	}
	jobs, err := client.ListJobs(ctx, "indrasvat/gh-hound", 30433642)
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 || len(jobs[0].Steps) != 1 || jobs[0].Steps[0].Name != "go test ./..." {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}
	job, err := client.GetJob(ctx, "indrasvat/gh-hound", 399444496)
	if err != nil || job.CheckRunURL == "" {
		t.Fatalf("GetJob = %#v, %v", job, err)
	}
	workflows, err := client.ListWorkflows(ctx, "indrasvat/gh-hound")
	if err != nil || len(workflows) != 1 || workflows[0].Path != ".github/workflows/ci.yml" {
		t.Fatalf("ListWorkflows = %#v, %v", workflows, err)
	}
	annotations, err := client.ListAnnotations(ctx, "indrasvat/gh-hound", job)
	if err != nil || len(annotations) != 1 || annotations[0].StartLine != 142 {
		t.Fatalf("ListAnnotations = %#v, %v", annotations, err)
	}
}

func TestReadEndpointErrorsAreTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.ListRuns(context.Background(), usecase.RunFilter{Repo: "openclaw/openclaw"})
	var apiErr usecase.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ListRuns err = %T %v, want usecase.APIError", err, err)
	}
	if apiErr.Kind != usecase.APIErrorPermission || apiErr.Status != http.StatusForbidden {
		t.Fatalf("api error = %#v", apiErr)
	}
}

func TestETagCacheReusesBodyOnNotModified(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if calls.Load() == 1 {
			w.Header().Set("ETag", `"runs-v1"`)
			writeJSON(w, runsFixture)
			return
		}
		if got := r.Header.Get("If-None-Match"); got != `"runs-v1"` {
			t.Fatalf("If-None-Match = %q, want cached etag", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ctx := context.Background()
	for i := range 2 {
		runs, err := client.ListRuns(ctx, usecase.RunFilter{Repo: "indrasvat/gh-hound"})
		if err != nil {
			t.Fatalf("ListRuns call %d returned error: %v", i+1, err)
		}
		if len(runs) != 1 || runs[0].ID != 30433642 {
			t.Fatalf("cached runs call %d = %#v", i+1, runs)
		}
	}
}

func TestQueueSerializesRequests(t *testing.T) {
	queue := NewQueue()
	start := make(chan struct{})
	done := make(chan struct{}, 2)
	var concurrent atomic.Int64
	var maxConcurrent atomic.Int64

	for range 2 {
		go func() {
			_ = queue.Do(context.Background(), func(context.Context) error {
				current := concurrent.Add(1)
				if current > maxConcurrent.Load() {
					maxConcurrent.Store(current)
				}
				<-start
				concurrent.Add(-1)
				done <- struct{}{}
				return nil
			})
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(start)
	<-done
	<-done
	if maxConcurrent.Load() != 1 {
		t.Fatalf("max concurrent queue calls = %d, want 1", maxConcurrent.Load())
	}
}

func TestPollerAdaptsToRunState(t *testing.T) {
	poller := Poller{Fast: 2 * time.Second, Slow: 30 * time.Second}
	if got := poller.Next([]model.Run{{Status: model.StatusInProgress}}, 0); got != 2*time.Second {
		t.Fatalf("running interval = %s", got)
	}
	if got := poller.Next([]model.Run{{Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}}, 2*time.Second); got != 4*time.Second {
		t.Fatalf("first idle backoff = %s", got)
	}
	if got := poller.Next([]model.Run{{Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}}, 30*time.Second); got != 30*time.Second {
		t.Fatalf("slow cap = %s", got)
	}
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

const runsFixture = `{
  "total_count": 1,
  "workflow_runs": [{
    "id": 30433642,
    "name": "CI",
    "display_title": "fix parser",
    "status": "completed",
    "conclusion": "failure",
    "event": "pull_request",
    "head_branch": "main",
    "head_sha": "a1b2c3d",
    "path": ".github/workflows/ci.yml",
    "run_number": 571,
    "run_attempt": 1,
    "workflow_id": 123,
    "actor": {"login": "indrasvat"},
    "triggering_actor": {"login": "indrasvat"},
    "created_at": "2026-06-07T17:42:00Z",
    "updated_at": "2026-06-07T17:44:00Z",
    "run_started_at": "2026-06-07T17:42:10Z",
    "html_url": "https://github.com/indrasvat/gh-hound/actions/runs/30433642",
    "jobs_url": "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/30433642/jobs",
    "logs_url": "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/30433642/logs",
    "pull_requests": [{"number": 7}]
  }]
}`

const runFixture = `{
  "id": 30433642,
  "name": "CI",
  "display_title": "fix parser",
  "status": "completed",
  "conclusion": "failure",
  "event": "pull_request",
  "head_branch": "main",
  "head_sha": "a1b2c3d",
  "path": ".github/workflows/ci.yml",
  "run_number": 571,
  "run_attempt": 1,
  "workflow_id": 123,
  "actor": {"login": "indrasvat"},
  "triggering_actor": {"login": "indrasvat"},
  "created_at": "2026-06-07T17:42:00Z",
  "updated_at": "2026-06-07T17:44:00Z",
  "run_started_at": "2026-06-07T17:42:10Z",
  "html_url": "https://github.com/indrasvat/gh-hound/actions/runs/30433642",
  "jobs_url": "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/30433642/jobs",
  "logs_url": "https://api.github.com/repos/indrasvat/gh-hound/actions/runs/30433642/logs",
  "pull_requests": [{"number": 7}]
}`

const jobsFixture = `{
  "total_count": 1,
  "jobs": [` + jobFixture + `]
}`

const jobFixture = `{
  "id": 399444496,
  "run_id": 30433642,
  "status": "completed",
  "conclusion": "failure",
  "started_at": "2026-06-07T17:42:40Z",
  "completed_at": "2026-06-07T17:44:39Z",
  "name": "build",
  "steps": [{
    "name": "go test ./...",
    "status": "completed",
    "conclusion": "failure",
    "number": 6,
    "started_at": "2026-06-07T17:43:00Z",
    "completed_at": "2026-06-07T17:44:00Z"
  }],
  "labels": ["ubuntu-latest"],
  "runner_name": "GitHub Actions 1",
  "runner_group_name": "GitHub Actions",
  "workflow_name": "CI",
  "head_branch": "main",
  "html_url": "https://github.com/indrasvat/gh-hound/actions/runs/30433642/job/399444496",
  "check_run_url": "https://api.github.com/repos/indrasvat/gh-hound/check-runs/399444496"
}`

const workflowsFixture = `{
  "total_count": 1,
  "workflows": [{
    "id": 123,
    "name": "CI",
    "path": ".github/workflows/ci.yml",
    "state": "active",
    "html_url": "https://github.com/indrasvat/gh-hound/actions/workflows/ci.yml"
  }]
}`

const annotationsFixture = `[{
  "path": "internal/parser/lexer.go",
  "start_line": 142,
  "end_line": 142,
  "annotation_level": "failure",
  "message": "identifier mismatch",
  "title": "go test"
}]`
