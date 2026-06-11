package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// The flake scan's dependency surface is satisfied by the live client.
// Annotations are deliberately NOT part of it: the API only serves
// check-run annotations for the latest attempt (community #103026), so
// flake evidence must come from attempt conclusions and logs.
var (
	_ usecase.WorkflowRunHistory = (*Client)(nil)
	_ usecase.AttemptJobs        = (*Client)(nil)
	_ usecase.JobLogs            = (*Client)(nil)
)

// Payloads pinned from api.github.com 2026-06-11: run 27308012916
// (Deploy Pages Preview #6) is a genuine attempt flip — the preview
// job failed on attempt 1 and passed on attempt 2.
func TestAttemptEndpointsDecodeLivePinnedFlipPayloads(t *testing.T) {
	attemptJobs, err := os.ReadFile("testdata/attempt_jobs_flip.json")
	if err != nil {
		t.Fatalf("read attempt jobs payload: %v", err)
	}
	attemptRun, err := os.ReadFile("testdata/run_attempt_flip.json")
	if err != nil {
		t.Fatalf("read attempt run payload: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/indrasvat/gh-hound/actions/runs/27308012916/attempts/1/jobs":
			_, _ = w.Write(attemptJobs)
		case "/repos/indrasvat/gh-hound/actions/runs/27308012916/attempts/1":
			_, _ = w.Write(attemptRun)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client())

	jobs, err := client.ListJobsForAttempt(context.Background(), "indrasvat/gh-hound", 27308012916, 1)
	if err != nil {
		t.Fatalf("ListJobsForAttempt: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Name != "preview" || jobs[0].Conclusion != model.ConclusionFailure {
		t.Fatalf("jobs = %+v, want the failed preview job from attempt 1", jobs)
	}

	run, err := client.GetRunAttempt(context.Background(), "indrasvat/gh-hound", 27308012916, 1)
	if err != nil {
		t.Fatalf("GetRunAttempt: %v", err)
	}
	if run.RunAttempt != 1 || run.Conclusion != model.ConclusionFailure {
		t.Fatalf("run = attempt %d %q, want attempt 1 failure", run.RunAttempt, run.Conclusion)
	}
}
