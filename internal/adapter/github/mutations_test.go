package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestMutationEndpointsUseExpectedMethodPathAndBodies(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-GitHub-Api-Version") != APIVersion {
			t.Fatalf("missing api version header")
		}
		switch r.URL.Path {
		case "/repos/indrasvat/gh-hound/actions/runs/571/rerun":
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rerun body: %v", err)
			}
			if !body["enable_debug_logging"] {
				t.Fatalf("rerun body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/indrasvat/gh-hound/actions/runs/571/rerun-failed-jobs":
			// Live-verified 2026-06-10: this endpoint accepts the debug
			// body on API v2026-03-10 (201 on run 27245877203).
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rerun-failed body: %v", err)
			}
			if !body["enable_debug_logging"] {
				t.Fatalf("rerun-failed body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/indrasvat/gh-hound/actions/jobs/399/rerun":
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode job rerun body: %v", err)
			}
			if body["enable_debug_logging"] {
				t.Fatalf("job rerun body = %#v, want debug false", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/indrasvat/gh-hound/actions/runs/571/cancel",
			"/repos/indrasvat/gh-hound/actions/runs/571/force-cancel":
			w.WriteHeader(http.StatusAccepted)
		case "/repos/indrasvat/gh-hound/actions/workflows/release.yml/dispatches":
			var body usecase.DispatchRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode dispatch body: %v", err)
			}
			if body.Ref != "main" || body.Inputs["version"] != "v0.12.0" {
				t.Fatalf("dispatch body = %#v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected mutation path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ctx := context.Background()
	calls := []func() error{
		func() error { _, err := client.RerunRun(ctx, "indrasvat/gh-hound", 571, true); return err },
		func() error { _, err := client.RerunFailedJobs(ctx, "indrasvat/gh-hound", 571, true); return err },
		func() error { _, err := client.RerunJob(ctx, "indrasvat/gh-hound", 399, false); return err },
		func() error { _, err := client.CancelRun(ctx, "indrasvat/gh-hound", 571); return err },
		func() error { _, err := client.ForceCancelRun(ctx, "indrasvat/gh-hound", 571); return err },
		func() error {
			_, err := client.DispatchWorkflow(ctx, "indrasvat/gh-hound", "release.yml", usecase.DispatchRequest{
				Ref: "main",
				Inputs: map[string]string{
					"version": "v0.12.0",
				},
			})
			return err
		},
	}
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("mutation returned error: %v", err)
		}
	}
	if len(seen) != 6 {
		t.Fatalf("mutation calls = %#v", seen)
	}
}

func TestMutationErrorsAreTyped(t *testing.T) {
	tests := []struct {
		status int
		want   usecase.ActionErrorKind
	}{
		{http.StatusForbidden, usecase.ActionErrorPermission},
		{http.StatusConflict, usecase.ActionErrorConflict},
		{http.StatusTooManyRequests, usecase.ActionErrorRateLimit},
	}
	for _, tt := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", tt.status)
		}))
		client := NewClient(server.URL, server.Client())
		_, err := client.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
		server.Close()
		actionErr, ok := usecase.AsActionError(err)
		if !ok || actionErr.Kind != tt.want {
			t.Fatalf("status %d error = %#v, want kind %s", tt.status, err, tt.want)
		}
	}
}

func TestMutationRateLimitErrorCarriesRetryMetadata(t *testing.T) {
	resetAt := time.Unix(1781033060, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "17")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
	actionErr, ok := usecase.AsActionError(err)
	if !ok {
		t.Fatalf("CancelRun err = %#v, want action error", err)
	}
	if actionErr.Kind != usecase.ActionErrorRateLimit || actionErr.RetryAfter != 17*time.Second || !actionErr.ResetAt.Equal(resetAt) {
		t.Fatalf("rate limit action error = %#v", actionErr)
	}
}
